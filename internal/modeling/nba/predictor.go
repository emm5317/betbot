package nba

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	minProbability = 0.01
	maxProbability = 0.99
)

// PlayerAvailability captures lineup state for a player.
//
// Availability is on [0, 1]:
// 1.0 = fully available, 0.0 = out.
// OffensiveImpact and DefensiveImpact are points per 100 possessions.
type PlayerAvailability struct {
	Name            string
	Availability    float64
	OffensiveImpact float64
	DefensiveImpact float64
}

// TeamProfile captures the team-level NBA inputs required by the model.
type TeamProfile struct {
	Name            string
	OffensiveRating float64
	DefensiveRating float64
	Pace            float64
	Lineup          []PlayerAvailability
}

type MatchupInput struct {
	HomeTeam       TeamProfile
	AwayTeam       TeamProfile
	HomeSpreadLine float64
	HomeRestDays   int
	AwayRestDays   int
}

type MatchupPrediction struct {
	ExpectedHomeAwayMargin float64
	ExpectedTotalPoints    float64
	ExpectedHomePoints     float64
	ExpectedAwayPoints     float64
	HomeWinProbability     float64
	AwayWinProbability     float64
	HomeCoverProbability   float64
	AwayCoverProbability   float64
	HomeAdjustedNetRating  float64
	AwayAdjustedNetRating  float64
}

type Config struct {
	LeagueOffensiveRating   float64
	LeaguePace              float64
	HomeCourtAdvantagePoint float64
	RestDayToMarginPoints   float64
	LineupImpactWeight      float64
	WinProbabilitySlope     float64
	SpreadStdDevPoints      float64
	MinTeamEfficiency       float64
	MaxTeamEfficiency       float64
	MinPace                 float64
	MaxPace                 float64
	MinExpectedTotalPoints  float64
	MaxExpectedTotalPoints  float64
}

type NetRatingModel struct {
	cfg Config
}

func DefaultConfig() Config {
	return Config{
		LeagueOffensiveRating:   114.5,
		LeaguePace:              99.2,
		HomeCourtAdvantagePoint: 2.2,
		RestDayToMarginPoints:   0.45,
		LineupImpactWeight:      1.0,
		WinProbabilitySlope:     0.185,
		SpreadStdDevPoints:      12.0,
		MinTeamEfficiency:       95.0,
		MaxTeamEfficiency:       130.0,
		MinPace:                 90.0,
		MaxPace:                 105.0,
		MinExpectedTotalPoints:  175.0,
		MaxExpectedTotalPoints:  260.0,
	}
}

func NewNetRatingModel(cfg Config) (NetRatingModel, error) {
	if err := validateConfig(cfg); err != nil {
		return NetRatingModel{}, err
	}
	return NetRatingModel{cfg: cfg}, nil
}

func NewDefaultNetRatingModel() NetRatingModel {
	return NetRatingModel{cfg: DefaultConfig()}
}

func (m NetRatingModel) Predict(input MatchupInput) (MatchupPrediction, error) {
	if err := validateMatchupInput(input); err != nil {
		return MatchupPrediction{}, err
	}

	home := m.adjustTeamRatings(input.HomeTeam)
	away := m.adjustTeamRatings(input.AwayTeam)

	rawPace := (home.pace + away.pace) / 2
	expectedPace := clamp(rawPace*0.75+m.cfg.LeaguePace*0.25, m.cfg.MinPace, m.cfg.MaxPace)

	rawHomeEfficiency := (home.offensiveRating + away.defensiveRating) / 2
	rawAwayEfficiency := (away.offensiveRating + home.defensiveRating) / 2
	homeEfficiency := clamp(rawHomeEfficiency*0.75+m.cfg.LeagueOffensiveRating*0.25, m.cfg.MinTeamEfficiency, m.cfg.MaxTeamEfficiency)
	awayEfficiency := clamp(rawAwayEfficiency*0.75+m.cfg.LeagueOffensiveRating*0.25, m.cfg.MinTeamEfficiency, m.cfg.MaxTeamEfficiency)

	expectedTotal := clamp((homeEfficiency+awayEfficiency)*expectedPace/100.0, m.cfg.MinExpectedTotalPoints, m.cfg.MaxExpectedTotalPoints)
	restMarginAdjustment := float64(input.HomeRestDays-input.AwayRestDays) * m.cfg.RestDayToMarginPoints
	expectedMargin := ((homeEfficiency-awayEfficiency)*expectedPace/100.0 + m.cfg.HomeCourtAdvantagePoint + restMarginAdjustment)

	maxMargin := expectedTotal - 1.0
	expectedMargin = clamp(expectedMargin, -maxMargin, maxMargin)

	expectedHomePoints := (expectedTotal + expectedMargin) / 2
	expectedAwayPoints := (expectedTotal - expectedMargin) / 2

	homeWinProbability := clamp(sigmoid(expectedMargin*m.cfg.WinProbabilitySlope), minProbability, maxProbability)

	// Home spread line follows market convention:
	// negative means home is favored, positive means home is an underdog.
	spreadEdgeMean := expectedMargin + input.HomeSpreadLine
	homeCoverProbability := clamp(standardNormalCDF(spreadEdgeMean/m.cfg.SpreadStdDevPoints), minProbability, maxProbability)

	return MatchupPrediction{
		ExpectedHomeAwayMargin: expectedMargin,
		ExpectedTotalPoints:    expectedTotal,
		ExpectedHomePoints:     expectedHomePoints,
		ExpectedAwayPoints:     expectedAwayPoints,
		HomeWinProbability:     homeWinProbability,
		AwayWinProbability:     1 - homeWinProbability,
		HomeCoverProbability:   homeCoverProbability,
		AwayCoverProbability:   1 - homeCoverProbability,
		HomeAdjustedNetRating:  home.offensiveRating - home.defensiveRating,
		AwayAdjustedNetRating:  away.offensiveRating - away.defensiveRating,
	}, nil
}

