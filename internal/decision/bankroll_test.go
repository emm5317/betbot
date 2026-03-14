package decision

import (
	"math"
	"testing"

	"betbot/internal/domain"
)

func TestRecommendStakeUsesSportDefaultsAndCap(t *testing.T) {
	result, err := RecommendStake(SizingRequest{
		Sport:     domain.SportMLB,
		Bankroll:  1000,
		ModelEdge: 0.20,
	})
	if err != nil {
		t.Fatalf("RecommendStake() error = %v", err)
	}
	if result.KellyFraction != 0.25 {
		t.Fatalf("KellyFraction = %.3f, want 0.25", result.KellyFraction)
	}
	if result.MaxBetFraction != 0.03 {
		t.Fatalf("MaxBetFraction = %.3f, want 0.03", result.MaxBetFraction)
	}
	if result.StakeDollars != 30 {
		t.Fatalf("StakeDollars = %.2f, want 30.00", result.StakeDollars)
	}
	if result.StakeFraction != 0.03 {
		t.Fatalf("StakeFraction = %.3f, want 0.03", result.StakeFraction)
	}
}

func TestRecommendStakeReturnsZeroForNonPositiveEdge(t *testing.T) {
	result, err := RecommendStake(SizingRequest{Sport: domain.SportNBA, Bankroll: 1000, ModelEdge: 0})
	if err != nil {
		t.Fatalf("RecommendStake() error = %v", err)
	}
	if result.StakeDollars != 0 || result.StakeFraction != 0 {
		t.Fatalf("expected zero stake, got dollars=%.2f fraction=%.4f", result.StakeDollars, result.StakeFraction)
	}

	result, err = RecommendStake(SizingRequest{Sport: domain.SportNBA, Bankroll: 1000, ModelEdge: -0.01})
	if err != nil {
		t.Fatalf("RecommendStake() error = %v", err)
	}
	if result.StakeDollars != 0 || result.StakeFraction != 0 {
		t.Fatalf("expected zero stake, got dollars=%.2f fraction=%.4f", result.StakeDollars, result.StakeFraction)
	}
}

func TestRecommendStakeSupportsManualPolicy(t *testing.T) {
	result, err := RecommendStake(SizingRequest{
		Bankroll:       800,
		ModelEdge:      0.20,
		KellyFraction:  0.50,
		MaxBetFraction: 0.03,
	})
	if err != nil {
		t.Fatalf("RecommendStake() error = %v", err)
	}
	if math.Abs(result.StakeDollars-24) > 1e-9 {
		t.Fatalf("StakeDollars = %.2f, want 24.00", result.StakeDollars)
	}
}

func TestRecommendStakeRejectsInvalidInputs(t *testing.T) {
	if _, err := RecommendStake(SizingRequest{Sport: domain.SportNFL, Bankroll: -1, ModelEdge: 0.1}); err == nil {
		t.Fatal("expected invalid bankroll error")
	}
	if _, err := RecommendStake(SizingRequest{Sport: domain.SportNFL, Bankroll: 1000, ModelEdge: math.NaN()}); err == nil {
		t.Fatal("expected invalid model edge error")
	}
}
