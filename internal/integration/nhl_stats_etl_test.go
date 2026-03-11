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

type integrationNHLProvider struct {
	snapshot statsetl.NHLSnapshot
}

func (p *integrationNHLProvider) Fetch(context.Context, statsetl.NHLRequest) (statsetl.NHLSnapshot, error) {
	return p.snapshot, nil
}

func TestNHLStatsETLIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	provider := &integrationNHLProvider{snapshot: nhlSnapshotForIntegration(64, 3.41, 2.67)}
	etl := statsetl.NewNHLETL(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := statsetl.NHLRequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	}

	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nhl_team_stats"); got != 1 {
		t.Fatalf("expected 1 nhl team stat row after first run, got %d", got)
	}

	goalsForPerGame, updatedAt := loadNHLState(ctx, t, pool)
	if goalsForPerGame != 3.41 {
		t.Fatalf("expected goals for per game 3.41 after first run, got %.2f", goalsForPerGame)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nhl_team_stats"); got != 1 {
		t.Fatalf("expected row count to remain 1 after identical rerun, got %d", got)
	}

	repeatedGoalsForPerGame, repeatedUpdatedAt := loadNHLState(ctx, t, pool)
	if repeatedGoalsForPerGame != goalsForPerGame {
		t.Fatalf("expected identical rerun to preserve goals for per game %.2f, got %.2f", goalsForPerGame, repeatedGoalsForPerGame)
	}
	if !repeatedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected identical rerun to preserve updated_at, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), repeatedUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshot = nhlSnapshotForIntegration(65, 3.55, 2.58)
	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed etl run: %v", err)
	}

	updatedGoalsForPerGame, updatedUpdatedAt := loadNHLState(ctx, t, pool)
	if updatedGoalsForPerGame != 3.55 {
		t.Fatalf("expected updated goals for per game 3.55, got %.2f", updatedGoalsForPerGame)
	}
	if !updatedUpdatedAt.After(updatedAt) {
		t.Fatalf("expected updated_at to advance, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), updatedUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestNHLStatsETLWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	worker := workerpkg.NewNHLStatsETLWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationNHLProvider{snapshot: nhlSnapshotForIntegration(64, 3.30, 2.71)},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.NHLStatsETLArgs]{
		Args: workerpkg.NHLStatsETLArgs{
			RequestedAt: time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
			Season:      2026,
			SeasonType:  "regular",
			StatDate:    time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nhl_team_stats"); got != 1 {
		t.Fatalf("expected 1 nhl team stat row, got %d", got)
	}
}

func nhlSnapshotForIntegration(gamesPlayed int32, goalsForPerGame float64, goalsAgainstPerGame float64) statsetl.NHLSnapshot {
	return statsetl.NHLSnapshot{
		Source:     "nhl-web-api",
		Season:     2026,
		SeasonType: "regular",
		StatDate:   time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
		Teams: []statsetl.NHLTeamStat{{
			ExternalID:          "bos",
			TeamName:            "Boston Bruins",
			GamesPlayed:         gamesPlayed,
			Wins:                39,
			Losses:              18,
			OTLosses:            7,
			GoalsForPerGame:     float64Pointer(goalsForPerGame),
			GoalsAgainstPerGame: float64Pointer(goalsAgainstPerGame),
			ExpectedGoalsShare:  nil,
			SavePercentage:      nil,
		}},
	}
}

func loadNHLState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (float64, time.Time) {
	t.Helper()

	var goalsForPerGame float64
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT goals_for_per_game, updated_at FROM nhl_team_stats WHERE source = 'nhl-web-api' AND external_id = 'bos'").Scan(&goalsForPerGame, &updatedAt); err != nil {
		t.Fatalf("load nhl state: %v", err)
	}
	return goalsForPerGame, updatedAt.UTC()
}
