package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/ingestion/injuries"
	"betbot/internal/store"
	workerpkg "betbot/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type integrationInjuryProvider struct {
	snapshot injuries.Snapshot
}

func (p *integrationInjuryProvider) Fetch(context.Context, injuries.Request) (injuries.Snapshot, error) {
	return p.snapshot, nil
}

func TestInjuryIngestionIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	provider := &integrationInjuryProvider{snapshot: injurySnapshotForIntegration("Questionable")}
	scraper := injuries.NewScraper(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := injuries.Request{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		ReportDate:  time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Sport:       "nfl",
		Source:      "rotowire",
	}

	if _, err := scraper.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first injury run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM player_injury_reports"); got != 1 {
		t.Fatalf("expected 1 injury row after first run, got %d", got)
	}

	status, updatedAt := loadInjuryState(ctx, t, pool)
	if status != "Questionable" {
		t.Fatalf("expected status Questionable after first run, got %q", status)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := scraper.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical injury run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM player_injury_reports"); got != 1 {
		t.Fatalf("expected injury row count to remain 1 after identical rerun, got %d", got)
	}

	repeatedStatus, repeatedUpdatedAt := loadInjuryState(ctx, t, pool)
	if repeatedStatus != status {
		t.Fatalf("expected identical rerun to preserve status %q, got %q", status, repeatedStatus)
	}
	if !repeatedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected identical rerun to preserve updated_at, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), repeatedUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshot = injurySnapshotForIntegration("Out")
	time.Sleep(20 * time.Millisecond)
	if _, err := scraper.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed injury run: %v", err)
	}

	updatedStatus, updatedUpdatedAt := loadInjuryState(ctx, t, pool)
	if updatedStatus != "Out" {
		t.Fatalf("expected updated status Out, got %q", updatedStatus)
	}
	if !updatedUpdatedAt.After(updatedAt) {
		t.Fatalf("expected updated_at to advance, before=%s after=%s", updatedAt.Format(time.RFC3339Nano), updatedUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestInjurySyncWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	worker := workerpkg.NewInjurySyncWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationInjuryProvider{snapshot: injurySnapshotForIntegration("Questionable")},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.InjurySyncArgs]{
		Args: workerpkg.InjurySyncArgs{
			RequestedAt: time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
			ReportDate:  time.Date(2026, time.March, 11, 10, 0, 0, 0, time.UTC),
			Sport:       "nfl",
			Source:      "rotowire",
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM player_injury_reports"); got != 1 {
		t.Fatalf("expected 1 injury row, got %d", got)
	}
}

func injurySnapshotForIntegration(status string) injuries.Snapshot {
	return injuries.Snapshot{
		Source:     "rotowire",
		Sport:      "nfl",
		ReportDate: time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
		Records: []injuries.Record{{
			ExternalID:      "12483",
			PlayerName:      "Josh Allen",
			TeamExternalID:  "buf",
			Position:        "QB",
			Injury:          "Foot",
			Status:          status,
			EstimatedReturn: nil,
			PlayerURL:       "https://www.rotowire.com/football/player/josh-allen-12483",
			RawJSON:         json.RawMessage(`{"ID":"12483"}`),
		}},
	}
}

func loadInjuryState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (string, time.Time) {
	t.Helper()

	var status string
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT status, updated_at FROM player_injury_reports WHERE source = 'rotowire' AND sport = 'nfl' AND external_id = '12483'").Scan(&status, &updatedAt); err != nil {
		t.Fatalf("load injury state: %v", err)
	}
	return status, updatedAt.UTC()
}
