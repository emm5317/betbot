package execution

import (
	"context"
	"strings"
	"time"
)

const (
	AdapterPaper      = "paper"
	AdapterDraftKings = "draftkings"
	AdapterFanDuel    = "fanduel"
	AdapterBetMGM     = "betmgm"
	AdapterPinnacle   = "pinnacle"
)

// BookAdapter is the interface that all sportsbook adapters must implement.
// Defined where consumed (placement orchestrator), not where implemented.
type BookAdapter interface {
	// PlaceBet submits a bet to the book and returns a confirmation.
	PlaceBet(ctx context.Context, req PlaceBetRequest) (PlacementConfirmation, error)

	// GetBetStatus queries the book for the current status of a placed bet.
	GetBetStatus(ctx context.Context, externalBetID string) (BetStatusResponse, error)

	// Name returns the adapter identifier (e.g. "paper", "pinnacle").
	Name() string
}

// PlaceBetRequest contains all information needed to place a bet with a book.
type PlaceBetRequest struct {
	IdempotencyKey string
	GameID         int64
	Sport          string
	MarketKey      string
	Side           string // "home" or "away"
	AmericanOdds   int
	StakeCents     int64
}

// PlacementConfirmation is returned by a BookAdapter after a successful placement.
type PlacementConfirmation struct {
	ExternalBetID string
	AcceptedOdds  int
	StakeCents    int64
	ConfirmedAt   time.Time
}

// BetStatusResponse is returned by GetBetStatus.
type BetStatusResponse struct {
	ExternalBetID string
	Status        string // "pending", "accepted", "settled", "cancelled"
	Result        string // "win", "loss", "push" — only set when settled
	PayoutCents   int64  // only set when settled
}

func NormalizeAdapterName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func IsSupportedAdapter(name string) bool {
	switch NormalizeAdapterName(name) {
	case AdapterPaper, AdapterDraftKings, AdapterFanDuel, AdapterBetMGM, AdapterPinnacle:
		return true
	default:
		return false
	}
}
