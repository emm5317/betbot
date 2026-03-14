package decision

import (
	"fmt"
	"math"

	"betbot/internal/domain"
)

type KellyPolicy struct {
	Sport          domain.Sport `json:"sport,omitempty"`
	KellyFraction  float64      `json:"kelly_fraction"`
	MaxBetFraction float64      `json:"max_bet_fraction"`
}

var defaultKellyPolicies = map[domain.Sport]KellyPolicy{
	domain.SportMLB: {Sport: domain.SportMLB, KellyFraction: 0.25, MaxBetFraction: 0.03},
	domain.SportNBA: {Sport: domain.SportNBA, KellyFraction: 0.18, MaxBetFraction: 0.025},
	domain.SportNHL: {Sport: domain.SportNHL, KellyFraction: 0.14, MaxBetFraction: 0.02},
	domain.SportNFL: {Sport: domain.SportNFL, KellyFraction: 0.10, MaxBetFraction: 0.015},
}

func DefaultKellyPolicy(sport domain.Sport) (KellyPolicy, error) {
	policy, ok := defaultKellyPolicies[sport]
	if !ok {
		return KellyPolicy{}, fmt.Errorf("unsupported sport %q for kelly defaults", sport)
	}
	return policy, nil
}

func ResolveKellyPolicy(sport domain.Sport, kellyFraction float64, maxBetFraction float64) (KellyPolicy, error) {
	if err := validateFractionInput(kellyFraction, "kelly fraction"); err != nil {
		return KellyPolicy{}, err
	}
	if err := validateFractionInput(maxBetFraction, "max bet fraction"); err != nil {
		return KellyPolicy{}, err
	}

	hasKellyOverride := kellyFraction > 0
	hasCapOverride := maxBetFraction > 0
	if sport == "" && (hasKellyOverride != hasCapOverride) {
		return KellyPolicy{}, fmt.Errorf("kelly fraction and max bet fraction overrides must both be set when sport is empty")
	}

	policy := KellyPolicy{
		Sport:          sport,
		KellyFraction:  kellyFraction,
		MaxBetFraction: maxBetFraction,
	}
	if sport != "" {
		defaults, err := DefaultKellyPolicy(sport)
		if err != nil {
			return KellyPolicy{}, err
		}
		policy = defaults
		if hasKellyOverride {
			policy.KellyFraction = kellyFraction
		}
		if hasCapOverride {
			policy.MaxBetFraction = maxBetFraction
		}
	}

	if policy.KellyFraction <= 0 {
		return KellyPolicy{}, fmt.Errorf("kelly fraction must be in (0,1]")
	}
	if policy.MaxBetFraction <= 0 {
		return KellyPolicy{}, fmt.Errorf("max bet fraction must be in (0,1]")
	}
	return policy, nil
}

func validateFractionInput(value float64, field string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be finite in [0,1]", field)
	}
	return nil
}
