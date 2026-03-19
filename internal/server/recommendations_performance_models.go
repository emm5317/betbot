package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
)

const (
	defaultRecommendationsPerformanceModelsLimit = 20
	maxRecommendationsPerformanceModelsLimit     = 200
)

type recommendationsPerformanceModelsFilterEcho struct {
	Sport    string `json:"sport,omitempty"`
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
	Limit    int    `json:"limit"`
}

type recommendationPerformanceModelRow struct {
	ModelFamily      string  `json:"model_family"`
	ModelVersion     string  `json:"model_version"`
	Source           string  `json:"source"`
	Count            int     `json:"count"`
	SettledCount     int     `json:"settled_count"`
	AvgEdge          float64 `json:"avg_edge"`
	AvgCLV           float64 `json:"avg_clv"`
	BankrollPassRate float64 `json:"bankroll_pass_rate"`
}

type recommendationsPerformanceModelsSummary struct {
	GroupCount    int `json:"group_count"`
	SnapshotCount int `json:"snapshot_count"`
	SettledCount  int `json:"settled_count"`
}

type recommendationsPerformanceModelsResponse struct {
	Filters recommendationsPerformanceModelsFilterEcho `json:"filters"`
	Rows    []recommendationPerformanceModelRow        `json:"rows"`
	Summary recommendationsPerformanceModelsSummary    `json:"summary"`
}

type modelPerformanceGroupKey struct {
	modelFamily  string
	modelVersion string
	source       string
}

type modelPerformanceAccumulator struct {
	count          int
	settledCount   int
	edgeSum        float64
	clvSum         float64
	clvCount       int
	bankrollPasses int
	modelFamily    string
	modelVersion   string
	source         string
}

type recommendationModelAttribution struct {
	modelFamily  string
	modelVersion string
	source       string
}

func (a *App) handleRecommendationsPerformanceModels(c fiber.Ctx) error {
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

	limit, err := parseRecommendationsPerformanceModelsLimit(c.Query("limit"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	rows, err := a.queries.ListRecommendationPerformanceSnapshots(c.Context(), store.ListRecommendationPerformanceSnapshotsParams{
		Sport:    sportFilter.storeParam(),
		DateFrom: optionalDateParam(dateFrom),
		DateTo:   optionalDateParam(dateTo),
		RowLimit: maxRecommendationsPerformanceLimit,
	})
	if err != nil {
		return fmt.Errorf("list recommendation performance snapshots: %w", err)
	}

	aggregates := make(map[modelPerformanceGroupKey]*modelPerformanceAccumulator)
	summary := recommendationsPerformanceModelsSummary{
		SnapshotCount: len(rows),
	}

	for i := range rows {
		perfRow, insertParams, err := buildRecommendationPerformanceRow(rows[i])
		if err != nil {
			return fmt.Errorf("build recommendation performance snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}
		if _, err := a.queries.InsertRecommendationOutcomeIfChanged(c.Context(), insertParams); err != nil {
			return fmt.Errorf("insert recommendation outcome snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}

		model := parseModelAttribution(rows[i].SnapshotMetadata)
		key := modelPerformanceGroupKey{
			modelFamily:  model.modelFamily,
			modelVersion: model.modelVersion,
			source:       model.source,
		}
		acc, ok := aggregates[key]
		if !ok {
			acc = &modelPerformanceAccumulator{
				modelFamily:  model.modelFamily,
				modelVersion: model.modelVersion,
				source:       model.source,
			}
			aggregates[key] = acc
		}

		acc.count++
		acc.edgeSum += perfRow.Edge
		if perfRow.BankrollCheckPass {
			acc.bankrollPasses++
		}
		if perfRow.CLVDelta != nil {
			acc.clvCount++
			acc.clvSum += *perfRow.CLVDelta
		}
		if isSettledResult(perfRow.RealizedResult) {
			acc.settledCount++
			summary.SettledCount++
		}
	}

	grouped := make([]recommendationPerformanceModelRow, 0, len(aggregates))
	for _, acc := range aggregates {
		row := recommendationPerformanceModelRow{
			ModelFamily:      acc.modelFamily,
			ModelVersion:     acc.modelVersion,
			Source:           acc.source,
			Count:            acc.count,
			SettledCount:     acc.settledCount,
			AvgEdge:          acc.edgeSum / float64(acc.count),
			BankrollPassRate: float64(acc.bankrollPasses) / float64(acc.count),
		}
		if acc.clvCount > 0 {
			row.AvgCLV = acc.clvSum / float64(acc.clvCount)
		}
		grouped = append(grouped, row)
	}

	sort.SliceStable(grouped, func(i, j int) bool {
		left := grouped[i]
		right := grouped[j]
		if left.SettledCount != right.SettledCount {
			return left.SettledCount > right.SettledCount
		}
		if left.Count != right.Count {
			return left.Count > right.Count
		}
		if left.AvgCLV != right.AvgCLV {
			return left.AvgCLV > right.AvgCLV
		}
		if left.ModelFamily != right.ModelFamily {
			return left.ModelFamily < right.ModelFamily
		}
		if left.ModelVersion != right.ModelVersion {
			return left.ModelVersion < right.ModelVersion
		}
		return left.Source < right.Source
	})

	if len(grouped) > limit {
		grouped = grouped[:limit]
	}
	summary.GroupCount = len(grouped)

	filters := recommendationsPerformanceModelsFilterEcho{
		Limit: limit,
	}
	if sportFilter.DBSport != "" {
		filters.Sport = sportFilter.Key
	}
	if dateFrom != nil {
		filters.DateFrom = dateFrom.Format("2006-01-02")
	}
	if dateTo != nil {
		filters.DateTo = dateTo.Format("2006-01-02")
	}

	return c.JSON(recommendationsPerformanceModelsResponse{
		Filters: filters,
		Rows:    grouped,
		Summary: summary,
	})
}

func parseRecommendationsPerformanceModelsLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRecommendationsPerformanceModelsLimit, nil
	}

	value, err := parseRecommendationsPerformanceLimit(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsPerformanceModelsLimit)
	}
	if value > maxRecommendationsPerformanceModelsLimit {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsPerformanceModelsLimit)
	}
	return value, nil
}

func parseModelAttribution(snapshotMetadata json.RawMessage) recommendationModelAttribution {
	attribution := recommendationModelAttribution{
		modelFamily:  "unknown",
		modelVersion: "unknown",
		source:       "unknown",
	}
	if len(snapshotMetadata) == 0 {
		return attribution
	}

	var payload map[string]any
	if err := json.Unmarshal(snapshotMetadata, &payload); err != nil {
		return attribution
	}

	if modelBlock, ok := payload["model"].(map[string]any); ok {
		if value := normalizedMetadataString(modelBlock["family"]); value != "" {
			attribution.modelFamily = value
		}
		if value := normalizedMetadataString(modelBlock["version"]); value != "" {
			attribution.modelVersion = value
		}
		if value := normalizedMetadataString(modelBlock["source"]); value != "" {
			attribution.source = value
		}
	}
	if value := normalizedMetadataString(payload["model_family"]); value != "" {
		attribution.modelFamily = value
	}
	if value := normalizedMetadataString(payload["model_version"]); value != "" {
		attribution.modelVersion = value
	}
	if value := normalizedMetadataString(payload["source"]); value != "" {
		attribution.source = value
	}
	return attribution
}

func normalizedMetadataString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return text
}
