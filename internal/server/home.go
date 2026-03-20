package server

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
)

const (
	homeRecommendationsLimit = 6
	homeOpenBetsLimit        = 6
	homeUpcomingGamesLimit   = 8
)

func (a *App) renderHome(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	pipeline, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Title":         "Game Day Hub",
		"ActiveNav":     "home",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"Pipeline":      pipeline,
		"Status":        overallStatus,
		"PageStatus":    overallStatus,
	}
	applySportFilterView(view, "/", sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("pages/home", view, "layouts/base")
	}

	a.populateHomeView(c.Context(), view, sportFilter)
	return c.Render("pages/home", view, "layouts/base")
}

func (a *App) handlePartialHomeRecommendations(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	view := map[string]any{}
	applySportFilterView(view, c.Path(), sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("partials/fragment_error", view)
	}

	games, _ := a.loadUpcomingGames(c.Context(), sportFilter, homeUpcomingGamesLimit)
	recommendations, recErr := a.buildLiveRecommendations(c.Context(), sportFilter, homeRecommendationsLimit)
	view["Recommendations"] = mapRecommendationCards(recommendations, gameLookupByID(games))
	view["RecommendationsError"] = homeSectionError(recErr)
	view["RecommendationsCount"] = len(recommendations)
	return c.Render("partials/home_recommendations_block", view)
}

func (a *App) handlePartialHomeOpenBets(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	view := map[string]any{}
	applySportFilterView(view, c.Path(), sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("partials/fragment_error", view)
	}

	openBets, totalExposure, openBetErr := a.loadOpenBets(c.Context(), sportFilter, homeOpenBetsLimit)
	view["OpenBets"] = mapOpenBetCards(openBets)
	view["OpenBetExposure"] = formatCents(totalExposure)
	view["OpenBetsError"] = homeSectionError(openBetErr)
	view["OpenBetsCount"] = len(openBets)
	return c.Render("partials/home_open_bets_block", view)
}

func (a *App) populateHomeView(ctx context.Context, view map[string]any, sportFilter sportFilterSelection) {
	games, upcomingErr := a.loadUpcomingGames(ctx, sportFilter, homeUpcomingGamesLimit)
	gameByID := gameLookupByID(games)

	recommendations, recommendationsErr := a.buildLiveRecommendations(ctx, sportFilter, homeRecommendationsLimit)
	recommendationCards := mapRecommendationCards(recommendations, gameByID)
	recommendationGameIDs := make(map[int64]struct{}, len(recommendations))
	for _, lr := range recommendations {
		recommendationGameIDs[lr.GameID] = struct{}{}
	}

	openBets, totalExposure, openBetsErr := a.loadOpenBets(ctx, sportFilter, homeOpenBetsLimit)
	openBetCards := mapOpenBetCards(openBets)
	openBetGameIDs := make(map[int64]struct{}, len(openBets))
	for _, bet := range openBets {
		openBetGameIDs[bet.GameID] = struct{}{}
	}

	balanceText := "Unavailable"
	balanceErr := error(nil)
	if balance, err := a.queries.GetBankrollBalanceCents(ctx); err != nil {
		balanceErr = err
	} else {
		balanceText = formatCents(balance)
	}

	pnlSummary, pnlErr := a.queries.GetBetPnLSummary(ctx, sportFilter.DBSport)
	if pnlErr != nil {
		pnlSummary = store.GetBetPnLSummaryRow{}
	}

	view["HeroTitle"] = "Game-day decisions without admin-tool drag"
	view["HeroLede"] = "Recommendations, bankroll posture, open positions, and near-term games in one calm surface."
	view["HeroRecommendationCount"] = len(recommendationCards)
	view["HeroOpenBetsCount"] = pnlSummary.OpenBets
	view["HeroExposure"] = formatCents(totalExposure)
	view["HeroBalance"] = balanceText
	view["BalanceUnavailable"] = balanceErr != nil

	view["BankrollBalance"] = balanceText
	view["BankrollOpenExposure"] = formatCents(totalExposure)
	view["BankrollNetPnL"] = formatCentsSigned(int64(pnlSummary.NetPnlCents))
	view["BankrollOpenBets"] = pnlSummary.OpenBets
	view["BankrollSettledBets"] = pnlSummary.SettledBets
	view["BankrollPendingCount"] = len(openBetCards)
	view["BankrollError"] = firstNonNilError(balanceErr, pnlErr)

	view["Recommendations"] = recommendationCards
	view["RecommendationsCount"] = len(recommendationCards)
	view["RecommendationsError"] = homeSectionError(recommendationsErr)
	view["OpenBets"] = openBetCards
	view["OpenBetsCount"] = len(openBetCards)
	view["OpenBetExposure"] = formatCents(totalExposure)
	view["OpenBetsError"] = homeSectionError(openBetsErr)
	view["UpcomingGames"] = mapUpcomingGameCards(games, recommendationGameIDs, openBetGameIDs)
	view["UpcomingGamesError"] = homeSectionError(upcomingErr)
}

