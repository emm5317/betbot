package decision

import (
	"fmt"
	"math"
	"sort"

	"betbot/internal/domain"
)

type SizingRequest struct {
	Sport          domain.Sport
	Bankroll       float64
	ModelEdge      float64
	KellyFraction  float64
	MaxBetFraction float64
}

type SizingResult struct {
	Sport          domain.Sport `json:"sport,omitempty"`
	KellyFraction  float64      `json:"kelly_fraction"`
	MaxBetFraction float64      `json:"max_bet_fraction"`
	StakeFraction  float64      `json:"stake_fraction"`
	StakeDollars   float64      `json:"stake_dollars"`
}

const (
	stakeReasonInvalidModelProbability = "invalid_model_probability"
	stakeReasonInvalidMarketOdds       = "invalid_market_odds"
	stakeReasonInvalidBankroll         = "invalid_bankroll"
	stakeReasonBankrollNonPositive     = "bankroll_non_positive"
	stakeReasonNonPositiveEdge         = "non_positive_edge"
	stakeReasonCappedByMaxFraction     = "capped_by_max_fraction"
	stakeReasonStakeRoundedToZero      = "stake_rounded_to_zero"
	stakeReasonBankrollInsufficient    = "bankroll_insufficient"
	stakeReasonBankrollCapped          = "bankroll_capped_to_available"
	stakeReasonStakeNonPositive        = "stake_non_positive"
	stakeReasonSized                   = "sized"
)

var recommendationStakeReasonOrder = map[string]int{
	stakeReasonInvalidModelProbability: 0,
	stakeReasonInvalidMarketOdds:       1,
	stakeReasonInvalidBankroll:         2,
	stakeReasonBankrollNonPositive:     3,
	stakeReasonNonPositiveEdge:         4,
	stakeReasonCappedByMaxFraction:     5,
	stakeReasonStakeRoundedToZero:      6,
	stakeReasonBankrollInsufficient:    7,
	stakeReasonBankrollCapped:          8,
	stakeReasonStakeNonPositive:        9,
	stakeReasonSized:                   10,
}

type RecommendationStakeRequest struct {
	Sport                  domain.Sport
	ModelProbability       float64
	SelectedAmericanOdds   int
	Bankroll               float64
	AvailableBankrollCents int64
	KellyFraction          float64
	MaxBetFraction         float64
}

type RecommendationStakeResult struct {
	Sport                    domain.Sport `json:"sport,omitempty"`
	KellyFraction            float64      `json:"kelly_fraction"`
	MaxBetFraction           float64      `json:"max_bet_fraction"`
	RawKellyFraction         float64      `json:"raw_kelly_fraction"`
	AppliedFractionalKelly   float64      `json:"applied_fractional_kelly"`
	CappedFraction           float64      `json:"capped_fraction"`
	PreBankrollStakeDollars  float64      `json:"pre_bankroll_stake_dollars"`
	PreBankrollStakeCents    int64        `json:"pre_bankroll_stake_cents"`
	BankrollAvailableCents   int64        `json:"bankroll_available_cents"`
	RecommendedStakeDollars  float64      `json:"recommended_stake_dollars"`
	RecommendedStakeCents    int64        `json:"recommended_stake_cents"`
	RecommendedStakeFraction float64      `json:"recommended_stake_fraction"`
	BankrollCheckPass        bool         `json:"bankroll_check_passed"`
	BankrollCheckReason      string       `json:"bankroll_check_reason"`
	Reasons                  []string     `json:"reasons"`
}

func RecommendStake(req SizingRequest) (SizingResult, error) {
	if math.IsNaN(req.Bankroll) || math.IsInf(req.Bankroll, 0) || req.Bankroll < 0 {
		return SizingResult{}, fmt.Errorf("bankroll must be finite and >= 0")
	}
	if math.IsNaN(req.ModelEdge) || math.IsInf(req.ModelEdge, 0) {
		return SizingResult{}, fmt.Errorf("model edge must be finite")
	}

	policy, err := ResolveKellyPolicy(req.Sport, req.KellyFraction, req.MaxBetFraction)
	if err != nil {
		return SizingResult{}, err
	}
	result := SizingResult{
		Sport:          policy.Sport,
		KellyFraction:  policy.KellyFraction,
		MaxBetFraction: policy.MaxBetFraction,
	}

	if req.Bankroll == 0 || req.ModelEdge <= 0 {
		return result, nil
	}

	edge := req.ModelEdge
	if edge > 1 {
		edge = 1
	}

	stakeFraction := edge * policy.KellyFraction
	if stakeFraction > policy.MaxBetFraction {
		stakeFraction = policy.MaxBetFraction
	}
	if stakeFraction > 1 {
		stakeFraction = 1
	}
	if stakeFraction <= 0 {
		return result, nil
	}

	stake := req.Bankroll * stakeFraction
	if stake > req.Bankroll {
		stake = req.Bankroll
	}

	result.StakeFraction = stakeFraction
	result.StakeDollars = stake
	return result, nil
}

