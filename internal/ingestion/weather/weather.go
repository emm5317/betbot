package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"betbot/internal/store"
)

const (
	defaultWeatherSource = "open-meteo"
	forecastWindowDays   = 7
)

var ErrProviderUnconfigured = errors.New("weather provider is not configured")

type Provider interface {
	Fetch(ctx context.Context, req ProviderRequest) (Snapshot, error)
}

type Store interface {
	ListUpcomingWeatherGames(ctx context.Context, arg store.ListUpcomingWeatherGamesParams) ([]store.Game, error)
	UpsertGameWeatherSnapshot(ctx context.Context, arg store.UpsertGameWeatherSnapshotParams) error
}

type Request struct {
	RequestedAt  time.Time
	ForecastDate time.Time
	Sport        string
}

type ProviderRequest struct {
	GameID       int64
	Sport        string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
	RequestedAt  time.Time
	ForecastDate time.Time
	Venue        Venue
}

type Snapshot struct {
	Source                   string
	ForecastTime             time.Time
	Venue                    Venue
	WeatherCode              *int32
	TemperatureF             *float64
	ApparentTemperatureF     *float64
	PrecipitationProbability *float64
	PrecipitationInches      *float64
	WindSpeedMph             *float64
	WindGustMph              *float64
	WindDirectionDegrees     *int32
	RawJSON                  json.RawMessage
}

type RunMetrics struct {
	GamesConsidered   int
	PersistedRows     int
	ProviderCalls     int
	OutdoorRows       int
	IndoorRows        int
	DomeRows          int
	RetractableRows   int
	MissingVenueGames int
}

type Syncer struct {
	provider Provider
	logger   *slog.Logger
}

type UnconfiguredProvider struct{}

func (UnconfiguredProvider) Fetch(context.Context, ProviderRequest) (Snapshot, error) {
	return Snapshot{}, ErrProviderUnconfigured
}