type adjustedTeamRatings struct {
	offensiveRating float64
	defensiveRating float64
	pace            float64
}

func (m NetRatingModel) adjustTeamRatings(team TeamProfile) adjustedTeamRatings {
	adjustedOffense := team.OffensiveRating
	adjustedDefense := team.DefensiveRating

	for _, player := range team.Lineup {
		unavailableWeight := 1 - player.Availability
		adjustedOffense -= player.OffensiveImpact * unavailableWeight * m.cfg.LineupImpactWeight
		adjustedDefense += player.DefensiveImpact * unavailableWeight * m.cfg.LineupImpactWeight
	}

	return adjustedTeamRatings{
		offensiveRating: clamp(adjustedOffense, m.cfg.MinTeamEfficiency, m.cfg.MaxTeamEfficiency),
		defensiveRating: clamp(adjustedDefense, m.cfg.MinTeamEfficiency, m.cfg.MaxTeamEfficiency),
		pace:            clamp(team.Pace, m.cfg.MinPace, m.cfg.MaxPace),
	}
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.LeagueOffensiveRating <= 0:
		return errors.New("league offensive rating must be > 0")
	case cfg.LeaguePace <= 0:
		return errors.New("league pace must be > 0")
	case cfg.HomeCourtAdvantagePoint < -10 || cfg.HomeCourtAdvantagePoint > 10:
		return errors.New("home court advantage point must be between -10 and 10")
	case cfg.RestDayToMarginPoints < 0:
		return errors.New("rest day to margin points must be >= 0")
	case cfg.LineupImpactWeight <= 0 || cfg.LineupImpactWeight > 2:
		return errors.New("lineup impact weight must be > 0 and <= 2")
	case cfg.WinProbabilitySlope <= 0:
		return errors.New("win probability slope must be > 0")
	case cfg.SpreadStdDevPoints <= 0:
		return errors.New("spread std dev points must be > 0")
	case cfg.MinTeamEfficiency <= 0 || cfg.MaxTeamEfficiency <= 0 || cfg.MinTeamEfficiency >= cfg.MaxTeamEfficiency:
		return errors.New("invalid team efficiency bounds")
	case cfg.MinPace <= 0 || cfg.MaxPace <= 0 || cfg.MinPace >= cfg.MaxPace:
		return errors.New("invalid pace bounds")
	case cfg.MinExpectedTotalPoints <= 0 || cfg.MaxExpectedTotalPoints <= 0 || cfg.MinExpectedTotalPoints >= cfg.MaxExpectedTotalPoints:
		return errors.New("invalid expected total points bounds")
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
	if input.HomeSpreadLine < -30 || input.HomeSpreadLine > 30 {
		return errors.New("home spread line must be between -30 and 30")
	}
	if input.HomeRestDays < 0 || input.HomeRestDays > 5 {
		return errors.New("home rest days must be between 0 and 5")
	}
	if input.AwayRestDays < 0 || input.AwayRestDays > 5 {
		return errors.New("away rest days must be between 0 and 5")
	}

	return nil
}

func validateTeamProfile(team TeamProfile, side string) error {
	if strings.TrimSpace(team.Name) == "" {
		return fmt.Errorf("%s team name is required", side)
	}
	if team.OffensiveRating <= 70 || team.OffensiveRating > 160 {
		return fmt.Errorf("%s offensive rating must be > 70 and <= 160", side)
	}
	if team.DefensiveRating <= 70 || team.DefensiveRating > 160 {
		return fmt.Errorf("%s defensive rating must be > 70 and <= 160", side)
	}
	if team.Pace <= 80 || team.Pace > 120 {
		return fmt.Errorf("%s pace must be > 80 and <= 120", side)
	}
	if len(team.Lineup) > 20 {
		return fmt.Errorf("%s lineup cannot exceed 20 players", side)
	}

	for i, player := range team.Lineup {
		if err := validatePlayerAvailability(player); err != nil {
			return fmt.Errorf("%s lineup player %d: %w", side, i, err)
		}
	}

	return nil
}

func validatePlayerAvailability(player PlayerAvailability) error {
	if strings.TrimSpace(player.Name) == "" {
		return errors.New("player name is required")
	}
	if player.Availability < 0 || player.Availability > 1 {
		return errors.New("availability must be between 0 and 1")
	}
	if player.OffensiveImpact < -15 || player.OffensiveImpact > 15 {
		return errors.New("offensive impact must be between -15 and 15")
	}
	if player.DefensiveImpact < -15 || player.DefensiveImpact > 15 {
		return errors.New("defensive impact must be between -15 and 15")
	}

	return nil
}

func sigmoid(v float64) float64 {
	return 1 / (1 + math.Exp(-v))
}

func standardNormalCDF(v float64) float64 {
	return 0.5 * (1 + math.Erf(v/math.Sqrt2))
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
