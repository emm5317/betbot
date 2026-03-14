package decision

import (
	"testing"

	"betbot/internal/domain"
)

func TestDefaultKellyPolicyBySport(t *testing.T) {
	tests := []struct {
		name      string
		sport     domain.Sport
		kelly     float64
		maxBetCap float64
	}{
		{name: "mlb", sport: domain.SportMLB, kelly: 0.25, maxBetCap: 0.03},
		{name: "nba", sport: domain.SportNBA, kelly: 0.18, maxBetCap: 0.025},
		{name: "nhl", sport: domain.SportNHL, kelly: 0.14, maxBetCap: 0.02},
		{name: "nfl", sport: domain.SportNFL, kelly: 0.10, maxBetCap: 0.015},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := DefaultKellyPolicy(tt.sport)
			if err != nil {
				t.Fatalf("DefaultKellyPolicy() error = %v", err)
			}
			if policy.KellyFraction != tt.kelly {
				t.Fatalf("KellyFraction = %.3f, want %.3f", policy.KellyFraction, tt.kelly)
			}
			if policy.MaxBetFraction != tt.maxBetCap {
				t.Fatalf("MaxBetFraction = %.3f, want %.3f", policy.MaxBetFraction, tt.maxBetCap)
			}
		})
	}
}

func TestResolveKellyPolicySupportsOverrides(t *testing.T) {
	policy, err := ResolveKellyPolicy(domain.SportNFL, 0.12, 0.02)
	if err != nil {
		t.Fatalf("ResolveKellyPolicy() error = %v", err)
	}
	if policy.KellyFraction != 0.12 {
		t.Fatalf("KellyFraction = %.3f, want 0.12", policy.KellyFraction)
	}
	if policy.MaxBetFraction != 0.02 {
		t.Fatalf("MaxBetFraction = %.3f, want 0.02", policy.MaxBetFraction)
	}
}

func TestResolveKellyPolicyWithManualConfig(t *testing.T) {
	policy, err := ResolveKellyPolicy("", 0.20, 0.03)
	if err != nil {
		t.Fatalf("ResolveKellyPolicy() error = %v", err)
	}
	if policy.KellyFraction != 0.20 {
		t.Fatalf("KellyFraction = %.3f, want 0.20", policy.KellyFraction)
	}
	if policy.MaxBetFraction != 0.03 {
		t.Fatalf("MaxBetFraction = %.3f, want 0.03", policy.MaxBetFraction)
	}
}

func TestResolveKellyPolicyRejectsInvalidValues(t *testing.T) {
	if _, err := ResolveKellyPolicy(domain.SportMLB, -0.1, 0.03); err == nil {
		t.Fatal("expected negative kelly fraction error")
	}
	if _, err := ResolveKellyPolicy(domain.SportMLB, 0.25, 1.1); err == nil {
		t.Fatal("expected max bet fraction >1 error")
	}
	if _, err := ResolveKellyPolicy("", 0.25, 0); err == nil {
		t.Fatal("expected error when sport empty and only one override set")
	}
}
