package decision

import (
	"errors"
	"testing"
)

func TestCheckBankrollAvailabilityPassesWhenStakeCovered(t *testing.T) {
	result, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: 10000,
		StakeCents:     1500,
	})
	if err != nil {
		t.Fatalf("CheckBankrollAvailability() error = %v", err)
	}
	if !result.Pass {
		t.Fatal("expected pass=true")
	}
	if result.Reason != bankrollCheckReasonOK {
		t.Fatalf("Reason = %q, want %q", result.Reason, bankrollCheckReasonOK)
	}
}

func TestCheckBankrollAvailabilityFailsOnInsufficientFunds(t *testing.T) {
	result, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: 1000,
		StakeCents:     1200,
	})
	if err != nil {
		t.Fatalf("CheckBankrollAvailability() error = %v", err)
	}
	if result.Pass {
		t.Fatal("expected pass=false")
	}
	if result.Reason != bankrollCheckReasonInsufficientFunds {
		t.Fatalf("Reason = %q, want %q", result.Reason, bankrollCheckReasonInsufficientFunds)
	}
	if result.ShortfallCents != 200 {
		t.Fatalf("ShortfallCents = %d, want 200", result.ShortfallCents)
	}
}

func TestCheckBankrollAvailabilityTreatsZeroStakeAsNonActionable(t *testing.T) {
	result, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: 1000,
		StakeCents:     0,
	})
	if err != nil {
		t.Fatalf("CheckBankrollAvailability() error = %v", err)
	}
	if result.Pass {
		t.Fatal("expected pass=false for zero stake")
	}
	if result.Reason != bankrollCheckReasonStakeNonPositive {
		t.Fatalf("Reason = %q, want %q", result.Reason, bankrollCheckReasonStakeNonPositive)
	}
}

func TestCheckBankrollAvailabilityRejectsNegativeInputs(t *testing.T) {
	if _, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: -1,
		StakeCents:     100,
	}); !errors.Is(err, ErrInvalidBankrollCents) {
		t.Fatalf("expected ErrInvalidBankrollCents, got %v", err)
	}

	if _, err := CheckBankrollAvailability(BankrollAvailabilityInput{
		AvailableCents: 100,
		StakeCents:     -1,
	}); !errors.Is(err, ErrInvalidStakeCents) {
		t.Fatalf("expected ErrInvalidStakeCents, got %v", err)
	}
}
