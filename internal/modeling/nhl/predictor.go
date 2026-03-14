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
	PDORegressionWeight      float64
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
		HomeIceGoalAdvantage:     0.14,
		ExpectedGoalsShareWeight: 2.2,
		GoalieGSAxWeight:         0.028,
		GoalsAgainstWeight:       0.34,
		PDORegressionWeight:      1.9,
		WinProbabilitySlope:      1.28,
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
	pdoRegressionEdge := ((1.0 - input.HomeTeam.PDO) - (1.0 - input.AwayTeam.PDO)) * m.cfg.PDORegressionWeight

	homeGoals := clamp(
		m.cfg.LeagueGoalsPerTeam+m.cfg.HomeIceGoalAdvantage+xgEdge*0.52+goalieEdge*0.28+defenseEdge*0.20+pdoRegressionEdge*0.15,
		m.cfg.MinTeamGoals,
		m.cfg.MaxTeamGoals,
	)
	awayGoals := clamp(
		m.cfg.LeagueGoalsPerTeam-xgEdge*0.52-goalieEdge*0.28-defenseEdge*0.20-pdoRegressionEdge*0.15,
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
	case cfg.PDORegressionWeight <= 0:
		return errors.New("pdo regression weight must be > 0")
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
