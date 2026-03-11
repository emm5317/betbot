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

type integrationNFLProvider struct {
	snapshot statsetl.NFLSnapshot
}

func (p *integrationNFLProvider) Fetch(context.Context, statsetl.NFLRequest) (statsetl.NFLSnapshot, error) {
	return p.snapshot, nil
}

func TestNFLStatsETLIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	provider := &integrationNFLProvider{snapshot: nflSnapshotForIntegration(17, 0.102)}
	etl := statsetl.NewNFLETL(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := statsetl.NFLRequest{
		RequestedAt: time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
		Season:      2025,
		StatDate:    time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
	}

	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nfl_team_stats"); got != 1 {
		t.Fatalf("expected 1 nfl team stat row after first run, got %d", got)
	}

	offensiveEPA, updatedAt := loadNFLState(ctx, t, pool)
	if offensiveEPA != 0.102 {
		t.Fatalf("expected offensive epa/play 0.102 after first run, got %.3f", offensiveEPA)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical etl run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nfl_team_stats"); got != 1 {
		t.Fatalf("expected row count to remain 1 after identical rerun, got %d", got)
	}

	repeatedEPA, repeatedUpdatedAt := loadNFLState(ctx, t, pool)
	if repeatedEPA != offensiveEPA {
		t.Fatalf("expected identical rerun to preserve offensive epa/play %.3f, got %.3f", offensiveEPA, repeatedEPA)
	}
	if !repeatedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected identical rerun to preserve updated_at, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), repeatedUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshot = nflSnapshotForIntegration(17, 0.119)
	time.Sleep(20 * time.Millisecond)
	if _, err := etl.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed etl run: %v", err)
	}

	updatedEPA, updatedUpdatedAt := loadNFLState(ctx, t, pool)
	if updatedEPA != 0.119 {
		t.Fatalf("expected updated offensive epa/play 0.119, got %.3f", updatedEPA)
	}
	if !updatedUpdatedAt.After(updatedAt) {
		t.Fatalf("expected updated_at to advance, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), updatedUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestNFLStatsETLWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	worker := workerpkg.NewNFLStatsETLWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationNFLProvider{snapshot: nflSnapshotForIntegration(17, 0.108)},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.NFLStatsETLArgs]{
		Args: workerpkg.NFLStatsETLArgs{
			RequestedAt: time.Date(2026, time.January, 19, 10, 0, 0, 0, time.UTC),
			Season:      2025,
			SeasonType:  "regular",
			StatDate:    time.Date(2026, time.January, 19, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM nfl_team_stats"); got != 1 {
		t.Fatalf("expected 1 nfl team stat row, got %d", got)
	}
}

func nflSnapshotForIntegration(gamesPlayed int32, offensiveEPAPerPlay float64) statsetl.NFLSnapshot {
	return statsetl.NFLSnapshot{
		Source:     "nflverse-nfl-com",
		Season:     2025,
		SeasonType: "regular",
		StatDate:   time.Date(2026, time.January, 19, 0, 0, 0, 0, time.UTC),
		Teams: []statsetl.NFLTeamStat{{
			ExternalID:           "buf",
			TeamName:             "Buffalo Bills",
			GamesPlayed:          gamesPlayed,
			Wins:                 13,
			Losses:               4,
			Ties:                 0,
			PointsFor:            525,
			PointsAgainst:        368,
			OffensiveEPAPerPlay:  float64Pointer(offensiveEPAPerPlay),
			DefensiveEPAPerPlay:  nil,
			OffensiveSuccessRate: nil,
			DefensiveSuccessRate: nil,
		}},
	}
}

func loadNFLState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (float64, time.Time) {
	t.Helper()

	var offensiveEPAPerPlay float64
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT offensive_epa_per_play, updated_at FROM nfl_team_stats WHERE source = 'nflverse-nfl-com' AND external_id = 'buf'").Scan(&offensiveEPAPerPlay, &updatedAt); err != nil {
		t.Fatalf("load nfl state: %v", err)
	}
	return offensiveEPAPerPlay, updatedAt.UTC()
}