type liveRecommendation struct {
	decision.Recommendation
	SnapshotID int64
}

func (a *App) buildLiveRecommendations(ctx context.Context, sportFilter sportFilterSelection, limit int) ([]liveRecommendation, error) {
	circuitMetrics, err := a.queries.GetBankrollCircuitMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("bankroll unavailable: %w", err)
	}
	if circuitMetrics.CurrentBalanceCents < 0 {
		return nil, fmt.Errorf("bankroll unavailable")
	}

	candidates, predictionsByKey, err := a.recommendationCandidates(ctx, sportFilter, nil, limit)
	if err != nil {
		return nil, fmt.Errorf("load recommendation candidates: %w", err)
	}

	cbMetrics := decision.CircuitBreakerMetrics{
		CurrentBalanceCents:   circuitMetrics.CurrentBalanceCents,
		DayStartBalanceCents:  circuitMetrics.DayStartBalanceCents,
		WeekStartBalanceCents: circuitMetrics.WeekStartBalanceCents,
		PeakBalanceCents:      circuitMetrics.PeakBalanceCents,
	}

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
		CircuitMetrics:                       cbMetrics,
		AvailableBankrollCents:               circuitMetrics.CurrentBalanceCents,
		GeneratedAt:                          time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("build recommendations: %w", err)
	}
	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}

	snapshotIDs, err := a.persistRecommendationSnapshotsWithIDs(ctx, recommendations, cbMetrics, predictionsByKey)
	if err != nil {
		return nil, fmt.Errorf("persist recommendation snapshots: %w", err)
	}

	result := make([]liveRecommendation, len(recommendations))
	for i, rec := range recommendations {
		result[i] = liveRecommendation{
			Recommendation: rec,
			SnapshotID:     snapshotIDs[i],
		}
	}
	return result, nil
}

func (a *App) loadOpenBets(ctx context.Context, sportFilter sportFilterSelection, limit int) ([]store.ListOpenBetsWithGameRow, int64, error) {
	rows, err := a.queries.ListOpenBetsWithGame(ctx)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]store.ListOpenBetsWithGameRow, 0, len(rows))
	var totalExposure int64
	for _, row := range rows {
		if sportFilter.DBSport != "" && !strings.EqualFold(row.GameSport, sportFilter.DBSport) && !strings.EqualFold(row.Sport, sportFilter.DBSport) {
			continue
		}
		totalExposure += row.StakeCents
		filtered = append(filtered, row)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := filtered[i]
		right := filtered[j]
		if left.CreatedAt.Valid && right.CreatedAt.Valid && !left.CreatedAt.Time.Equal(right.CreatedAt.Time) {
			return left.CreatedAt.Time.After(right.CreatedAt.Time)
		}
		return left.ID > right.ID
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, totalExposure, nil
}

