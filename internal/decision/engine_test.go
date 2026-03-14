package decision

import (
	"strings"
	"testing"

	"betbot/internal/domain"
)

func TestEngineEvaluateUsesConfiguredSportDefaultThreshold(t *testing.T) {
	engine, err := NewEngine(EngineConfig{Sport: domain.SportMLB})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	decision, err := engine.Evaluate(EvaluateInput{
		ModelHomeProbability:  0.566,
		MarketHomeProbability: 0.550,
	})
	if err != nil {
		t.Fatalf("Engine.Evaluate() error = %v", err)
	}
	if decision.Sport != domain.SportMLB {
		t.Fatalf("Sport = %q, want %q", decision.Sport, domain.SportMLB)
	}
	if !decision.Pass {
		t.Fatalf("expected pass for edge %.3f with threshold %.3f", decision.ModelEdge, decision.Threshold)
	}
}

func TestEngineEvaluateRejectsSportMismatch(t *testing.T) {
	engine, err := NewEngine(EngineConfig{Sport: domain.SportNBA})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	_, err = engine.Evaluate(EvaluateInput{
		Sport:                 domain.SportNFL,
		ModelHomeProbability:  0.6,
		MarketHomeProbability: 0.5,
	})
	if err == nil {
		t.Fatal("expected sport mismatch error")
	}
	if !strings.Contains(err.Error(), "cannot evaluate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngineFilterPassing(t *testing.T) {
	engine, err := NewEngine(EngineConfig{EVThreshold: 0.03})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	filtered, err := engine.FilterPassing([]EvaluateInput{
		{Sport: domain.SportMLB, ModelHomeProbability: 0.54, MarketHomeProbability: 0.50},
		{Sport: domain.SportNHL, ModelHomeProbability: 0.51, MarketHomeProbability: 0.50},
		{Sport: domain.SportNFL, ModelHomeProbability: 0.47, MarketHomeProbability: 0.52},
	})
	if err != nil {
		t.Fatalf("FilterPassing() error = %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].RecommendedSide != homeSide {
		t.Fatalf("filtered[0].RecommendedSide = %q, want %q", filtered[0].RecommendedSide, homeSide)
	}
	if filtered[1].RecommendedSide != awaySide {
		t.Fatalf("filtered[1].RecommendedSide = %q, want %q", filtered[1].RecommendedSide, awaySide)
	}
}

func TestNewEngineRejectsInvalidThreshold(t *testing.T) {
	if _, err := NewEngine(EngineConfig{EVThreshold: 1.1}); err == nil {
		t.Fatal("expected threshold validation error")
	}
}
