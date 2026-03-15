package paper

import (
	"context"
	"fmt"
	"time"

	"betbot/internal/execution"
)

// Adapter is the paper-mode BookAdapter. It returns synthetic confirmations
// without making any real HTTP calls. All other pipeline steps (ledger writes,
// bet table inserts) still execute normally.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "paper" }

func (a *Adapter) PlaceBet(_ context.Context, req execution.PlaceBetRequest) (execution.PlacementConfirmation, error) {
	return execution.PlacementConfirmation{
		ExternalBetID: fmt.Sprintf("paper-%s", req.IdempotencyKey),
		AcceptedOdds:  req.AmericanOdds,
		StakeCents:    req.StakeCents,
		ConfirmedAt:   time.Now().UTC(),
	}, nil
}

func (a *Adapter) GetBetStatus(_ context.Context, externalBetID string) (execution.BetStatusResponse, error) {
	return execution.BetStatusResponse{
		ExternalBetID: externalBetID,
		Status:        "accepted",
	}, nil
}
