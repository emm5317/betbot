package store

import (
	"strings"
	"testing"
)

func TestGetBankrollBalanceCentsQueryUsesLedgerAggregate(t *testing.T) {
	query := strings.ToUpper(getBankrollBalanceCents)
	if !strings.Contains(query, "FROM BANKROLL_LEDGER") {
		t.Fatalf("GetBankrollBalanceCents query must read bankroll_ledger: %s", getBankrollBalanceCents)
	}
	if !strings.Contains(query, "SUM(AMOUNT_CENTS)") {
		t.Fatalf("GetBankrollBalanceCents query must aggregate amount_cents: %s", getBankrollBalanceCents)
	}
}

func TestGetBankrollCircuitMetricsQueryUsesDeterministicLedgerState(t *testing.T) {
	query := strings.ToUpper(getBankrollCircuitMetrics)
	if !strings.Contains(query, "FROM BANKROLL_LEDGER") {
		t.Fatalf("GetBankrollCircuitMetrics query must read bankroll_ledger: %s", getBankrollCircuitMetrics)
	}
	if !strings.Contains(query, "ORDER BY CREATED_AT ASC, ID ASC") {
		t.Fatalf("GetBankrollCircuitMetrics query must use deterministic running-order: %s", getBankrollCircuitMetrics)
	}
	if !strings.Contains(query, "DATE_TRUNC('DAY', NOW())") || !strings.Contains(query, "DATE_TRUNC('WEEK', NOW())") {
		t.Fatalf("GetBankrollCircuitMetrics query must compute day/week anchors from database NOW(): %s", getBankrollCircuitMetrics)
	}
	if !strings.Contains(query, "MAX(BALANCE_AFTER_CENTS)") {
		t.Fatalf("GetBankrollCircuitMetrics query must compute peak from running balance: %s", getBankrollCircuitMetrics)
	}
}
