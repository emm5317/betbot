package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeProvider struct {
	err       error
	snapshots map[int64]Snapshot
	calls     []ProviderRequest
}

func (p *fakeProvider) Fetch(_ context.Context, req ProviderRequest) (Snapshot, error) {
	p.calls = append(p.calls, req)
	if p.err != nil {
		return Snapshot{}, p.err
	}
	snapshot, ok := p.snapshots[req.GameID]
	if !ok {
		return Snapshot{}, errors.New("missing fake weather snapshot")
	}
	return snapshot, nil
}

type fakeStore struct {
	games     []store.Game
	listArgs  store.ListUpcomingWeatherGamesParams
	upserts   []store.UpsertGameWeatherSnapshotParams
	listErr   error
	upsertErr error
}

func (s *fakeStore) ListUpcomingWeatherGames(_ context.Context, arg store.ListUpcomingWeatherGamesParams) ([]store.Game, error) {
	s.listArgs = arg
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.games, nil
}

func (s *fakeStore) UpsertGameWeatherSnapshot(_ context.Context, arg store.UpsertGameWeatherSnapshotParams) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.upserts = append(s.upserts, arg)
	return nil
}

func TestNormalizeRequestDefaults(t *testing.T) {
	normalized, err := NormalizeRequest(Request{
		ForecastDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Sport:        " nfl ",
	})
	if err != nil {
		t.Fatalf("NormalizeRequest() error = %v", err)
	}

	expectedDate := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	if !normalized.ForecastDate.Equal(expectedDate) {
		t.Fatalf("forecast date = %s, want %s", normalized.ForecastDate.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if !normalized.RequestedAt.Equal(expectedDate) {
		t.Fatalf("requested at = %s, want %s", normalized.RequestedAt.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if normalized.Sport != "NFL" {
		t.Fatalf("sport = %q, want NFL", normalized.Sport)
	}
}

func TestSyncerRunPersistsOutdoorDomeAndRetractableGames(t *testing.T) {
	storeStub := &fakeStore{
		games: []store.Game{
			{
				ID:           1,
				Sport:        "MLB",
				HomeTeam:     "Boston Red Sox",
				AwayTeam:     "New York Yankees",
				CommenceTime: store.Timestamptz(time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC)),
			},
			{
				ID:           2,
				Sport:        "MLB",
				HomeTeam:     "Tampa Bay Rays",
				AwayTeam:     "Toronto Blue Jays",
				CommenceTime: store.Timestamptz(time.Date(2026, time.March, 12, 23, 40, 0, 0, time.UTC)),
			},
			{
				ID:           3,
				Sport:        "MLB",
				HomeTeam:     "Toronto Blue Jays",
				AwayTeam:     "Seattle Mariners",
				CommenceTime: store.Timestamptz(time.Date(2026, time.March, 12, 23, 7, 0, 0, time.UTC)),
			},
		},
	}
	provider := &fakeProvider{snapshots: map[int64]Snapshot{
		1: {
			ForecastTime:             time.Date(2026, time.March, 12, 19, 0, 0, 0, time.UTC),
			TemperatureF:             floatPtr(68.5),
			ApparentTemperatureF:     floatPtr(67.0),
			PrecipitationProbability: floatPtr(15),
			PrecipitationInches:      floatPtr(0.02),
			WindSpeedMph:             floatPtr(9.4),
			WindGustMph:              floatPtr(14.1),
			WindDirectionDegrees:     int32Ptr(220),
			WeatherCode:              int32Ptr(3),
			RawJSON:                  json.RawMessage(`{"hourly":{"temperature_2m":68.5}}`),
		},
	}}

	syncer := NewSyncer(provider, nil)
	metrics, err := syncer.Run(context.Background(), storeStub, Request{
		RequestedAt:  time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
		ForecastDate: time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if metrics.GamesConsidered != 3 || metrics.PersistedRows != 3 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if metrics.ProviderCalls != 1 || metrics.OutdoorRows != 1 || metrics.IndoorRows != 2 || metrics.DomeRows != 1 || metrics.RetractableRows != 1 {
		t.Fatalf("unexpected provider/roof metrics: %+v", metrics)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider call count = %d, want 1", len(provider.calls))
	}
	if provider.calls[0].Venue.Name != "Fenway Park" {
		t.Fatalf("provider venue = %q, want Fenway Park", provider.calls[0].Venue.Name)
	}
	if len(storeStub.upserts) != 3 {
		t.Fatalf("upserts = %d, want 3", len(storeStub.upserts))
	}

	outdoor := storeStub.upserts[0]
	if outdoor.RoofType != string(RoofTypeOutdoor) {
		t.Fatalf("outdoor roof type = %q, want %q", outdoor.RoofType, RoofTypeOutdoor)
	}
	if outdoor.VenueName != "Fenway Park" {
		t.Fatalf("outdoor venue = %q, want Fenway Park", outdoor.VenueName)
	}
	if outdoor.WeatherCode == nil || *outdoor.WeatherCode != 3 {
		t.Fatalf("outdoor weather code = %+v, want 3", outdoor.WeatherCode)
	}
	if outdoor.TemperatureF == nil || *outdoor.TemperatureF != 68.5 {
		t.Fatalf("outdoor temperature = %+v, want 68.5", outdoor.TemperatureF)
	}

	dome := storeStub.upserts[1]
	if dome.RoofType != string(RoofTypeDome) {
		t.Fatalf("dome roof type = %q, want %q", dome.RoofType, RoofTypeDome)
	}
	if dome.VenueName != "Tropicana Field" {
		t.Fatalf("dome venue = %q, want Tropicana Field", dome.VenueName)
	}
	if dome.TemperatureF != nil || dome.WindSpeedMph != nil {
		t.Fatalf("expected dome row to skip weather metrics, got %+v", dome)
	}
	if !bytes.Contains(dome.RawJson, []byte(`"reason":"fixed-roof-indoor"`)) {
		t.Fatalf("expected dome raw_json to include fixed-roof reason, got %s", string(dome.RawJson))
	}

	retractable := storeStub.upserts[2]
	if retractable.RoofType != string(RoofTypeRetractable) {
		t.Fatalf("retractable roof type = %q, want %q", retractable.RoofType, RoofTypeRetractable)
	}
	if retractable.VenueName != "Rogers Centre" {
		t.Fatalf("retractable venue = %q, want Rogers Centre", retractable.VenueName)
	}
	if retractable.TemperatureF != nil || retractable.WindSpeedMph != nil {
		t.Fatalf("expected retractable row to skip weather metrics, got %+v", retractable)
	}
	if !bytes.Contains(retractable.RawJson, []byte(`"reason":"retractable-roof-state-unknown"`)) {
		t.Fatalf("expected retractable raw_json to include unknown-roof reason, got %s", string(retractable.RawJson))
	}
}

func TestSyncerRunPropagatesProviderError(t *testing.T) {
	storeStub := &fakeStore{games: []store.Game{{
		ID:           7,
		Sport:        "NFL",
		HomeTeam:     "Buffalo Bills",
		AwayTeam:     "Miami Dolphins",
		CommenceTime: store.Timestamptz(time.Date(2026, time.September, 13, 17, 0, 0, 0, time.UTC)),
	}}}
	provider := &fakeProvider{err: errors.New("boom")}

	_, err := NewSyncer(provider, nil).Run(context.Background(), storeStub, Request{
		RequestedAt:  time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC),
		ForecastDate: time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC),
	})
	if err == nil || err.Error() != "sync weather for game 7: boom" {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}
