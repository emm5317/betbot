package server

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultRecommendationsPerformanceLimit = 50
	maxRecommendationsPerformanceLimit     = 500
)

type recommendationsPerformanceResponse struct {
	Rows    []recommendationPerformanceRow   `json:"rows"`
	Summary recommendationPerformanceSummary `json:"summary"`
}

type recommendationPerformanceRow struct {
	SnapshotID             int64      `json:"snapshot_id"`
	GeneratedAt            time.Time  `json:"generated_at"`
	Sport                  string     `json:"sport"`
	GameID                 int64      `json:"game_id"`
	HomeTeam               string     `json:"home_team"`
	AwayTeam               string     `json:"away_team"`
	EventTime              time.Time  `json:"event_time"`
	MarketKey              string     `json:"market_key"`
	RecommendedSide        string     `json:"recommended_side"`
	BestBook               string     `json:"best_book"`
	BestAmericanOdds       int        `json:"best_american_odds"`
	ModelProbability       float64    `json:"model_probability"`
	MarketProbability      float64    `json:"market_probability"`
	Edge                   float64    `json:"edge"`
	SuggestedStakeFraction float64    `json:"suggested_stake_fraction"`
	SuggestedStakeCents    int64      `json:"suggested_stake_cents"`
	BankrollCheckPass      bool       `json:"bankroll_check_pass"`
	BankrollCheckReason    string     `json:"bankroll_check_reason"`
	RankScore              float64    `json:"rank_score"`
	Status                 string     `json:"status"`
	CloseAmericanOdds      *int32     `json:"close_american_odds,omitempty"`
	CloseProbability       *float64   `json:"close_probability,omitempty"`
	CLVDelta               *float64   `json:"clv_delta,omitempty"`
	RealizedResult         string     `json:"realized_result"`
	SettledAt              *time.Time `json:"settled_at,omitempty"`
	Notes                  string     `json:"notes"`
}

type recommendationPerformanceSummary struct {
	Count            int     `json:"count"`
	AvgEdge          float64 `json:"avg_edge"`
	AvgCLV           float64 `json:"avg_clv"`
	BankrollPassRate float64 `json:"bankroll_pass_rate"`
	SettledCount     int     `json:"settled_count"`
}

type recommendationOutcomeMetadata struct {
	Mode              string `json:"mode"`
	CloseSource       string `json:"close_source"`
	ResultSource      string `json:"result_source"`
	ResultParseFailed bool   `json:"result_parse_failed"`
}

