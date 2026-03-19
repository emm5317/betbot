package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"betbot/internal/execution"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

const autoPlacementInterval = 15 * time.Minute

const autoPlacementRowLimit int32 = 200

type AutoPlacementArgs struct {
	RequestedAt time.Time `json:"requested_at"`
}

func (AutoPlacementArgs) Kind() string { return "auto_placement" }

func (AutoPlacementArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: autoPlacementInterval,
		},
	}
}

type autoPlacementReadQueries interface {
	ListPlaceableRecommendationSnapshots(ctx context.Context, rowLimit int32) ([]store.ListPlaceableRecommendationSnapshotsRow, error)
}

type autoPlacementOrchestrator interface {
	Place(ctx context.Context, input execution.PlaceInput) (execution.PlaceResult, error)
}

type AutoPlacementWorker struct {
	river.WorkerDefaults[AutoPlacementArgs]
	logger                *slog.Logger
	pool                  *pgxpool.Pool
	readQueries           autoPlacementReadQueries
	placementOrchestrator autoPlacementOrchestrator
}

func NewAutoPlacementWorker(pool *pgxpool.Pool, logger *slog.Logger, adapter execution.BookAdapter) *AutoPlacementWorker {
	if logger == nil {
		logger = slog.Default()
	}

	worker := &AutoPlacementWorker{
		logger: logger,
		pool:   pool,
	}
	if pool != nil && adapter != nil {
		worker.readQueries = store.New(pool)
		worker.placementOrchestrator = execution.NewPlacementOrchestrator(pool, adapter)
	}

	return worker
}

func (w *AutoPlacementWorker) Work(ctx context.Context, job *river.Job[AutoPlacementArgs]) error {
	if w.readQueries == nil {
		return fmt.Errorf("auto placement read queries is nil")
	}
	if w.placementOrchestrator == nil {
		return fmt.Errorf("auto placement orchestrator is nil")
	}

	started := time.Now()
	candidates, err := w.readQueries.ListPlaceableRecommendationSnapshots(ctx, autoPlacementRowLimit)
	if err != nil {
		return fmt.Errorf("list placeable recommendation snapshots: %w", err)
	}

	attempted := 0
	placed := 0
	skipped := 0
	failed := 0

	for _, candidate := range candidates {
		input, reason, ok := buildAutoPlacementInput(candidate)
		if !ok {
			skipped++
			w.logger.WarnContext(ctx, "auto placement skipped candidate",
				slog.Int64("snapshot_id", candidate.SnapshotID),
				slog.String("reason", reason),
			)
			continue
		}

		attempted++
		result, err := w.placementOrchestrator.Place(ctx, input)
		if err != nil {
			failed++
			w.logger.ErrorContext(ctx, "auto placement failed candidate",
				slog.Int64("snapshot_id", candidate.SnapshotID),
				slog.String("idempotency_key", input.IdempotencyKey),
				slog.String("error", err.Error()),
			)
			continue
		}
		if result.AlreadyExists {
			skipped++
			w.logger.InfoContext(ctx, "auto placement candidate already exists",
				slog.Int64("snapshot_id", candidate.SnapshotID),
				slog.Int64("bet_id", result.BetID),
				slog.String("idempotency_key", input.IdempotencyKey),
			)
			continue
		}

		placed++
		w.logger.InfoContext(ctx, "auto placement candidate placed",
			slog.Int64("snapshot_id", candidate.SnapshotID),
			slog.Int64("bet_id", result.BetID),
			slog.String("external_bet_id", result.ExternalBetID),
			slog.String("idempotency_key", input.IdempotencyKey),
		)
	}

	w.logger.InfoContext(ctx, "auto placement job finished",
		slog.Time("requested_at", job.Args.RequestedAt),
		slog.Duration("duration", time.Since(started)),
		slog.Int("candidates", len(candidates)),
		slog.Int("attempted", attempted),
		slog.Int("placed", placed),
		slog.Int("skipped", skipped),
		slog.Int("failed", failed),
	)

	return nil
}

func buildAutoPlacementInput(candidate store.ListPlaceableRecommendationSnapshotsRow) (execution.PlaceInput, string, bool) {
	if candidate.SnapshotID <= 0 {
		return execution.PlaceInput{}, "snapshot_id must be > 0", false
	}
	if candidate.GameID <= 0 {
		return execution.PlaceInput{}, "game_id must be > 0", false
	}

	sport := strings.TrimSpace(candidate.Sport)
	marketKey := strings.TrimSpace(candidate.MarketKey)
	recommendedSide := strings.ToLower(strings.TrimSpace(candidate.RecommendedSide))
	bookKey := strings.TrimSpace(candidate.BestBook)
	if sport == "" || marketKey == "" || bookKey == "" {
		return execution.PlaceInput{}, "required fields are empty", false
	}
	if recommendedSide != "home" && recommendedSide != "away" {
		return execution.PlaceInput{}, "recommended_side unsupported", false
	}
	if candidate.BestAmericanOdds > -100 && candidate.BestAmericanOdds < 100 {
		return execution.PlaceInput{}, "best_american_odds is invalid", false
	}
	if candidate.SuggestedStakeCents <= 0 {
		return execution.PlaceInput{}, "suggested_stake_cents must be > 0", false
	}
	if candidate.ModelProbability <= 0 || candidate.ModelProbability >= 1 {
		return execution.PlaceInput{}, "model_probability must be between 0 and 1", false
	}
	if candidate.MarketProbability <= 0 || candidate.MarketProbability >= 1 {
		return execution.PlaceInput{}, "market_probability must be between 0 and 1", false
	}
	if candidate.Edge < 0 {
		return execution.PlaceInput{}, "edge must be >= 0", false
	}

	return execution.PlaceInput{
		IdempotencyKey:    autoPlacementIdempotencyKey(candidate.SnapshotID, candidate.GameID, marketKey, bookKey),
		SnapshotID:        candidate.SnapshotID,
		GameID:            candidate.GameID,
		Sport:             sport,
		MarketKey:         marketKey,
		RecommendedSide:   recommendedSide,
		BookKey:           bookKey,
		AmericanOdds:      int(candidate.BestAmericanOdds),
		StakeCents:        candidate.SuggestedStakeCents,
		ModelProbability:  candidate.ModelProbability,
		MarketProbability: candidate.MarketProbability,
		Edge:              candidate.Edge,
	}, "", true
}

func autoPlacementIdempotencyKey(snapshotID, gameID int64, marketKey, bookKey string) string {
	return fmt.Sprintf(
		"auto-placement:%d:%d:%s:%s",
		snapshotID,
		gameID,
		strings.ToLower(strings.TrimSpace(marketKey)),
		strings.ToLower(strings.TrimSpace(bookKey)),
	)
}
