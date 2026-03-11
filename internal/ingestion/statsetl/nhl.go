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
	defaultNHLSource     = "nhl-web-api"
	defaultNHLSeasonType = "regular"
)

var ErrNHLProviderUnconfigured = errors.New("nhl stats provider is not configured")

type NHLProvider interface {
	Fetch(ctx context.Context, req NHLRequest) (NHLSnapshot, error)
}

type NHLStore interface {
	UpsertNHLTeamStats(ctx context.Context, arg store.UpsertNHLTeamStatsParams) error
}

type NHLRequest struct {
	RequestedAt time.Time
	Season      int32
	SeasonType  string
	StatDate    time.Time
}

type NHLSnapshot struct {
	Source     string
	Season     int32
	SeasonType string
	StatDate   time.Time
	Teams      []NHLTeamStat
}

type NHLTeamStat struct {
	ExternalID          string
	TeamName            string
	GamesPlayed         int32
	Wins                int32
	Losses              int32
	OTLosses            int32
	GoalsForPerGame     *float64
	GoalsAgainstPerGame *float64
	ExpectedGoalsShare  *float64
	SavePercentage      *float64
}

type NHLRunMetrics struct {
	TeamRows int
}

type NHLETL struct {
	provider NHLProvider
	logger   *slog.Logger
}

type UnconfiguredNHLProvider struct{}

func (UnconfiguredNHLProvider) Fetch(context.Context, NHLRequest) (NHLSnapshot, error) {
	return NHLSnapshot{}, ErrNHLProviderUnconfigured
}

func NewNHLETL(provider NHLProvider, logger *slog.Logger) *NHLETL {
	if provider == nil {
		provider = UnconfiguredNHLProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &NHLETL{provider: provider, logger: logger}
}

func (e *NHLETL) Run(ctx context.Context, queries NHLStore, req NHLRequest) (NHLRunMetrics, error) {
	if queries == nil {
		return NHLRunMetrics{}, errors.New("nhl etl store is nil")
	}

	normalizedReq, err := NormalizeNHLRequest(req)
	if err != nil {
		return NHLRunMetrics{}, err
	}

	snapshot, err := e.provider.Fetch(ctx, normalizedReq)
	if err != nil {
		return NHLRunMetrics{}, fmt.Errorf("fetch nhl stats: %w", err)
	}

	normalizedSnapshot, err := normalizeNHLSnapshot(snapshot, normalizedReq)
	if err != nil {
		return NHLRunMetrics{}, err
	}

	metrics := NHLRunMetrics{TeamRows: len(normalizedSnapshot.Teams)}
	for _, team := range normalizedSnapshot.Teams {
		if err := queries.UpsertNHLTeamStats(ctx, store.UpsertNHLTeamStatsParams{
			Source:              normalizedSnapshot.Source,
			ExternalID:          team.ExternalID,
			Season:              normalizedSnapshot.Season,
			SeasonType:          normalizedSnapshot.SeasonType,
			StatDate:            pgDate(normalizedSnapshot.StatDate),
			TeamName:            team.TeamName,
			GamesPlayed:         team.GamesPlayed,
			Wins:                team.Wins,
			Losses:              team.Losses,
			OtLosses:            team.OTLosses,
			GoalsForPerGame:     team.GoalsForPerGame,
			GoalsAgainstPerGame: team.GoalsAgainstPerGame,
			ExpectedGoalsShare:  team.ExpectedGoalsShare,
			SavePercentage:      team.SavePercentage,
		}); err != nil {
			return NHLRunMetrics{}, fmt.Errorf("upsert nhl team stats %s: %w", team.ExternalID, err)
		}
	}

	e.logger.InfoContext(ctx, "nhl stats etl completed",
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(normalizedSnapshot.Season)),
		slog.String("season_type", normalizedSnapshot.SeasonType),
		slog.String("stat_date", normalizedSnapshot.StatDate.Format(time.DateOnly)),
		slog.String("source", normalizedSnapshot.Source),
	)

	return metrics, nil
}

func NormalizeNHLRequest(req NHLRequest) (NHLRequest, error) {
	statDate := req.StatDate
	if statDate.IsZero() {
		statDate = req.RequestedAt
	}
	if statDate.IsZero() {
		return NHLRequest{}, errors.New("nhl stat date is required")
	}

	normalized := NHLRequest{
		RequestedAt: req.RequestedAt.UTC(),
		SeasonType:  normalizeSlug(req.SeasonType),
		StatDate:    normalizeDate(statDate),
		Season:      req.Season,
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = defaultNHLSeasonType
	}
	if normalized.Season == 0 {
		normalized.Season = inferNHLSeason(normalized.StatDate)
	}
	if normalized.Season <= 0 {
		return NHLRequest{}, fmt.Errorf("invalid nhl season %d", normalized.Season)
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.StatDate
	}

	return normalized, nil
}

func normalizeNHLSnapshot(snapshot NHLSnapshot, req NHLRequest) (NHLSnapshot, error) {
	normalized := NHLSnapshot{
		Source:     normalizeSlug(snapshot.Source),
		Season:     snapshot.Season,
		SeasonType: normalizeSlug(snapshot.SeasonType),
		StatDate:   snapshot.StatDate,
		Teams:      make([]NHLTeamStat, 0, len(snapshot.Teams)),
	}
	if normalized.Source == "" {
		normalized.Source = defaultNHLSource
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
		normalized.SeasonType = defaultNHLSeasonType
	}
	if normalized.Season <= 0 {
		return NHLSnapshot{}, fmt.Errorf("invalid nhl season %d", normalized.Season)
	}
	if normalized.StatDate.IsZero() {
		return NHLSnapshot{}, errors.New("nhl stat date is required")
	}

	for _, team := range snapshot.Teams {
		normalizedTeam, err := normalizeNHLTeamStat(team)
		if err != nil {
			return NHLSnapshot{}, err
		}
		normalized.Teams = append(normalized.Teams, normalizedTeam)
	}

	return normalized, nil
}

func normalizeNHLTeamStat(team NHLTeamStat) (NHLTeamStat, error) {
	normalized := NHLTeamStat{
		ExternalID:          normalizeSlug(team.ExternalID),
		TeamName:            normalizeLabel(team.TeamName),
		GamesPlayed:         team.GamesPlayed,
		Wins:                team.Wins,
		Losses:              team.Losses,
		OTLosses:            team.OTLosses,
		GoalsForPerGame:     team.GoalsForPerGame,
		GoalsAgainstPerGame: team.GoalsAgainstPerGame,
		ExpectedGoalsShare:  team.ExpectedGoalsShare,
		SavePercentage:      team.SavePercentage,
	}
	if normalized.ExternalID == "" {
		return NHLTeamStat{}, errors.New("nhl team external id is required")
	}
	if normalized.TeamName == "" {
		return NHLTeamStat{}, fmt.Errorf("nhl team name is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func inferNHLSeason(statDate time.Time) int32 {
	if statDate.Month() >= time.July {
		return int32(statDate.Year() + 1)
	}
	return int32(statDate.Year())
}
