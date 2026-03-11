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

type integrationMLBProvider struct {
	snapshot statsetl.MLBSnapshot
}

func (p *integrationMLBProvider) Fetch(context.Context, statsetl.MLBRequest) (statsetl.MLBSnapshot, error) {
	return p.snapshot, nil
}

func TestMLBStatsETLIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	provider := &integrationMLBProvider{snapshot: mlbSnapshotForIntegration(11, 3.42, 2.91)}
	etl := statsetl.NewMLBETL(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := statsetl.MLBRequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	}

	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first etl run: %v", err)
	}

	teamCount := countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_team_stats")
	pitcherCount := countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_pitcher_stats")
	if teamCount != 1 || pitcherCount != 1 {
		t.Fatalf("unexpected row counts after first run: teams=%d pitchers=%d", teamCount, pitcherCount)
	}

	teamGamesPlayed, teamUpdatedAt := loadMLBTeamState(t, ctx, pool)
	pitcherEra, pitcherUpdatedAt := loadMLBPitcherState(t, ctx, pool)
	if teamGamesPlayed != 11 {
		t.Fatalf("expected games_played 11 after first run, got %d", teamGamesPlayed)
	}
	if pitcherEra != 2.91 {
		t.Fatalf("expected era 2.91 after first run, got %.2f", pitcherEra)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical etl run: %v", err)
	}

	if countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_team_stats") != 1 {
		t.Fatal("expected team row count to remain 1 after identical rerun")
	}
	if countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_pitcher_stats") != 1 {
		t.Fatal("expected pitcher row count to remain 1 after identical rerun")
	}

	repeatedTeamGamesPlayed, repeatedTeamUpdatedAt := loadMLBTeamState(t, ctx, pool)
	repeatedPitcherEra, repeatedPitcherUpdatedAt := loadMLBPitcherState(t, ctx, pool)
	if repeatedTeamGamesPlayed != teamGamesPlayed || repeatedPitcherEra != pitcherEra {
		t.Fatal("expected identical rerun to preserve stored values")
	}
	if !repeatedTeamUpdatedAt.Equal(teamUpdatedAt) {
		t.Fatalf("expected identical rerun to preserve team updated_at, before=%s after=%s", teamUpdatedAt.Format(time.RFC3339Nano), repeatedTeamUpdatedAt.Format(time.RFC3339Nano))
	}
	if !repeatedPitcherUpdatedAt.Equal(pitcherUpdatedAt) {
		t.Fatalf("expected identical rerun to preserve pitcher updated_at, before=%s after=%s", pitcherUpdatedAt.Format(time.RFC3339Nano), repeatedPitcherUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshot = mlbSnapshotForIntegration(12, 3.18, 2.77)
	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed etl run: %v", err)
	}

	if countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_team_stats") != 1 {
		t.Fatal("expected team row count to stay 1 after changed rerun")
	}
	if countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_pitcher_stats") != 1 {
		t.Fatal("expected pitcher row count to stay 1 after changed rerun")
	}

	updatedGamesPlayed, updatedTeamUpdatedAt := loadMLBTeamState(t, ctx, pool)
	updatedPitcherEra, updatedPitcherUpdatedAt := loadMLBPitcherState(t, ctx, pool)
	if updatedGamesPlayed != 12 {
		t.Fatalf("expected updated games_played 12, got %d", updatedGamesPlayed)
	}
	if updatedPitcherEra != 2.77 {
		t.Fatalf("expected updated era 2.77, got %.2f", updatedPitcherEra)
	}
	if !updatedTeamUpdatedAt.After(teamUpdatedAt) {
		t.Fatalf("expected team updated_at to advance, before=%s after=%s", teamUpdatedAt.Format(time.RFC3339Nano), updatedTeamUpdatedAt.Format(time.RFC3339Nano))
	}
	if !updatedPitcherUpdatedAt.After(pitcherUpdatedAt) {
		t.Fatalf("expected pitcher updated_at to advance, before=%s after=%s", pitcherUpdatedAt.Format(time.RFC3339Nano), updatedPitcherUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestMLBStatsETLWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	worker := workerpkg.NewMLBStatsETLWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationMLBProvider{snapshot: mlbSnapshotForIntegration(9, 3.51, 3.08)},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.MLBStatsETLArgs]{
		Args: workerpkg.MLBStatsETLArgs{
			RequestedAt: time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
			Season:      2026,
			SeasonType:  "regular",
			StatDate:    time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_team_stats"); got != 1 {
		t.Fatalf("expected 1 mlb team stat row, got %d", got)
	}
	if got := countRows(t, ctx, pool, "SELECT COUNT(*) FROM mlb_pitcher_stats"); got != 1 {
		t.Fatalf("expected 1 mlb pitcher stat row, got %d", got)
	}
}

func mlbSnapshotForIntegration(gamesPlayed int32, teamERA float64, pitcherERA float64) statsetl.MLBSnapshot {
	return statsetl.MLBSnapshot{
		Source:     "statcast",
		Season:     2026,
		SeasonType: "regular",
		StatDate:   time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
		Teams: []statsetl.MLBTeamStat{{
			ExternalID:  "bos",
			TeamName:    "Boston Red Sox",
			GamesPlayed: gamesPlayed,
			Wins:        7,
			Losses:      4,
			RunsScored:  61,
			RunsAllowed: 44,
			BattingOPS:  float64Pointer(0.782),
			TeamERA:     float64Pointer(teamERA),
		}},
		Pitchers: []statsetl.MLBPitcherStat{{
			ExternalID:     "sale",
			PlayerName:     "Chris Sale",
			TeamExternalID: "bos",
			TeamName:       "Boston Red Sox",
			GamesStarted:   3,
			InningsPitched: float64Pointer(19.1),
			Era:            float64Pointer(pitcherERA),
			Fip:            float64Pointer(3.04),
			Whip:           float64Pointer(1.02),
			StrikeoutRate:  float64Pointer(0.311),
			WalkRate:       float64Pointer(0.064),
		}},
	}
}

func loadMLBTeamState(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (int32, time.Time) {
	t.Helper()

	var gamesPlayed int32
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT games_played, updated_at FROM mlb_team_stats WHERE source = 'statcast' AND external_id = 'bos'").Scan(&gamesPlayed, &updatedAt); err != nil {
		t.Fatalf("load mlb team state: %v", err)
	}
	return gamesPlayed, updatedAt.UTC()
}

func loadMLBPitcherState(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (float64, time.Time) {
	t.Helper()

	var era float64
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT era, updated_at FROM mlb_pitcher_stats WHERE source = 'statcast' AND external_id = 'sale'").Scan(&era, &updatedAt); err != nil {
		t.Fatalf("load mlb pitcher state: %v", err)
	}
	return era, updatedAt.UTC()
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string) int64 {
	t.Helper()

	var count int64
	if err := pool.QueryRow(ctx, sql).Scan(&count); err != nil {
		t.Fatalf("count rows for %q: %v", sql, err)
	}
	return count
}

func float64Pointer(value float64) *float64 {
	return &value
}
