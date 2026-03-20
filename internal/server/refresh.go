package server

import (
	"fmt"
	"time"

	"betbot/internal/decision"
	"betbot/internal/domain"

	"github.com/gofiber/fiber/v3"
)

func (a *App) handleRecommendationsRefresh(c fiber.Ctx) error {
	started := time.Now()
	result := map[string]any{
		"odds_poll":   map[string]any{"skipped": true, "reason": "odds polling disabled"},
		"predictions": map[string]any{},
	}

	// Phase 1: Poll odds.
	if a.oddsPoller != nil {
		registry := domain.DefaultSportRegistry()
		sports := registry.ActiveOddsAPISports(time.Now().UTC(), a.cfg.OddsAPISports)
		if len(sports) == 0 {
			result["odds_poll"] = map[string]any{"skipped": true, "reason": "no active sports in season"}
		} else {
			metrics, err := a.oddsPoller.Run(c.Context(), sports)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(map[string]any{
					"error":    fmt.Sprintf("odds poll failed: %s", err.Error()),
					"duration": time.Since(started).String(),
				})
			}
			result["odds_poll"] = map[string]any{
				"skipped":        false,
				"sports":         sports,
				"games_seen":     metrics.GamesSeen,
				"snapshots_seen": metrics.SnapshotsSeen,
				"inserts":        metrics.Inserts,
				"dedup_skips":    metrics.DedupSkips,
			}
		}
	}

	// Phase 2: Run predictions.
	nhlPredicted, err := a.nhlPredictionService.PredictUpcomingGames(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(map[string]any{
			"error":    fmt.Sprintf("NHL prediction failed: %s", err.Error()),
			"duration": time.Since(started).String(),
		})
	}
	result["predictions"] = map[string]any{
		"NHL": nhlPredicted,
	}

	// Phase 3: Build and return recommendations.
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	if filterErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": filterErr.Error()})
	}

	limit, err := parseRecommendationsLimit(c.Query("limit"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	circuitMetrics, err := a.queries.GetBankrollCircuitMetrics(c.Context())
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]any{
			"error": "bankroll balance unavailable",
		})
	}
	if circuitMetrics.CurrentBalanceCents < 0 {
		return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]any{
			"error": "bankroll balance unavailable",
		})
	}

	candidates, predictionsByKey, err := a.recommendationCandidates(c.Context(), sportFilter, nil, limit)
	if err != nil {
		return fmt.Errorf("load recommendation candidates: %w", err)
	}

	generatedAt := time.Now().UTC()
	recommendations, err := decision.BuildRecommendations(candidates, decision.RecommendationBuildConfig{
		EVThreshold:                          a.cfg.EVThreshold,
		KellyFraction:                        a.cfg.KellyFraction,
		MaxBetFraction:                       a.cfg.MaxBetFraction,
		CorrelationMaxPicksPerGame:           a.cfg.CorrelationMaxPicksPerGame,
		CorrelationMaxStakeFractionPerGame:   a.cfg.CorrelationMaxStakeFractionPerGame,
		CorrelationMaxPicksPerSportDayWindow: a.cfg.CorrelationMaxPicksPerSportDay,
		CircuitDailyLossStop:                 a.cfg.DailyLossStop,
		CircuitWeeklyLossStop:                a.cfg.WeeklyLossStop,
		CircuitDrawdownBreaker:               a.cfg.DrawdownBreaker,
		CircuitMetrics: decision.CircuitBreakerMetrics{
			CurrentBalanceCents:   circuitMetrics.CurrentBalanceCents,
			DayStartBalanceCents:  circuitMetrics.DayStartBalanceCents,
			WeekStartBalanceCents: circuitMetrics.WeekStartBalanceCents,
			PeakBalanceCents:      circuitMetrics.PeakBalanceCents,
		},
		AvailableBankrollCents: circuitMetrics.CurrentBalanceCents,
		GeneratedAt:            generatedAt,
	})
	if err != nil {
		return fmt.Errorf("build recommendations: %w", err)
	}

	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}
	if err := a.persistRecommendationSnapshots(c.Context(), recommendations, decision.CircuitBreakerMetrics{
		CurrentBalanceCents:   circuitMetrics.CurrentBalanceCents,
		DayStartBalanceCents:  circuitMetrics.DayStartBalanceCents,
		WeekStartBalanceCents: circuitMetrics.WeekStartBalanceCents,
		PeakBalanceCents:      circuitMetrics.PeakBalanceCents,
	}, predictionsByKey); err != nil {
		return fmt.Errorf("persist recommendation snapshots: %w", err)
	}

	result["recommendations"] = recommendations
	result["duration"] = time.Since(started).String()
	return c.JSON(result)
}
