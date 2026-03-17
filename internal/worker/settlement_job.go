package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"betbot/internal/domain"
	"betbot/internal/execution"
	"betbot/internal/ingestion/scores"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

const autoSettlementInterval = 30 * time.Minute

type AutoSettlementArgs struct {
	RequestedAt time.Time `json:"requested_at"`
}

func (AutoSettlementArgs) Kind() string { return "auto_settlement" }

func (AutoSettlementArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: autoSettlementInterval,
		},
	}
}

type AutoSettlementWorker struct {
	river.WorkerDefaults[AutoSettlementArgs]
	pool        *pgxpool.Pool
	scores      *scores.Client
	logger      *slog.Logger
	oddsSource  string
	sportLookup map[string]string
}

func NewAutoSettlementWorker(pool *pgxpool.Pool, logger *slog.Logger, scoresClient *scores.Client, oddsSource string) *AutoSettlementWorker {
	if logger == nil {
		logger = slog.Default()
	}

	registry := domain.DefaultSportRegistry()
	sportLookup := make(map[string]string, len(registry.All())*2)
	for _, cfg := range registry.All() {
		sportLookup[strings.ToUpper(string(cfg.ID))] = cfg.OddsAPIKey
		sportLookup[strings.ToLower(cfg.OddsAPIKey)] = cfg.OddsAPIKey
	}

	return &AutoSettlementWorker{
		pool:        pool,
		scores:      scoresClient,
		logger:      logger,
		oddsSource:  strings.TrimSpace(oddsSource),
		sportLookup: sportLookup,
	}
}

func (w *AutoSettlementWorker) Work(ctx context.Context, job *river.Job[AutoSettlementArgs]) error {
	if w.scores == nil {
		return fmt.Errorf("auto settlement scores client is nil")
	}

	started := time.Now()
	queries := store.New(w.pool)

	openBets, err := queries.ListOpenBetsWithGame(ctx)
	if err != nil {
		return fmt.Errorf("list open bets with game: %w", err)
	}
	if len(openBets) == 0 {
		w.logger.InfoContext(ctx, "auto settlement job finished",
			slog.Time("requested_at", job.Args.RequestedAt),
			slog.Duration("duration", time.Since(started)),
			slog.Int("open_bets", 0),
			slog.Int("settled_bets", 0),
			slog.Int("skipped_bets", 0),
			slog.Int("failed_bets", 0),
		)
		return nil
	}

	betsBySport := make(map[string][]store.ListOpenBetsWithGameRow)
	skipped := 0
	for _, bet := range openBets {
		if w.oddsSource != "" && !strings.EqualFold(strings.TrimSpace(bet.GameSource), w.oddsSource) {
			skipped++
			w.logger.InfoContext(ctx, "auto settlement skipped bet: source mismatch",
				slog.Int64("bet_id", bet.ID),
				slog.String("game_source", bet.GameSource),
				slog.String("expected_source", w.oddsSource),
			)
			continue
		}

		sportKey, ok := w.toOddsAPISportKey(bet.GameSport)
		if !ok {
			sportKey, ok = w.toOddsAPISportKey(bet.Sport)
		}
		if !ok || strings.TrimSpace(bet.GameExternalID) == "" {
			skipped++
			w.logger.WarnContext(ctx, "auto settlement skipped bet: missing sport mapping or external id",
				slog.Int64("bet_id", bet.ID),
				slog.String("sport", bet.Sport),
				slog.String("game_sport", bet.GameSport),
				slog.String("game_source", bet.GameSource),
				slog.String("game_external_id", bet.GameExternalID),
			)
			continue
		}
		betsBySport[sportKey] = append(betsBySport[sportKey], bet)
	}

	settled := 0
	failed := 0

	for sportKey, bets := range betsBySport {
		gameScores, err := w.scores.FetchSport(ctx, sportKey)
		if err != nil {
			failed += len(bets)
			w.logger.ErrorContext(ctx, "auto settlement scores fetch failed",
				slog.String("sport_key", sportKey),
				slog.Int("bets", len(bets)),
				slog.String("error", err.Error()),
			)
			continue
		}

		scoreByExternalID := make(map[string]scores.GameScore, len(gameScores))
		for _, gs := range gameScores {
			if strings.TrimSpace(gs.ExternalID) == "" || !gs.Completed {
				continue
			}
			scoreByExternalID[gs.ExternalID] = gs
		}

		for _, bet := range bets {
			gameScore, ok := scoreByExternalID[bet.GameExternalID]
			if !ok {
				skipped++
				continue
			}

			result, payoutCents, ok := determineAutoSettlementResult(bet, gameScore)
			if !ok {
				skipped++
				w.logger.InfoContext(ctx, "auto settlement skipped bet: unsupported or ambiguous result",
					slog.Int64("bet_id", bet.ID),
					slog.String("market_key", bet.MarketKey),
					slog.String("recommended_side", bet.RecommendedSide),
					slog.String("sport", bet.GameSport),
					slog.Int("home_score", gameScore.HomeScore),
					slog.Int("away_score", gameScore.AwayScore),
				)
				continue
			}

			if err := w.settleOneBet(ctx, bet, result, payoutCents); err != nil {
				failed++
				w.logger.ErrorContext(ctx, "auto settlement failed for bet",
					slog.Int64("bet_id", bet.ID),
					slog.String("result", result),
					slog.String("error", err.Error()),
				)
				continue
			}
			settled++
		}
	}

	w.logger.InfoContext(ctx, "auto settlement job finished",
		slog.Time("requested_at", job.Args.RequestedAt),
		slog.Duration("duration", time.Since(started)),
		slog.Int("open_bets", len(openBets)),
		slog.Int("settled_bets", settled),
		slog.Int("skipped_bets", skipped),
		slog.Int("failed_bets", failed),
	)
	return nil
}

