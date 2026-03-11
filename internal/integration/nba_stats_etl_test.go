package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/ingestion/statsetl"
	"betbot/internal/store"
	workerpkg "betbot/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type integrationNBAProvider struct {
	snapshot statsetl.NBASnapshot
}

func (p *integrationNBAProvider) Fetch(context.Context, statsetl.NBARequest) (statsetl.NBASnapshot, error) {
	return p.snapshot, nil
}

func TestNBAStatsETLIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	provider := &integrationNBAProvider{snapshot: nbaSnapshotForIntegration(65, 119.2, 111.0, 8.2, 98.1)}
	etl := statsetl.NewNBAETL(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := statsetl.NBARequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	}

	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nba_team_stats"); got != 1 {
		t.Fatalf("expected 1 nba team stat row after first run, got %d", got)
	}

	offensiveRating, updatedAt := loadNBAState(ctx, t, pool)
	if offensiveRating != 119.2 {
		t.Fatalf("expected offensive rating 119.2 after first run, got %.1f", offensiveRating)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nba_team_stats"); got != 1 {
		t.Fatalf("expected row count to remain 1 after identical rerun, got %d", got)
	}

	repeatedOffensiveRating, repeatedUpdatedAt := loadNBAState(ctx, t, pool)
	if repeatedOffensiveRating != offensiveRating {
		t.Fatalf("expected identical rerun to preserve offensive rating %.1f, got %.1f", offensiveRating, repeatedOffensiveRating)
	}
	if !repeatedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected identical rerun to preserve updated_at, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), repeatedUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshot = nbaSnapshotForIntegration(66, 120.4, 110.3, 10.1, 98.9)
	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed etl run: %v", err)
	}

	updatedOffensiveRating, updatedUpdatedAt := loadNBAState(ctx, t, pool)
	if updatedOffensiveRating != 120.4 {
		t.Fatalf("expected updated offensive rating 120.4, got %.1f", updatedOffensiveRating)
	}
	if !updatedUpdatedAt.After(updatedAt) {
		t.Fatalf("expected updated_at to advance, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), updatedUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestNBAStatsETLWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	worker := workerpkg.NewNBAStatsETLWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationNBAProvider{snapshot: nbaSnapshotForIntegration(64, 118.8, 111.4, 7.4, 97.8)},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.NBAStatsETLArgs]{
		Args: workerpkg.NBAStatsETLArgs{
			RequestedAt: time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
			Season:      2026,
			SeasonType:  "regular",
			StatDate:    time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nba_team_stats"); got != 1 {
		t.Fatalf("expected 1 nba team stat row, got %d", got)
	}
}

func nbaSnapshotForIntegration(gamesPlayed int32, offensiveRating float64, defensiveRating float64, netRating float64, pace float64) statsetl.NBASnapshot {
	return statsetl.NBASnapshot{
		Source:     "nba-stats-api",
		Season:     2026,
		SeasonType: "regular",
		StatDate:   time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
		Teams: []statsetl.NBATeamStat{{
			ExternalID:      "bos",
			TeamName:        "Boston Celtics",
			GamesPlayed:     gamesPlayed,
			Wins:            48,
			Losses:          17,
			OffensiveRating: float64Pointer(offensiveRating),
			DefensiveRating: float64Pointer(defensiveRating),
			NetRating:       float64Pointer(netRating),
			Pace:            float64Pointer(pace),
		}},
	}
}

func loadNBAState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (float64, time.Time) {
	t.Helper()

	var offensiveRating float64
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT offensive_rating, updated_at FROM nba_team_stats WHERE source = 'nba-stats-api' AND external_id = 'bos'").Scan(&offensiveRating, &updatedAt); err != nil {
		t.Fatalf("load nba state: %v", err)
	}
	return offensiveRating, updatedAt.UTC()
}
