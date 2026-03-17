package nhl

import (
	"math"
	"testing"
)

func TestNewXGGoalieModelRejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WinProbabilitySlope = 0

	if _, err := NewXGGoalieModel(cfg); err == nil {
		t.Fatal("expected config validation error, got nil")
	}
}

func TestPredictRejectsInvalidInput(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	input := baseMatchupInput()
	input.HomeTeam.PDO = 1.4

	if _, err := model.Predict(input); err == nil {
		t.Fatal("expected input validation error, got nil")
	}
}

func TestPredictFavorsBetterHomeGoalieAndXGProfile(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	prediction, err := model.Predict(baseMatchupInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if prediction.HomeWinProbability <= 0.60 {
		t.Fatalf("home win probability = %.4f, want > 0.60", prediction.HomeWinProbability)
	}
	if prediction.ExpectedHomeGoals <= prediction.ExpectedAwayGoals {
		t.Fatalf("expected home goals %.3f should exceed away goals %.3f", prediction.ExpectedHomeGoals, prediction.ExpectedAwayGoals)
	}
}

func TestPredictAppliesPDORegression(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	input := baseMatchupInput()

	baseline, err := model.Predict(input)
	if err != nil {
		t.Fatalf("Predict(baseline) error = %v", err)
	}

	input.HomeTeam.PDO = 1.060
	input.AwayTeam.PDO = 0.965
	regression, err := model.Predict(input)
	if err != nil {
		t.Fatalf("Predict(regression) error = %v", err)
	}

	if regression.HomeWinProbability >= baseline.HomeWinProbability {
		t.Fatalf("regression home probability %.4f should be lower than baseline %.4f", regression.HomeWinProbability, baseline.HomeWinProbability)
	}
}

func TestPredictProbabilityBoundsAndSums(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	prediction, err := model.Predict(baseMatchupInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if prediction.HomeWinProbability < 0 || prediction.HomeWinProbability > 1 {
		t.Fatalf("home probability %.4f out of bounds", prediction.HomeWinProbability)
	}
	if prediction.AwayWinProbability < 0 || prediction.AwayWinProbability > 1 {
		t.Fatalf("away probability %.4f out of bounds", prediction.AwayWinProbability)
	}
	if math.Abs(prediction.HomeWinProbability+prediction.AwayWinProbability-1.0) > 1e-9 {
		t.Fatalf("win probabilities must sum to 1, got %.12f", prediction.HomeWinProbability+prediction.AwayWinProbability)
	}
}

func TestPredictIncorporatesOffenseAndCorsi(t *testing.T) {
	model := NewDefaultXGGoalieModel()

	// Home team has better offense and corsi
	strong := baseMatchupInput()
	strong.HomeTeam.GoalsForPerGame = 3.60
	strong.AwayTeam.GoalsForPerGame = 2.70
	strong.HomeTeam.CorsiShare = 0.56
	strong.AwayTeam.CorsiShare = 0.44

	// Neutral offense and corsi
	neutral := baseMatchupInput()
	neutral.HomeTeam.GoalsForPerGame = 3.10
	neutral.AwayTeam.GoalsForPerGame = 3.10
	neutral.HomeTeam.CorsiShare = 0.50
	neutral.AwayTeam.CorsiShare = 0.50

	strongPred, err := model.Predict(strong)
	if err != nil {
		t.Fatalf("Predict(strong) error = %v", err)
	}
	neutralPred, err := model.Predict(neutral)
	if err != nil {
		t.Fatalf("Predict(neutral) error = %v", err)
	}

	if strongPred.HomeWinProbability <= neutralPred.HomeWinProbability {
		t.Fatalf("strong home %.4f should exceed neutral %.4f when offense and corsi favor home",
			strongPred.HomeWinProbability, neutralPred.HomeWinProbability)
	}
}

func TestOverUnderProbabilitySumsToOne(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	pred, err := model.Predict(baseMatchupInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	over, under := pred.OverUnderProbability(5.5)
	sum := over + under
	if math.Abs(sum-1.0) > 0.02 {
		t.Fatalf("over (%.4f) + under (%.4f) = %.4f, want ~1.0", over, under, sum)
	}
	if over < 0.01 || over > 0.99 {
		t.Fatalf("over probability %.4f out of reasonable bounds", over)
	}
}

func TestOverUnderHigherLineIncreasesUnder(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	pred, err := model.Predict(baseMatchupInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	overLow, _ := pred.OverUnderProbability(4.5)
	overHigh, _ := pred.OverUnderProbability(7.5)

	if overHigh >= overLow {
		t.Fatalf("over probability at 7.5 (%.4f) should be less than at 4.5 (%.4f)", overHigh, overLow)
	}
}

func TestOverUnderHighScoringMatchup(t *testing.T) {
	model := NewDefaultXGGoalieModel()
	input := baseMatchupInput()
	input.HomeTeam.GoalsForPerGame = 4.2
	input.AwayTeam.GoalsForPerGame = 3.8
	input.HomeTeam.GoalsAgainstPerGame = 3.5
	input.AwayTeam.GoalsAgainstPerGame = 3.6

	pred, err := model.Predict(input)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	over, _ := pred.OverUnderProbability(5.5)
	if over < 0.50 {
		t.Fatalf("high-scoring matchup should favor over at 5.5, got %.4f", over)
	}
}

func TestOverUnderCorrelationIncreasesVariance(t *testing.T) {
	// With correlation, variance increases → more extreme outcomes → more overs at high lines
	pred := MatchupPrediction{ExpectedHomeGoals: 3.0, ExpectedAwayGoals: 3.0}

	// Compare independent (rho=0) vs correlated
	overIndep, _ := bivariatePoisson(3.0, 3.0, 0.0, 6.5)
	overCorr, _ := bivariatePoisson(3.0, 3.0, 0.12, 6.5)

	// Higher correlation should increase over probability above the mean
	// (more variance means heavier tails)
	if overCorr <= overIndep {
		t.Fatalf("correlated over (%.4f) should exceed independent over (%.4f) above mean", overCorr, overIndep)
	}

	// Sanity: expected total = 6.0, line = 5.5, so over should be >50%
	over55, _ := pred.OverUnderProbability(5.5)
	if over55 < 0.50 {
		t.Fatalf("with expected total 6.0, over 5.5 should be >50%%, got %.4f", over55)
	}
}

func TestTotalsVaryAcrossMatchups(t *testing.T) {
	model := NewDefaultXGGoalieModel()

	// High-scoring environment: both teams score a lot, both allow a lot, bad goalies
	highScoring := MatchupInput{
		HomeTeam: TeamProfile{
			Name: "High Home", ExpectedGoalsShare: 0.54,
			GoalsForPerGame: 3.80, GoalsAgainstPerGame: 3.50,
			GoalieGSAx: -5.0, PDO: 1.02, CorsiShare: 0.54,
		},
		AwayTeam: TeamProfile{
			Name: "High Away", ExpectedGoalsShare: 0.53,
			GoalsForPerGame: 3.60, GoalsAgainstPerGame: 3.40,
			GoalieGSAx: -4.0, PDO: 1.01, CorsiShare: 0.53,
		},
	}

	// Low-scoring environment: both teams low offense, both tight defense, elite goalies
	lowScoring := MatchupInput{
		HomeTeam: TeamProfile{
			Name: "Low Home", ExpectedGoalsShare: 0.48,
			GoalsForPerGame: 2.50, GoalsAgainstPerGame: 2.40,
			GoalieGSAx: 12.0, PDO: 0.98, CorsiShare: 0.47,
		},
		AwayTeam: TeamProfile{
			Name: "Low Away", ExpectedGoalsShare: 0.47,
			GoalsForPerGame: 2.40, GoalsAgainstPerGame: 2.50,
			GoalieGSAx: 10.0, PDO: 0.97, CorsiShare: 0.46,
		},
	}

	highPred, err := model.Predict(highScoring)
	if err != nil {
		t.Fatalf("Predict(highScoring) error = %v", err)
	}
	lowPred, err := model.Predict(lowScoring)
	if err != nil {
		t.Fatalf("Predict(lowScoring) error = %v", err)
	}

	t.Logf("High-scoring total: %.2f (home=%.2f, away=%.2f)", highPred.ExpectedTotalGoals, highPred.ExpectedHomeGoals, highPred.ExpectedAwayGoals)
	t.Logf("Low-scoring total:  %.2f (home=%.2f, away=%.2f)", lowPred.ExpectedTotalGoals, lowPred.ExpectedHomeGoals, lowPred.ExpectedAwayGoals)

	spread := highPred.ExpectedTotalGoals - lowPred.ExpectedTotalGoals
	t.Logf("Spread: %.2f goals", spread)

	if spread < 1.0 {
		t.Fatalf("high-scoring total (%.2f) should exceed low-scoring (%.2f) by at least 1 goal, got %.2f",
			highPred.ExpectedTotalGoals, lowPred.ExpectedTotalGoals, spread)
	}

	// Verify over/under probabilities diverge at a standard line
	highOver, _ := highPred.OverUnderProbability(5.5)
	lowOver, _ := lowPred.OverUnderProbability(5.5)
	t.Logf("Over 5.5: high=%.3f, low=%.3f", highOver, lowOver)

	if highOver <= lowOver {
		t.Fatalf("high-scoring over prob (%.3f) must exceed low-scoring (%.3f)", highOver, lowOver)
	}
	if highOver-lowOver < 0.10 {
		t.Fatalf("over probability spread (%.3f) should be substantial (>0.10)", highOver-lowOver)
	}
}

func TestPoissonPMFBasicProperties(t *testing.T) {
	// P(X=0) for Poisson(3) = e^-3 ≈ 0.0498
	p0 := poissonPMF(3.0, 0)
	expected := math.Exp(-3.0)
	if math.Abs(p0-expected) > 1e-10 {
		t.Fatalf("poissonPMF(3,0) = %.10f, want %.10f", p0, expected)
	}

	// Sum of PMF should be ~1
	sum := 0.0
	for k := 0; k <= 20; k++ {
		sum += poissonPMF(3.0, k)
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Fatalf("sum of poissonPMF(3, 0..20) = %.10f, want ~1.0", sum)
	}
}

func baseMatchupInput() MatchupInput {
	return MatchupInput{
		HomeTeam: TeamProfile{
			Name:                "Carolina Hurricanes",
			ExpectedGoalsShare:  0.56,
			GoalsForPerGame:     3.45,
			GoalsAgainstPerGame: 2.72,
			GoalieGSAx:          14.2,
			PDO:                 0.994,
			CorsiShare:          0.54,
		},
		AwayTeam: TeamProfile{
			Name:                "Pittsburgh Penguins",
			ExpectedGoalsShare:  0.49,
			GoalsForPerGame:     3.03,
			GoalsAgainstPerGame: 3.12,
			GoalieGSAx:          -2.0,
			PDO:                 1.013,
			CorsiShare:          0.48,
		},
	}
}