func (a *App) loadUpcomingGames(ctx context.Context, sportFilter sportFilterSelection, limit int) ([]store.Game, error) {
	rows, err := a.queries.ListUpcomingGames(ctx, int32(limit*2))
	if err != nil {
		return nil, err
	}
	filtered := make([]store.Game, 0, len(rows))
	for _, row := range rows {
		if sportFilter.DBSport != "" && !strings.EqualFold(row.Sport, sportFilter.DBSport) {
			continue
		}
		filtered = append(filtered, row)
		if len(filtered) == limit {
			break
		}
	}
	return filtered, nil
}

func gameLookupByID(games []store.Game) map[int64]store.Game {
	lookup := make(map[int64]store.Game, len(games))
	for _, game := range games {
		lookup[game.ID] = game
	}
	return lookup
}

func mapRecommendationCards(recommendations []liveRecommendation, gameByID map[int64]store.Game) []map[string]any {
	cards := make([]map[string]any, 0, len(recommendations))
	for _, lr := range recommendations {
		rec := lr.Recommendation
		game, ok := gameByID[rec.GameID]
		matchup := fmt.Sprintf("Game #%d", rec.GameID)
		if ok {
			matchup = fmt.Sprintf("%s vs %s", game.HomeTeam, game.AwayTeam)
		}
		cards = append(cards, map[string]any{
			"GameID":              rec.GameID,
			"SnapshotID":         lr.SnapshotID,
			"Sport":               strings.ToUpper(string(rec.Sport)),
			"Matchup":             matchup,
			"HomeTeam":            game.HomeTeam,
			"AwayTeam":            game.AwayTeam,
			"Market":              marketLabel(rec.Market),
			"MarketKey":           rec.Market,
			"RecommendedSide":     recommendationSideLabel(rec.RecommendedSide),
			"BestBook":            rec.BestBook,
			"BestAmericanOdds":    formatAmericanOdds(int32(rec.BestAmericanOdds)),
			"ModelProbability":    formatPercent(rec.ModelProbability),
			"MarketProbability":   formatPercent(rec.MarketProbability),
			"Edge":                formatPercent(rec.Edge),
			"SuggestedStake":      formatCents(rec.SuggestedStakeCents),
			"SuggestedStakeCents": rec.SuggestedStakeCents,
			"StakeFraction":       formatPercent(rec.SuggestedStakeFraction),
			"EventTime":           rec.EventTime.UTC().Format(time.RFC3339),
			"SizingReasons":       strings.Join(rec.SizingReasons, " • "),
			"BookNote":            bankrollReasonLabel(rec.BankrollCheckReason),
			"CanRecord":           rec.SuggestedStakeCents > 0,
			"CanPlace":            lr.SnapshotID > 0 && rec.SuggestedStakeCents > 0,
			"PrefillURL":          recommendationPrefillURL(rec, game),
		})
	}
	return cards
}

func mapOpenBetCards(rows []store.ListOpenBetsWithGameRow) []map[string]any {
	nowUTC := time.Now().UTC()
	cards := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		state := "Queued"
		tone := "muted"
		startAt := "pending"
		if row.PlacedAt.Valid {
			startAt = row.PlacedAt.Time.UTC().Format(time.RFC3339)
		}
		if row.CreatedAt.Valid {
			startAt = row.CreatedAt.Time.UTC().Format(time.RFC3339)
		}
		if row.CreatedAt.Valid && row.CreatedAt.Time.UTC().After(nowUTC.Add(-2*time.Hour)) {
			state = "Fresh"
			tone = "ok"
		}
		if row.CreatedAt.Valid && row.CreatedAt.Time.UTC().Before(nowUTC.Add(-24*time.Hour)) {
			state = "Aging"
			tone = "warn"
		}
		if row.PlacedAt.Valid && row.PlacedAt.Time.UTC().Before(nowUTC.Add(-4*time.Hour)) {
			state = "Working"
			tone = "warn"
		}
		cards = append(cards, map[string]any{
			"ID":           row.ID,
			"Sport":        strings.ToUpper(row.GameSport),
			"Matchup":      fmt.Sprintf("%s vs %s", row.HomeTeam, row.AwayTeam),
			"Side":         recommendationSideLabel(row.RecommendedSide),
			"BookKey":      row.BookKey,
			"AmericanOdds": formatAmericanOdds(row.AmericanOdds),
			"Stake":        formatCents(row.StakeCents),
			"PlacedAt":     startAt,
			"Exposure":     formatCents(row.StakeCents),
			"Source":       openBetSourceLabel(row),
			"SnapshotID":   optionalSnapshotID(row.SnapshotID),
			"State":        state,
			"StateTone":    tone,
		})
	}
	return cards
}

