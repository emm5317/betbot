package backtest

import (
	"testing"

	"betbot/internal/domain"
)

func TestNewVirtualBankrollRejectsInvalidConfig(t *testing.T) {
	_, err := NewVirtualBankroll(BankrollConfig{StartingBankroll: 0, KellyFraction: 0.25, MaxStakeFraction: 0.03})
	if err == nil {
		t.Fatal("expected invalid config error")
	}

	_, err = NewVirtualBankroll(BankrollConfig{StartingBankroll: 1000, KellyFraction: 0.25})
	if err == nil {
		t.Fatal("expected override pairing error")
	}
}

func TestRecommendStakeUsesManualConfigAndCapsStake(t *testing.T) {
	bankroll, err := NewVirtualBankroll(BankrollConfig{StartingBankroll: 1000, KellyFraction: 0.5, MaxStakeFraction: 0.03, MinimumStakeDollars: 1})
	if err != nil {
		t.Fatalf("NewVirtualBankroll() error = %v", err)
	}

	if stake := bankroll.RecommendStake(-0.01); stake != 0 {
		t.Fatalf("negative edge stake = %.2f, want 0", stake)
	}

	stake := bankroll.RecommendStake(0.20)
	if stake <= 0 {
		t.Fatalf("expected positive stake for positive edge, got %.2f", stake)
	}
	if stake > 30.0 {
		t.Fatalf("stake %.2f should be capped at 3%% of bankroll (30.00)", stake)
	}
}

func TestRecommendStakeForSportUsesSportDefaults(t *testing.T) {
	bankroll, err := NewVirtualBankroll(BankrollConfig{StartingBankroll: 1000, MinimumStakeDollars: 1})
	if err != nil {
		t.Fatalf("NewVirtualBankroll() error = %v", err)
	}

	mlb, err := bankroll.RecommendStakeForSport(domain.SportMLB, 0.20)
	if err != nil {
		t.Fatalf("RecommendStakeForSport(MLB) error = %v", err)
	}
	nfl, err := bankroll.RecommendStakeForSport(domain.SportNFL, 0.20)
	if err != nil {
		t.Fatalf("RecommendStakeForSport(NFL) error = %v", err)
	}

	if mlb.KellyFraction != 0.25 || mlb.MaxBetFraction != 0.03 {
		t.Fatalf("MLB policy = %.3f/%.3f, want 0.25/0.03", mlb.KellyFraction, mlb.MaxBetFraction)
	}
	if nfl.KellyFraction != 0.10 || nfl.MaxBetFraction != 0.015 {
		t.Fatalf("NFL policy = %.3f/%.3f, want 0.10/0.015", nfl.KellyFraction, nfl.MaxBetFraction)
	}
	if mlb.StakeDollars != 30 {
		t.Fatalf("MLB stake = %.2f, want 30.00", mlb.StakeDollars)
	}
	if nfl.StakeDollars != 15 {
		t.Fatalf("NFL stake = %.2f, want 15.00", nfl.StakeDollars)
	}
}

func TestApplyCLVAdjustsBalance(t *testing.T) {
	bankroll, err := NewVirtualBankroll(BankrollConfig{StartingBankroll: 500, Sport: domain.SportNBA, MinimumStakeDollars: 1})
	if err != nil {
		t.Fatalf("NewVirtualBankroll() error = %v", err)
	}

	before := bankroll.Balance()
	bankroll.ApplyCLV(50, 0.02)
	afterWin := bankroll.Balance()
	if afterWin <= before {
		t.Fatalf("balance %.2f should increase from %.2f", afterWin, before)
	}

	bankroll.ApplyCLV(50, -0.03)
	afterLoss := bankroll.Balance()
	if afterLoss >= afterWin {
		t.Fatalf("balance %.2f should decrease from %.2f", afterLoss, afterWin)
	}
}
