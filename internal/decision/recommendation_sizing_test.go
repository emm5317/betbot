package decision

import (
	"math"
	"reflect"
	"testing"

	"betbot/internal/domain"
)

func TestEvaluateRecommendationStakePositiveEdgeAppliesFractionAndCap(t *testing.T) {
	result, err := EvaluateRecommendationStake(RecommendationStakeRequest{
		Sport:                  domain.SportMLB,
		ModelProbability:       0.60,
		SelectedAmericanOdds:   110,
		Bankroll:               1000,
		AvailableBankrollCents: 100000,
	})
	if err != nil {
		t.Fatalf("EvaluateRecommendationStake() error = %v", err)
	}

	rawKelly := (0.60*2.10 - 1.0) / (2.10 - 1.0)
	if math.Abs(result.RawKellyFraction-rawKelly) > 1e-9 {
		t.Fatalf("RawKellyFraction = %.9f, want %.9f", result.RawKellyFraction, rawKelly)
	}
	if math.Abs(result.AppliedFractionalKelly-(rawKelly*0.25)) > 1e-9 {
		t.Fatalf("AppliedFractionalKelly = %.9f, want %.9f", result.AppliedFractionalKelly, rawKelly*0.25)
	}
	if result.CappedFraction != 0.03 {
		t.Fatalf("CappedFraction = %.6f, want 0.03", result.CappedFraction)
	}
	if result.PreBankrollStakeCents != 3000 {
		t.Fatalf("PreBankrollStakeCents = %d, want 3000", result.PreBankrollStakeCents)
	}
	if result.RecommendedStakeCents != 3000 {
		t.Fatalf("RecommendedStakeCents = %d, want 3000", result.RecommendedStakeCents)
	}
	if !result.BankrollCheckPass {
		t.Fatal("BankrollCheckPass = false, want true")
	}
	if !reflect.DeepEqual(result.Reasons, []string{stakeReasonCappedByMaxFraction, stakeReasonSized}) {
		t.Fatalf("Reasons = %v, want [capped_by_max_fraction sized]", result.Reasons)
	}
}

func TestEvaluateRecommendationStakeReturnsZeroOnNonPositiveEdge(t *testing.T) {
	result, err := EvaluateRecommendationStake(RecommendationStakeRequest{
		Sport:                  domain.SportNFL,
		ModelProbability:       0.45,
		SelectedAmericanOdds:   100,
		Bankroll:               1000,
		AvailableBankrollCents: 100000,
	})
	if err != nil {
		t.Fatalf("EvaluateRecommendationStake() error = %v", err)
	}
	if result.RecommendedStakeCents != 0 {
		t.Fatalf("RecommendedStakeCents = %d, want 0", result.RecommendedStakeCents)
	}
	if !reflect.DeepEqual(result.Reasons, []string{stakeReasonNonPositiveEdge}) {
		t.Fatalf("Reasons = %v, want [non_positive_edge]", result.Reasons)
	}
}

func TestEvaluateRecommendationStakeReducesToAvailableBankrollWhenInsufficient(t *testing.T) {
	result, err := EvaluateRecommendationStake(RecommendationStakeRequest{
		Sport:                  domain.SportMLB,
		ModelProbability:       0.60,
		SelectedAmericanOdds:   110,
		Bankroll:               1000,
		AvailableBankrollCents: 500,
	})
	if err != nil {
		t.Fatalf("EvaluateRecommendationStake() error = %v", err)
	}
	if result.BankrollCheckPass {
		t.Fatal("BankrollCheckPass = true, want false")
	}
	if result.BankrollCheckReason != bankrollCheckReasonInsufficientFunds {
		t.Fatalf("BankrollCheckReason = %q, want %q", result.BankrollCheckReason, bankrollCheckReasonInsufficientFunds)
	}
	if result.PreBankrollStakeCents != 3000 {
		t.Fatalf("PreBankrollStakeCents = %d, want 3000", result.PreBankrollStakeCents)
	}
	if result.RecommendedStakeCents != 500 {
		t.Fatalf("RecommendedStakeCents = %d, want 500", result.RecommendedStakeCents)
	}
	expectedReasons := []string{
		stakeReasonCappedByMaxFraction,
		stakeReasonBankrollInsufficient,
		stakeReasonBankrollCapped,
		stakeReasonSized,
	}
	if !reflect.DeepEqual(result.Reasons, expectedReasons) {
		t.Fatalf("Reasons = %v, want %v", result.Reasons, expectedReasons)
	}
}

func TestEvaluateRecommendationStakeInvalidInputsProduceDeterministicReasons(t *testing.T) {
	input := RecommendationStakeRequest{
		Sport:                  domain.SportNBA,
		ModelProbability:       1.2,
		SelectedAmericanOdds:   50,
		Bankroll:               -1,
		AvailableBankrollCents: -20,
	}

	first, err := EvaluateRecommendationStake(input)
	if err != nil {
		t.Fatalf("first EvaluateRecommendationStake() error = %v", err)
	}
	second, err := EvaluateRecommendationStake(input)
	if err != nil {
		t.Fatalf("second EvaluateRecommendationStake() error = %v", err)
	}
	expected := []string{
		stakeReasonInvalidModelProbability,
		stakeReasonInvalidMarketOdds,
		stakeReasonInvalidBankroll,
		stakeReasonBankrollNonPositive,
	}
	if !reflect.DeepEqual(first.Reasons, expected) {
		t.Fatalf("first reasons = %v, want %v", first.Reasons, expected)
	}
	if !reflect.DeepEqual(second.Reasons, expected) {
		t.Fatalf("second reasons = %v, want %v", second.Reasons, expected)
	}
}