func (a *App) handleRecommendationsPerformance(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	if filterErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": filterErr.Error()})
	}

	dateFrom, err := parseRecommendationsRangeDate(c.Query("date_from"), "date_from")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	dateTo, err := parseRecommendationsRangeDate(c.Query("date_to"), "date_to")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	if dateFrom != nil && dateTo != nil && dateFrom.After(*dateTo) {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{
			"error": fmt.Sprintf("invalid date range: date_from %q is after date_to %q", dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02")),
		})
	}

	limit, err := parseRecommendationsPerformanceLimit(c.Query("limit"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	rows, err := a.queries.ListRecommendationPerformanceSnapshots(c.Context(), store.ListRecommendationPerformanceSnapshotsParams{
		Sport:    sportFilter.storeParam(),
		DateFrom: optionalDateParam(dateFrom),
		DateTo:   optionalDateParam(dateTo),
		RowLimit: int32(limit),
	})
	if err != nil {
		return fmt.Errorf("list recommendation performance snapshots: %w", err)
	}

	responseRows := make([]recommendationPerformanceRow, 0, len(rows))
	summary := recommendationPerformanceSummary{}
	clvCount := 0
	clvSum := 0.0
	edgeSum := 0.0
	bankrollPassCount := 0

	for i := range rows {
		performanceRow, insertParams, err := buildRecommendationPerformanceRow(rows[i])
		if err != nil {
			return fmt.Errorf("build recommendation performance snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}

		if _, err := a.queries.InsertRecommendationOutcomeIfChanged(c.Context(), insertParams); err != nil {
			return fmt.Errorf("insert recommendation outcome snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}

		responseRows = append(responseRows, performanceRow)
		edgeSum += performanceRow.Edge
		if performanceRow.CLVDelta != nil {
			clvCount++
			clvSum += *performanceRow.CLVDelta
		}
		if performanceRow.BankrollCheckPass {
			bankrollPassCount++
		}
		if isSettledResult(performanceRow.RealizedResult) {
			summary.SettledCount++
		}
	}

	summary.Count = len(responseRows)
	if summary.Count > 0 {
		n := float64(summary.Count)
		summary.AvgEdge = edgeSum / n
		summary.BankrollPassRate = float64(bankrollPassCount) / n
	}
	if clvCount > 0 {
		summary.AvgCLV = clvSum / float64(clvCount)
	}

	return c.JSON(recommendationsPerformanceResponse{
		Rows:    responseRows,
		Summary: summary,
	})
}

func buildRecommendationPerformanceRow(row store.ListRecommendationPerformanceSnapshotsRow) (recommendationPerformanceRow, store.InsertRecommendationOutcomeIfChangedParams, error) {
	closeSource := "unavailable"
	resultSource := "unavailable"
	resultParseFailed := false

	var closeProbability *float64
	var closeAmericanOdds *int32
	if row.CloseLineID > 0 {
		closeSource = "odds_history"
		closeProbability = float64Ptr(row.CloseProbability)
		closeAmericanOdds = int32Ptr(row.CloseAmericanOdds)
	}
	if closeProbability == nil && row.PersistedOutcomeID > 0 && row.PersistedCloseProbability != nil {
		closeSource = "persisted_outcome"
		closeProbability = float64Ptr(*row.PersistedCloseProbability)
		if row.PersistedCloseAmericanOdds != nil {
			closeAmericanOdds = int32Ptr(*row.PersistedCloseAmericanOdds)
		}
	}

	var homeScore *int
	var awayScore *int
	if row.CloseLineID > 0 {
		parsedHomeScore, parsedAwayScore, ok, err := extractFinalScores(row.CloseRawJson, row.HomeTeam, row.AwayTeam)
		if err != nil {
			resultParseFailed = true
		}
		if ok {
			homeScore = parsedHomeScore
			awayScore = parsedAwayScore
			resultSource = "odds_history_scores"
		}
	}

	perf, err := decision.ComputeRecommendationPerformance(decision.RecommendationPerformanceInput{
		MarketKey:                     row.MarketKey,
		RecommendedSide:               row.RecommendedSide,
		RecommendationHomeProbability: row.MarketProbability,
		ClosingSideProbability:        closeProbability,
		HomeScore:                     homeScore,
		AwayScore:                     awayScore,
	})
	if err != nil {
		return recommendationPerformanceRow{}, store.InsertRecommendationOutcomeIfChangedParams{}, err
	}

	if row.PersistedOutcomeID > 0 {
		if perf.CLVDelta == nil && row.PersistedClvDelta != nil {
			perf.CLVDelta = float64Ptr(*row.PersistedClvDelta)
		}
		if perf.RealizedResult == decision.RecommendationResultUnknown && row.PersistedRealizedResult != nil {
			perf.RealizedResult = *row.PersistedRealizedResult
			if isSettledResult(perf.RealizedResult) {
				perf.Status = decision.RecommendationPerformanceStatusSettled
				if resultSource == "unavailable" {
					resultSource = "persisted_outcome"
				}
			}
		}
		if perf.Status == decision.RecommendationPerformanceStatusCloseUnavailable && row.PersistedStatus != "" {
			perf.Status = row.PersistedStatus
		}
	}

	perf.Status = normalizePerformanceStatus(perf.Status, perf.CLVDelta, perf.RealizedResult)
	if perf.RealizedResult == "" {
		perf.RealizedResult = decision.RecommendationResultUnknown
	}
	if resultSource == "unavailable" && isSettledResult(perf.RealizedResult) {
		resultSource = "persisted_outcome"
	}

	notes := recommendationPerformanceNote(perf.Status, closeSource, resultSource, resultParseFailed)
	metadata, err := json.Marshal(recommendationOutcomeMetadata{
		Mode:              "recommendation-only",
		CloseSource:       closeSource,
		ResultSource:      resultSource,
		ResultParseFailed: resultParseFailed,
	})
	if err != nil {
		return recommendationPerformanceRow{}, store.InsertRecommendationOutcomeIfChangedParams{}, fmt.Errorf("marshal recommendation outcome metadata: %w", err)
	}

	var settledAt *time.Time
	if perf.Status == decision.RecommendationPerformanceStatusSettled {
		if row.CloseLineID > 0 && row.CloseCapturedAt.Valid {
			value := row.CloseCapturedAt.Time.UTC()
			settledAt = &value
		} else if row.PersistedSettledAt.Valid {
			value := row.PersistedSettledAt.Time.UTC()
			settledAt = &value
		}
	}

	insertParams := store.InsertRecommendationOutcomeIfChangedParams{
		SnapshotID:        row.SnapshotID,
		EvaluationStatus:  perf.Status,
		CloseAmericanOdds: closeAmericanOdds,
		CloseProbability:  closeProbability,
		RealizedResult:    nullableRealizedResult(perf.RealizedResult),
		ClvDelta:          perf.CLVDelta,
		SettledAt:         optionalTimestamp(settledAt),
		Notes:             notes,
		Metadata:          metadata,
	}

	response := recommendationPerformanceRow{
		SnapshotID:             row.SnapshotID,
		GeneratedAt:            row.GeneratedAt.Time.UTC(),
		Sport:                  row.Sport,
		GameID:                 row.GameID,
		HomeTeam:               row.HomeTeam,
		AwayTeam:               row.AwayTeam,
		EventTime:              row.EventTime.Time.UTC(),
		MarketKey:              row.MarketKey,
		RecommendedSide:        row.RecommendedSide,
		BestBook:               row.BestBook,
		BestAmericanOdds:       int(row.BestAmericanOdds),
		ModelProbability:       row.ModelProbability,
		MarketProbability:      row.MarketProbability,
		Edge:                   row.Edge,
		SuggestedStakeFraction: row.SuggestedStakeFraction,
		SuggestedStakeCents:    row.SuggestedStakeCents,
		BankrollCheckPass:      row.BankrollCheckPass,
		BankrollCheckReason:    row.BankrollCheckReason,
		RankScore:              row.RankScore,
		Status:                 perf.Status,
		CloseAmericanOdds:      closeAmericanOdds,
		CloseProbability:       closeProbability,
		CLVDelta:               perf.CLVDelta,
		RealizedResult:         perf.RealizedResult,
		SettledAt:              settledAt,
		Notes:                  notes,
	}

	return response, insertParams, nil
}

func parseRecommendationsRangeDate(raw string, fieldName string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	value, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q; expected YYYY-MM-DD", fieldName, raw)
	}
	utc := value.UTC()
	return &utc, nil
}

func parseRecommendationsPerformanceLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRecommendationsPerformanceLimit, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsPerformanceLimit)
	}
	if value < 1 || value > maxRecommendationsPerformanceLimit {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsPerformanceLimit)
	}
	return value, nil
}

