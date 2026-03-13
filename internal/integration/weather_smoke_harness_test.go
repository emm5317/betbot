package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/ingestion/weather"
	workerpkg "betbot/internal/worker"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

func TestWeatherSyncRiverSmokeHarness(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	driver := riverpgxv5.New(pool)
	migrator, err := rivermigrate.New(driver, &rivermigrate.Config{Schema: "public"})
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("migrate river schema: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	provider := &integrationWeatherProvider{
		snapshots: map[string]weather.Snapshot{
			"Boston Red Sox": weatherSnapshotForIntegration(68.5, 0.02),
		},
	}
	river.AddWorker(workers, workerpkg.NewWeatherSyncWorker(pool, logger, provider))

	client, err := river.NewClient(driver, &river.Config{
		Logger: logger,
		Schema: "public",
		Queues: map[string]river.QueueConfig{
			workerpkg.QueueMaintenance: {MaxWorkers: 1},
		},
		Workers:           workers,
		ReindexerSchedule: river.NeverSchedule(),
	})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("stop river client: %v", err)
		}
	}()

	insertWeatherGame(ctx, t, pool, "smoke-weather-outdoor-mlb", "MLB", "Boston Red Sox", "New York Yankees", time.Date(2026, time.September, 13, 18, 30, 0, 0, time.UTC))
	insertWeatherGame(ctx, t, pool, "smoke-weather-dome-nfl", "NFL", "Atlanta Falcons", "Carolina Panthers", time.Date(2026, time.September, 13, 17, 0, 0, 0, time.UTC))
	insertWeatherGame(ctx, t, pool, "smoke-weather-retractable-nfl", "NFL", "Dallas Cowboys", "Philadelphia Eagles", time.Date(2026, time.September, 13, 20, 25, 0, 0, time.UTC))

	forecastDate := time.Date(2026, time.September, 13, 0, 0, 0, 0, time.UTC)
	firstInsert, err := workerpkg.EnqueueWeatherSync(ctx, client, weather.Request{
		RequestedAt:  forecastDate,
		ForecastDate: forecastDate,
	})
	if err != nil {
		t.Fatalf("enqueue first weather_sync job: %v", err)
	}
	if firstInsert.UniqueSkippedAsDuplicate {
		t.Fatal("first weather_sync enqueue unexpectedly skipped as duplicate")
	}
	if firstInsert.Job == nil {
		t.Fatal("first weather_sync enqueue did not return a job row")
	}

	firstJob := waitForWeatherJob(t, client, firstInsert.Job.ID, 20*time.Second)
	if firstJob.State != rivertype.JobStateCompleted {
		t.Fatalf("first weather_sync state = %q, want %q", firstJob.State, rivertype.JobStateCompleted)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM game_weather_snapshots"); got != 3 {
		t.Fatalf("expected 3 weather rows after first smoke run, got %d", got)
	}

	outdoorBefore := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-outdoor-mlb")
	domeBefore := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-dome-nfl")
	retractableBefore := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-retractable-nfl")

	assertOutdoorSmokeState(t, outdoorBefore)
	assertIndoorSmokeState(t, domeBefore, "dome", "fixed-roof-indoor")
	assertIndoorSmokeState(t, retractableBefore, "retractable", "retractable-roof-state-unknown")

	secondInsert, err := workerpkg.EnqueueWeatherSync(ctx, client, weather.Request{
		ForecastDate: forecastDate.AddDate(0, 0, -1),
	})
	if err != nil {
		t.Fatalf("enqueue second weather_sync job: %v", err)
	}
	if secondInsert.UniqueSkippedAsDuplicate {
		t.Fatal("second weather_sync enqueue unexpectedly skipped as duplicate")
	}
	if secondInsert.Job == nil {
		t.Fatal("second weather_sync enqueue did not return a job row")
	}

	secondJob := waitForWeatherJob(t, client, secondInsert.Job.ID, 20*time.Second)
	if secondJob.State != rivertype.JobStateCompleted {
		t.Fatalf("second weather_sync state = %q, want %q", secondJob.State, rivertype.JobStateCompleted)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM game_weather_snapshots"); got != 3 {
		t.Fatalf("expected 3 weather rows after rerun, got %d", got)
	}

	outdoorAfter := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-outdoor-mlb")
	domeAfter := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-dome-nfl")
	retractableAfter := loadWeatherSmokeState(ctx, t, pool, "smoke-weather-retractable-nfl")

	assertSmokeUpdatedAtUnchanged(t, "outdoor", outdoorBefore.UpdatedAt, outdoorAfter.UpdatedAt)
	assertSmokeUpdatedAtUnchanged(t, "dome", domeBefore.UpdatedAt, domeAfter.UpdatedAt)
	assertSmokeUpdatedAtUnchanged(t, "retractable", retractableBefore.UpdatedAt, retractableAfter.UpdatedAt)
}

