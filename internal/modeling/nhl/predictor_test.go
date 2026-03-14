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

func baseMatchupInput() MatchupInput {
	return MatchupInput{
		HomeTeam: TeamProfile{
			Name:                "Carolina Hurricanes",
			ExpectedGoalsShare:  0.56,
			GoalsForPerGame:     3.45,
			GoalsAgainstPerGame: 2.72,
			GoalieGSAx:          14.2,
			PDO:                 0.994,
		},
		AwayTeam: TeamProfile{
			Name:                "Pittsburgh Penguins",
			ExpectedGoalsShare:  0.49,
			GoalsForPerGame:     3.03,
			GoalsAgainstPerGame: 3.12,
			GoalieGSAx:          -2.0,
			PDO:                 1.013,
		},
	}
}