func extractFinalScores(raw json.RawMessage, homeTeam string, awayTeam string) (*int, *int, bool, error) {
	type rawScore struct {
		Name  string          `json:"name"`
		Score json.RawMessage `json:"score"`
	}
	type rawGame struct {
		Completed *bool      `json:"completed"`
		Scores    []rawScore `json:"scores"`
	}

	var payload rawGame
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, false, fmt.Errorf("decode odds raw json: %w", err)
	}
	if payload.Completed == nil || !*payload.Completed || len(payload.Scores) == 0 {
		return nil, nil, false, nil
	}

	normalizedHome := strings.ToLower(strings.TrimSpace(homeTeam))
	normalizedAway := strings.ToLower(strings.TrimSpace(awayTeam))
	var homeScore *int
	var awayScore *int

	for _, score := range payload.Scores {
		value, ok, err := parseScoreValue(score.Score)
		if err != nil {
			return nil, nil, false, err
		}
		if !ok {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(score.Name))
		switch name {
		case normalizedHome:
			homeScore = intPtr(value)
		case normalizedAway:
			awayScore = intPtr(value)
		}
	}

	if homeScore == nil || awayScore == nil {
		return nil, nil, false, nil
	}

	return homeScore, awayScore, true, nil
}

func parseScoreValue(raw json.RawMessage) (int, bool, error) {
	if len(raw) == 0 {
		return 0, false, nil
	}

	var intValue int
	if err := json.Unmarshal(raw, &intValue); err == nil {
		return intValue, true, nil
	}

	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err != nil {
		return 0, false, fmt.Errorf("decode score: %w", err)
	}
	trimmed := strings.TrimSpace(stringValue)
	if trimmed == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false, fmt.Errorf("parse score %q: %w", trimmed, err)
	}
	return value, true, nil
}

