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

// defaultGoalCorrelation is the empirical correlation between NHL team goal scoring
// in the same game. NHL goals are positively correlated (~0.10-0.15) because trailing
// teams pull goalies and play aggressively, generating more total goals.
const defaultGoalCorrelation = 0.12

// OverUnderProbability returns the probability that total goals exceed the given line.
// Uses a bivariate Poisson model (Karlis & Ntzoufras, 2003) with a correlation parameter
// to account for the positive goal dependency in hockey.
//
// The bivariate Poisson decomposes into: X = X_ind + Z, Y = Y_ind + Z
// where X_ind ~ Poisson(λ₁-cov), Y_ind ~ Poisson(λ₂-cov), Z ~ Poisson(cov)
// and cov = correlation * sqrt(λ₁ * λ₂).
func (p MatchupPrediction) OverUnderProbability(line float64) (overProb, underProb float64) {
	return bivariatePoisson(p.ExpectedHomeGoals, p.ExpectedAwayGoals, defaultGoalCorrelation, line)
}

// bivariatePoisson computes P(X+Y > line) where (X,Y) follow a bivariate Poisson
// with means lambda1, lambda2 and correlation rho.
func bivariatePoisson(lambda1, lambda2, rho, line float64) (overProb, underProb float64) {
	// Compute covariance parameter: cov = rho * sqrt(λ₁ * λ₂)
	cov := rho * math.Sqrt(lambda1*lambda2)
	// Ensure independent components remain positive
	if cov >= lambda1 {
		cov = lambda1 * 0.9
	}
	if cov >= lambda2 {
		cov = lambda2 * 0.9
	}
	if cov < 0 {
		cov = 0
	}

	l1 := lambda1 - cov // independent home rate
	l2 := lambda2 - cov // independent away rate

	threshold := int(math.Floor(line))
	maxGoals := threshold + 6
	if maxGoals > 20 {
		maxGoals = 20
	}
	maxZ := maxGoals / 2
	if maxZ > 8 {
		maxZ = 8
	}

	underCum := 0.0
	for z := 0; z <= maxZ; z++ {
		pZ := poissonPMF(cov, z)
		if pZ < 1e-15 {
			continue
		}
		for h := 0; h <= maxGoals-z; h++ {
			pH := poissonPMF(l1, h)
			if pH < 1e-15 {
				continue
			}
			for a := 0; a <= maxGoals-z-h; a++ {
				totalGoals := (h + z) + (a + z) // home=(h+z), away=(a+z)
				if totalGoals > threshold {
					continue // only summing under
				}
				pA := poissonPMF(l2, a)
				underCum += pZ * pH * pA
			}
		}
	}

	underProb = clamp(underCum, minProbability, maxProbability)
	overProb = clamp(1-underCum, minProbability, maxProbability)
	return overProb, underProb
}

// poissonPMF computes P(X = k) for X ~ Poisson(lambda).
func poissonPMF(lambda float64, k int) float64 {
	if k < 0 {
		return 0.0
	}
	if lambda <= 0 {
		if k == 0 {
			return 1.0
		}
		return 0.0
	}
	// Use log-space to avoid overflow: log(P) = k*log(lambda) - lambda - log(k!)
	logP := float64(k)*math.Log(lambda) - lambda - logFactorial(k)
	return math.Exp(logP)
}

// logFactorial returns ln(k!) computed exactly for k <= 20.
func logFactorial(k int) float64 {
	if k <= 1 {
		return 0
	}
	result := 0.0
	for i := 2; i <= k; i++ {
		result += math.Log(float64(i))
	}
	return result
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
		ExpectedGoalsShareWeight: 1.0,    // xG% edge; reduced from 1.8 to limit double-counting with GF/GA
		GoalieGSAxWeight:         0.011,  // GSAx edge; range -15 to +20 cumulative; increased for totals
		GoalsAgainstWeight:       0.25,   // defensive edge; moderate trust in 20-game rolling avg
		GoalsForWeight:           0.35,   // offensive edge; moderate trust in 20-game rolling avg
		PDORegressionWeight:      0.35,   // luck regression; PDO diff ~0.01-0.04
		CorsiShareWeight:         0.55,   // possession/pace edge; reduced to limit overlap with GF/GA
		WinProbabilitySlope:      0.90,   // sigmoid steepness; reduced for larger goal differentials
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

	// --- Level-based expected goals per team ---
	// Each team's goals = f(own offense, opponent defense, opponent goalie, pace, luck regression)
	// This produces independent estimates that DON'T cancel to a constant total.

	league := m.cfg.LeagueGoalsPerTeam // 3.05

	// Offensive contribution: how much better/worse than league average a team scores.
	homeOffAdj := (input.HomeTeam.GoalsForPerGame - league) * m.cfg.GoalsForWeight
	awayOffAdj := (input.AwayTeam.GoalsForPerGame - league) * m.cfg.GoalsForWeight

	// Defensive contribution: opponent's GA/game tells us how porous their defense is.
	// High GA = opponent lets in more goals = good for the attacking team.
	homeDefOppAdj := (input.AwayTeam.GoalsAgainstPerGame - league) * m.cfg.GoalsAgainstWeight
	awayDefOppAdj := (input.HomeTeam.GoalsAgainstPerGame - league) * m.cfg.GoalsAgainstWeight

	// xG share: team's underlying shot quality dominance.
	homeXGAdj := (input.HomeTeam.ExpectedGoalsShare - 0.50) * m.cfg.ExpectedGoalsShareWeight
	awayXGAdj := (input.AwayTeam.ExpectedGoalsShare - 0.50) * m.cfg.ExpectedGoalsShareWeight

	// Opponent goalie: good opposing goalie reduces goals scored.
	// Negative GSAx = bad goalie = more goals conceded.
	homeGoalieOppAdj := -input.AwayTeam.GoalieGSAx * m.cfg.GoalieGSAxWeight
	awayGoalieOppAdj := -input.HomeTeam.GoalieGSAx * m.cfg.GoalieGSAxWeight

	// Pace: combined Corsi share reflects shot generation environment.
	// Two high-Corsi teams = more shots on both sides = more total goals.
	combinedPace := (input.HomeTeam.CorsiShare + input.AwayTeam.CorsiShare) - 1.0 // 0 when both at 0.50
	homePaceAdj := combinedPace * m.cfg.CorsiShareWeight * 0.5
	awayPaceAdj := combinedPace * m.cfg.CorsiShareWeight * 0.5

	// PDO regression: teams above 1.0 are "lucky" (high shooting + save %).
	// Expect their OPPONENTS to score more (their save % regresses down).
	// Expect THEM to score less (their shooting % regresses down).
	homePDOAdj := (1.0 - input.HomeTeam.PDO) * m.cfg.PDORegressionWeight  // home team's luck regresses
	awayPDOAdj := (1.0 - input.AwayTeam.PDO) * m.cfg.PDORegressionWeight

	homeGoals := clamp(
		league+m.cfg.HomeIceGoalAdvantage+homeOffAdj+homeDefOppAdj+homeXGAdj+homeGoalieOppAdj+homePaceAdj+homePDOAdj,
		m.cfg.MinTeamGoals,
		m.cfg.MaxTeamGoals,
	)
	awayGoals := clamp(
		league+awayOffAdj+awayDefOppAdj+awayXGAdj+awayGoalieOppAdj+awayPaceAdj+awayPDOAdj,
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
