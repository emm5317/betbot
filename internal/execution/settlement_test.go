package execution

import "testing"

func TestCalculateWinnings(t *testing.T) {
	tests := []struct {
		name       string
		stake      int64
		odds       int
		wantProfit int64
	}{
		{"favorite -150", 15000, -150, 10000},  // $150 at -150 wins $100
		{"underdog +130", 10000, +130, 13000},  // $100 at +130 wins $130
		{"even -100", 10000, -100, 10000},      // $100 at -100 wins $100
		{"even +100", 10000, +100, 10000},      // $100 at +100 wins $100
		{"heavy fav -200", 20000, -200, 10000}, // $200 at -200 wins $100
		{"long shot +300", 5000, +300, 15000},  // $50 at +300 wins $150
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateWinnings(tt.stake, tt.odds)
			if got != tt.wantProfit {
				t.Fatalf("CalculateWinnings(%d, %d) = %d, want %d", tt.stake, tt.odds, got, tt.wantProfit)
			}
		})
	}
}

func TestLedgerAmount(t *testing.T) {
	tests := []struct {
		result string
		stake  int64
		payout int64
		want   int64
	}{
		{"win", 5000, 8000, 8000},  // payout returned
		{"loss", 5000, 0, 0},       // nothing returned (stake was reserved)
		{"push", 5000, 5000, 5000}, // original stake refunded
	}

	for _, tt := range tests {
		t.Run(tt.result, func(t *testing.T) {
			got := SettlementLedgerAmount(tt.result, tt.stake, tt.payout)
			if got != tt.want {
				t.Fatalf("SettlementLedgerAmount(%q, %d, %d) = %d, want %d", tt.result, tt.stake, tt.payout, got, tt.want)
			}
		})
	}
}
