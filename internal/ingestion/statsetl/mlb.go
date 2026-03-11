package statsetl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultMLBSource     = "statcast"
	defaultMLBSeasonType = "regular"
)

var ErrMLBProviderUnconfigured = errors.New("mlb stats provider is not configured")

type MLBProvider interface {
	Fetch(ctx context.Context, req MLBRequest) (MLBSnapshot, error)
}

type MLBStore interface {
	UpsertMLBTeamStats(ctx context.Context, arg store.UpsertMLBTeamStatsParams) error
	UpsertMLBPitcherStats(ctx context.Context, arg store.UpsertMLBPitcherStatsParams) error
}

type MLBRequest struct {
	RequestedAt time.Time
	Season      int32
	SeasonType  string
	StatDate    time.Time
}

type MLBSnapshot struct {
	Source     string
	Season     int32
	SeasonType string
	StatDate   time.Time
	Teams      []MLBTeamStat
	Pitchers   []MLBPitcherStat
}

type MLBTeamStat struct {
	ExternalID  string
	TeamName    string
	GamesPlayed int32
	Wins        int32
	Losses      int32
	RunsScored  int32
	RunsAllowed int32
	BattingOPS  *float64
	TeamERA     *float64
}

type MLBPitcherStat struct {
	ExternalID     string
	PlayerName     string
	TeamExternalID string
	TeamName       string
	GamesStarted   int32
	InningsPitched *float64
	Era            *float64
	Fip            *float64
	Whip           *float64
	StrikeoutRate  *float64
	WalkRate       *float64
}

type MLBRunMetrics struct {
	TeamRows    int
	PitcherRows int
}

type MLBETL struct {
	provider MLBProvider
	logger   *slog.Logger
}

type UnconfiguredMLBProvider struct{}

func (UnconfiguredMLBProvider) Fetch(context.Context, MLBRequest) (MLBSnapshot, error) {
	return MLBSnapshot{}, ErrMLBProviderUnconfigured
}

