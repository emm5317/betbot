package paper

import (
	"context"
	"strings"
	"testing"

	"betbot/internal/execution"
)

func TestPaperAdapterName(t *testing.T) {
	adapter := New()
	if adapter.Name() != "paper" {
		t.Fatalf("expected name 'paper', got %q", adapter.Name())
	}
}

func TestPaperAdapterPlaceBet(t *testing.T) {
	adapter := New()
	ctx := context.Background()

	req := execution.PlaceBetRequest{
		IdempotencyKey: "42:h2h:paper:1710510000",
		GameID:         42,
		Sport:          "NHL",
		MarketKey:      "h2h",
		Side:           "home",
		AmericanOdds:   -150,
		StakeCents:     5000,
	}

	conf, err := adapter.PlaceBet(ctx, req)
	if err != nil {
		t.Fatalf("PlaceBet error: %v", err)
	}

	if !strings.HasPrefix(conf.ExternalBetID, "paper-") {
		t.Fatalf("expected paper- prefix, got %q", conf.ExternalBetID)
	}
	if conf.AcceptedOdds != req.AmericanOdds {
		t.Fatalf("expected odds %d, got %d", req.AmericanOdds, conf.AcceptedOdds)
	}
	if conf.StakeCents != req.StakeCents {
		t.Fatalf("expected stake %d, got %d", req.StakeCents, conf.StakeCents)
	}
	if conf.ConfirmedAt.IsZero() {
		t.Fatal("expected non-zero ConfirmedAt")
	}
}

func TestPaperAdapterGetBetStatus(t *testing.T) {
	adapter := New()
	ctx := context.Background()

	resp, err := adapter.GetBetStatus(ctx, "paper-test-123")
	if err != nil {
		t.Fatalf("GetBetStatus error: %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected status 'accepted', got %q", resp.Status)
	}
}
