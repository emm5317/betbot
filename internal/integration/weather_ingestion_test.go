package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/ingestion/weather"
	"betbot/internal/store"
	workerpkg "betbot/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type integrationWeatherProvider struct {
	snapshots map[string]weather.Snapshot
}

func (p *integrationWeatherProvider) Fetch(_ context.Context, req weather.ProviderRequest) (weather.Snapshot, error) {
	return p.snapshots[req.HomeTeam], nil
}

func TestWeatherIngestionIdempotentUpserts(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	insertWeatherGame(ctx, t, pool, "game-weather-1", "MLB", "Boston Red Sox", "New York Yankees", time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC))
	insertWeatherGame(ctx, t, pool, "game-weather-2", "MLB", "Tampa Bay Rays", "Toronto Blue Jays", time.Date(2026, time.March, 12, 23, 40, 0, 0, time.UTC))
	insertWeatherGame(ctx, t, pool, "game-weather-3", "MLB", "Toronto Blue Jays", "Seattle Mariners", time.Date(2026, time.March, 12, 23, 7, 0, 0, time.UTC))

	provider := &integrationWeatherProvider{snapshots: map[string]weather.Snapshot{
		"Boston Red Sox": weatherSnapshotForIntegration(68.5, 0.02),
	}}
	syncer := weather.NewSyncer(provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := weather.Request{
		RequestedAt:  time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
		ForecastDate: time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
	}

	if _, err := syncer.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("first weather run: %v", err)
	}
	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM game_weather_snapshots"); got != 3 {
		t.Fatalf("expected 3 weather rows after first run, got %d", got)
	}

	outdoorTemp, outdoorRoof, outdoorUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-1")
	domeTemp, domeRoof, domeUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-2")
	retractableTemp, retractableRoof, retractableUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-3")
	if outdoorTemp == nil || *outdoorTemp != 68.5 {
		t.Fatalf("expected outdoor temperature 68.5, got %+v", outdoorTemp)
	}
	if outdoorRoof != "outdoor" {
		t.Fatalf("expected outdoor roof type, got %q", outdoorRoof)
	}
	if domeTemp != nil {
		t.Fatalf("expected dome temperature to be nil, got %+v", domeTemp)
	}
	if domeRoof != "dome" {
		t.Fatalf("expected dome roof type dome, got %q", domeRoof)
	}
	if retractableTemp != nil {
		t.Fatalf("expected retractable temperature to be nil, got %+v", retractableTemp)
	}
	if retractableRoof != "retractable" {
		t.Fatalf("expected retractable roof type, got %q", retractableRoof)
	}

	time.Sleep(20 * time.Millisecond)
	if _, err := syncer.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("second identical weather run: %v", err)
	}

	repeatedOutdoorTemp, repeatedOutdoorRoof, repeatedOutdoorUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-1")
	repeatedDomeTemp, repeatedDomeRoof, repeatedDomeUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-2")
	repeatedRetractableTemp, repeatedRetractableRoof, repeatedRetractableUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-3")
	if repeatedOutdoorTemp == nil || *repeatedOutdoorTemp != *outdoorTemp || repeatedOutdoorRoof != outdoorRoof {
		t.Fatalf("expected identical outdoor rerun to preserve values, got temp=%+v roof=%q", repeatedOutdoorTemp, repeatedOutdoorRoof)
	}
	if repeatedDomeTemp != nil || repeatedDomeRoof != domeRoof {
		t.Fatalf("expected identical dome rerun to preserve values, got temp=%+v roof=%q", repeatedDomeTemp, repeatedDomeRoof)
	}
	if repeatedRetractableTemp != nil || repeatedRetractableRoof != retractableRoof {
		t.Fatalf("expected identical retractable rerun to preserve values, got temp=%+v roof=%q", repeatedRetractableTemp, repeatedRetractableRoof)
	}
	if !repeatedOutdoorUpdatedAt.Equal(outdoorUpdatedAt) {
		t.Fatalf("expected identical outdoor rerun to preserve updated_at, before=%s after=%s", outdoorUpdatedAt.Format(time.RFC3339Nano), repeatedOutdoorUpdatedAt.Format(time.RFC3339Nano))
	}
	if !repeatedDomeUpdatedAt.Equal(domeUpdatedAt) {
		t.Fatalf("expected identical dome rerun to preserve updated_at, before=%s after=%s", domeUpdatedAt.Format(time.RFC3339Nano), repeatedDomeUpdatedAt.Format(time.RFC3339Nano))
	}
	if !repeatedRetractableUpdatedAt.Equal(retractableUpdatedAt) {
		t.Fatalf("expected identical retractable rerun to preserve updated_at, before=%s after=%s", retractableUpdatedAt.Format(time.RFC3339Nano), repeatedRetractableUpdatedAt.Format(time.RFC3339Nano))
	}

	provider.snapshots["Boston Red Sox"] = weatherSnapshotForIntegration(72.0, 0.10)
	time.Sleep(20 * time.Millisecond)
	if _, err := syncer.Run(ctx, store.New(pool), request); err != nil {
		t.Fatalf("third changed weather run: %v", err)
	}

	updatedOutdoorTemp, _, updatedOutdoorUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-1")
	_, _, updatedDomeUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-2")
	_, _, updatedRetractableUpdatedAt := loadWeatherState(ctx, t, pool, "game-weather-3")
	if updatedOutdoorTemp == nil || *updatedOutdoorTemp != 72.0 {
		t.Fatalf("expected updated outdoor temperature 72.0, got %+v", updatedOutdoorTemp)
	}
	if !updatedOutdoorUpdatedAt.After(outdoorUpdatedAt) {
		t.Fatalf("expected outdoor updated_at to advance, before=%s after=%s", outdoorUpdatedAt.Format(time.RFC3339Nano), updatedOutdoorUpdatedAt.Format(time.RFC3339Nano))
	}
	if !updatedDomeUpdatedAt.Equal(domeUpdatedAt) {
		t.Fatalf("expected dome updated_at to remain unchanged, before=%s after=%s", domeUpdatedAt.Format(time.RFC3339Nano), updatedDomeUpdatedAt.Format(time.RFC3339Nano))
	}
	if !updatedRetractableUpdatedAt.Equal(retractableUpdatedAt) {
		t.Fatalf("expected retractable updated_at to remain unchanged, before=%s after=%s", retractableUpdatedAt.Format(time.RFC3339Nano), updatedRetractableUpdatedAt.Format(time.RFC3339Nano))
	}
}

