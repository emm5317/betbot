package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
	"betbot/internal/worker"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
)

func TestAutoPlacementWorkerIdempotentAcrossReruns(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	queries := store.New(pool)
	commence := time.Date(2026, time.March, 15, 18, 0, 0, 0, time.UTC)
	game, err := queries.UpsertGame(ctx, store.UpsertGameParams{
		Source:       "the-odds-api",
		ExternalID:   "auto-place-event-1",
		Sport:        "NHL",
		HomeTeam:     "Chicago Blackhawks",
		AwayTeam:     "Dallas Stars",
		CommenceTime: store.Timestamptz(commence),
	})
	if err != nil {
		t.Fatalf("upsert game: %v", err)
	}

	snapshotTime := commence.Add(-45 * time.Minute)
	snapshot, err := queries.InsertRecommendationSnapshot(ctx, store.InsertRecommendationSnapshotParams{
		GeneratedAt:            store.Timestamptz(snapshotTime),
		Sport:                  "NHL",
		GameID:                 game.ID,
		EventTime:              store.Timestamptz(commence),
		EventDate:              pgtype.Date{Time: commence, Valid: true},
		MarketKey:              "h2h",
		RecommendedSide:        "home",
		BestBook:               "draftkings",
		BestAmericanOdds:       -110,
		ModelProbability:       0.57,
		MarketProbability:      0.52,
		Edge:                   0.05,
		SuggestedStakeFraction: 0.02,
		SuggestedStakeCents:    2500,
		BankrollCheckPass:      true,
		BankrollCheckReason:    "ok",
		RankScore:              1.23,
		Metadata:               []byte(`{"source":"integration"}`),
	})
	if err != nil {
		t.Fatalf("insert recommendation snapshot: %v", err)
	}

	placementWorker := worker.NewAutoPlacementWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	job := &river.Job[worker.AutoPlacementArgs]{
		Args: worker.AutoPlacementArgs{RequestedAt: time.Now().UTC()},
	}

	if err := placementWorker.Work(ctx, job); err != nil {
		t.Fatalf("auto placement first run: %v", err)
	}
	if err := placementWorker.Work(ctx, job); err != nil {
		t.Fatalf("auto placement second run: %v", err)
	}

	var betCount int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM bets WHERE snapshot_id = $1", snapshot.ID).Scan(&betCount); err != nil {
		t.Fatalf("count bets by snapshot_id: %v", err)
	}
	if betCount != 1 {
		t.Fatalf("bets for snapshot_id=%d = %d, want 1", snapshot.ID, betCount)
	}

	var betID int64
	var idempotencyKey string
	var status string
	if err := pool.QueryRow(ctx, "SELECT id, idempotency_key, status::text FROM bets WHERE snapshot_id = $1", snapshot.ID).Scan(&betID, &idempotencyKey, &status); err != nil {
		t.Fatalf("fetch placed bet: %v", err)
	}
	if status != "placed" {
		t.Fatalf("bet status = %q, want placed", status)
	}

	var reserveCount int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bankroll_ledger
		WHERE entry_type = 'bet_stake_reserved'
		  AND reference_type = 'bet'
		  AND reference_id = $1
	`, idempotencyKey).Scan(&reserveCount); err != nil {
		t.Fatalf("count stake reserve entries: %v", err)
	}
	if reserveCount != 1 {
		t.Fatalf("stake reserve entries for reference_id=%q = %d, want 1", idempotencyKey, reserveCount)
	}
}