func NewMLBETL(provider MLBProvider, logger *slog.Logger) *MLBETL {
	if provider == nil {
		provider = UnconfiguredMLBProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MLBETL{provider: provider, logger: logger}
}

func (e *MLBETL) Run(ctx context.Context, queries MLBStore, req MLBRequest) (MLBRunMetrics, error) {
	if queries == nil {
		return MLBRunMetrics{}, errors.New("mlb etl store is nil")
	}

	normalizedReq, err := NormalizeMLBRequest(req)
	if err != nil {
		return MLBRunMetrics{}, err
	}

	snapshot, err := e.provider.Fetch(ctx, normalizedReq)
	if err != nil {
		return MLBRunMetrics{}, fmt.Errorf("fetch mlb stats: %w", err)
	}

	normalizedSnapshot, err := normalizeMLBSnapshot(snapshot, normalizedReq)
	if err != nil {
		return MLBRunMetrics{}, err
	}

	metrics := MLBRunMetrics{
		TeamRows:    len(normalizedSnapshot.Teams),
		PitcherRows: len(normalizedSnapshot.Pitchers),
	}

	for _, team := range normalizedSnapshot.Teams {
		if err := queries.UpsertMLBTeamStats(ctx, store.UpsertMLBTeamStatsParams{
			Source:      normalizedSnapshot.Source,
			ExternalID:  team.ExternalID,
			Season:      normalizedSnapshot.Season,
			SeasonType:  normalizedSnapshot.SeasonType,
			StatDate:    pgDate(normalizedSnapshot.StatDate),
			TeamName:    team.TeamName,
			GamesPlayed: team.GamesPlayed,
			Wins:        team.Wins,
			Losses:      team.Losses,
			RunsScored:  team.RunsScored,
			RunsAllowed: team.RunsAllowed,
			BattingOps:  team.BattingOPS,
			TeamEra:     team.TeamERA,
		}); err != nil {
			return MLBRunMetrics{}, fmt.Errorf("upsert mlb team stats %s: %w", team.ExternalID, err)
		}
	}

	for _, pitcher := range normalizedSnapshot.Pitchers {
		if err := queries.UpsertMLBPitcherStats(ctx, store.UpsertMLBPitcherStatsParams{
			Source:         normalizedSnapshot.Source,
			ExternalID:     pitcher.ExternalID,
			Season:         normalizedSnapshot.Season,
			SeasonType:     normalizedSnapshot.SeasonType,
			StatDate:       pgDate(normalizedSnapshot.StatDate),
			PlayerName:     pitcher.PlayerName,
			TeamExternalID: pitcher.TeamExternalID,
			TeamName:       pitcher.TeamName,
			GamesStarted:   pitcher.GamesStarted,
			InningsPitched: pitcher.InningsPitched,
			Era:            pitcher.Era,
			Fip:            pitcher.Fip,
			Whip:           pitcher.Whip,
			StrikeoutRate:  pitcher.StrikeoutRate,
			WalkRate:       pitcher.WalkRate,
		}); err != nil {
			return MLBRunMetrics{}, fmt.Errorf("upsert mlb pitcher stats %s: %w", pitcher.ExternalID, err)
		}
	}

	e.logger.InfoContext(ctx, "mlb stats etl completed",
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("pitcher_rows", metrics.PitcherRows),
		slog.Int("season", int(normalizedSnapshot.Season)),
		slog.String("season_type", normalizedSnapshot.SeasonType),
		slog.String("stat_date", normalizedSnapshot.StatDate.Format(time.DateOnly)),
		slog.String("source", normalizedSnapshot.Source),
	)

	return metrics, nil
}

func NormalizeMLBRequest(req MLBRequest) (MLBRequest, error) {
	statDate := req.StatDate
	if statDate.IsZero() {
		statDate = req.RequestedAt
	}
	if statDate.IsZero() {
		return MLBRequest{}, errors.New("mlb stat date is required")
	}

	normalized := MLBRequest{
		RequestedAt: req.RequestedAt.UTC(),
		SeasonType:  normalizeSlug(req.SeasonType),
		StatDate:    normalizeDate(statDate),
		Season:      req.Season,
	}
	if normalized.SeasonType == "" {
		normalized.SeasonType = defaultMLBSeasonType
	}
	if normalized.Season == 0 {
		normalized.Season = int32(normalized.StatDate.Year())
	}
	if normalized.Season <= 0 {
		return MLBRequest{}, fmt.Errorf("invalid mlb season %d", normalized.Season)
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.StatDate
	}

	return normalized, nil
}

func normalizeMLBSnapshot(snapshot MLBSnapshot, req MLBRequest) (MLBSnapshot, error) {
	normalized := MLBSnapshot{
		Source:     normalizeSlug(snapshot.Source),
		Season:     snapshot.Season,
		SeasonType: normalizeSlug(snapshot.SeasonType),
		StatDate:   snapshot.StatDate,
		Teams:      make([]MLBTeamStat, 0, len(snapshot.Teams)),
		Pitchers:   make([]MLBPitcherStat, 0, len(snapshot.Pitchers)),
	}
	if normalized.Source == "" {
		normalized.Source = defaultMLBSource
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
		normalized.SeasonType = defaultMLBSeasonType
	}
	if normalized.Season <= 0 {
		return MLBSnapshot{}, fmt.Errorf("invalid mlb season %d", normalized.Season)
	}
	if normalized.StatDate.IsZero() {
		return MLBSnapshot{}, errors.New("mlb stat date is required")
	}

	for _, team := range snapshot.Teams {
		normalizedTeam, err := normalizeMLBTeamStat(team)
		if err != nil {
			return MLBSnapshot{}, err
		}
		normalized.Teams = append(normalized.Teams, normalizedTeam)
	}

	for _, pitcher := range snapshot.Pitchers {
		normalizedPitcher, err := normalizeMLBPitcherStat(pitcher)
		if err != nil {
			return MLBSnapshot{}, err
		}
		normalized.Pitchers = append(normalized.Pitchers, normalizedPitcher)
	}

	return normalized, nil
}

func normalizeMLBTeamStat(team MLBTeamStat) (MLBTeamStat, error) {
	normalized := MLBTeamStat{
		ExternalID:  normalizeSlug(team.ExternalID),
		TeamName:    normalizeLabel(team.TeamName),
		GamesPlayed: team.GamesPlayed,
		Wins:        team.Wins,
		Losses:      team.Losses,
		RunsScored:  team.RunsScored,
		RunsAllowed: team.RunsAllowed,
		BattingOPS:  team.BattingOPS,
		TeamERA:     team.TeamERA,
	}
	if normalized.ExternalID == "" {
		return MLBTeamStat{}, errors.New("mlb team external id is required")
	}
	if normalized.TeamName == "" {
		return MLBTeamStat{}, fmt.Errorf("mlb team name is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func normalizeMLBPitcherStat(pitcher MLBPitcherStat) (MLBPitcherStat, error) {
	normalized := MLBPitcherStat{
		ExternalID:     normalizeSlug(pitcher.ExternalID),
		PlayerName:     normalizeLabel(pitcher.PlayerName),
		TeamExternalID: normalizeSlug(pitcher.TeamExternalID),
		TeamName:       normalizeLabel(pitcher.TeamName),
		GamesStarted:   pitcher.GamesStarted,
		InningsPitched: pitcher.InningsPitched,
		Era:            pitcher.Era,
		Fip:            pitcher.Fip,
		Whip:           pitcher.Whip,
		StrikeoutRate:  pitcher.StrikeoutRate,
		WalkRate:       pitcher.WalkRate,
	}
	if normalized.ExternalID == "" {
		return MLBPitcherStat{}, errors.New("mlb pitcher external id is required")
	}
	if normalized.PlayerName == "" {
		return MLBPitcherStat{}, fmt.Errorf("mlb pitcher name is required for %s", normalized.ExternalID)
	}
	if normalized.TeamExternalID == "" {
		return MLBPitcherStat{}, fmt.Errorf("mlb pitcher team external id is required for %s", normalized.ExternalID)
	}
	if normalized.TeamName == "" {
		return MLBPitcherStat{}, fmt.Errorf("mlb pitcher team name is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func pgDate(value time.Time) pgtype.Date {
	return pgtype.Date{Time: normalizeDate(value), Valid: true}
}

func normalizeDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLabel(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
