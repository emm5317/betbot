package server

import (
	"time"

	"github.com/gofiber/fiber/v3"
)

func (a *App) handlePredictionsRun(c fiber.Ctx) error {
	started := time.Now()

	predicted, err := a.nhlPredictionService.PredictUpcomingGames(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(map[string]any{
			"error":    err.Error(),
			"duration": time.Since(started).String(),
		})
	}

	return c.JSON(map[string]any{
		"sport":            "NHL",
		"predictions_made": predicted,
		"duration":         time.Since(started).String(),
	})
}