func NewSyncer(provider Provider, logger *slog.Logger) *Syncer {
	if provider == nil {
		provider = UnconfiguredProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{provider: provider, logger: logger}
}

func (s *Syncer) Run(ctx context.Context, queries Store, req Request) (RunMetrics, error) {
	if queries == nil {
		return RunMetrics{}, errors.New("weather sync store is nil")
	}

	normalizedReq, err := NormalizeRequest(req)
	if err != nil {
		return RunMetrics{}, err
	}

	windowStart := normalizedReq.RequestedAt
	if windowStart.Before(normalizedReq.ForecastDate) {
		windowStart = normalizedReq.ForecastDate
	}
	windowEnd := normalizedReq.ForecastDate.AddDate(0, 0, forecastWindowDays)

	games, err := queries.ListUpcomingWeatherGames(ctx, store.ListUpcomingWeatherGamesParams{
		Sports:      sportsForRequest(normalizedReq.Sport),
		WindowStart: store.Timestamptz(windowStart),
		WindowEnd:   store.Timestamptz(windowEnd),
	})
	if err != nil {
		return RunMetrics{}, fmt.Errorf("list upcoming weather games: %w", err)
	}

	metrics := RunMetrics{GamesConsidered: len(games)}
	for _, game := range games {
		if !game.CommenceTime.Valid {
			return RunMetrics{}, fmt.Errorf("game %d commence time is invalid", game.ID)
		}

		venue, ok := LookupVenue(game.Sport, game.HomeTeam, game.CommenceTime.Time.UTC())
		if !ok {
			metrics.MissingVenueGames++
			s.logger.WarnContext(ctx, "weather sync skipped game without venue metadata",
				slog.Int64("game_id", game.ID),
				slog.String("sport", game.Sport),
				slog.String("home_team", game.HomeTeam),
			)
			continue
		}

		providerReq := ProviderRequest{
			GameID:       game.ID,
			Sport:        game.Sport,
			HomeTeam:     game.HomeTeam,
			AwayTeam:     game.AwayTeam,
			CommenceTime: game.CommenceTime.Time.UTC(),
			RequestedAt:  normalizedReq.RequestedAt,
			ForecastDate: normalizedReq.ForecastDate,
			Venue:        venue,
		}

		policy := venue.WeatherPolicy()

		snapshot, err := s.snapshotForGame(ctx, providerReq, policy)
		if err != nil {
			return RunMetrics{}, fmt.Errorf("sync weather for game %d: %w", game.ID, err)
		}

		switch policy {
		case RoofWeatherPolicyOutdoor:
			metrics.ProviderCalls++
			metrics.OutdoorRows++
		case RoofWeatherPolicyFixedIndoor:
			metrics.IndoorRows++
			metrics.DomeRows++
		case RoofWeatherPolicyRetractableUnknown:
			metrics.IndoorRows++
			metrics.RetractableRows++
		default:
			metrics.IndoorRows++
		}

		normalizedSnapshot, err := normalizeSnapshot(snapshot, providerReq)
		if err != nil {
			return RunMetrics{}, fmt.Errorf("normalize weather snapshot for game %d: %w", game.ID, err)
		}

		if err := queries.UpsertGameWeatherSnapshot(ctx, store.UpsertGameWeatherSnapshotParams{
			GameID:                   game.ID,
			Source:                   normalizedSnapshot.Source,
			ForecastTime:             store.Timestamptz(normalizedSnapshot.ForecastTime),
			VenueName:                normalizedSnapshot.Venue.Name,
			VenueTimezone:            normalizedSnapshot.Venue.Timezone,
			Latitude:                 normalizedSnapshot.Venue.Latitude,
			Longitude:                normalizedSnapshot.Venue.Longitude,
			RoofType:                 string(normalizedSnapshot.Venue.RoofType),
			WeatherCode:              normalizedSnapshot.WeatherCode,
			TemperatureF:             normalizedSnapshot.TemperatureF,
			ApparentTemperatureF:     normalizedSnapshot.ApparentTemperatureF,
			PrecipitationProbability: normalizedSnapshot.PrecipitationProbability,
			PrecipitationInches:      normalizedSnapshot.PrecipitationInches,
			WindSpeedMph:             normalizedSnapshot.WindSpeedMph,
			WindGustMph:              normalizedSnapshot.WindGustMph,
			WindDirectionDegrees:     normalizedSnapshot.WindDirectionDegrees,
			RawJson:                  normalizedSnapshot.RawJSON,
		}); err != nil {
			return RunMetrics{}, fmt.Errorf("upsert game weather snapshot %d: %w", game.ID, err)
		}
		metrics.PersistedRows++
	}

	s.logger.InfoContext(ctx, "weather sync completed",
		slog.Int("games_considered", metrics.GamesConsidered),
		slog.Int("persisted_rows", metrics.PersistedRows),
		slog.Int("provider_calls", metrics.ProviderCalls),
		slog.Int("outdoor_rows", metrics.OutdoorRows),
		slog.Int("indoor_rows", metrics.IndoorRows),
		slog.Int("dome_rows", metrics.DomeRows),
		slog.Int("retractable_rows", metrics.RetractableRows),
		slog.Int("missing_venue_games", metrics.MissingVenueGames),
	)

	return metrics, nil
}

func (s *Syncer) snapshotForGame(ctx context.Context, req ProviderRequest, policy RoofWeatherPolicy) (Snapshot, error) {
	if policy.RequiresProvider() {
		return s.provider.Fetch(ctx, req)
	}
	return indoorSnapshot(req, policy)
}

func NormalizeRequest(req Request) (Request, error) {
	forecastDate := req.ForecastDate
	if forecastDate.IsZero() {
		forecastDate = req.RequestedAt
	}
	if forecastDate.IsZero() {
		return Request{}, errors.New("weather forecast date is required")
	}

	normalized := Request{
		RequestedAt:  req.RequestedAt.UTC(),
		ForecastDate: normalizeDate(forecastDate),
		Sport:        normalizeSport(req.Sport),
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.ForecastDate
	}
	if strings.TrimSpace(req.Sport) != "" && normalized.Sport == "" {
		return Request{}, fmt.Errorf("unsupported weather sport %q", req.Sport)
	}
	return normalized, nil
}

func normalizeProviderRequest(req ProviderRequest) (ProviderRequest, error) {
	if req.GameID <= 0 {
		return ProviderRequest{}, errors.New("weather game id is required")
	}
	if req.CommenceTime.IsZero() {
		return ProviderRequest{}, errors.New("weather commence time is required")
	}
	if req.Venue.Name == "" {
		return ProviderRequest{}, errors.New("weather venue name is required")
	}
	if req.Venue.Timezone == "" {
		return ProviderRequest{}, errors.New("weather venue timezone is required")
	}

	normalized := req
	normalized.Sport = normalizeSport(req.Sport)
	if normalized.Sport == "" {
		return ProviderRequest{}, errors.New("weather sport is required")
	}
	normalized.HomeTeam = normalizeLabel(req.HomeTeam)
	normalized.AwayTeam = normalizeLabel(req.AwayTeam)
	normalized.CommenceTime = req.CommenceTime.UTC()
	normalized.RequestedAt = req.RequestedAt.UTC()
	normalized.ForecastDate = normalizeDate(req.ForecastDate)
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.ForecastDate
	}
	if normalized.ForecastDate.IsZero() {
		normalized.ForecastDate = normalizeDate(normalized.CommenceTime)
	}
	return normalized, nil
}

