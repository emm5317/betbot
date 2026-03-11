package oddspoller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/config"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Poller struct {
	cfg        config.Config
	logger     *slog.Logger
	pool       *pgxpool.Pool
	client     *Client
	normalizer *Normalizer
}

func NewPoller(cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) *Poller {
	return &Poller{
		cfg:        cfg,
		logger:     logger,
		pool:       pool,
		client:     NewClient(cfg.OddsAPIKey, cfg.OddsAPIBaseURL, cfg.OddsAPIRegions, cfg.OddsAPIMarkets, cfg.OddsAPIOddsFormat, cfg.OddsAPIDateFormat, cfg.OddsAPITimeout, cfg.OddsAPIRateLimit),
		normalizer: NewNormalizer(cfg.OddsAPISource),
	}
}

func (p *Poller) Run(ctx context.Context) (PollMetrics, error) {
	queries := store.New(p.pool)
	startedAt := time.Now().UTC()
	run, err := queries.InsertPollRun(ctx, store.InsertPollRunParams{
		Source:    p.cfg.OddsAPISource,
		StartedAt: store.Timestamptz(startedAt),
	})
	if err != nil {
		return PollMetrics{}, fmt.Errorf("insert poll run: %w", err)
	}

	metrics, runErr := p.run(ctx)
	status := "success"
	errText := ""
	if runErr != nil {
		status = "failed"
		errText = runErr.Error()
	}

	finishedAt := time.Now().UTC()
	completeErr := queries.CompletePollRun(ctx, store.CompletePollRunParams{
		ID:            run.ID,
		FinishedAt:    store.Timestamptz(finishedAt),
		Status:        status,
		GamesSeen:     int32(metrics.GamesSeen),
		SnapshotsSeen: int32(metrics.SnapshotsSeen),
		InsertsCount:  int32(metrics.Inserts),
		DedupSkips:    int32(metrics.DedupSkips),
		ErrorText:     errText,
	})
	if completeErr != nil {
		return metrics, fmt.Errorf("complete poll run: %w", completeErr)
	}

	return metrics, runErr
}

func (p *Poller) run(ctx context.Context) (PollMetrics, error) {
	var allGames []APIGame
	for _, sport := range p.cfg.OddsAPISports {
		games, err := p.client.FetchSport(ctx, sport)
		if err != nil {
			return PollMetrics{}, err
		}
		allGames = append(allGames, games...)
	}

	payload, err := p.normalizer.Normalize(allGames, time.Now().UTC())
	if err != nil {
		return PollMetrics{}, err
	}

	metrics := PollMetrics{GamesSeen: len(payload.Games), SnapshotsSeen: len(payload.Snapshots)}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return metrics, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	queries := store.New(tx)
	gameIDs := make(map[string]int64, len(payload.Games))
	for _, game := range payload.Games {
		row, err := queries.UpsertGame(ctx, store.UpsertGameParams{
			Source:       game.Source,
			ExternalID:   game.ExternalID,
			Sport:        game.Sport,
			HomeTeam:     game.HomeTeam,
			AwayTeam:     game.AwayTeam,
			CommenceTime: store.Timestamptz(game.CommenceTime),
		})
		if err != nil {
			return metrics, fmt.Errorf("upsert game %s: %w", game.ExternalID, err)
		}
		gameIDs[game.ExternalID] = row.ID
	}

	for _, snapshot := range payload.Snapshots {
		gameID, ok := gameIDs[snapshot.GameExternalID]
		if !ok {
			return metrics, fmt.Errorf("missing game id for %s", snapshot.GameExternalID)
		}

		previousHash, err := queries.GetLatestSnapshotHash(ctx, store.GetLatestSnapshotHashParams{
			GameID:      gameID,
			Source:      snapshot.Source,
			BookKey:     snapshot.BookKey,
			MarketKey:   snapshot.MarketKey,
			OutcomeName: snapshot.OutcomeName,
			OutcomeSide: snapshot.OutcomeSide,
			Point:       snapshot.Point,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return metrics, fmt.Errorf("lookup latest snapshot hash: %w", err)
		}
		if err == nil && !ShouldInsertSnapshot(previousHash, snapshot) {
			metrics.DedupSkips++
			continue
		}

		if _, err := queries.InsertOddsSnapshot(ctx, store.InsertOddsSnapshotParams{
			GameID:             gameID,
			Source:             snapshot.Source,
			BookKey:            snapshot.BookKey,
			BookName:           snapshot.BookName,
			MarketKey:          snapshot.MarketKey,
			MarketName:         snapshot.MarketName,
			OutcomeName:        snapshot.OutcomeName,
			OutcomeSide:        snapshot.OutcomeSide,
			PriceAmerican:      int32(snapshot.PriceAmerican),
			Point:              snapshot.Point,
			ImpliedProbability: snapshot.ImpliedProbability,
			SnapshotHash:       snapshot.SnapshotHash,
			RawJson:            snapshot.RawJSON,
			CapturedAt:         store.Timestamptz(snapshot.CapturedAt),
		}); err != nil {
			return metrics, fmt.Errorf("insert odds snapshot: %w", err)
		}
		metrics.Inserts++
	}

	if err := tx.Commit(ctx); err != nil {
		return metrics, fmt.Errorf("commit tx: %w", err)
	}

	p.logger.InfoContext(ctx, "odds poll completed",
		slog.Int("games_seen", metrics.GamesSeen),
		slog.Int("snapshots_seen", metrics.SnapshotsSeen),
		slog.Int("inserts", metrics.Inserts),
		slog.Int("dedup_skips", metrics.DedupSkips),
	)

	return metrics, nil
}