type weatherSmokeState struct {
	RoofType    string
	Temperature *float64
	WindSpeed   *float64
	Reason      *string
	UpdatedAt   time.Time
}

func waitForWeatherJob(t *testing.T, client *river.Client[pgx.Tx], jobID int64, timeout time.Duration) *rivertype.JobRow {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := client.JobGet(context.Background(), jobID)
		if err != nil {
			t.Fatalf("get weather_sync job %d: %v", jobID, err)
		}
		switch job.State {
		case rivertype.JobStateCompleted:
			return job
		case rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			t.Fatalf("weather_sync job %d reached terminal non-success state %q", jobID, job.State)
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for weather_sync job %d completion", jobID)
	return nil
}

func loadWeatherSmokeState(ctx context.Context, t *testing.T, pool *pgxpool.Pool, externalID string) weatherSmokeState {
	t.Helper()

	var state weatherSmokeState
	if err := pool.QueryRow(ctx, `
        SELECT
            gws.roof_type,
            gws.temperature_f,
            gws.wind_speed_mph,
            gws.raw_json ->> 'reason',
            gws.updated_at
        FROM game_weather_snapshots gws
        JOIN games g ON g.id = gws.game_id
        WHERE g.external_id = $1
    `, externalID).Scan(&state.RoofType, &state.Temperature, &state.WindSpeed, &state.Reason, &state.UpdatedAt); err != nil {
		t.Fatalf("load weather smoke state for %s: %v", externalID, err)
	}

	state.UpdatedAt = state.UpdatedAt.UTC()
	return state
}

func assertOutdoorSmokeState(t *testing.T, state weatherSmokeState) {
	t.Helper()

	if state.RoofType != "outdoor" {
		t.Fatalf("outdoor roof_type = %q, want outdoor", state.RoofType)
	}
	if state.Temperature == nil {
		t.Fatal("outdoor temperature_f is nil, expected provider metrics")
	}
	if state.WindSpeed == nil {
		t.Fatal("outdoor wind_speed_mph is nil, expected provider metrics")
	}
	if state.Reason != nil {
		t.Fatalf("outdoor reason = %q, want nil", *state.Reason)
	}
}

func assertIndoorSmokeState(t *testing.T, state weatherSmokeState, wantRoof string, wantReason string) {
	t.Helper()

	if state.RoofType != wantRoof {
		t.Fatalf("indoor roof_type = %q, want %q", state.RoofType, wantRoof)
	}
	if state.Temperature != nil {
		t.Fatalf("indoor temperature_f = %+v, want nil", state.Temperature)
	}
	if state.WindSpeed != nil {
		t.Fatalf("indoor wind_speed_mph = %+v, want nil", state.WindSpeed)
	}
	if state.Reason == nil || *state.Reason != wantReason {
		t.Fatalf("indoor reason = %+v, want %q", state.Reason, wantReason)
	}
}

func assertSmokeUpdatedAtUnchanged(t *testing.T, label string, before time.Time, after time.Time) {
	t.Helper()
	if !after.Equal(before) {
		t.Fatalf("%s updated_at changed on idempotent rerun, before=%s after=%s", label, before.Format(time.RFC3339Nano), after.Format(time.RFC3339Nano))
	}
}
