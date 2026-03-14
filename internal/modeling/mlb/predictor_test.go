package mlb

import (
	"math"
	"testing"
)

func TestNewPitcherMatchupModelRejectsInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StarterInningsShare = 1.0

	_, err := NewPitcherMatchupModel(cfg)
	if err == nil {
		t.Fatal("expected config validation error, got nil")
	}
}

func TestPredictRejectsInvalidInput(t *testing.T) {
	model := NewDefaultPitcherMatchupModel()

	_, err := model.Predict(MatchupInput{
		HomeTeam: TeamProfile{Name: "Boston Red Sox", RunsPerGame: 4.9, TeamERA: 3.9},
		AwayTeam: TeamProfile{Name: "New York Yankees", RunsPerGame: 5.2, TeamERA: 4.0},
		HomeStarter: PitcherProfile{
			Name: "Chris Sale",
		},
		AwayStarter: PitcherProfile{
			Name: "Gerrit Cole",
			ERA:  ptr(3.18),
		},
	})
	if err == nil {
		t.Fatal("expected missing starter metric error, got nil")
	}
}

func TestPredictFavorsBetterHomeStarter(t *testing.T) {
	model := NewDefaultPitcherMatchupModel()

	prediction, err := model.Predict(MatchupInput{
		HomeTeam: TeamProfile{Name: "Atlanta Braves", RunsPerGame: 4.9, TeamERA: 3.8, BattingOPS: ptr(0.751)},
		AwayTeam: TeamProfile{Name: "Philadelphia Phillies", RunsPerGame: 4.8, TeamERA: 3.9, BattingOPS: ptr(0.748)},
		HomeStarter: PitcherProfile{
			Name:          "Spencer Strider",
			ERA:           ptr(2.88),
			FIP:           ptr(2.96),
			WHIP:          ptr(1.03),
			StrikeoutRate: ptr(0.34),
			WalkRate:      ptr(0.07),
		},
		AwayStarter: PitcherProfile{
			Name:          "Depth Arm",
			ERA:           ptr(4.92),
			FIP:           ptr(4.78),
			WHIP:          ptr(1.38),
			StrikeoutRate: ptr(0.20),
			WalkRate:      ptr(0.10),
		},
	})
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}

	if prediction.HomeMoneylineProbability <= 0.60 {
		t.Fatalf("home moneyline probability = %.4f, want > 0.60", prediction.HomeMoneylineProbability)
	}
	if prediction.FirstFiveHomeProbability <= 0.70 {
		t.Fatalf("first-five home probability = %.4f, want > 0.70", prediction.FirstFiveHomeProbability)
	}
	if prediction.ExpectedTotalRuns <= prediction.FirstFiveExpectedTotalRuns {
		t.Fatalf("full-game total %.4f should exceed first-five total %.4f", prediction.ExpectedTotalRuns, prediction.FirstFiveExpectedTotalRuns)
	}
}

func TestPredictRespondsToHomeFieldAndOffenseInputs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HomeFieldRuns = 0
	neutralModel, err := NewPitcherMatchupModel(cfg)
	if err != nil {
		t.Fatalf("NewPitcherMatchupModel() error = %v", err)
	}

	homeEdgeModel := NewDefaultPitcherMatchupModel()

	input := MatchupInput{
		HomeTeam: TeamProfile{Name: "Chicago Cubs", RunsPerGame: 4.7, TeamERA: 4.1, BattingOPS: ptr(0.733)},
		AwayTeam: TeamProfile{Name: "Milwaukee Brewers", RunsPerGame: 4.6, TeamERA: 4.1, BattingOPS: ptr(0.729)},
		HomeStarter: PitcherProfile{
			Name:          "Home Starter",
			ERA:           ptr(3.71),
			FIP:           ptr(3.69),
			WHIP:          ptr(1.19),
			StrikeoutRate: ptr(0.26),
			WalkRate:      ptr(0.07),
		},
		AwayStarter: PitcherProfile{
			Name:          "Away Starter",
			ERA:           ptr(3.74),
			FIP:           ptr(3.77),
			WHIP:          ptr(1.20),
			StrikeoutRate: ptr(0.25),
			WalkRate:      ptr(0.07),
		},
	}

	neutralPrediction, err := neutralModel.Predict(input)
	if err != nil {
		t.Fatalf("neutral Predict() error = %v", err)
	}
	edgePrediction, err := homeEdgeModel.Predict(input)
	if err != nil {
		t.Fatalf("edge Predict() error = %v", err)
	}

	if edgePrediction.HomeMoneylineProbability <= neutralPrediction.HomeMoneylineProbability {
		t.Fatalf("home-field probability %.4f should exceed neutral %.4f", edgePrediction.HomeMoneylineProbability, neutralPrediction.HomeMoneylineProbability)
	}
	if math.Abs(edgePrediction.HomeMoneylineProbability+edgePrediction.AwayMoneylineProbability-1.0) > 1e-9 {
		t.Fatalf("moneyline probabilities must sum to 1, got %.12f", edgePrediction.HomeMoneylineProbability+edgePrediction.AwayMoneylineProbability)
	}
	if math.Abs(edgePrediction.FirstFiveHomeProbability+edgePrediction.FirstFiveAwayProbability-1.0) > 1e-9 {
		t.Fatalf("first-five probabilities must sum to 1, got %.12f", edgePrediction.FirstFiveHomeProbability+edgePrediction.FirstFiveAwayProbability)
	}
}

func ptr(v float64) *float64 {
	return &v
}
