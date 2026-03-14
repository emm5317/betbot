package decision

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidBankrollCents = errors.New("invalid bankroll cents")
	ErrInvalidStakeCents    = errors.New("invalid stake cents")
)

const (
	bankrollCheckReasonOK                = "ok"
	bankrollCheckReasonStakeNonPositive  = "stake_non_positive"
	bankrollCheckReasonInsufficientFunds = "insufficient_funds"
)

type BankrollAvailabilityInput struct {
	AvailableCents int64
	StakeCents     int64
}

type BankrollAvailabilityResult struct {
	Pass           bool   `json:"pass"`
	Reason         string `json:"reason"`
	AvailableCents int64  `json:"available_cents"`
	StakeCents     int64  `json:"stake_cents"`
	ShortfallCents int64  `json:"shortfall_cents"`
}

func CheckBankrollAvailability(input BankrollAvailabilityInput) (BankrollAvailabilityResult, error) {
	if input.AvailableCents < 0 {
		return BankrollAvailabilityResult{}, fmt.Errorf("%w: available_cents must be >= 0", ErrInvalidBankrollCents)
	}
	if input.StakeCents < 0 {
		return BankrollAvailabilityResult{}, fmt.Errorf("%w: stake_cents must be >= 0", ErrInvalidStakeCents)
	}

	result := BankrollAvailabilityResult{
		Reason:         bankrollCheckReasonOK,
		AvailableCents: input.AvailableCents,
		StakeCents:     input.StakeCents,
	}
	if input.StakeCents <= 0 {
		result.Reason = bankrollCheckReasonStakeNonPositive
		return result, nil
	}
	if input.StakeCents > input.AvailableCents {
		result.Reason = bankrollCheckReasonInsufficientFunds
		result.ShortfallCents = input.StakeCents - input.AvailableCents
		return result, nil
	}

	result.Pass = true
	return result, nil
}
