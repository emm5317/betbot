package server

import (
	"betbot/internal/execution"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
)

type executionPlaceRequest struct {
	SnapshotID int64 `json:"snapshot_id"`
}

func (a *App) handleExecutionPlace(c fiber.Ctx) error {
	var req executionPlaceRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}
	if req.SnapshotID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "snapshot_id must be > 0",
		})
	}

	// Look up the snapshot to build the placement input
	snapshot, err := a.queries.GetRecommendationSnapshotByID(c.Context(), req.SnapshotID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "snapshot not found",
		})
	}

	idempotencyKey := execution.GenerateIdempotencyKey(
		snapshot.GameID, snapshot.MarketKey, snapshot.BestBook, snapshot.GeneratedAt.Time,
	)

	result, err := a.placementOrchestrator.Place(c.Context(), execution.PlaceInput{
		IdempotencyKey:    idempotencyKey,
		SnapshotID:        snapshot.ID,
		GameID:            snapshot.GameID,
		Sport:             snapshot.Sport,
		MarketKey:         snapshot.MarketKey,
		RecommendedSide:   snapshot.RecommendedSide,
		BookKey:           snapshot.BestBook,
		AmericanOdds:      int(snapshot.BestAmericanOdds),
		StakeCents:        snapshot.SuggestedStakeCents,
		ModelProbability:  snapshot.ModelProbability,
		MarketProbability: snapshot.MarketProbability,
		Edge:              snapshot.Edge,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	status := fiber.StatusCreated
	if result.AlreadyExists {
		status = fiber.StatusOK
	}

	return c.Status(status).JSON(fiber.Map{
		"bet_id":          result.BetID,
		"external_bet_id": result.ExternalBetID,
		"already_exists":  result.AlreadyExists,
	})
}

func (a *App) handleExecutionBets(c fiber.Ctx) error {
	statusFilter := c.Query("status")

	var bets []store.Bet
	var err error

	if statusFilter != "" {
		bets, err = a.queries.ListBetsByStatus(c.Context(), store.ListBetsByStatusParams{
			Status:   store.BetStatus(statusFilter),
			RowLimit: 100,
		})
	} else {
		bets, err = a.queries.ListOpenBets(c.Context())
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"bets":  bets,
		"count": len(bets),
	})
}
