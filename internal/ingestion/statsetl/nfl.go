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
	defaultNFLSource     = "nflverse-nfl-com"
	defaultNFLSeasonType = "regular"
)

var ErrNFLProviderUnconfigured = errors.New("nfl stats provider is not configured")

type NFLProvider interface {
	Fetch(ctx context.Context, req NFLRequest) (NFLSnapshot, error)
}

type NFLStore interface {
	UpsertNFLTeamStats(ctx context.Context, arg store.UpsertNFLTeamStatsParams) error
}

type NFLRequest struct {
	RequestedAt time.Time
	Season      int32
	SeasonType  string
	StatDate    time.Time
}

type NFLSnapshot struct {
	Source     string
	Season     int32
	SeasonType string
	StatDate   time.Time
	Teams      []NFLTeamStat
}

type NFLTeamStat struct {
	ExternalID           string
	TeamName             string
	GamesPlayed          int32
	Wins                 int32
	Losses               int32
	Ties                 int32
	PointsFor            int32
	PointsAgainst        int32
	OffensiveEPAPerPlay  *float64
	DefensiveEPAPerPlay  *float64
	OffensiveSuccessRate *float64
	DefensiveSuccessRate *float64
}

type NFLRunMetrics struct {
	TeamRows int
}

type NFLETL struct {
	provider NFLProvider
	logger   *slog.Logger
}

type UnconfiguredNFLProvider struct{}

func (UnconfiguredNFLProvider) Fetch(context.Context, NFLRequest) (NFLSnapshot, error) {
	return NFLSnapshot{}, ErrNFLProviderUnconfigured
}

func NewNFLETL(provider NFLProvider, logger *slog.Logger) *NFLETL {
	if provider == nil {
		provider = UnconfiguredNFLProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &NFLETL{provider: provider, logger: logger}
}

func (e *NFLETL) Run(ctx context.Context, queries NFLStore, req NFLRequest) (NFLRunMetrics, error) {
	if queries == nil {
		return NFLRunMetrics{}, errors.New("nfl etl store is nil")
	}

	normalizedReq, err := NormalizeNFLRequest(req)
	if err != nil {
		return NFLRunMetrics{}, err
	}

	snapshot, err := e.provider.Fetch(ctx, normalizedReq)
	if err != nil {
		return NFLRunMetrics{}, fmt.Errorf("fetch nfl stats: %w", err)
	}

	normalizedSnapshot, err := normalizeNFLSnapshot(snapshot, normalizedReq)
	if err != nil {
		return NFLRunMetrics{}, err
	}

	metrics := NFLRunMetrics{TeamRows: len(normalizedSnapshot.Teams)}
	for _, team := range normalizedSnapshot.Teams {
		if err := queries.UpsertNFLTeamStats(ctx, store.UpsertNFLTeamStatsParams{
			Source:               normalizedSnapshot.Source,
			ExternalID:           team.ExternalID,
			Season:               normalizedSnapshot.Season,
			SeasonType:           normalizedSnapshot.SeasonType,
			StatDate:             pgDate(normalizedSnapshot.StatDate),
			TeamName:             team.TeamName,
			GamesPlayed:          team.GamesPlayed,
			Wins:                 team.Wins,
			Losses:               team.Losses,
			Ties:                 team.Ties,
			PointsFor:            team.PointsFor,
			PointsAgainst:        team.PointsAgainst,
			OffensiveEpaPerPlay:  team.OffensiveEPAPerPlay,
			DefensiveEpaPerPlay:  team.DefensiveEPAPerPlay,
			OffensiveSuccessRate: team.OffensiveSuccessRate,
			DefensiveSuccessRate: team.DefensiveSuccessRate,
		}); err != nil {
			return NFLRunMetrics{}, fmt.Errorf("upsert nfl team stats %s: %w", team.ExternalID, err)
		}
	}

	e.logger.InfoContext(ctx, "nfl stats etl completed",
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(normalizedSnapshot.Season)),
		slog.String("season_type", normalizedSnapshot.SeasonType),
		slog.String("stat_date", normalizedSnapshot.StatDate.Format(time.DateOnly)),
		slog.String("source", normalizedSnapshot.Source),
	)

	return metrics, nil
}

func NormalizeNFLRequest(req NFLRequest) (NFLRequest, error) {
	statDate := req.StatDate
	if statDate.IsZero() {
		statDate = req.RequestedAt
	}
	if statDate.IsZero() {
		return NFLRequest{}, errors.New("nfl stat date is required")
	}

	normalized := NFLRequest{
		RequestedAt: req.RequestedAt.UTC(),
		SeasonType:  normalizeSlug(req.SeasonType),
		StatDate:    normalizeDate(statDate),
		Season:      req.Season,
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = defaultNFLSeasonType
	}
	if normalized.Season == 0 {
		normalized.Season = inferNFLSeason(normalized.StatDate)
	}
	if normalized.Season <= 0 {
		return NFLRequest{}, fmt.Errorf("invalid nfl season %d", normalized.Season)
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.StatDate
	}

	return normalized, nil
}

func normalizeNFLSnapshot(snapshot NFLSnapshot, req NFLRequest) (NFLSnapshot, error) {
	normalized := NFLSnapshot{
		Source:     normalizeSlug(snapshot.Source),
		Season:     snapshot.Season,
		SeasonType: normalizeSlug(snapshot.SeasonType),
		StatDate:   snapshot.StatDate,
		Teams:      make([]NFLTeamStat, 0, len(snapshot.Teams)),
	}
	if normalized.Source == "" {
		normalized.Source = defaultNFLSource
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
		normalized.SeasonType = defaultNFLSeasonType
	}
	if normalized.Season <= 0 {
		return NFLSnapshot{}, fmt.Errorf("invalid nfl season %d", normalized.Season)
	}
	if normalized.StatDate.IsZero() {
		return NFLSnapshot{}, errors.New("nfl stat date is required")
	}

	for _, team := range snapshot.Teams {
		normalizedTeam, err := normalizeNFLTeamStat(team)
		if err != nil {
			return NFLSnapshot{}, err
		}
		normalized.Teams = append(normalized.Teams, normalizedTeam)
	}

	return normalized, nil
}

func normalizeNFLTeamStat(team NFLTeamStat) (NFLTeamStat, error) {
	normalized := NFLTeamStat{
		ExternalID:           normalizeSlug(team.ExternalID),
		TeamName:             normalizeLabel(team.TeamName),
		GamesPlayed:          team.GamesPlayed,
		Wins:                 team.Wins,
		Losses:               team.Losses,
		Ties:                 team.Ties,
		PointsFor:            team.PointsFor,
		PointsAgainst:        team.PointsAgainst,
		OffensiveEPAPerPlay:  team.OffensiveEPAPerPlay,
		DefensiveEPAPerPlay:  team.DefensiveEPAPerPlay,
		OffensiveSuccessRate: team.OffensiveSuccessRate,
		DefensiveSuccessRate: team.DefensiveSuccessRate,
	}
	if normalized.ExternalID == "" {
		return NFLTeamStat{}, errors.New("nfl team external id is required")
	}
	if normalized.TeamName == "" {
		return NFLTeamStat{}, fmt.Errorf("nfl team name is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func inferNFLSeason(statDate time.Time) int32 {
	if statDate.Month() >= time.July {
		return int32(statDate.Year())
	}
	return int32(statDate.Year() - 1)
}
