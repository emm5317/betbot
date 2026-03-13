package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultOpenMeteoBaseURL = "https://api.open-meteo.com"
	defaultOpenMeteoTimeout = 10 * time.Second
)

type OpenMeteoProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewOpenMeteoProvider(baseURL string, timeout time.Duration) *OpenMeteoProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultOpenMeteoBaseURL
	}
	if timeout <= 0 {
		timeout = defaultOpenMeteoTimeout
	}
	return &OpenMeteoProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (p *OpenMeteoProvider) Fetch(ctx context.Context, req ProviderRequest) (Snapshot, error) {
	normalizedReq, err := normalizeProviderRequest(req)
	if err != nil {
		return Snapshot{}, err
	}
	if !normalizedReq.Venue.IsOutdoor() {
		return Snapshot{}, fmt.Errorf("open-meteo provider requires outdoor venue, got %s", normalizedReq.Venue.RoofType)
	}

	endpoint, err := p.forecastURL(normalizedReq)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build open-meteo endpoint: %w", err)
	}
	body, err := p.getBody(ctx, endpoint.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch open-meteo forecast: %w", err)
	}

	var payload openMeteoForecastResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode open-meteo forecast: %w", err)
	}
	return mapOpenMeteoSnapshot(payload, normalizedReq)
}

func (p *OpenMeteoProvider) forecastURL(req ProviderRequest) (*url.URL, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(endpoint.Path, "v1", "forecast")

	query := endpoint.Query()
	query.Set("latitude", formatCoordinate(req.Venue.Latitude))
	query.Set("longitude", formatCoordinate(req.Venue.Longitude))
	query.Set("timezone", "UTC")
	query.Set("temperature_unit", "fahrenheit")
	query.Set("wind_speed_unit", "mph")
	query.Set("precipitation_unit", "inch")
	query.Set("start_date", req.CommenceTime.UTC().Format(time.DateOnly))
	query.Set("end_date", req.CommenceTime.UTC().Format(time.DateOnly))
	query.Set("hourly", strings.Join([]string{
		"temperature_2m",
		"apparent_temperature",
		"precipitation_probability",
		"precipitation",
		"wind_speed_10m",
		"wind_gusts_10m",
		"wind_direction_10m",
		"weather_code",
	}, ","))
	endpoint.RawQuery = query.Encode()
	return endpoint, nil
}

func (p *OpenMeteoProvider) getBody(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func mapOpenMeteoSnapshot(payload openMeteoForecastResponse, req ProviderRequest) (Snapshot, error) {
	idx, forecastTime, err := closestHourlyForecast(payload.Hourly.Time, req.CommenceTime)
	if err != nil {
		return Snapshot{}, err
	}

	rawJSON, err := json.Marshal(map[string]any{
		"latitude":      payload.Latitude,
		"longitude":     payload.Longitude,
		"timezone":      payload.Timezone,
		"forecast_time": forecastTime.UTC().Format(time.RFC3339),
		"hourly": map[string]any{
			"temperature_2m":            valueAt(payload.Hourly.Temperature2M, idx),
			"apparent_temperature":      valueAt(payload.Hourly.ApparentTemperature, idx),
			"precipitation_probability": valueAt(payload.Hourly.PrecipitationProbability, idx),
			"precipitation":             valueAt(payload.Hourly.Precipitation, idx),
			"wind_speed_10m":            valueAt(payload.Hourly.WindSpeed10M, idx),
			"wind_gusts_10m":            valueAt(payload.Hourly.WindGusts10M, idx),
			"wind_direction_10m":        valueAt(payload.Hourly.WindDirection10M, idx),
			"weather_code":              valueAt(payload.Hourly.WeatherCode, idx),
		},
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal selected open-meteo payload: %w", err)
	}

	return Snapshot{
		Source:                   defaultWeatherSource,
		ForecastTime:             forecastTime,
		Venue:                    req.Venue,
		WeatherCode:              int32Pointer(valueAt(payload.Hourly.WeatherCode, idx)),
		TemperatureF:             float64Pointer(valueAt(payload.Hourly.Temperature2M, idx)),
		ApparentTemperatureF:     float64Pointer(valueAt(payload.Hourly.ApparentTemperature, idx)),
		PrecipitationProbability: float64Pointer(valueAt(payload.Hourly.PrecipitationProbability, idx)),
		PrecipitationInches:      float64Pointer(valueAt(payload.Hourly.Precipitation, idx)),
		WindSpeedMph:             float64Pointer(valueAt(payload.Hourly.WindSpeed10M, idx)),
		WindGustMph:              float64Pointer(valueAt(payload.Hourly.WindGusts10M, idx)),
		WindDirectionDegrees:     int32Pointer(valueAt(payload.Hourly.WindDirection10M, idx)),
		RawJSON:                  rawJSON,
	}, nil
}

func closestHourlyForecast(values []string, commenceTime time.Time) (int, time.Time, error) {
	if len(values) == 0 {
		return 0, time.Time{}, fmt.Errorf("open-meteo hourly time series is empty")
	}

	var (
		bestIdx   = -1
		bestTime  time.Time
		bestDelta time.Duration
	)
	for idx, value := range values {
		forecastTime, err := time.ParseInLocation("2006-01-02T15:04", strings.TrimSpace(value), time.UTC)
		if err != nil {
			return 0, time.Time{}, fmt.Errorf("parse hourly time %q: %w", value, err)
		}

		delta := absDuration(forecastTime.Sub(commenceTime.UTC()))
		if bestIdx == -1 || delta < bestDelta || (delta == bestDelta && forecastTime.After(bestTime)) {
			bestIdx = idx
			bestTime = forecastTime.UTC()
			bestDelta = delta
		}
	}

	if bestIdx == -1 {
		return 0, time.Time{}, fmt.Errorf("no hourly forecast matched commence time")
	}
	return bestIdx, bestTime, nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func formatCoordinate(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", value), "0"), ".")
}

func float64Pointer(value float64) *float64 {
	if math.IsNaN(value) {
		return nil
	}
	return &value
}

func int32Pointer(value int32) *int32 {
	return &value
}

func valueAt[T any](values []T, idx int) T {
	var zero T
	if idx < 0 || idx >= len(values) {
		return zero
	}
	return values[idx]
}

type openMeteoForecastResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Hourly    struct {
		Time                     []string  `json:"time"`
		Temperature2M            []float64 `json:"temperature_2m"`
		ApparentTemperature      []float64 `json:"apparent_temperature"`
		PrecipitationProbability []float64 `json:"precipitation_probability"`
		Precipitation            []float64 `json:"precipitation"`
		WindSpeed10M             []float64 `json:"wind_speed_10m"`
		WindGusts10M             []float64 `json:"wind_gusts_10m"`
		WindDirection10M         []int32   `json:"wind_direction_10m"`
		WeatherCode              []int32   `json:"weather_code"`
	} `json:"hourly"`
}
