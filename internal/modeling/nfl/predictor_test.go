package nfl

import (
	"math"
	"testing"
)

func TestNewEPADVOAModelRejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SpreadStdDevPoints = 0

	if _, err := NewEPADVOAModel(cfg); err == nil {
		t.Fatal("expected config validation error, got nil")
	}
}

func TestPredictRejectsInvalidInput(t *testing.T) {
	model := NewDefaultEPADVOAModel()
	input := baseInput()
	input.PrimaryKeyNumber = 0

	if _, err := model.Predict(input); err == nil {
		t.Fatal("expected input validation error, got nil")
	}
}

func TestPredictFavorsBetterHomeQBAndDVOA(t *testing.T) {
	model := NewDefaultEPADVOAModel()
	prediction, err := model.Predict(baseInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if prediction.HomeWinProbability <= 0.60 {
		t.Fatalf("home win probability = %.4f, want > 0.60", prediction.HomeWinProbability)
	}
	if prediction.ExpectedHomeAwayMargin <= 0 {
		t.Fatalf("expected margin %.3f should be positive", prediction.ExpectedHomeAwayMargin)
	}
}

func TestPredictKeyNumberAwarenessAdjustsCoverProbability(t *testing.T) {
	model := NewDefaultEPADVOAModel()

	nearKey := baseInput()
	nearKey.HomeSpreadLine = -3.0
	nearKey.PrimaryKeyNumber = 3.0
	nearPrediction, err := model.Predict(nearKey)
	if err != nil {
		t.Fatalf("Predict(near key) error = %v", err)
	}

	farFromKey := nearKey
	farFromKey.HomeSpreadLine = -5.5
	farFromKey.PrimaryKeyNumber = 3.0
	farPrediction, err := model.Predict(farFromKey)
	if err != nil {
		t.Fatalf("Predict(far key) error = %v", err)
	}

	if nearPrediction.KeyNumberProximity <= farPrediction.KeyNumberProximity {
		t.Fatalf("key proximity %.3f should exceed %.3f", nearPrediction.KeyNumberProximity, farPrediction.KeyNumberProximity)
	}
	if nearPrediction.HomeCoverProbability <= farPrediction.HomeCoverProbability {
		t.Fatalf("near-key cover probability %.4f should exceed far-key %.4f", nearPrediction.HomeCoverProbability, farPrediction.HomeCoverProbability)
	}
}

func TestPredictProbabilityBoundsAndSums(t *testing.T) {
	model := NewDefaultEPADVOAModel()
	prediction, err := model.Predict(baseInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if prediction.HomeWinProbability < 0 || prediction.HomeWinProbability > 1 {
		t.Fatalf("home win probability %.4f out of bounds", prediction.HomeWinProbability)
	}
	if prediction.HomeCoverProbability < 0 || prediction.HomeCoverProbability > 1 {
		t.Fatalf("home cover probability %.4f out of bounds", prediction.HomeCoverProbability)
	}
	if math.Abs(prediction.HomeWinProbability+prediction.AwayWinProbability-1.0) > 1e-9 {
		t.Fatalf("win probabilities must sum to 1, got %.12f", prediction.HomeWinProbability+prediction.AwayWinProbability)
	}
	if math.Abs(prediction.HomeCoverProbability+prediction.AwayCoverProbability-1.0) > 1e-9 {
		t.Fatalf("cover probabilities must sum to 1, got %.12f", prediction.HomeCoverProbability+prediction.AwayCoverProbability)
	}
}

func baseInput() MatchupInput {
	return MatchupInput{
		HomeTeam: TeamProfile{
			Name:       "Kansas City Chiefs",
			QBEPA:      0.22,
			DVOA:       0.18,
			OffenseEPA: 0.14,
			DefenseEPA: -0.08,
		},
		AwayTeam: TeamProfile{
			Name:       "Denver Broncos",
			QBEPA:      -0.02,
			DVOA:       -0.06,
			OffenseEPA: -0.01,
			DefenseEPA: 0.04,
		},
		HomeSpreadLine:   -3.0,
		TotalPointsLine:  47.5,
		WindMPH:          9,
		PrimaryKeyNumber: 3.0,
		HomeRestDays:     7,
		AwayRestDays:     6,
	}
}
