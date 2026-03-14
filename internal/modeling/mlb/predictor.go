package mlb

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	fullGameInnings = 9.0
)

// PitcherProfile captures the starter-level run prevention inputs used by the model.
type PitcherProfile struct {
	Name          string
	ERA           *float64
	FIP           *float64
	WHIP          *float64
	StrikeoutRate *float64
	WalkRate      *float64
}

// TeamProfile captures team-level context needed for an MLB matchup estimate.
type TeamProfile struct {
	Name        string
	RunsPerGame float64
	BattingOPS  *float64
	TeamERA     float64
}

type MatchupInput struct {
	HomeTeam    TeamProfile
	AwayTeam    TeamProfile
	HomeStarter PitcherProfile
	AwayStarter PitcherProfile
}

type MatchupPrediction struct {
	ExpectedHomeRuns           float64
	ExpectedAwayRuns           float64
	ExpectedTotalRuns          float64
	HomeMoneylineProbability   float64
	AwayMoneylineProbability   float64
	FirstFiveExpectedHomeRuns  float64
	FirstFiveExpectedAwayRuns  float64
	FirstFiveExpectedTotalRuns float64
	FirstFiveHomeProbability   float64
	FirstFiveAwayProbability   float64
}

type Config struct {
	LeagueRunsPerTeam    float64
	LeagueERA            float64
	LeagueWHIP           float64
	LeagueOPS            float64
	LeagueKMinusBB       float64
	HomeFieldRuns        float64
	RunsPerGameWeight    float64
	OPSRunsMultiplier    float64
	WHIPRunsMultiplier   float64
	KBBRunsMultiplier    float64
	StarterInningsShare  float64
	FirstFiveInnings     float64
	FirstFiveHomeScale   float64
	FirstFiveStarterRate float64
	MoneylineSlope       float64
	FirstFiveSlope       float64
	MinTeamRuns          float64
	MaxTeamRuns          float64
}

type PitcherMatchupModel struct {
	cfg Config
}

func DefaultConfig() Config {
	return Config{
		LeagueRunsPerTeam:    4.40,
		LeagueERA:            4.20,
		LeagueWHIP:           1.30,
		LeagueOPS:            0.720,
		LeagueKMinusBB:       0.140,
		HomeFieldRuns:        0.15,
		RunsPerGameWeight:    0.95,
		OPSRunsMultiplier:    8.0,
		WHIPRunsMultiplier:   0.90,
		KBBRunsMultiplier:    2.10,
		StarterInningsShare:  0.62,
		FirstFiveInnings:     5.0,
		FirstFiveHomeScale:   0.55,
		FirstFiveStarterRate: 0.86,
		MoneylineSlope:       0.68,
		FirstFiveSlope:       0.88,
		MinTeamRuns:          0.50,
		MaxTeamRuns:          12.0,
	}
}

func NewPitcherMatchupModel(cfg Config) (PitcherMatchupModel, error) {
	if err := validateConfig(cfg); err != nil {
		return PitcherMatchupModel{}, err
	}
	return PitcherMatchupModel{cfg: cfg}, nil
}

func NewDefaultPitcherMatchupModel() PitcherMatchupModel {
	return PitcherMatchupModel{cfg: DefaultConfig()}
}

