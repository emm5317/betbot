package decision

import (
	"fmt"

	"betbot/internal/domain"
)

type EngineConfig struct {
	Sport       domain.Sport
	EVThreshold float64
}

type EvaluateInput struct {
	Sport                 domain.Sport
	ModelHomeProbability  float64
	MarketHomeProbability float64
}

type Engine struct {
	sport       domain.Sport
	evThreshold float64
}

func NewEngine(cfg EngineConfig) (Engine, error) {
	if cfg.Sport != "" {
		if _, err := DefaultEVThresholdPolicy(cfg.Sport); err != nil {
			return Engine{}, err
		}
	}
	if _, err := ResolveEVThresholdPolicy("", cfg.EVThreshold); err != nil {
		return Engine{}, err
	}

	return Engine{sport: cfg.Sport, evThreshold: cfg.EVThreshold}, nil
}

func (e Engine) Evaluate(input EvaluateInput) (EVThresholdDecision, error) {
	sport := input.Sport
	if sport == "" {
		sport = e.sport
	}
	if e.sport != "" && sport != "" && sport != e.sport {
		return EVThresholdDecision{}, fmt.Errorf("engine configured for sport %q cannot evaluate %q", e.sport, sport)
	}

	decision, err := EvaluateEVThreshold(EVThresholdInput{
		Sport:                 sport,
		ModelHomeProbability:  input.ModelHomeProbability,
		MarketHomeProbability: input.MarketHomeProbability,
		MinEdge:               e.evThreshold,
	})
	if err != nil {
		return EVThresholdDecision{}, err
	}
	return decision, nil
}

func (e Engine) FilterPassing(inputs []EvaluateInput) ([]EVThresholdDecision, error) {
	passing := make([]EVThresholdDecision, 0, len(inputs))
	for i := range inputs {
		decision, err := e.Evaluate(inputs[i])
		if err != nil {
			return nil, fmt.Errorf("evaluate candidate %d: %w", i, err)
		}
		if decision.Pass {
			passing = append(passing, decision)
		}
	}
	return passing, nil
}
