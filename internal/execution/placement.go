package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"betbot/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlaceInput contains everything needed to place a bet.
type PlaceInput struct {
	IdempotencyKey    string
	SnapshotID        int64
	GameID            int64
	Sport             string
	MarketKey         string
	RecommendedSide   string
	BookKey           string
	AmericanOdds      int
	StakeCents        int64
	ModelProbability  float64
	MarketProbability float64
	Edge              float64
}

// PlaceResult is the outcome of a placement attempt.
type PlaceResult struct {
	BetID         int64
	ExternalBetID string
	AlreadyExists bool
}

// PlacementOrchestrator coordinates the exactly-once bet placement flow.
type PlacementOrchestrator struct {
	pool    *pgxpool.Pool
	adapter BookAdapter
}

// NewPlacementOrchestrator creates a new orchestrator with the given pool and adapter.
func NewPlacementOrchestrator(pool *pgxpool.Pool, adapter BookAdapter) *PlacementOrchestrator {
	return &PlacementOrchestrator{pool: pool, adapter: adapter}
}

// Place executes the two-transaction placement protocol:
//  1. Check idempotency — if bet exists and is not failed, return it.
//  2. Tx1: insert pending bet + reserve stake in bankroll ledger.
//  3. Call adapter.PlaceBet (outside transaction — network call).
//  4. Tx2 (success): update bet to placed with external_bet_id.
//  5. Tx2 (failure): update bet to failed + release stake in bankroll ledger.
func (o *PlacementOrchestrator) Place(ctx context.Context, input PlaceInput) (PlaceResult, error) {
	queries := store.New(o.pool)

	// Step 1: idempotency check
	existing, err := queries.GetBetByIdempotencyKey(ctx, input.IdempotencyKey)
	if err == nil && existing.Status != store.BetStatusFailed {
		extID := ""
		if existing.ExternalBetID != nil {
			extID = *existing.ExternalBetID
		}
		return PlaceResult{
			BetID:         existing.ID,
			ExternalBetID: extID,
			AlreadyExists: true,
		}, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PlaceResult{}, fmt.Errorf("check idempotency key: %w", err)
	}

	// Step 2: Tx1 — insert pending bet + reserve stake
	tx1, err := o.pool.Begin(ctx)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("begin placement tx: %w", err)
	}
	defer tx1.Rollback(ctx) //nolint:errcheck

	txQueries := store.New(tx1)

	bet, err := txQueries.InsertBet(ctx, store.InsertBetParams{
		IdempotencyKey:    input.IdempotencyKey,
		SnapshotID:        input.SnapshotID,
		GameID:            input.GameID,
		Sport:             input.Sport,
		MarketKey:         input.MarketKey,
		RecommendedSide:   input.RecommendedSide,
		BookKey:           input.BookKey,
		AmericanOdds:      int32(input.AmericanOdds),
		StakeCents:        input.StakeCents,
		ModelProbability:  input.ModelProbability,
		MarketProbability: input.MarketProbability,
		Edge:              input.Edge,
		AdapterName:       o.adapter.Name(),
		Metadata:          json.RawMessage(`{}`),
	})
	if err != nil {
		return PlaceResult{}, fmt.Errorf("insert pending bet: %w", err)
	}

	_, err = txQueries.InsertBankrollEntry(ctx, store.InsertBankrollEntryParams{
		EntryType:     "bet_stake_reserved",
		AmountCents:   -input.StakeCents,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   input.IdempotencyKey,
		Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d}`, bet.ID)),
	})
	if err != nil {
		return PlaceResult{}, fmt.Errorf("reserve stake in ledger: %w", err)
	}

	if err := tx1.Commit(ctx); err != nil {
		return PlaceResult{}, fmt.Errorf("commit placement tx: %w", err)
	}

	// Step 3: call adapter (outside transaction — network call)
	confirmation, placementErr := o.adapter.PlaceBet(ctx, PlaceBetRequest{
		IdempotencyKey: input.IdempotencyKey,
		GameID:         input.GameID,
		Sport:          input.Sport,
		MarketKey:      input.MarketKey,
		Side:           input.RecommendedSide,
		AmericanOdds:   input.AmericanOdds,
		StakeCents:     input.StakeCents,
	})

	// Step 4/5: Tx2 — update based on adapter result
	tx2, err := o.pool.Begin(ctx)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("begin result tx: %w", err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck

	tx2Queries := store.New(tx2)

	if placementErr != nil {
		// Step 5: failed — mark bet failed + release stake
		errMsg := placementErr.Error()
		if err := tx2Queries.UpdateBetFailed(ctx, store.UpdateBetFailedParams{
			ID:           bet.ID,
			ErrorMessage: &errMsg,
		}); err != nil {
			return PlaceResult{}, fmt.Errorf("update bet failed: %w", err)
		}

		if _, err := tx2Queries.InsertBankrollEntry(ctx, store.InsertBankrollEntryParams{
			EntryType:     "bet_stake_released",
			AmountCents:   input.StakeCents,
			Currency:      "USD",
			ReferenceType: "bet",
			ReferenceID:   input.IdempotencyKey,
			Metadata:      json.RawMessage(fmt.Sprintf(`{"bet_id":%d,"reason":"placement_failed"}`, bet.ID)),
		}); err != nil {
			return PlaceResult{}, fmt.Errorf("release stake in ledger: %w", err)
		}

		if err := tx2.Commit(ctx); err != nil {
			return PlaceResult{}, fmt.Errorf("commit failure tx: %w", err)
		}

		return PlaceResult{}, fmt.Errorf("adapter placement failed: %w", placementErr)
	}

	// Step 4: success — mark bet placed
	if err := tx2Queries.UpdateBetPlaced(ctx, store.UpdateBetPlacedParams{
		ID:            bet.ID,
		ExternalBetID: &confirmation.ExternalBetID,
	}); err != nil {
		return PlaceResult{}, fmt.Errorf("update bet placed: %w", err)
	}

	if err := tx2.Commit(ctx); err != nil {
		return PlaceResult{}, fmt.Errorf("commit success tx: %w", err)
	}

	return PlaceResult{
		BetID:         bet.ID,
		ExternalBetID: confirmation.ExternalBetID,
	}, nil
}
