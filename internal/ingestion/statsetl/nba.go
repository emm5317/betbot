package statsetl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/store"
)

const (
	defaultNBASource     = "nba-stats-api"
	defaultNBASeasonType = "regular"
)

var ErrNBAProviderUnconfigured = errors.New("nba stats provider is not configured")

type NBAProvider interface {
	Fetch(ctx context.Context, req NBARequest) (NBASnapshot, error)
}

type NBAStore interface {
	UpsertNBATeamStats(ctx context.Context, arg store.UpsertNBATeamStatsParams) error
}

type NBARequest struct {
	RequestedAt time.Time
	Season      int32
	SeasonType  string
	StatDate    time.Time
}

type NBASnapshot struct {
	Source     string
	Season     int32
	SeasonType string
	StatDate   time.Time
	Teams      []NBATeamStat
}

type NBATeamStat struct {
	ExternalID      string
	TeamName        string
	GamesPlayed     int32
	Wins            int32
	Losses          int32
	OffensiveRating *float64
	DefensiveRating *float64
	NetRating       *float64
	Pace            *float64
}

type NBARunMetrics struct {
	TeamRows int
}

type NBAETL struct {
	provider NBAProvider
	logger   *slog.Logger
}

type UnconfiguredNBAProvider struct{}

func (UnconfiguredNBAProvider) Fetch(context.Context, NBARequest) (NBASnapshot, error) {
	return NBASnapshot{}, ErrNBAProviderUnconfigured
}

func NewNBAETL(provider NBAProvider, logger *slog.Logger) *NBAETL {
	if provider == nil {
		provider = UnconfiguredNBAProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &NBAETL{provider: provider, logger: logger}
}

func (e *NBAETL) Run(ctx context.Context, queries NBAStore, req NBARequest) (NBARunMetrics, error) {
	if queries == nil {
		return NBARunMetrics{}, errors.New("nba etl store is nil")
	}

	normalizedReq, err := NormalizeNBARequest(req)
	if err != nil {
		return NBARunMetrics{}, err
	}

	snapshot, err := e.provider.Fetch(ctx, normalizedReq)
	if err != nil {
		return NBARunMetrics{}, fmt.Errorf("fetch nba stats: %w", err)
	}

	normalizedSnapshot, err := normalizeNBASnapshot(snapshot, normalizedReq)
	if err != nil {
		return NBARunMetrics{}, err
	}

	metrics := NBARunMetrics{TeamRows: len(normalizedSnapshot.Teams)}
	for _, team := range normalizedSnapshot.Teams {
		if err := queries.UpsertNBATeamStats(ctx, store.UpsertNBATeamStatsParams{
			Source:          normalizedSnapshot.Source,
			ExternalID:      team.ExternalID,
			Season:          normalizedSnapshot.Season,
			SeasonType:      normalizedSnapshot.SeasonType,
			StatDate:        pgDate(normalizedSnapshot.StatDate),
			TeamName:        team.TeamName,
			GamesPlayed:     team.GamesPlayed,
			Wins:            team.Wins,
			Losses:          team.Losses,
			OffensiveRating: team.OffensiveRating,
			DefensiveRating: team.DefensiveRating,
			NetRating:       team.NetRating,
			Pace:            team.Pace,
		}); err != nil {
			return NBARunMetrics{}, fmt.Errorf("upsert nba team stats %s: %w", team.ExternalID, err)
		}
	}

	e.logger.InfoContext(ctx, "nba stats etl completed",
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(normalizedSnapshot.Season)),
		slog.String("season_type", normalizedSnapshot.SeasonType),
		slog.String("stat_date", normalizedSnapshot.StatDate.Format(time.DateOnly)),
		slog.String("source", normalizedSnapshot.Source),
	)

	return metrics, nil
}

func NormalizeNBARequest(req NBARequest) (NBARequest, error) {
	statDate := req.StatDate
	if statDate.IsZero() {
		statDate = req.RequestedAt
	}
	if statDate.IsZero() {
		return NBARequest{}, errors.New("nba stat date is required")
	}

	normalized := NBARequest{
		RequestedAt: req.RequestedAt.UTC(),
		SeasonType:  normalizeSlug(req.SeasonType),
		StatDate:    normalizeDate(statDate),
		Season:      req.Season,
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = defaultNBASeasonType
	}
	if normalized.Season == 0 {
		normalized.Season = inferNBASeason(normalized.StatDate)
	}
	if normalized.Season <= 0 {
		return NBARequest{}, fmt.Errorf("invalid nba season %d", normalized.Season)
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.StatDate
	}

	return normalized, nil
}

func normalizeNBASnapshot(snapshot NBASnapshot, req NBARequest) (NBASnapshot, error) {
	normalized := NBASnapshot{
		Source:     normalizeSlug(snapshot.Source),
		Season:     snapshot.Season,
		SeasonType: normalizeSlug(snapshot.SeasonType),
		StatDate:   snapshot.StatDate,
		Teams:      make([]NBATeamStat, 0, len(snapshot.Teams)),
	}
	if normalized.Source == "" {
		normalized.Source = defaultNBASource
	}
	if normalized.Season == 0 {
		normalized.Season = req.Season
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = req.SeasonType
	}
	if normalized.StatDate.IsZero() {
		normalized.StatDate = req.StatDate
	} else {
		normalized.StatDate = normalizeDate(normalized.StatDate)
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = defaultNBASeasonType
	}
	if normalized.Season <= 0 {
		return NBASnapshot{}, fmt.Errorf("invalid nba season %d", normalized.Season)
	}
	if normalized.StatDate.IsZero() {
		return NBASnapshot{}, errors.New("nba stat date is required")
	}

	for _, team := range snapshot.Teams {
		normalizedTeam, err := normalizeNBATeamStat(team)
		if err != nil {
			return NBASnapshot{}, err
		}
		normalized.Teams = append(normalized.Teams, normalizedTeam)
	}

	return normalized, nil
}

func normalizeNBATeamStat(team NBATeamStat) (NBATeamStat, error) {
	normalized := NBATeamStat{
		ExternalID:      normalizeSlug(team.ExternalID),
		TeamName:        normalizeLabel(team.TeamName),
		GamesPlayed:     team.GamesPlayed,
		Wins:            team.Wins,
		Losses:          team.Losses,
		OffensiveRating: team.OffensiveRating,
		DefensiveRating: team.DefensiveRating,
		NetRating:       team.NetRating,
		Pace:            team.Pace,
	}
	if normalized.ExternalID == "" {
		return NBATeamStat{}, errors.New("nba team external id is required")
	}
	if normalized.TeamName == "" {
		return NBATeamStat{}, fmt.Errorf("nba team name is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func inferNBASeason(statDate time.Time) int32 {
	if statDate.Month() >= time.July {
		return int32(statDate.Year() + 1)
	}
	return int32(statDate.Year())
}
