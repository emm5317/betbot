package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"
	"betbot/internal/domain"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultRecommendationsLimit = 20
	maxRecommendationsLimit     = 200
)

func (a *App) handleRecommendations(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	if filterErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": filterErr.Error()})
	}

	dateFilter, err := parseRecommendationsDate(c.Query("date"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
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

	candidates, err := a.recommendationCandidates(c.Context(), sportFilter, dateFilter, limit)
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
	}); err != nil {
		return fmt.Errorf("persist recommendation snapshots: %w", err)
	}

	return c.JSON(recommendations)
}

func (a *App) recommendationCandidates(ctx context.Context, sportFilter sportFilterSelection, dateFilter *time.Time, limit int) ([]decision.RecommendationCandidate, error) {
	sports := recommendationSportsForFilter(sportFilter)

	seasonYear := int32(time.Now().UTC().Year())
	if dateFilter != nil {
		seasonYear = int32(dateFilter.UTC().Year())
	}
	season := &seasonYear

	predictionsByKey := make(map[string]store.ModelPrediction)
	for _, sport := range sports {
		rows, err := a.queries.ListModelPredictionsForSportSeason(ctx, store.ListModelPredictionsForSportSeasonParams{
			Sport:  sport,
			Season: season,
		})
		if err != nil {
			return nil, fmt.Errorf("list model predictions for %s: %w", sport, err)
		}
		for _, row := range rows {
			if !row.EventTime.Valid {
				continue
			}
			if dateFilter != nil && !sameUTCDate(row.EventTime.Time, *dateFilter) {
				continue
			}
			key := recommendationKey(row.GameID, row.MarketKey)
			existing, ok := predictionsByKey[key]
			if !ok || row.EventTime.Time.After(existing.EventTime.Time) || row.ID > existing.ID {
				predictionsByKey[key] = row
			}
		}
	}

	oddsRows, err := a.queries.ListLatestOddsForUpcoming(ctx, store.ListLatestOddsForUpcomingParams{
		RowLimit: recommendationOddsRowLimit(limit),
		Sport:    sportFilter.storeParam(),
	})
	if err != nil {
		return nil, fmt.Errorf("list latest odds: %w", err)
	}

	quotesByMarket := latestQuotesByGameAndMarketUpcoming(oddsRows)
	candidates := make([]decision.RecommendationCandidate, 0, len(predictionsByKey))
	for _, prediction := range predictionsByKey {
		sport := domain.Sport(prediction.Sport)
		if _, err := decision.DefaultEVThresholdPolicy(sport); err != nil {
			continue
		}

		quotes := quotesByMarket[recommendationKey(prediction.GameID, prediction.MarketKey)]
		if len(quotes) == 0 {
			continue
		}

		candidates = append(candidates, decision.RecommendationCandidate{
			Sport:                 sport,
			GameID:                prediction.GameID,
			Market:                prediction.MarketKey,
			EventTime:             prediction.EventTime.Time.UTC(),
			ModelHomeProbability:  prediction.PredictedProbability,
			MarketHomeProbability: prediction.MarketProbability,
			Quotes:                quotes,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.GameID != right.GameID {
			return left.GameID < right.GameID
		}
		if left.Market != right.Market {
			return left.Market < right.Market
		}
		return left.Sport < right.Sport
	})

	return candidates, nil
}

func (a *App) persistRecommendationSnapshots(ctx context.Context, recommendations []decision.Recommendation, circuitMetrics decision.CircuitBreakerMetrics) error {
	baseMetadata := map[string]any{
		"mode":         "recommendation-only",
		"ev_threshold": a.cfg.EVThreshold,
	}
	if a.cfg.KellyFraction > 0 {
		baseMetadata["kelly_fraction_override"] = a.cfg.KellyFraction
	}
	if a.cfg.MaxBetFraction > 0 {
		baseMetadata["max_bet_fraction_override"] = a.cfg.MaxBetFraction
	}

	for i := range recommendations {
		rec := recommendations[i]
		metadata := make(map[string]any, len(baseMetadata)+1)
		for key, value := range baseMetadata {
			metadata[key] = value
		}
		metadata["sizing"] = map[string]any{
			"raw_kelly_fraction":       rec.RawKellyFraction,
			"applied_fractional_kelly": rec.AppliedFractionalKelly,
			"capped_fraction":          rec.CappedFraction,
			"pre_bankroll_stake_cents": rec.PreBankrollStakeCents,
			"bankroll_available_cents": rec.BankrollAvailableCents,
			"bankroll_check_passed":    rec.BankrollCheckPass,
			"bankroll_check_reason":    rec.BankrollCheckReason,
			"reasons":                  append([]string(nil), rec.SizingReasons...),
		}
		metadata["correlation"] = map[string]any{
			"check_passed": rec.CorrelationCheckPass,
			"check_reason": rec.CorrelationCheckReason,
			"group_key":    rec.CorrelationGroupKey,
		}
		metadata["circuit"] = map[string]any{
			"check_passed":             rec.CircuitCheckPass,
			"check_reason":             rec.CircuitCheckReason,
			"daily_loss_stop":          a.cfg.DailyLossStop,
			"weekly_loss_stop":         a.cfg.WeeklyLossStop,
			"drawdown_breaker":         a.cfg.DrawdownBreaker,
			"current_balance_cents":    circuitMetrics.CurrentBalanceCents,
			"day_start_balance_cents":  circuitMetrics.DayStartBalanceCents,
			"week_start_balance_cents": circuitMetrics.WeekStartBalanceCents,
			"peak_balance_cents":       circuitMetrics.PeakBalanceCents,
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal recommendation metadata game_id=%d market=%s: %w", rec.GameID, rec.Market, err)
		}

		eventDate := rec.EventTime.UTC()
		if _, err := a.queries.InsertRecommendationSnapshot(ctx, store.InsertRecommendationSnapshotParams{
			GeneratedAt:            store.Timestamptz(rec.GeneratedAt),
			Sport:                  string(rec.Sport),
			GameID:                 rec.GameID,
			EventTime:              store.Timestamptz(rec.EventTime),
			EventDate:              pgtype.Date{Time: time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:              rec.Market,
			RecommendedSide:        rec.RecommendedSide,
			BestBook:               rec.BestBook,
			BestAmericanOdds:       int32(rec.BestAmericanOdds),
			ModelProbability:       rec.ModelProbability,
			MarketProbability:      rec.MarketProbability,
			Edge:                   rec.Edge,
			SuggestedStakeFraction: rec.SuggestedStakeFraction,
			SuggestedStakeCents:    rec.SuggestedStakeCents,
			BankrollCheckPass:      rec.BankrollCheckPass,
			BankrollCheckReason:    rec.BankrollCheckReason,
			RankScore:              rec.RankScore,
			Metadata:               metadataJSON,
		}); err != nil {
			return fmt.Errorf("insert recommendation snapshot for game_id=%d market=%s: %w", rec.GameID, rec.Market, err)
		}
	}

	return nil
}

func parseRecommendationsDate(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	value, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q; expected YYYY-MM-DD", raw)
	}
	utc := value.UTC()
	return &utc, nil
}

func parseRecommendationsLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRecommendationsLimit, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsLimit)
	}
	if value < 1 || value > maxRecommendationsLimit {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsLimit)
	}
	return value, nil
}

