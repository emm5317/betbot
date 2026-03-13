package weather

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenMeteoProviderFetchMapsForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			t.Fatalf("path = %q, want /v1/forecast", r.URL.Path)
		}
		if r.URL.Query().Get("temperature_unit") != "fahrenheit" {
			t.Fatalf("temperature_unit = %q", r.URL.Query().Get("temperature_unit"))
		}
		if r.URL.Query().Get("wind_speed_unit") != "mph" {
			t.Fatalf("wind_speed_unit = %q", r.URL.Query().Get("wind_speed_unit"))
		}
		if r.URL.Query().Get("precipitation_unit") != "inch" {
			t.Fatalf("precipitation_unit = %q", r.URL.Query().Get("precipitation_unit"))
		}
		if r.URL.Query().Get("timezone") != "UTC" {
			t.Fatalf("timezone = %q", r.URL.Query().Get("timezone"))
		}
		if r.URL.Query().Get("start_date") != "2026-03-12" || r.URL.Query().Get("end_date") != "2026-03-12" {
			t.Fatalf("unexpected date query: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{
  "latitude": 42.3467,
  "longitude": -71.0972,
  "timezone": "UTC",
  "hourly": {
    "time": ["2026-03-12T18:00", "2026-03-12T19:00"],
    "temperature_2m": [65.0, 68.5],
    "apparent_temperature": [64.0, 67.0],
    "precipitation_probability": [10.0, 15.0],
    "precipitation": [0.0, 0.02],
    "wind_speed_10m": [8.0, 9.4],
    "wind_gusts_10m": [12.0, 14.1],
    "wind_direction_10m": [210, 220],
    "weather_code": [1, 3]
  }
}`)
	}))
	defer server.Close()

	provider := NewOpenMeteoProvider(server.URL, time.Second)
	snapshot, err := provider.Fetch(context.Background(), ProviderRequest{
		GameID:       1,
		Sport:        "MLB",
		HomeTeam:     "Boston Red Sox",
		AwayTeam:     "New York Yankees",
		CommenceTime: time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC),
		ForecastDate: time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC),
		Venue: Venue{
			Name:      "Fenway Park",
			Timezone:  "America/New_York",
			Latitude:  42.3467,
			Longitude: -71.0972,
			RoofType:  RoofTypeOutdoor,
		},
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ForecastTime.Format(time.RFC3339) != "2026-03-12T19:00:00Z" {
		t.Fatalf("forecast time = %s, want 2026-03-12T19:00:00Z", snapshot.ForecastTime.Format(time.RFC3339))
	}
	if snapshot.TemperatureF == nil || *snapshot.TemperatureF != 68.5 {
		t.Fatalf("temperature = %+v, want 68.5", snapshot.TemperatureF)
	}
	if snapshot.WindDirectionDegrees == nil || *snapshot.WindDirectionDegrees != 220 {
		t.Fatalf("wind direction = %+v, want 220", snapshot.WindDirectionDegrees)
	}
	if !strings.Contains(string(snapshot.RawJSON), `"forecast_time":"2026-03-12T19:00:00Z"`) {
		t.Fatalf("raw json missing selected forecast time: %s", string(snapshot.RawJSON))
	}
}

func TestOpenMeteoProviderFetchHandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := NewOpenMeteoProvider(server.URL, time.Second)
	_, err := provider.Fetch(context.Background(), ProviderRequest{
		GameID:       2,
		Sport:        "NFL",
		HomeTeam:     "Buffalo Bills",
		AwayTeam:     "Miami Dolphins",
		CommenceTime: time.Date(2026, time.September, 13, 17, 0, 0, 0, time.UTC),
		ForecastDate: time.Date(2026, time.September, 13, 0, 0, 0, 0, time.UTC),
		Venue: Venue{
			Name:      "Highmark Stadium",
			Timezone:  "America/New_York",
			Latitude:  42.7738,
			Longitude: -78.7868,
			RoofType:  RoofTypeOutdoor,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("expected status error, got %v", err)
	}
}