func (w *AutoSettlementWorker) settleOneBet(ctx context.Context, bet store.ListOpenBetsWithGameRow, result string, payoutCents int64) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin settle tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txQueries := store.New(tx)
	if err := txQueries.UpdateBetSettled(ctx, store.UpdateBetSettledParams{
		SettlementResult: &result,
		PayoutCents:      &payoutCents,
		ID:               bet.ID,
	}); err != nil {
		return fmt.Errorf("update bet settled: %w", err)
	}

	if err := execution.WriteSettlementLedgerEntry(ctx, txQueries, store.Bet{
		ID:             bet.ID,
		IdempotencyKey: bet.IdempotencyKey,
	}, result, execution.SettlementLedgerAmount(result, bet.StakeCents, payoutCents)); err != nil {
		return fmt.Errorf("write settlement ledger entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settle tx: %w", err)
	}
	return nil
}

func (w *AutoSettlementWorker) toOddsAPISportKey(sport string) (string, bool) {
	normalized := strings.TrimSpace(sport)
	if normalized == "" {
		return "", false
	}
	if key, ok := w.sportLookup[strings.ToUpper(normalized)]; ok {
		return key, true
	}
	if key, ok := w.sportLookup[strings.ToLower(normalized)]; ok {
		return key, true
	}
	return "", false
}

func determineAutoSettlementResult(bet store.ListOpenBetsWithGameRow, score scores.GameScore) (string, int64, bool) {
	if !strings.EqualFold(strings.TrimSpace(bet.MarketKey), "h2h") {
		return "", 0, false
	}

	side := strings.ToLower(strings.TrimSpace(bet.RecommendedSide))
	if side != "home" && side != "away" {
		return "", 0, false
	}

	if score.HomeScore == score.AwayScore {
		// NHL does not support ties in final settlement scoring.
		if strings.EqualFold(strings.TrimSpace(bet.GameSport), "NHL") {
			return "", 0, false
		}
		return "push", bet.StakeCents, true
	}

	homeWon := score.HomeScore > score.AwayScore
	won := (side == "home" && homeWon) || (side == "away" && !homeWon)
	if won {
		payout := bet.StakeCents + execution.CalculateWinnings(bet.StakeCents, int(bet.AmericanOdds))
		return "win", payout, true
	}

	return "loss", 0, true
}