func recommendationSportsForFilter(sportFilter sportFilterSelection) []string {
	if sportFilter.DBSport != "" {
		return []string{sportFilter.DBSport}
	}
	return []string{"MLB", "NBA", "NHL", "NFL"}
}

func recommendationKey(gameID int64, market string) string {
	return fmt.Sprintf("%d|%s", gameID, market)
}

func recommendationOddsRowLimit(limit int) int32 {
	rowLimit := limit * 120
	if rowLimit < 500 {
		rowLimit = 500
	}
	if rowLimit > 10000 {
		rowLimit = 10000
	}
	return int32(rowLimit)
}

func latestQuotesByGameAndMarket(rows []store.ListLatestOddsRow) map[string][]decision.BookQuote {
	type marketKey struct {
		gameID int64
		market string
	}
	type quoteAccumulator struct {
		book    string
		home    int
		away    int
		hasHome bool
		hasAway bool
	}
	type bookKey struct {
		market marketKey
		book   string
	}

	byBook := make(map[bookKey]*quoteAccumulator)
	for _, row := range rows {
		side := strings.ToLower(strings.TrimSpace(row.OutcomeSide))
		if side != "home" && side != "away" {
			continue
		}

		book := strings.TrimSpace(row.BookName)
		if book == "" {
			book = strings.TrimSpace(row.BookKey)
		}
		if book == "" {
			continue
		}

		key := bookKey{
			market: marketKey{gameID: row.GameID, market: row.MarketKey},
			book:   book,
		}
		acc, ok := byBook[key]
		if !ok {
			acc = &quoteAccumulator{book: book}
			byBook[key] = acc
		}

		price := int(row.PriceAmerican)
		if side == "home" {
			acc.home = price
			acc.hasHome = true
		} else {
			acc.away = price
			acc.hasAway = true
		}
	}

	quotesByMarket := make(map[string][]decision.BookQuote)
	for key, acc := range byBook {
		if !acc.hasHome || !acc.hasAway {
			continue
		}
		groupKey := recommendationKey(key.market.gameID, key.market.market)
		quotesByMarket[groupKey] = append(quotesByMarket[groupKey], decision.BookQuote{
			Book:         acc.book,
			HomeAmerican: acc.home,
			AwayAmerican: acc.away,
		})
	}

	for marketKey := range quotesByMarket {
		sort.SliceStable(quotesByMarket[marketKey], func(i, j int) bool {
			return quotesByMarket[marketKey][i].Book < quotesByMarket[marketKey][j].Book
		})
	}

	return quotesByMarket
}

