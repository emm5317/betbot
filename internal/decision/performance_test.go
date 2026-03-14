package decision

import (
	"math"
	"testing"
)

func TestComputeRecommendationPerformanceAwaySideCLV(t *testing.T) {
	closeProb := 0.51
	result, err := ComputeRecommendationPerformance(RecommendationPerformanceInput{
		MarketKey:                     "h2h",
		RecommendedSide:               "away",
		RecommendationHomeProbability: 0.52,
		ClosingSideProbability:        &closeProb,
	})
	if err != nil {
		t.Fatalf("ComputeRecommendationPerformance() error = %v", err)
	}

	if result.Status != RecommendationPerformanceStatusPendingOutcome {
		t.Fatalf("Status = %q, want %q", result.Status, RecommendationPerformanceStatusPendingOutcome)
	}
	if result.CLVDelta == nil {
		t.Fatal("CLVDelta expected non-nil")
	}
	wantCLV := 0.03 // 0.51 - (1 - 0.52)
	if math.Abs(*result.CLVDelta-wantCLV) > 1e-9 {
		t.Fatalf("CLVDelta = %.4f, want %.4f", *result.CLVDelta, wantCLV)
	}
}

func TestComputeRecommendationPerformanceWithoutCloseData(t *testing.T) {
	result, err := ComputeRecommendationPerformance(RecommendationPerformanceInput{
		MarketKey:                     "h2h",
		RecommendedSide:               "home",
		RecommendationHomeProbability: 0.57,
	})
	if err != nil {
		t.Fatalf("ComputeRecommendationPerformance() error = %v", err)
	}
	if result.Status != RecommendationPerformanceStatusCloseUnavailable {
		t.Fatalf("Status = %q, want %q", result.Status, RecommendationPerformanceStatusCloseUnavailable)
	}
	if result.CLVDelta != nil {
		t.Fatal("CLVDelta expected nil")
	}
	if result.RealizedResult != RecommendationResultUnknown {
		t.Fatalf("RealizedResult = %q, want %q", result.RealizedResult, RecommendationResultUnknown)
	}
}

func TestComputeRecommendationPerformanceSettledWin(t *testing.T) {
	closeProb := 0.54
	home := 4
	away := 2
	result, err := ComputeRecommendationPerformance(RecommendationPerformanceInput{
		MarketKey:                     "h2h",
		RecommendedSide:               "home",
		RecommendationHomeProbability: 0.50,
		ClosingSideProbability:        &closeProb,
		HomeScore:                     &home,
		AwayScore:                     &away,
	})
	if err != nil {
		t.Fatalf("ComputeRecommendationPerformance() error = %v", err)
	}
	if result.Status != RecommendationPerformanceStatusSettled {
		t.Fatalf("Status = %q, want %q", result.Status, RecommendationPerformanceStatusSettled)
	}
	if result.RealizedResult != RecommendationResultWin {
		t.Fatalf("RealizedResult = %q, want %q", result.RealizedResult, RecommendationResultWin)
	}
}

func TestGradeRecommendationOutcomeUnknownForUnsupportedMarket(t *testing.T) {
	home := 20
	away := 17
	outcome, err := GradeRecommendationOutcome(OutcomeGradeInput{
		MarketKey:       "spreads",
		RecommendedSide: "home",
		HomeScore:       &home,
		AwayScore:       &away,
	})
	if err != nil {
		t.Fatalf("GradeRecommendationOutcome() error = %v", err)
	}
	if outcome != RecommendationResultUnknown {
		t.Fatalf("outcome = %q, want %q", outcome, RecommendationResultUnknown)
	}
}

func TestComputeRecommendationPerformanceRejectsInvalidSide(t *testing.T) {
	closeProb := 0.52
	_, err := ComputeRecommendationPerformance(RecommendationPerformanceInput{
		MarketKey:                     "h2h",
		RecommendedSide:               "over",
		RecommendationHomeProbability: 0.50,
		ClosingSideProbability:        &closeProb,
	})
	if err == nil {
		t.Fatal("expected error for invalid side")
	}
}
