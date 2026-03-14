package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
)

const (
	defaultRecommendationsCalibrationLimit = 1000
	maxRecommendationsCalibrationLimit     = 5000
)

type recommendationsCalibrationResponse struct {
	Filters recommendationsCalibrationFilterEcho `json:"filters"`
	Summary recommendationCalibrationSummary     `json:"summary"`
	Buckets []recommendationCalibrationBucketRow `json:"buckets"`
}

type recommendationsCalibrationFilterEcho struct {
	Sport       string  `json:"sport"`
	DateFrom    *string `json:"date_from"`
	DateTo      *string `json:"date_to"`
	BucketCount int     `json:"bucket_count"`
	Limit       int     `json:"limit"`
}

type recommendationCalibrationSummary struct {
	TotalRows              int     `json:"total_rows"`
	SettledRows            int     `json:"settled_rows"`
	ExcludedRows           int     `json:"excluded_rows"`
	OverallObservedWinRate float64 `json:"overall_observed_win_rate"`
	OverallExpectedWinRate float64 `json:"overall_expected_win_rate"`
	OverallBrier           float64 `json:"overall_brier"`
	OverallECE             float64 `json:"overall_ece"`
	AverageCLV             float64 `json:"avg_clv"`
}

type recommendationCalibrationBucketRow struct {
	BucketIndex     int      `json:"bucket_index"`
	RankMin         *float64 `json:"rank_min"`
	RankMax         *float64 `json:"rank_max"`
	Count           int      `json:"count"`
	SettledCount    int      `json:"settled_count"`
	ObservedWinRate float64  `json:"observed_win_rate"`
	ExpectedWinRate float64  `json:"expected_win_rate"`
	CalibrationGap  float64  `json:"calibration_gap"`
	Brier           float64  `json:"brier"`
	AverageCLV      float64  `json:"avg_clv"`
}

func (a *App) handleRecommendationsCalibration(c fiber.Ctx) error {
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

	bucketCount, err := parseRecommendationsCalibrationBucketCount(c.Query("bucket_count"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	limit, err := parseRecommendationsCalibrationLimit(c.Query("limit"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	calibrationRows, err := a.loadRecommendationCalibrationRows(
		c,
		sportFilter.storeParam(),
		dateFrom,
		dateTo,
		limit,
	)
	if err != nil {
		return err
	}

	report, err := decision.ComputeCalibrationReport(calibrationRows, decision.CalibrationOptions{
		BucketCount: bucketCount,
	})
	if err != nil {
		return fmt.Errorf("compute calibration report: %w", err)
	}

	bucketRows := make([]recommendationCalibrationBucketRow, len(report.Buckets))
	for i := range report.Buckets {
		bucket := report.Buckets[i]
		bucketRows[i] = recommendationCalibrationBucketRow{
			BucketIndex:     bucket.BucketIndex,
			RankMin:         bucket.RankMin,
			RankMax:         bucket.RankMax,
			Count:           bucket.Count,
			SettledCount:    bucket.SettledCount,
			ObservedWinRate: bucket.ObservedWinRate,
			ExpectedWinRate: bucket.ExpectedWinRate,
			CalibrationGap:  bucket.CalibrationGap,
			Brier:           bucket.Brier,
			AverageCLV:      bucket.AverageCLV,
		}
	}

	return c.JSON(recommendationsCalibrationResponse{
		Filters: recommendationsCalibrationFilterEcho{
			Sport:       sportFilter.Key,
			DateFrom:    nullableDateString(dateFrom),
			DateTo:      nullableDateString(dateTo),
			BucketCount: report.BucketCount,
			Limit:       limit,
		},
		Summary: recommendationCalibrationSummary{
			TotalRows:              report.Summary.TotalRows,
			SettledRows:            report.Summary.SettledRows,
			ExcludedRows:           report.Summary.ExcludedRows,
			OverallObservedWinRate: report.Summary.OverallObservedWinRate,
			OverallExpectedWinRate: report.Summary.OverallExpectedWinRate,
			OverallBrier:           report.Summary.OverallBrier,
			OverallECE:             report.Summary.OverallECE,
			AverageCLV:             report.Summary.AverageCLV,
		},
		Buckets: bucketRows,
	})
}

func (a *App) loadRecommendationCalibrationRows(
	c fiber.Ctx,
	sport *string,
	dateFrom *time.Time,
	dateTo *time.Time,
	limit int,
) ([]decision.CalibrationInputRow, error) {
	rows, err := a.queries.ListRecommendationPerformanceSnapshots(c.Context(), store.ListRecommendationPerformanceSnapshotsParams{
		Sport:    sport,
		DateFrom: optionalDateParam(dateFrom),
		DateTo:   optionalDateParam(dateTo),
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list recommendation snapshots for calibration: %w", err)
	}

	calibrationRows := make([]decision.CalibrationInputRow, 0, len(rows))
	for i := range rows {
		perfRow, insertParams, err := buildRecommendationPerformanceRow(rows[i])
		if err != nil {
			return nil, fmt.Errorf("build recommendation performance for calibration snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}
		if _, err := a.queries.InsertRecommendationOutcomeIfChanged(c.Context(), insertParams); err != nil {
			return nil, fmt.Errorf("insert recommendation outcome for calibration snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}

		expectedWinProbability, err := recommendationExpectedWinProbability(perfRow.RecommendedSide, perfRow.MarketProbability)
		if err != nil {
			return nil, fmt.Errorf("recommendation expected win probability snapshot_id=%d: %w", rows[i].SnapshotID, err)
		}

		calibrationRows = append(calibrationRows, decision.CalibrationInputRow{
			RowID:                  perfRow.SnapshotID,
			RankScore:              perfRow.RankScore,
			ExpectedWinProbability: expectedWinProbability,
			Outcome:                perfRow.RealizedResult,
			CLVDelta:               perfRow.CLVDelta,
		})
	}
	return calibrationRows, nil
}

func parseRecommendationsCalibrationBucketCount(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decision.DefaultCalibrationBucketCount, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid bucket_count %q; expected integer in [1,%d]", raw, decision.MaxCalibrationBucketCount)
	}
	if value < 1 || value > decision.MaxCalibrationBucketCount {
		return 0, fmt.Errorf("invalid bucket_count %q; expected integer in [1,%d]", raw, decision.MaxCalibrationBucketCount)
	}
	return value, nil
}

func parseRecommendationsCalibrationLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRecommendationsCalibrationLimit, nil
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsCalibrationLimit)
	}
	if value < 1 || value > maxRecommendationsCalibrationLimit {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsCalibrationLimit)
	}
	return value, nil
}

func recommendationExpectedWinProbability(recommendedSide string, homeProbability float64) (float64, error) {
	if math.IsNaN(homeProbability) || math.IsInf(homeProbability, 0) || homeProbability < 0 || homeProbability > 1 {
		return 0, fmt.Errorf("invalid market probability %.6f", homeProbability)
	}

	switch strings.ToLower(strings.TrimSpace(recommendedSide)) {
	case "home":
		return homeProbability, nil
	case "away":
		return 1 - homeProbability, nil
	default:
		return 0, fmt.Errorf("invalid recommended_side %q", recommendedSide)
	}
}