func latestQuotesByGameAndMarketUpcoming(rows []store.ListLatestOddsForUpcomingRow) map[string][]decision.BookQuote {
	type marketKey struct {
		gameID int64
		market string
	}
	type quoteAccumulator struct {
		book    string
		home    int
		away    int
		hasHome bool
		hasAway bool
	}
	type bookKey struct {
		market marketKey
		book   string
	}

	byBook := make(map[bookKey]*quoteAccumulator)
	for _, row := range rows {
		side := strings.ToLower(strings.TrimSpace(row.OutcomeSide))
		if side != "home" && side != "away" {
			continue
		}

		book := strings.TrimSpace(row.BookName)
		if book == "" {
			book = strings.TrimSpace(row.BookKey)
		}
		if book == "" {
			continue
		}

		key := bookKey{
			market: marketKey{gameID: row.GameID, market: row.MarketKey},
			book:   book,
		}
		acc, ok := byBook[key]
		if !ok {
			acc = &quoteAccumulator{book: book}
			byBook[key] = acc
		}

		price := int(row.PriceAmerican)
		if side == "home" {
			acc.home = price
			acc.hasHome = true
		} else {
			acc.away = price
			acc.hasAway = true
		}
	}

	quotesByMarket := make(map[string][]decision.BookQuote)
	for key, acc := range byBook {
		if !acc.hasHome || !acc.hasAway {
			continue
		}
		groupKey := recommendationKey(key.market.gameID, key.market.market)
		quotesByMarket[groupKey] = append(quotesByMarket[groupKey], decision.BookQuote{
			Book:         acc.book,
			HomeAmerican: acc.home,
			AwayAmerican: acc.away,
		})
	}

	for mk := range quotesByMarket {
		sort.SliceStable(quotesByMarket[mk], func(i, j int) bool {
			return quotesByMarket[mk][i].Book < quotesByMarket[mk][j].Book
		})
	}

	return quotesByMarket
}

func sameUTCDate(left time.Time, right time.Time) bool {
	leftUTC := left.UTC()
	rightUTC := right.UTC()
	return leftUTC.Year() == rightUTC.Year() &&
		leftUTC.Month() == rightUTC.Month() &&
		leftUTC.Day() == rightUTC.Day()
}