func TestWeatherSyncWorkerWork(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	insertWeatherGame(ctx, t, pool, "game-weather-worker", "NFL", "Buffalo Bills", "Miami Dolphins", time.Date(2026, time.September, 13, 17, 0, 0, 0, time.UTC))

	worker := workerpkg.NewWeatherSyncWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&integrationWeatherProvider{snapshots: map[string]weather.Snapshot{
			"Buffalo Bills": weatherSnapshotForIntegration(55.0, 0.25),
		}},
	)

	err := worker.Work(ctx, &river.Job[workerpkg.WeatherSyncArgs]{
		Args: workerpkg.WeatherSyncArgs{
			RequestedAt:  time.Date(2026, time.September, 13, 9, 0, 0, 0, time.UTC),
			ForecastDate: time.Date(2026, time.September, 13, 0, 0, 0, 0, time.UTC),
			Sport:        "NFL",
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}

	if got := countRows(ctx, t, pool, "SELECT COUNT(*) FROM game_weather_snapshots"); got != 1 {
		t.Fatalf("expected 1 weather row, got %d", got)
	}
}

func weatherSnapshotForIntegration(temp float64, precip float64) weather.Snapshot {
	return weather.Snapshot{
		Source:                   "open-meteo",
		ForecastTime:             time.Date(2026, time.March, 12, 19, 0, 0, 0, time.UTC),
		TemperatureF:             float64Pointer(temp),
		ApparentTemperatureF:     float64Pointer(temp - 1.5),
		PrecipitationProbability: float64Pointer(20),
		PrecipitationInches:      float64Pointer(precip),
		WindSpeedMph:             float64Pointer(11.2),
		WindGustMph:              float64Pointer(18.4),
		WindDirectionDegrees:     int32Pointer(240),
		WeatherCode:              int32Pointer(61),
		RawJSON:                  json.RawMessage(`{"forecast_time":"2026-03-12T19:00:00Z"}`),
	}
}

func insertWeatherGame(ctx context.Context, t *testing.T, pool *pgxpool.Pool, externalID string, sport string, homeTeam string, awayTeam string, commenceTime time.Time) {
	t.Helper()

	if _, err := store.New(pool).UpsertGame(ctx, store.UpsertGameParams{
		Source:       "the-odds-api",
		ExternalID:   externalID,
		Sport:        sport,
		HomeTeam:     homeTeam,
		AwayTeam:     awayTeam,
		CommenceTime: store.Timestamptz(commenceTime),
	}); err != nil {
		t.Fatalf("upsert weather game %s: %v", externalID, err)
	}
}

func loadWeatherState(ctx context.Context, t *testing.T, pool *pgxpool.Pool, externalID string) (*float64, string, time.Time) {
	t.Helper()

	var temp *float64
	var roofType string
	var updatedAt time.Time
	if err := pool.QueryRow(ctx, `
        SELECT gws.temperature_f, gws.roof_type, gws.updated_at
        FROM game_weather_snapshots gws
        JOIN games g ON g.id = gws.game_id
        WHERE g.external_id = $1
    `, externalID).Scan(&temp, &roofType, &updatedAt); err != nil {
		t.Fatalf("load weather state for %s: %v", externalID, err)
	}
	return temp, roofType, updatedAt.UTC()
}

func int32Pointer(value int32) *int32 {
	return &value
}