func (m PitcherMatchupModel) Predict(input MatchupInput) (MatchupPrediction, error) {
	if err := validateMatchupInput(input); err != nil {
		return MatchupPrediction{}, err
	}

	homeOffenseAdj := m.offenseAdjustment(input.HomeTeam)
	awayOffenseAdj := m.offenseAdjustment(input.AwayTeam)

	homeStarterRA9 := m.starterRunsAllowedPerNine(input.HomeStarter)
	awayStarterRA9 := m.starterRunsAllowedPerNine(input.AwayStarter)

	homeDefenseAdj := m.fullGamePitchingAdjustment(homeStarterRA9, input.HomeTeam.TeamERA)
	awayDefenseAdj := m.fullGamePitchingAdjustment(awayStarterRA9, input.AwayTeam.TeamERA)

	homeRuns := clamp(
		m.cfg.LeagueRunsPerTeam+homeOffenseAdj+awayDefenseAdj+m.cfg.HomeFieldRuns,
		m.cfg.MinTeamRuns,
		m.cfg.MaxTeamRuns,
	)
	awayRuns := clamp(
		m.cfg.LeagueRunsPerTeam+awayOffenseAdj+homeDefenseAdj,
		m.cfg.MinTeamRuns,
		m.cfg.MaxTeamRuns,
	)

	firstFiveHomeRuns := clamp(
		(m.cfg.LeagueRunsPerTeam+homeOffenseAdj+m.firstFivePitchingAdjustment(awayStarterRA9, input.AwayTeam.TeamERA)+m.cfg.HomeFieldRuns*m.cfg.FirstFiveHomeScale)*m.cfg.FirstFiveInnings/fullGameInnings,
		m.cfg.MinTeamRuns*0.4,
		m.cfg.MaxTeamRuns*0.8,
	)
	firstFiveAwayRuns := clamp(
		(m.cfg.LeagueRunsPerTeam+awayOffenseAdj+m.firstFivePitchingAdjustment(homeStarterRA9, input.HomeTeam.TeamERA))*m.cfg.FirstFiveInnings/fullGameInnings,
		m.cfg.MinTeamRuns*0.4,
		m.cfg.MaxTeamRuns*0.8,
	)

	homeMoneyline := clamp(sigmoid((homeRuns-awayRuns)*m.cfg.MoneylineSlope), 0.01, 0.99)
	firstFiveHome := clamp(sigmoid((firstFiveHomeRuns-firstFiveAwayRuns)*m.cfg.FirstFiveSlope), 0.01, 0.99)

	return MatchupPrediction{
		ExpectedHomeRuns:           homeRuns,
		ExpectedAwayRuns:           awayRuns,
		ExpectedTotalRuns:          homeRuns + awayRuns,
		HomeMoneylineProbability:   homeMoneyline,
		AwayMoneylineProbability:   1 - homeMoneyline,
		FirstFiveExpectedHomeRuns:  firstFiveHomeRuns,
		FirstFiveExpectedAwayRuns:  firstFiveAwayRuns,
		FirstFiveExpectedTotalRuns: firstFiveHomeRuns + firstFiveAwayRuns,
		FirstFiveHomeProbability:   firstFiveHome,
		FirstFiveAwayProbability:   1 - firstFiveHome,
	}, nil
}

func (m PitcherMatchupModel) offenseAdjustment(team TeamProfile) float64 {
	runsComponent := (team.RunsPerGame - m.cfg.LeagueRunsPerTeam) * m.cfg.RunsPerGameWeight

	opsComponent := 0.0
	if team.BattingOPS != nil {
		opsComponent = (*team.BattingOPS - m.cfg.LeagueOPS) * m.cfg.OPSRunsMultiplier
	}

	return runsComponent + opsComponent
}

func (m PitcherMatchupModel) starterRunsAllowedPerNine(pitcher PitcherProfile) float64 {
	base := m.cfg.LeagueERA
	metrics := 0.0
	weight := 0.0

	if pitcher.ERA != nil {
		metrics += *pitcher.ERA * 0.65
		weight += 0.65
	}
	if pitcher.FIP != nil {
		metrics += *pitcher.FIP * 0.35
		weight += 0.35
	}
	if weight > 0 {
		base = metrics / weight
	}

	if pitcher.WHIP != nil {
		base += (*pitcher.WHIP - m.cfg.LeagueWHIP) * m.cfg.WHIPRunsMultiplier
	}
	if pitcher.StrikeoutRate != nil && pitcher.WalkRate != nil {
		kMinusBB := *pitcher.StrikeoutRate - *pitcher.WalkRate
		base += (m.cfg.LeagueKMinusBB - kMinusBB) * m.cfg.KBBRunsMultiplier
	}

	return clamp(base, 2.20, 7.50)
}

func (m PitcherMatchupModel) fullGamePitchingAdjustment(starterRA9 float64, teamERA float64) float64 {
	nonStarterWeight := 1 - m.cfg.StarterInningsShare
	starterComponent := (starterRA9 - m.cfg.LeagueERA) * m.cfg.StarterInningsShare
	nonStarterComponent := (teamERA - m.cfg.LeagueERA) * nonStarterWeight
	return starterComponent + nonStarterComponent
}

