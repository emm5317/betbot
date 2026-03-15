package nhl

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

type TeamProfile struct {
	Name                string
	ExpectedGoalsShare  float64
	GoalsForPerGame     float64
	GoalsAgainstPerGame float64
	GoalieGSAx          float64
	PDO                 float64
	CorsiShare          float64
}

type MatchupInput struct {
	HomeTeam TeamProfile
	AwayTeam TeamProfile
}

type MatchupPrediction struct {
	ExpectedHomeGoals    float64
	ExpectedAwayGoals    float64
	ExpectedTotalGoals   float64
	HomeWinProbability   float64
	AwayWinProbability   float64
	HomeGoalDifferential float64
}

type Config struct {
	LeagueGoalsPerTeam       float64
	HomeIceGoalAdvantage     float64
	ExpectedGoalsShareWeight float64
	GoalieGSAxWeight         float64
	GoalsAgainstWeight       float64
	GoalsForWeight           float64
	PDORegressionWeight      float64
	CorsiShareWeight         float64
	WinProbabilitySlope      float64
	MinTeamGoals             float64
	MaxTeamGoals             float64
}

type XGGoalieModel struct {
	cfg Config
}

func DefaultConfig() Config {
	return Config{
		LeagueGoalsPerTeam:       3.05,
		HomeIceGoalAdvantage:     0.06,   // modern NHL home win ~50.5%
		ExpectedGoalsShareWeight: 1.8,   // xG% edge; diff ~0.04-0.10 typical
		GoalieGSAxWeight:         0.008, // GSAx edge; range -15 to +20 cumulative
		GoalsAgainstWeight:       0.08,  // defensive edge; GA/game diff ~0.3-0.8
		GoalsForWeight:           0.14,  // offensive edge; GF/game diff ~0.3-1.0
		PDORegressionWeight:      0.35,  // luck regression; PDO diff ~0.01-0.04
		CorsiShareWeight:         0.70,  // possession edge; Corsi diff ~0.02-0.08
		WinProbabilitySlope:      1.40,  // sigmoid steepness
		MinTeamGoals:             0.8,
		MaxTeamGoals:             6.8,
	}
}

func NewXGGoalieModel(cfg Config) (XGGoalieModel, error) {
	if err := validateConfig(cfg); err != nil {
		return XGGoalieModel{}, err
	}
	return XGGoalieModel{cfg: cfg}, nil
}

func NewDefaultXGGoalieModel() XGGoalieModel {
	return XGGoalieModel{cfg: DefaultConfig()}
}

func (m XGGoalieModel) Predict(input MatchupInput) (MatchupPrediction, error) {
	if err := validateMatchupInput(input); err != nil {
		return MatchupPrediction{}, err
	}

	xgEdge := (input.HomeTeam.ExpectedGoalsShare - input.AwayTeam.ExpectedGoalsShare) * m.cfg.ExpectedGoalsShareWeight
	goalieEdge := (input.HomeTeam.GoalieGSAx - input.AwayTeam.GoalieGSAx) * m.cfg.GoalieGSAxWeight
	defenseEdge := (input.AwayTeam.GoalsAgainstPerGame - input.HomeTeam.GoalsAgainstPerGame) * m.cfg.GoalsAgainstWeight
	offenseEdge := (input.HomeTeam.GoalsForPerGame - input.AwayTeam.GoalsForPerGame) * m.cfg.GoalsForWeight
	pdoRegressionEdge := ((1.0 - input.HomeTeam.PDO) - (1.0 - input.AwayTeam.PDO)) * m.cfg.PDORegressionWeight
	corsiEdge := (input.HomeTeam.CorsiShare - input.AwayTeam.CorsiShare) * m.cfg.CorsiShareWeight

	totalEdge := xgEdge + goalieEdge + defenseEdge + offenseEdge + pdoRegressionEdge + corsiEdge

	homeGoals := clamp(
		m.cfg.LeagueGoalsPerTeam+m.cfg.HomeIceGoalAdvantage+totalEdge,
		m.cfg.MinTeamGoals,
		m.cfg.MaxTeamGoals,
	)
	awayGoals := clamp(
		m.cfg.LeagueGoalsPerTeam-totalEdge,
		m.cfg.MinTeamGoals,
		m.cfg.MaxTeamGoals,
	)

	homeDifferential := homeGoals - awayGoals
	homeWin := clamp(sigmoid(homeDifferential*m.cfg.WinProbabilitySlope), minProbability, maxProbability)

	return MatchupPrediction{
		ExpectedHomeGoals:    homeGoals,
		ExpectedAwayGoals:    awayGoals,
		ExpectedTotalGoals:   homeGoals + awayGoals,
		HomeWinProbability:   homeWin,
		AwayWinProbability:   1 - homeWin,
		HomeGoalDifferential: homeDifferential,
	}, nil
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.LeagueGoalsPerTeam <= 0:
		return errors.New("league goals per team must be > 0")
	case cfg.WinProbabilitySlope <= 0:
		return errors.New("win probability slope must be > 0")
	case cfg.ExpectedGoalsShareWeight <= 0:
		return errors.New("expected goals share weight must be > 0")
	case cfg.GoalieGSAxWeight <= 0:
		return errors.New("goalie gsax weight must be > 0")
	case cfg.GoalsAgainstWeight <= 0:
		return errors.New("goals against weight must be > 0")
	case cfg.GoalsForWeight <= 0:
		return errors.New("goals for weight must be > 0")
	case cfg.PDORegressionWeight <= 0:
		return errors.New("pdo regression weight must be > 0")
	case cfg.CorsiShareWeight <= 0:
		return errors.New("corsi share weight must be > 0")
	case cfg.MinTeamGoals <= 0 || cfg.MaxTeamGoals <= 0 || cfg.MinTeamGoals >= cfg.MaxTeamGoals:
		return errors.New("invalid team goal bounds")
	}
	return nil
}

func validateMatchupInput(input MatchupInput) error {
	if err := validateTeam(input.HomeTeam, "home"); err != nil {
		return err
	}
	if err := validateTeam(input.AwayTeam, "away"); err != nil {
		return err
	}
	return nil
}

func validateTeam(team TeamProfile, side string) error {
	if strings.TrimSpace(team.Name) == "" {
		return fmt.Errorf("%s team name is required", side)
	}
	if team.ExpectedGoalsShare < 0 || team.ExpectedGoalsShare > 1 {
		return fmt.Errorf("%s expected goals share must be in [0,1]", side)
	}
	if team.GoalsForPerGame <= 0 || team.GoalsForPerGame > 10 {
		return fmt.Errorf("%s goals for per game must be in (0,10]", side)
	}
	if team.GoalsAgainstPerGame <= 0 || team.GoalsAgainstPerGame > 10 {
		return fmt.Errorf("%s goals against per game must be in (0,10]", side)
	}
	if team.GoalieGSAx < -80 || team.GoalieGSAx > 80 {
		return fmt.Errorf("%s goalie gsax must be in [-80,80]", side)
	}
	if team.PDO < 0.85 || team.PDO > 1.15 {
		return fmt.Errorf("%s pdo must be in [0.85,1.15]", side)
	}
	if team.CorsiShare < 0 || team.CorsiShare > 1 {
		return fmt.Errorf("%s corsi share must be in [0,1]", side)
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