func mapUpcomingGameCards(games []store.Game, recommendationGameIDs map[int64]struct{}, openBetGameIDs map[int64]struct{}) []map[string]any {
	cards := make([]map[string]any, 0, len(games))
	for _, game := range games {
		reason := "Watch"
		tone := "muted"
		if _, ok := recommendationGameIDs[game.ID]; ok {
			reason = "Recommended"
			tone = "accent"
		} else if _, ok := openBetGameIDs[game.ID]; ok {
			reason = "Open bet"
			tone = "ok"
		}
		cards = append(cards, map[string]any{
			"Sport":        strings.ToUpper(game.Sport),
			"Matchup":      fmt.Sprintf("%s vs %s", game.HomeTeam, game.AwayTeam),
			"CommenceTime": formatTimestamp(game.CommenceTime, "pending"),
			"Reason":       reason,
			"ReasonTone":   tone,
		})
	}
	return cards
}

func recommendationPrefillURL(rec decision.Recommendation, game store.Game) string {
	values := url.Values{}
	values.Set("game_id", strconv.FormatInt(rec.GameID, 10))
	values.Set("sport", strings.ToUpper(string(rec.Sport)))
	values.Set("side", rec.RecommendedSide)
	values.Set("book_key", rec.BestBook)
	values.Set("american_odds", strconv.Itoa(rec.BestAmericanOdds))
	values.Set("stake_dollars", fmt.Sprintf("%.2f", rec.SuggestedStakeDollars))
	values.Set("edge", formatPercent(rec.Edge))
	values.Set("model_prob", formatPercent(rec.ModelProbability))
	if game.HomeTeam != "" || game.AwayTeam != "" {
		values.Set("game_label", fmt.Sprintf("%s vs %s", game.HomeTeam, game.AwayTeam))
	}
	return "/bets/new?" + values.Encode()
}

func recommendationSideLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "home":
		return "Home"
	case "away":
		return "Away"
	case "over":
		return "Over"
	case "under":
		return "Under"
	default:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return ""
		}
		return strings.ToUpper(trimmed[:1]) + strings.ToLower(trimmed[1:])
	}
}

func marketLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "h2h":
		return "Moneyline"
	case "totals":
		return "Total"
	case "spreads":
		return "Spread"
	default:
		if value == "" {
			return "Market"
		}
		return strings.ToUpper(value)
	}
}

func bankrollReasonLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "ok":
		return "Bankroll clear"
	case "insufficient_funds":
		return "Trimmed to available bankroll"
	default:
		if value == "" {
			return "Sizing available"
		}
		return strings.ReplaceAll(value, "_", " ")
	}
}

func openBetSourceLabel(row store.ListOpenBetsWithGameRow) string {
	if strings.EqualFold(row.AdapterName, "manual") {
		return "Manual"
	}
	if row.SnapshotID != nil && strings.EqualFold(row.AdapterName, "paper") {
		return "Paper auto"
	}
	if row.SnapshotID != nil {
		return "Recommendation"
	}
	return strings.TrimSpace(row.AdapterName)
}

func optionalSnapshotID(snapshotID *int64) string {
	if snapshotID == nil {
		return "—"
	}
	return strconv.FormatInt(*snapshotID, 10)
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func homeSectionError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonNilError(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}