func normalizeSnapshot(snapshot Snapshot, req ProviderRequest) (Snapshot, error) {
	normalized := snapshot
	if normalized.Source == "" {
		normalized.Source = defaultWeatherSource
	}
	if normalized.ForecastTime.IsZero() {
		normalized.ForecastTime = roundToNearestHour(req.CommenceTime)
	} else {
		normalized.ForecastTime = normalized.ForecastTime.UTC()
	}
	if normalized.Venue.Name == "" {
		normalized.Venue = req.Venue
	}
	if len(normalized.RawJSON) == 0 {
		return Snapshot{}, errors.New("weather raw json is required")
	}
	if normalized.Venue.Name == "" || normalized.Venue.Timezone == "" {
		return Snapshot{}, errors.New("weather venue metadata is required")
	}
	return normalized, nil
}

func indoorSnapshot(req ProviderRequest, policy RoofWeatherPolicy) (Snapshot, error) {
	rawJSON, err := json.Marshal(map[string]any{
		"reason":         policy.IndoorReason(),
		"weather_policy": policy,
		"roof_type":      req.Venue.RoofType,
		"venue_name":     req.Venue.Name,
		"latitude":       req.Venue.Latitude,
		"longitude":      req.Venue.Longitude,
		"provider_fetch": "skipped",
		"game_id":        req.GameID,
		"commence_at":    req.CommenceTime.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal indoor weather payload: %w", err)
	}
	return Snapshot{
		Source:       defaultWeatherSource,
		ForecastTime: roundToNearestHour(req.CommenceTime),
		Venue:        req.Venue,
		RawJSON:      rawJSON,
	}, nil
}

func sportsForRequest(sport string) []string {
	if sport == "" {
		return []string{"MLB", "NFL"}
	}
	return []string{sport}
}

func normalizeSport(value string) string {
	switch normalizeSlug(value) {
	case "mlb":
		return "MLB"
	case "nfl":
		return "NFL"
	default:
		return ""
	}
}

func normalizeDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func roundToNearestHour(value time.Time) time.Time {
	utc := value.UTC()
	truncated := utc.Truncate(time.Hour)
	if utc.Sub(truncated) >= 30*time.Minute {
		return truncated.Add(time.Hour)
	}
	return truncated
}

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLabel(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