func EvaluateRecommendationStake(req RecommendationStakeRequest) (RecommendationStakeResult, error) {
	policy, err := ResolveKellyPolicy(req.Sport, req.KellyFraction, req.MaxBetFraction)
	if err != nil {
		return RecommendationStakeResult{}, err
	}

	result := RecommendationStakeResult{
		Sport:                  policy.Sport,
		KellyFraction:          policy.KellyFraction,
		MaxBetFraction:         policy.MaxBetFraction,
		BankrollAvailableCents: req.AvailableBankrollCents,
		BankrollCheckReason:    bankrollCheckReasonStakeNonPositive,
	}
	reasonSet := map[string]struct{}{}
	addReason := func(reason string) {
		if reason == "" {
			return
		}
		reasonSet[reason] = struct{}{}
	}

	validModelProbability := isFinite(req.ModelProbability) && req.ModelProbability > 0 && req.ModelProbability < 1
	if !validModelProbability {
		addReason(stakeReasonInvalidModelProbability)
	}

	decimalOdds, oddsErr := americanOddsToDecimal(req.SelectedAmericanOdds)
	if oddsErr != nil {
		addReason(stakeReasonInvalidMarketOdds)
	}

	validBankroll := isFinite(req.Bankroll) && req.Bankroll >= 0 && req.AvailableBankrollCents >= 0
	if !validBankroll {
		addReason(stakeReasonInvalidBankroll)
	}
	if req.Bankroll <= 0 || req.AvailableBankrollCents == 0 {
		addReason(stakeReasonBankrollNonPositive)
	}

	if !validModelProbability || oddsErr != nil || !validBankroll {
		result.Reasons = orderedRecommendationStakeReasons(reasonSet)
		return result, nil
	}

	result.RawKellyFraction = kellyFractionFromProbabilityAndDecimalOdds(req.ModelProbability, decimalOdds)
	if !isFinite(result.RawKellyFraction) || result.RawKellyFraction <= 0 {
		addReason(stakeReasonNonPositiveEdge)
		result.Reasons = orderedRecommendationStakeReasons(reasonSet)
		return result, nil
	}

	result.AppliedFractionalKelly = result.RawKellyFraction * policy.KellyFraction
	result.CappedFraction = result.AppliedFractionalKelly
	if result.CappedFraction > policy.MaxBetFraction {
		result.CappedFraction = policy.MaxBetFraction
		addReason(stakeReasonCappedByMaxFraction)
	}
	if result.CappedFraction > 1 {
		result.CappedFraction = 1
		addReason(stakeReasonCappedByMaxFraction)
	}
	if result.CappedFraction <= 0 {
		addReason(stakeReasonNonPositiveEdge)
		result.Reasons = orderedRecommendationStakeReasons(reasonSet)
		return result, nil
	}

	result.PreBankrollStakeDollars = req.Bankroll * result.CappedFraction
	if !isFinite(result.PreBankrollStakeDollars) || result.PreBankrollStakeDollars <= 0 {
		addReason(stakeReasonStakeNonPositive)
		result.Reasons = orderedRecommendationStakeReasons(reasonSet)
		return result, nil
	}
	result.PreBankrollStakeCents = int64(math.Round(result.PreBankrollStakeDollars * 100))
	if result.PreBankrollStakeCents <= 0 {
		addReason(stakeReasonStakeRoundedToZero)
		addReason(stakeReasonStakeNonPositive)
		result.Reasons = orderedRecommendationStakeReasons(reasonSet)
		return result, nil
	}

	bankrollCheck, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: req.AvailableBankrollCents,
		StakeCents:     result.PreBankrollStakeCents,
	})
	if err != nil {
		return RecommendationStakeResult{}, fmt.Errorf("check bankroll availability: %w", err)
	}
	result.BankrollCheckPass = bankrollCheck.Pass
	result.BankrollCheckReason = bankrollCheck.Reason

	recommendedCents := result.PreBankrollStakeCents
	switch bankrollCheck.Reason {
	case bankrollCheckReasonOK:
		// passthrough
	case bankrollCheckReasonInsufficientFunds:
		addReason(stakeReasonBankrollInsufficient)
		if bankrollCheck.AvailableCents > 0 {
			addReason(stakeReasonBankrollCapped)
		}
		recommendedCents = bankrollCheck.AvailableCents
	case bankrollCheckReasonStakeNonPositive:
		addReason(stakeReasonStakeNonPositive)
		recommendedCents = 0
	default:
		recommendedCents = 0
	}

	if recommendedCents < 0 {
		recommendedCents = 0
	}
	result.RecommendedStakeCents = recommendedCents
	result.RecommendedStakeDollars = float64(recommendedCents) / 100.0
	if req.Bankroll > 0 {
		result.RecommendedStakeFraction = result.RecommendedStakeDollars / req.Bankroll
	}
	if result.RecommendedStakeCents > 0 {
		addReason(stakeReasonSized)
	} else {
		addReason(stakeReasonStakeNonPositive)
	}
	result.Reasons = orderedRecommendationStakeReasons(reasonSet)
	return result, nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func americanOddsToDecimal(american int) (float64, error) {
	if err := validateAmericanOdds(american, "selected_odds"); err != nil {
		return 0, err
	}
	if american >= 100 {
		return 1 + (float64(american) / 100.0), nil
	}
	return 1 + (100.0 / math.Abs(float64(american))), nil
}

func kellyFractionFromProbabilityAndDecimalOdds(probability float64, decimalOdds float64) float64 {
	netOdds := decimalOdds - 1
	if netOdds <= 0 {
		return 0
	}
	return ((probability * decimalOdds) - 1.0) / netOdds
}

func orderedRecommendationStakeReasons(reasonSet map[string]struct{}) []string {
	if len(reasonSet) == 0 {
		return []string{}
	}
	reasons := make([]string, 0, len(reasonSet))
	for reason := range reasonSet {
		reasons = append(reasons, reason)
	}
	sort.SliceStable(reasons, func(i, j int) bool {
		leftRank, leftKnown := recommendationStakeReasonOrder[reasons[i]]
		rightRank, rightKnown := recommendationStakeReasonOrder[reasons[j]]
		if leftKnown && rightKnown {
			return leftRank < rightRank
		}
		if leftKnown {
			return true
		}
		if rightKnown {
			return false
		}
		return reasons[i] < reasons[j]
	})
	return reasons
}
