package nba

import (
	"math"
	"testing"
)

func TestNewNetRatingModelRejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SpreadStdDevPoints = 0

	_, err := NewNetRatingModel(cfg)
	if err == nil {
		t.Fatal("expected config validation error, got nil")
	}
}

func TestPredictRejectsInvalidInput(t *testing.T) {
	model := NewDefaultNetRatingModel()
	input := baseMatchupInput()
	input.HomeTeam.Lineup = append(input.HomeTeam.Lineup, PlayerAvailability{
		Name:            "Invalid Availability",
		Availability:    1.20,
		OffensiveImpact: 2.0,
		DefensiveImpact: 1.0,
	})

	_, err := model.Predict(input)
	if err == nil {
		t.Fatal("expected input validation error, got nil")
	}
}

func TestPredictLineupDowngradeLowersHomeEdge(t *testing.T) {
	model := NewDefaultNetRatingModel()
	healthyInput := baseMatchupInput()

	healthyPrediction, err := model.Predict(healthyInput)
	if err != nil {
		t.Fatalf("Predict(healthy) error = %v", err)
	}

	downgradedInput := healthyInput
	downgradedInput.HomeTeam.Lineup[0].Availability = 0.0
	downgradedPrediction, err := model.Predict(downgradedInput)
	if err != nil {
		t.Fatalf("Predict(downgraded) error = %v", err)
	}

	if downgradedPrediction.ExpectedHomeAwayMargin >= healthyPrediction.ExpectedHomeAwayMargin {
		t.Fatalf("downgraded margin %.4f should be < healthy margin %.4f", downgradedPrediction.ExpectedHomeAwayMargin, healthyPrediction.ExpectedHomeAwayMargin)
	}
	if downgradedPrediction.HomeWinProbability >= healthyPrediction.HomeWinProbability {
		t.Fatalf("downgraded home win probability %.4f should be < healthy %.4f", downgradedPrediction.HomeWinProbability, healthyPrediction.HomeWinProbability)
	}
	if downgradedPrediction.HomeCoverProbability >= healthyPrediction.HomeCoverProbability {
		t.Fatalf("downgraded home cover probability %.4f should be < healthy %.4f", downgradedPrediction.HomeCoverProbability, healthyPrediction.HomeCoverProbability)
	}
}

func TestPredictAppliesHomeCourtImpact(t *testing.T) {
	neutralCfg := DefaultConfig()
	neutralCfg.HomeCourtAdvantagePoint = 0

	neutralModel, err := NewNetRatingModel(neutralCfg)
	if err != nil {
		t.Fatalf("NewNetRatingModel(neutral) error = %v", err)
	}

	homeEdgeModel := NewDefaultNetRatingModel()
	input := baseMatchupInput()
	input.HomeSpreadLine = 0

	neutralPrediction, err := neutralModel.Predict(input)
	if err != nil {
		t.Fatalf("neutral Predict() error = %v", err)
	}
	homeEdgePrediction, err := homeEdgeModel.Predict(input)
	if err != nil {
		t.Fatalf("home-edge Predict() error = %v", err)
	}

	if homeEdgePrediction.ExpectedHomeAwayMargin <= neutralPrediction.ExpectedHomeAwayMargin {
		t.Fatalf("home-edge margin %.4f should exceed neutral margin %.4f", homeEdgePrediction.ExpectedHomeAwayMargin, neutralPrediction.ExpectedHomeAwayMargin)
	}
	if homeEdgePrediction.HomeWinProbability <= neutralPrediction.HomeWinProbability {
		t.Fatalf("home-edge home win probability %.4f should exceed neutral %.4f", homeEdgePrediction.HomeWinProbability, neutralPrediction.HomeWinProbability)
	}
}

func TestPredictProbabilityBoundsAndSums(t *testing.T) {
	model := NewDefaultNetRatingModel()

	prediction, err := model.Predict(baseMatchupInput())
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	probabilities := map[string]float64{
		"home_win":   prediction.HomeWinProbability,
		"away_win":   prediction.AwayWinProbability,
		"home_cover": prediction.HomeCoverProbability,
		"away_cover": prediction.AwayCoverProbability,
	}
	for label, p := range probabilities {
		if p < 0 || p > 1 {
			t.Fatalf("%s probability %.6f must be in [0, 1]", label, p)
		}
	}

	if math.Abs(prediction.HomeWinProbability+prediction.AwayWinProbability-1.0) > 1e-9 {
		t.Fatalf("win probabilities must sum to 1, got %.12f", prediction.HomeWinProbability+prediction.AwayWinProbability)
	}
	if math.Abs(prediction.HomeCoverProbability+prediction.AwayCoverProbability-1.0) > 1e-9 {
		t.Fatalf("cover probabilities must sum to 1, got %.12f", prediction.HomeCoverProbability+prediction.AwayCoverProbability)
	}
}

func baseMatchupInput() MatchupInput {
	return MatchupInput{
		HomeTeam: TeamProfile{
			Name:            "Boston Celtics",
			OffensiveRating: 118.3,
			DefensiveRating: 110.7,
			Pace:            99.5,
			Lineup: []PlayerAvailability{
				{
					Name:            "Home Star",
					Availability:    1.0,
					OffensiveImpact: 4.2,
					DefensiveImpact: 1.8,
				},
				{
					Name:            "Home Wing",
					Availability:    1.0,
					OffensiveImpact: 1.4,
					DefensiveImpact: 0.8,
				},
			},
		},
		AwayTeam: TeamProfile{
			Name:            "Miami Heat",
			OffensiveRating: 114.2,
			DefensiveRating: 111.6,
			Pace:            97.8,
			Lineup: []PlayerAvailability{
				{
					Name:            "Away Star",
					Availability:    1.0,
					OffensiveImpact: 3.0,
					DefensiveImpact: 1.2,
				},
				{
					Name:            "Away Guard",
					Availability:    0.9,
					OffensiveImpact: 1.1,
					DefensiveImpact: 0.4,
				},
			},
		},
		HomeSpreadLine: -2.5,
		HomeRestDays:   2,
		AwayRestDays:   1,
	}
}
