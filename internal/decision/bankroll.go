package decision

import (
	"fmt"
	"math"

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