func recommendationPerformanceNote(status string, closeSource string, resultSource string, resultParseFailed bool) string {
	var note string
	switch status {
	case decision.RecommendationPerformanceStatusCloseUnavailable:
		note = "closing line unavailable for recommended side/book"
	case decision.RecommendationPerformanceStatusPendingOutcome:
		note = "closing line captured; final result unavailable"
	case decision.RecommendationPerformanceStatusSettled:
		if resultSource == "odds_history_scores" {
			note = "graded from odds_history raw scoreboard"
		} else {
			note = "graded from persisted recommendation outcome"
		}
	default:
		note = "recommendation performance evaluated"
	}

	if closeSource == "persisted_outcome" && status != decision.RecommendationPerformanceStatusCloseUnavailable {
		note += " (close sourced from persisted outcome)"
	}
	if resultParseFailed {
		note += "; result parse failed"
	}
	return note
}

func normalizePerformanceStatus(status string, clvDelta *float64, realizedResult string) string {
	switch status {
	case decision.RecommendationPerformanceStatusCloseUnavailable,
		decision.RecommendationPerformanceStatusPendingOutcome,
		decision.RecommendationPerformanceStatusSettled:
		// keep as-is
	default:
		status = ""
	}

	if status == decision.RecommendationPerformanceStatusSettled && isSettledResult(realizedResult) {
		return status
	}
	if clvDelta == nil {
		return decision.RecommendationPerformanceStatusCloseUnavailable
	}
	if isSettledResult(realizedResult) {
		return decision.RecommendationPerformanceStatusSettled
	}
	return decision.RecommendationPerformanceStatusPendingOutcome
}

func isSettledResult(result string) bool {
	switch result {
	case decision.RecommendationResultWin, decision.RecommendationResultLoss, decision.RecommendationResultPush:
		return true
	default:
		return false
	}
}

func nullableRealizedResult(result string) *string {
	if result == "" || result == decision.RecommendationResultUnknown {
		return nil
	}
	return stringPtr(result)
}

func optionalDateParam(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	utc := value.UTC()
	day := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return pgtype.Date{Time: day, Valid: true}
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return store.Timestamptz(*value)
}

func intPtr(value int) *int {
	v := value
	return &v
}

func int32Ptr(value int32) *int32 {
	v := value
	return &v
}

func float64Ptr(value float64) *float64 {
	v := value
	return &v
}

func stringPtr(value string) *string {
	v := value
	return &v
}
