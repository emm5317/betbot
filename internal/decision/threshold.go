package decision

import (
	"errors"
	"fmt"
	"math"

	"betbot/internal/domain"
)

const (
	defaultEVThreshold = 0.02

	homeSide = "home"
	awaySide = "away"
)

var (
	defaultEVThresholdPolicies = map[domain.Sport]EVThresholdPolicy{
		domain.SportMLB: {Sport: domain.SportMLB, MinEdge: 0.015},
		domain.SportNBA: {Sport: domain.SportNBA, MinEdge: 0.020},
		domain.SportNHL: {Sport: domain.SportNHL, MinEdge: 0.022},
		domain.SportNFL: {Sport: domain.SportNFL, MinEdge: 0.025},
	}

	ErrInvalidProbability = errors.New("invalid probability")
	ErrInvalidEVThreshold = errors.New("invalid ev threshold")
	ErrUnsupportedSport   = errors.New("unsupported sport")
)

type EVThresholdPolicy struct {
	Sport   domain.Sport `json:"sport,omitempty"`
	MinEdge float64      `json:"min_edge"`
}

type EVThresholdInput struct {
	Sport                 domain.Sport
	ModelHomeProbability  float64
	MarketHomeProbability float64
	MinEdge               float64
}

type EVThresholdDecision struct {
	Sport           domain.Sport `json:"sport,omitempty"`
	RecommendedSide string       `json:"recommended_side"`
	ModelEdge       float64      `json:"model_edge"`
	Threshold       float64      `json:"threshold"`
	Pass            bool         `json:"pass"`
}

func DefaultEVThresholdPolicy(sport domain.Sport) (EVThresholdPolicy, error) {
	policy, ok := defaultEVThresholdPolicies[sport]
	if !ok {
		return EVThresholdPolicy{}, fmt.Errorf("%w %q", ErrUnsupportedSport, sport)
	}
	return policy, nil
}

func ResolveEVThresholdPolicy(sport domain.Sport, minEdge float64) (EVThresholdPolicy, error) {
	if math.IsNaN(minEdge) || math.IsInf(minEdge, 0) || minEdge < 0 || minEdge > 1 {
		return EVThresholdPolicy{}, fmt.Errorf("%w: must be finite in [0,1]", ErrInvalidEVThreshold)
	}

	if sport == "" {
		if minEdge > 0 {
			return EVThresholdPolicy{MinEdge: minEdge}, nil
		}
		return EVThresholdPolicy{MinEdge: defaultEVThreshold}, nil
	}

	policy, err := DefaultEVThresholdPolicy(sport)
	if err != nil {
		return EVThresholdPolicy{}, err
	}
	if minEdge > 0 {
		policy.MinEdge = minEdge
	}
	return policy, nil
}

func EvaluateEVThreshold(input EVThresholdInput) (EVThresholdDecision, error) {
	if err := validateProbability(input.ModelHomeProbability, "model home probability"); err != nil {
		return EVThresholdDecision{}, err
	}
	if err := validateProbability(input.MarketHomeProbability, "market home probability"); err != nil {
		return EVThresholdDecision{}, err
	}

	policy, err := ResolveEVThresholdPolicy(input.Sport, input.MinEdge)
	if err != nil {
		return EVThresholdDecision{}, err
	}

	decision := EVThresholdDecision{
		Sport:           policy.Sport,
		RecommendedSide: homeSide,
		Threshold:       policy.MinEdge,
	}

	homeEdge := input.ModelHomeProbability - input.MarketHomeProbability
	if homeEdge < 0 {
		decision.RecommendedSide = awaySide
		decision.ModelEdge = -homeEdge
	} else {
		decision.ModelEdge = homeEdge
	}
	decision.Pass = decision.ModelEdge >= decision.Threshold && decision.ModelEdge > 0
	return decision, nil
}

func validateProbability(value float64, field string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%w: %s must be finite in [0,1]", ErrInvalidProbability, field)
	}
	return nil
}
