package server

import (
	"fmt"

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

func (a *App) handlePartialPlaceBet(c fiber.Ctx) error {
	snapshotIDStr := c.FormValue("snapshot_id")
	if snapshotIDStr == "" {
		c.Set("HX-Trigger", `{"show-toast": {"type": "error", "message": "Missing snapshot ID"}}`)
		return c.SendStatus(fiber.StatusBadRequest)
	}

	var snapshotID int64
	if _, err := fmt.Sscanf(snapshotIDStr, "%d", &snapshotID); err != nil || snapshotID <= 0 {
		c.Set("HX-Trigger", `{"show-toast": {"type": "error", "message": "Invalid snapshot ID"}}`)
		return c.SendStatus(fiber.StatusBadRequest)
	}

	snapshot, err := a.queries.GetRecommendationSnapshotByID(c.Context(), snapshotID)
	if err != nil {
		c.Set("HX-Trigger", `{"show-toast": {"type": "error", "message": "Snapshot not found"}}`)
		return c.SendStatus(fiber.StatusNotFound)
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
		msg := fmt.Sprintf("Placement failed: %s", err.Error())
		c.Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"type": "error", "message": %q}}`, msg))
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	msg := fmt.Sprintf("Bet #%d placed", result.BetID)
	if result.AlreadyExists {
		msg = fmt.Sprintf("Bet #%d already exists", result.BetID)
	}
	c.Set("HX-Trigger", fmt.Sprintf(`{"show-toast": {"type": "success", "message": %q}, "refreshBets": true}`, msg))
	return c.SendStatus(fiber.StatusOK)
}

func (a *App) handleExecutionBets(c fiber.Ctx) error {
	statusFilter := c.Query("status")

	var err error
	var result any

	if statusFilter != "" {
		var bets []store.ListBetsByStatusRow
		bets, err = a.queries.ListBetsByStatus(c.Context(), store.ListBetsByStatusParams{
			Status:   store.BetStatus(statusFilter),
			RowLimit: 100,
		})
		result = bets
	} else {
		var bets []store.ListOpenBetsRow
		bets, err = a.queries.ListOpenBets(c.Context())
		result = bets
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"bets":  result,
		"count": result,
	})
}
