package nfl

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
	Name       string
	QBEPA      float64
	DVOA       float64
	OffenseEPA float64
	DefenseEPA float64
}

type MatchupInput struct {
	HomeTeam         TeamProfile
	AwayTeam         TeamProfile
	HomeSpreadLine   float64
	TotalPointsLine  float64
	WindMPH          float64
	PrimaryKeyNumber float64
	HomeRestDays     int
	AwayRestDays     int
}

type MatchupPrediction struct {
	ExpectedHomeAwayMargin float64
	ExpectedTotalPoints    float64
	HomeWinProbability     float64
	AwayWinProbability     float64
	HomeCoverProbability   float64
	AwayCoverProbability   float64
	KeyNumberProximity     float64
}

type Config struct {
	HomeFieldAdvantagePoint float64
	QBEPAWeight             float64
	DVOAWeight              float64
	OffenseEPAWeight        float64
	DefenseEPAWeight        float64
	RestDayWeight           float64
	WindPenaltyThresholdMPH float64
	WindPenaltyPerMPH       float64
	SpreadStdDevPoints      float64
	WinProbabilitySlope     float64
	KeyNumberBoost          float64
	MinTotalPoints          float64
	MaxTotalPoints          float64
}

type EPADVOAModel struct {
	cfg Config
}

func DefaultConfig() Config {
	return Config{
		HomeFieldAdvantagePoint: 1.7,
		QBEPAWeight:             19.0,
		DVOAWeight:              8.5,
		OffenseEPAWeight:        12.5,
		DefenseEPAWeight:        10.5,
		RestDayWeight:           0.35,
		WindPenaltyThresholdMPH: 12.0,
		WindPenaltyPerMPH:       0.08,
		SpreadStdDevPoints:      13.5,
		WinProbabilitySlope:     0.175,
		KeyNumberBoost:          0.55,
		MinTotalPoints:          30,
		MaxTotalPoints:          64,
	}
}

func NewEPADVOAModel(cfg Config) (EPADVOAModel, error) {
	if err := validateConfig(cfg); err != nil {
		return EPADVOAModel{}, err
	}
	return EPADVOAModel{cfg: cfg}, nil
}

func NewDefaultEPADVOAModel() EPADVOAModel {
	return EPADVOAModel{cfg: DefaultConfig()}
}

func (m EPADVOAModel) Predict(input MatchupInput) (MatchupPrediction, error) {
	if err := validateMatchupInput(input); err != nil {
		return MatchupPrediction{}, err
	}

	qbEdge := input.HomeTeam.QBEPA - input.AwayTeam.QBEPA
	dvoaEdge := input.HomeTeam.DVOA - input.AwayTeam.DVOA
	offenseEdge := input.HomeTeam.OffenseEPA - input.AwayTeam.OffenseEPA
	defenseEdge := input.AwayTeam.DefenseEPA - input.HomeTeam.DefenseEPA
	restEdge := float64(input.HomeRestDays - input.AwayRestDays)

	expectedMargin := m.cfg.HomeFieldAdvantagePoint +
		qbEdge*m.cfg.QBEPAWeight +
		dvoaEdge*m.cfg.DVOAWeight +
		offenseEdge*m.cfg.OffenseEPAWeight +
		defenseEdge*m.cfg.DefenseEPAWeight +
		restEdge*m.cfg.RestDayWeight

	windOver := math.Max(0, input.WindMPH-m.cfg.WindPenaltyThresholdMPH)
	windPenalty := windOver * m.cfg.WindPenaltyPerMPH
	expectedTotal := clamp(input.TotalPointsLine-windPenalty, m.cfg.MinTotalPoints, m.cfg.MaxTotalPoints)

	keyGap := math.Abs(math.Abs(input.HomeSpreadLine) - input.PrimaryKeyNumber)
	keyProximity := 1 - clamp(keyGap/4.0, 0, 1)
	spreadEdge := expectedMargin + input.HomeSpreadLine
	keyDirection := 1.0
	if spreadEdge < 0 {
		keyDirection = -1.0
	}
	adjustedSpreadEdge := spreadEdge + keyDirection*keyProximity*m.cfg.KeyNumberBoost

	homeWin := clamp(sigmoid(expectedMargin*m.cfg.WinProbabilitySlope), minProbability, maxProbability)
	homeCover := clamp(standardNormalCDF(adjustedSpreadEdge/m.cfg.SpreadStdDevPoints), minProbability, maxProbability)

	return MatchupPrediction{
		ExpectedHomeAwayMargin: expectedMargin,
		ExpectedTotalPoints:    expectedTotal,
		HomeWinProbability:     homeWin,
		AwayWinProbability:     1 - homeWin,
		HomeCoverProbability:   homeCover,
		AwayCoverProbability:   1 - homeCover,
		KeyNumberProximity:     keyProximity,
	}, nil
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.QBEPAWeight <= 0:
		return errors.New("qb epa weight must be > 0")
	case cfg.DVOAWeight <= 0:
		return errors.New("dvoa weight must be > 0")
	case cfg.OffenseEPAWeight <= 0:
		return errors.New("offense epa weight must be > 0")
	case cfg.DefenseEPAWeight <= 0:
		return errors.New("defense epa weight must be > 0")
	case cfg.SpreadStdDevPoints <= 0:
		return errors.New("spread std dev points must be > 0")
	case cfg.WinProbabilitySlope <= 0:
		return errors.New("win probability slope must be > 0")
	case cfg.KeyNumberBoost < 0:
		return errors.New("key number boost must be >= 0")
	case cfg.MinTotalPoints <= 0 || cfg.MaxTotalPoints <= 0 || cfg.MinTotalPoints >= cfg.MaxTotalPoints:
		return errors.New("invalid total point bounds")
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
	if input.HomeSpreadLine < -30 || input.HomeSpreadLine > 30 {
		return errors.New("home spread line must be in [-30,30]")
	}
	if input.TotalPointsLine <= 0 || input.TotalPointsLine > 90 {
		return errors.New("total points line must be in (0,90]")
	}
	if input.WindMPH < 0 || input.WindMPH > 80 {
		return errors.New("wind mph must be in [0,80]")
	}
	if input.PrimaryKeyNumber <= 0 || input.PrimaryKeyNumber > 21 {
		return errors.New("primary key number must be in (0,21]")
	}
	if input.HomeRestDays < 0 || input.HomeRestDays > 14 {
		return errors.New("home rest days must be in [0,14]")
	}
	if input.AwayRestDays < 0 || input.AwayRestDays > 14 {
		return errors.New("away rest days must be in [0,14]")
	}
	return nil
}

func validateTeam(team TeamProfile, side string) error {
	if strings.TrimSpace(team.Name) == "" {
		return fmt.Errorf("%s team name is required", side)
	}
	if team.QBEPA < -1 || team.QBEPA > 1 {
		return fmt.Errorf("%s qb epa must be in [-1,1]", side)
	}
	if team.DVOA < -1 || team.DVOA > 1 {
		return fmt.Errorf("%s dvoa must be in [-1,1]", side)
	}
	if team.OffenseEPA < -1 || team.OffenseEPA > 1 {
		return fmt.Errorf("%s offense epa must be in [-1,1]", side)
	}
	if team.DefenseEPA < -1 || team.DefenseEPA > 1 {
		return fmt.Errorf("%s defense epa must be in [-1,1]", side)
	}
	return nil
}

func standardNormalCDF(v float64) float64 {
	return 0.5 * (1 + math.Erf(v/math.Sqrt2))
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