func (m PitcherMatchupModel) firstFivePitchingAdjustment(starterRA9 float64, teamERA float64) float64 {
	nonStarterWeight := 1 - m.cfg.FirstFiveStarterRate
	starterComponent := (starterRA9 - m.cfg.LeagueERA) * m.cfg.FirstFiveStarterRate
	nonStarterComponent := (teamERA - m.cfg.LeagueERA) * nonStarterWeight
	return starterComponent + nonStarterComponent
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.LeagueRunsPerTeam <= 0:
		return errors.New("league runs per team must be > 0")
	case cfg.LeagueERA <= 0:
		return errors.New("league era must be > 0")
	case cfg.LeagueWHIP <= 0:
		return errors.New("league whip must be > 0")
	case cfg.StarterInningsShare <= 0 || cfg.StarterInningsShare >= 1:
		return errors.New("starter innings share must be between 0 and 1")
	case cfg.FirstFiveStarterRate <= 0 || cfg.FirstFiveStarterRate >= 1:
		return errors.New("first five starter rate must be between 0 and 1")
	case cfg.FirstFiveInnings <= 0 || cfg.FirstFiveInnings >= fullGameInnings:
		return errors.New("first five innings must be > 0 and < 9")
	case cfg.MoneylineSlope <= 0:
		return errors.New("moneyline slope must be > 0")
	case cfg.FirstFiveSlope <= 0:
		return errors.New("first five slope must be > 0")
	case cfg.MinTeamRuns <= 0 || cfg.MaxTeamRuns <= 0 || cfg.MinTeamRuns >= cfg.MaxTeamRuns:
		return errors.New("invalid team run bounds")
	}
	return nil
}

func validateMatchupInput(input MatchupInput) error {
	if err := validateTeamProfile(input.HomeTeam, "home"); err != nil {
		return err
	}
	if err := validateTeamProfile(input.AwayTeam, "away"); err != nil {
		return err
	}
	if err := validatePitcherProfile(input.HomeStarter, "home"); err != nil {
		return err
	}
	if err := validatePitcherProfile(input.AwayStarter, "away"); err != nil {
		return err
	}
	return nil
}

func validateTeamProfile(team TeamProfile, side string) error {
	name := strings.TrimSpace(team.Name)
	if name == "" {
		return fmt.Errorf("%s team name is required", side)
	}
	if team.RunsPerGame <= 0 || team.RunsPerGame > 15 {
		return fmt.Errorf("%s runs per game must be > 0 and <= 15", side)
	}
	if team.TeamERA <= 0 || team.TeamERA > 15 {
		return fmt.Errorf("%s team era must be > 0 and <= 15", side)
	}
	if team.BattingOPS != nil {
		if *team.BattingOPS <= 0 || *team.BattingOPS > 2 {
			return fmt.Errorf("%s batting ops must be > 0 and <= 2", side)
		}
	}
	return nil
}

func validatePitcherProfile(pitcher PitcherProfile, side string) error {
	if strings.TrimSpace(pitcher.Name) == "" {
		return fmt.Errorf("%s starter name is required", side)
	}
	if pitcher.ERA == nil && pitcher.FIP == nil {
		return fmt.Errorf("%s starter must provide era or fip", side)
	}
	if pitcher.ERA != nil && (*pitcher.ERA <= 0 || *pitcher.ERA > 15) {
		return fmt.Errorf("%s starter era must be > 0 and <= 15", side)
	}
	if pitcher.FIP != nil && (*pitcher.FIP <= 0 || *pitcher.FIP > 15) {
		return fmt.Errorf("%s starter fip must be > 0 and <= 15", side)
	}
	if pitcher.WHIP != nil && (*pitcher.WHIP <= 0 || *pitcher.WHIP > 5) {
		return fmt.Errorf("%s starter whip must be > 0 and <= 5", side)
	}
	if pitcher.StrikeoutRate != nil && (*pitcher.StrikeoutRate < 0 || *pitcher.StrikeoutRate > 1) {
		return fmt.Errorf("%s starter strikeout rate must be between 0 and 1", side)
	}
	if pitcher.WalkRate != nil && (*pitcher.WalkRate < 0 || *pitcher.WalkRate > 1) {
		return fmt.Errorf("%s starter walk rate must be between 0 and 1", side)
	}
	if pitcher.StrikeoutRate != nil && pitcher.WalkRate != nil && *pitcher.WalkRate > *pitcher.StrikeoutRate {
		return fmt.Errorf("%s starter walk rate cannot exceed strikeout rate", side)
	}
	return nil
}

func sigmoid(v float64) float64 {
	return 1 / (1 + math.Exp(-v))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
