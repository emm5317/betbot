package decision

import (
	"errors"
	"math"
	"testing"

	"betbot/internal/domain"
)

func TestDefaultEVThresholdPolicyBySport(t *testing.T) {
	tests := []struct {
		name    string
		sport   domain.Sport
		minEdge float64
	}{
		{name: "mlb", sport: domain.SportMLB, minEdge: 0.015},
		{name: "nba", sport: domain.SportNBA, minEdge: 0.020},
		{name: "nhl", sport: domain.SportNHL, minEdge: 0.022},
		{name: "nfl", sport: domain.SportNFL, minEdge: 0.025},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := DefaultEVThresholdPolicy(tt.sport)
			if err != nil {
				t.Fatalf("DefaultEVThresholdPolicy() error = %v", err)
			}
			if policy.Sport != tt.sport {
				t.Fatalf("Sport = %q, want %q", policy.Sport, tt.sport)
			}
			if policy.MinEdge != tt.minEdge {
				t.Fatalf("MinEdge = %.3f, want %.3f", policy.MinEdge, tt.minEdge)
			}
		})
	}
}

func TestResolveEVThresholdPolicyDefaultsAndOverrides(t *testing.T) {
	policy, err := ResolveEVThresholdPolicy("", 0)
	if err != nil {
		t.Fatalf("ResolveEVThresholdPolicy() error = %v", err)
	}
	if policy.MinEdge != defaultEVThreshold {
		t.Fatalf("MinEdge = %.3f, want %.3f", policy.MinEdge, defaultEVThreshold)
	}

	policy, err = ResolveEVThresholdPolicy(domain.SportNHL, 0)
	if err != nil {
		t.Fatalf("ResolveEVThresholdPolicy() error = %v", err)
	}
	if policy.MinEdge != 0.022 {
		t.Fatalf("MinEdge = %.3f, want %.3f", policy.MinEdge, 0.022)
	}

	policy, err = ResolveEVThresholdPolicy(domain.SportNHL, 0.03)
	if err != nil {
		t.Fatalf("ResolveEVThresholdPolicy() error = %v", err)
	}
	if policy.MinEdge != 0.03 {
		t.Fatalf("MinEdge = %.3f, want %.3f", policy.MinEdge, 0.03)
	}
}

func TestResolveEVThresholdPolicyRejectsInvalidInputs(t *testing.T) {
	if _, err := ResolveEVThresholdPolicy(domain.SportMLB, -0.01); !errors.Is(err, ErrInvalidEVThreshold) {
		t.Fatalf("expected ErrInvalidEVThreshold, got %v", err)
	}
	if _, err := ResolveEVThresholdPolicy(domain.SportMLB, math.NaN()); !errors.Is(err, ErrInvalidEVThreshold) {
		t.Fatalf("expected ErrInvalidEVThreshold for NaN, got %v", err)
	}
	if _, err := ResolveEVThresholdPolicy("soccer", 0.02); !errors.Is(err, ErrUnsupportedSport) {
		t.Fatalf("expected ErrUnsupportedSport, got %v", err)
	}
}

func TestEvaluateEVThresholdHomeAndAway(t *testing.T) {
	home, err := EvaluateEVThreshold(EVThresholdInput{
		Sport:                 domain.SportMLB,
		ModelHomeProbability:  0.57,
		MarketHomeProbability: 0.55,
	})
	if err != nil {
		t.Fatalf("EvaluateEVThreshold(home) error = %v", err)
	}
	if home.RecommendedSide != homeSide {
		t.Fatalf("RecommendedSide = %q, want %q", home.RecommendedSide, homeSide)
	}
	if !home.Pass {
		t.Fatal("expected home decision to pass threshold")
	}

	away, err := EvaluateEVThreshold(EVThresholdInput{
		Sport:                 domain.SportNBA,
		ModelHomeProbability:  0.46,
		MarketHomeProbability: 0.52,
	})
	if err != nil {
		t.Fatalf("EvaluateEVThreshold(away) error = %v", err)
	}
	if away.RecommendedSide != awaySide {
		t.Fatalf("RecommendedSide = %q, want %q", away.RecommendedSide, awaySide)
	}
	if !away.Pass {
		t.Fatal("expected away decision to pass threshold")
	}
}

func TestEvaluateEVThresholdRejectsInvalidProbabilities(t *testing.T) {
	_, err := EvaluateEVThreshold(EVThresholdInput{
		Sport:                 domain.SportNFL,
		ModelHomeProbability:  1.1,
		MarketHomeProbability: 0.5,
	})
	if !errors.Is(err, ErrInvalidProbability) {
		t.Fatalf("expected ErrInvalidProbability, got %v", err)
	}
}

func TestEvaluateEVThresholdFailsBelowThreshold(t *testing.T) {
	decision, err := EvaluateEVThreshold(EVThresholdInput{
		Sport:                 domain.SportNFL,
		ModelHomeProbability:  0.515,
		MarketHomeProbability: 0.500,
	})
	if err != nil {
		t.Fatalf("EvaluateEVThreshold() error = %v", err)
	}
	if decision.Pass {
		t.Fatalf("Pass = true, want false (edge %.3f threshold %.3f)", decision.ModelEdge, decision.Threshold)
	}
}
