package execution

import (
	"context"
	"fmt"
	"math"

	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SettlementProcessor handles settling placed bets once game results are known.
type SettlementProcessor struct {
	pool *pgxpool.Pool
}

// NewSettlementProcessor creates a new settlement processor.
func NewSettlementProcessor(pool *pgxpool.Pool) *SettlementProcessor {
	return &SettlementProcessor{pool: pool}
}

// SettleInput contains the information needed to settle a bet.
type SettleInput struct {
	BetID      int64
	HomeWin    bool
	Push       bool
	CloseProb  *float64 // closing probability for CLV capture
}

// SettleBet settles a single bet, writing ledger entries and updating status.
func (s *SettlementProcessor) SettleBet(ctx context.Context, bet store.Bet, input SettleInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin settlement tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	queries := store.New(tx)

	result, payoutCents := determineResult(bet, input)

	// Compute CLV delta if closing probability available
	var clvDelta *float64
	if input.CloseProb != nil && bet.ModelProbability != nil {
		d := *bet.ModelProbability - *input.CloseProb
		clvDelta = &d
	}

	if err := queries.UpdateBetSettled(ctx, store.UpdateBetSettledParams{
		ID:                 bet.ID,
		SettlementResult:   &result,
		PayoutCents:        &payoutCents,
		ClvDelta:           clvDelta,
		ClosingProbability: input.CloseProb,
	}); err != nil {
		return fmt.Errorf("update bet settled: %w", err)
	}

	// Write ledger entry
	if err := WriteSettlementLedgerEntry(ctx, queries, bet, result, ledgerAmount(result, bet.StakeCents, payoutCents)); err != nil {
		return fmt.Errorf("write settlement ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit settlement tx: %w", err)
	}

	return nil
}

// determineResult determines whether a bet won, lost, or pushed.
func determineResult(bet store.Bet, input SettleInput) (string, int64) {
	if input.Push {
		return "push", bet.StakeCents // refund original stake
	}

	betOnHome := bet.RecommendedSide == "home"
	won := (betOnHome && input.HomeWin) || (!betOnHome && !input.HomeWin)

	if won {
		payout := bet.StakeCents + calculateWinnings(bet.StakeCents, int(bet.AmericanOdds))
		return "win", payout
	}

	return "loss", 0
}

// calculateWinnings computes the profit from a winning bet given American odds.
func calculateWinnings(stakeCents int64, americanOdds int) int64 {
	var multiplier float64
	if americanOdds > 0 {
		multiplier = float64(americanOdds) / 100.0
	} else {
		multiplier = 100.0 / math.Abs(float64(americanOdds))
	}
	return int64(math.Round(float64(stakeCents) * multiplier))
}

// ledgerAmount returns the amount to write to the bankroll ledger for a settlement.
// Win: +payout (includes returned stake + profit).
// Loss: 0 (stake was already reserved).
// Push: +original stake (refund).
func ledgerAmount(result string, stakeCents, payoutCents int64) int64 {
	switch result {
	case "win":
		return payoutCents
	case "push":
		return stakeCents
	default:
		return 0
	}
}
