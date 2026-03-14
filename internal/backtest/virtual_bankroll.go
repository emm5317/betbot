package backtest

import (
	"fmt"
	"math"

	"betbot/internal/decision"
	"betbot/internal/domain"
)

type BankrollConfig struct {
	Sport               domain.Sport
	StartingBankroll    float64
	KellyFraction       float64
	MaxStakeFraction    float64
	MinimumStakeDollars float64
}

type VirtualBankroll struct {
	cfg     BankrollConfig
	balance float64
}

func NewVirtualBankroll(cfg BankrollConfig) (VirtualBankroll, error) {
	switch {
	case math.IsNaN(cfg.StartingBankroll) || math.IsInf(cfg.StartingBankroll, 0) || cfg.StartingBankroll <= 0:
		return VirtualBankroll{}, fmt.Errorf("starting bankroll must be > 0")
	case math.IsNaN(cfg.KellyFraction) || math.IsInf(cfg.KellyFraction, 0) || cfg.KellyFraction < 0 || cfg.KellyFraction > 1:
		return VirtualBankroll{}, fmt.Errorf("kelly fraction must be in [0,1]")
	case math.IsNaN(cfg.MaxStakeFraction) || math.IsInf(cfg.MaxStakeFraction, 0) || cfg.MaxStakeFraction < 0 || cfg.MaxStakeFraction > 1:
		return VirtualBankroll{}, fmt.Errorf("max stake fraction must be in [0,1]")
	case math.IsNaN(cfg.MinimumStakeDollars) || math.IsInf(cfg.MinimumStakeDollars, 0) || cfg.MinimumStakeDollars < 0:
		return VirtualBankroll{}, fmt.Errorf("minimum stake dollars must be >= 0")
	}

	if cfg.Sport != "" {
		policy, err := decision.ResolveKellyPolicy(cfg.Sport, cfg.KellyFraction, cfg.MaxStakeFraction)
		if err != nil {
			return VirtualBankroll{}, err
		}
		cfg.KellyFraction = policy.KellyFraction
		cfg.MaxStakeFraction = policy.MaxBetFraction
	}
	if cfg.Sport == "" && (cfg.KellyFraction > 0) != (cfg.MaxStakeFraction > 0) {
		return VirtualBankroll{}, fmt.Errorf("kelly and max stake overrides must both be set when sport is empty")
	}

	return VirtualBankroll{cfg: cfg, balance: cfg.StartingBankroll}, nil
}

func (b *VirtualBankroll) Balance() float64 {
	return b.balance
}

// RecommendStake computes a stake from model edge; non-positive edges return 0.
func (b *VirtualBankroll) RecommendStake(modelEdge float64) float64 {
	result, err := b.RecommendStakeForSport(b.cfg.Sport, modelEdge)
	if err != nil {
		return 0
	}
	return result.StakeDollars
}

func (b *VirtualBankroll) RecommendStakeForSport(sport domain.Sport, modelEdge float64) (decision.SizingResult, error) {
	req := decision.SizingRequest{
		Sport:          sport,
		Bankroll:       b.balance,
		ModelEdge:      modelEdge,
		KellyFraction:  b.cfg.KellyFraction,
		MaxBetFraction: b.cfg.MaxStakeFraction,
	}
	if b.cfg.Sport != "" {
		req.Sport = b.cfg.Sport
	}

	result, err := decision.RecommendStake(req)
	if err != nil {
		return decision.SizingResult{}, err
	}
	if result.StakeDollars < b.cfg.MinimumStakeDollars {
		result.StakeDollars = 0
		result.StakeFraction = 0
	}
	return result, nil
}

// ApplyCLV books a synthetic bankroll delta from the captured CLV movement.
func (b *VirtualBankroll) ApplyCLV(stake float64, clvDelta float64) {
	if stake <= 0 {
		return
	}
	if stake > b.balance {
		stake = b.balance
	}
	b.balance += stake * clvDelta
	if b.balance < 0 {
		b.balance = 0
	}
}
