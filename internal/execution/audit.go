package execution

import (
	"context"
	"encoding/json"
	"fmt"

	"betbot/internal/store"
)

// WriteSettlementLedgerEntry records a settlement outcome in the bankroll ledger.
// For wins, amountCents is positive (the payout). For losses, amountCents is 0
// (stake was already reserved). For pushes, amountCents equals the original stake
// (full refund).
func WriteSettlementLedgerEntry(ctx context.Context, queries *store.Queries, bet store.Bet, result string, amountCents int64) error {
	entryType := "bet_settlement_loss"
	switch result {
	case "win":
		entryType = "bet_settlement_win"
	case "push":
		entryType = "bet_settlement_push"
	}

	_, err := queries.InsertBankrollEntry(ctx, store.InsertBankrollEntryParams{
		EntryType:     entryType,
		AmountCents:   amountCents,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   bet.IdempotencyKey,
		Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d,"result":"%s"}`, bet.ID, result)),
	})
	if err != nil {
		return fmt.Errorf("write settlement ledger entry: %w", err)
	}
	return nil
}
