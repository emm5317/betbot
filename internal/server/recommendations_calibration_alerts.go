package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"

	"github.com/gofiber/fiber/v3"
)

type recommendationsCalibrationAlertsResponse struct {
	Filters  recommendationsCalibrationAlertsFilterEcho `json:"filters"`
	Alert    recommendationsCalibrationAlertSummary     `json:"alert"`
	Samples  recommendationsCalibrationSampleSummary    `json:"samples"`
	Baseline recommendationCalibrationSummary           `json:"baseline"`
	Current  recommendationCalibrationSummary           `json:"current"`
	Buckets  []recommendationCalibrationAlertBucketRow  `json:"buckets"`
	Trend    []recommendationCalibrationAlertTrendRow   `json:"trend,omitempty"`
}

type recommendationsCalibrationAlertsFilterEcho struct {
	Sport               string  `json:"sport"`
	Mode                string  `json:"mode"`
	CurrentFrom         *string `json:"current_from"`
	CurrentTo           *string `json:"current_to"`
	BaselineFrom        *string `json:"baseline_from"`
	BaselineTo          *string `json:"baseline_to"`
	WindowDays          *int    `json:"window_days,omitempty"`
	Steps               *int    `json:"steps,omitempty"`
	BucketCount         int     `json:"bucket_count"`
	Limit               int     `json:"limit"`
	MinSettledOverall   int     `json:"min_settled_overall"`
	MinSettledPerBucket int     `json:"min_settled_per_bucket"`
	WarnECEDelta        float64 `json:"warn_ece_delta"`
	CriticalECEDelta    float64 `json:"critical_ece_delta"`
	WarnBrierDelta      float64 `json:"warn_brier_delta"`
	CriticalBrierDelta  float64 `json:"critical_brier_delta"`
}

type recommendationCalibrationAlertTrendRow struct {
	WindowStart     string  `json:"window_start"`
	WindowEnd       string  `json:"window_end"`
	AlertLevel      string  `json:"alert_level"`
	ECEDelta        float64 `json:"ece_delta"`
	BrierDelta      float64 `json:"brier_delta"`
	SettledCurrent  int     `json:"settled_current"`
	SettledBaseline int     `json:"settled_baseline"`
}

type recommendationsCalibrationAlertSummary struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

type recommendationsCalibrationSampleSummary struct {
	MinSettledOverall           int `json:"min_settled_overall"`
	MinSettledPerBucket         int `json:"min_settled_per_bucket"`
	CurrentSettledRows          int `json:"current_settled_rows"`
	BaselineSettledRows         int `json:"baseline_settled_rows"`
	InsufficientOverallWindows  int `json:"insufficient_overall_windows"`
	CurrentInsufficientBuckets  int `json:"current_insufficient_buckets"`
	BaselineInsufficientBuckets int `json:"baseline_insufficient_buckets"`
}

type recommendationCalibrationAlertBucketRow struct {
	BucketIndex             int     `json:"bucket_index"`
	SettledCountCurrent     int     `json:"settled_count_current"`
	SettledCountBaseline    int     `json:"settled_count_baseline"`
	ObservedWinRateCurrent  float64 `json:"observed_win_rate_current"`
	ObservedWinRateBaseline float64 `json:"observed_win_rate_baseline"`
	ExpectedWinRateCurrent  float64 `json:"expected_win_rate_current"`
	ExpectedWinRateBaseline float64 `json:"expected_win_rate_baseline"`
	CalibrationGapCurrent   float64 `json:"calibration_gap_current"`
	CalibrationGapBaseline  float64 `json:"calibration_gap_baseline"`
	CalibrationGapDelta     float64 `json:"calibration_gap_delta"`
	BrierCurrent            float64 `json:"brier_current"`
	BrierBaseline           float64 `json:"brier_baseline"`
	BrierDelta              float64 `json:"brier_delta"`
}

func (a *App) handleRecommendationsCalibrationAlerts(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	if filterErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": filterErr.Error()})
	}
	mode, err := parseRecommendationsCalibrationAlertMode(c.Query("mode"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	if mode == decision.CalibrationAlertModeRolling {
		return a.handleRecommendationsCalibrationAlertsRolling(c, sportFilter)
	}
	if strings.TrimSpace(c.Query("window_days")) != "" || strings.TrimSpace(c.Query("steps")) != "" {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{
			"error": "window_days and steps are only valid when mode=rolling",
		})
	}

	currentFrom, err := parseRecommendationsRangeDate(c.Query("current_from"), "current_from")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	currentTo, err := parseRecommendationsRangeDate(c.Query("current_to"), "current_to")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	if currentFrom != nil && currentTo != nil && currentFrom.After(*currentTo) {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{
			"error": fmt.Sprintf("invalid current date range: current_from %q is after current_to %q", currentFrom.Format("2006-01-02"), currentTo.Format("2006-01-02")),
		})
	}

	baselineFrom, err := parseRecommendationsRangeDate(c.Query("baseline_from"), "baseline_from")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	baselineTo, err := parseRecommendationsRangeDate(c.Query("baseline_to"), "baseline_to")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	if baselineFrom != nil && baselineTo != nil && baselineFrom.After(*baselineTo) {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{
			"error": fmt.Sprintf("invalid baseline date range: baseline_from %q is after baseline_to %q", baselineFrom.Format("2006-01-02"), baselineTo.Format("2006-01-02")),
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
	minSettledOverall, err := parseRecommendationsMinSettled(c.Query("min_settled_overall"), "min_settled_overall", decision.DefaultMinSettledOverall)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	minSettledPerBucket, err := parseRecommendationsMinSettled(c.Query("min_settled_per_bucket"), "min_settled_per_bucket", decision.DefaultMinSettledPerBucket)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	thresholds, err := parseRecommendationsCalibrationDriftThresholds(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	currentRows, err := a.loadRecommendationCalibrationRows(
		c,
		sportFilter.storeParam(),
		currentFrom,
		currentTo,
		limit,
	)
	if err != nil {
		return err
	}
	baselineRows, err := a.loadRecommendationCalibrationRows(
		c,
		sportFilter.storeParam(),
		baselineFrom,
		baselineTo,
		limit,
	)
	if err != nil {
		return err
	}

	currentReport, err := decision.ComputeCalibrationReport(currentRows, decision.CalibrationOptions{BucketCount: bucketCount})
	if err != nil {
		return fmt.Errorf("compute current calibration report: %w", err)
	}
	baselineReport, err := decision.ComputeCalibrationReport(baselineRows, decision.CalibrationOptions{BucketCount: bucketCount})
	if err != nil {
		return fmt.Errorf("compute baseline calibration report: %w", err)
	}

	drift, err := decision.EvaluateCalibrationDrift(decision.CalibrationDriftInput{
		Sport:      sportFilter.DBSport,
		Current:    currentReport,
		Baseline:   baselineReport,
		Thresholds: thresholds,
		Guardrails: decision.CalibrationDriftGuardrails{
			MinSettledOverall:   minSettledOverall,
			MinSettledPerBucket: minSettledPerBucket,
		},
	})
	if err != nil {
		return fmt.Errorf("evaluate calibration drift: %w", err)
	}

	bucketRows := make([]recommendationCalibrationAlertBucketRow, len(drift.Buckets))
	for i := range drift.Buckets {
		bucket := drift.Buckets[i]
		bucketRows[i] = recommendationCalibrationAlertBucketRow{
			BucketIndex:             bucket.BucketIndex,
			SettledCountCurrent:     bucket.SettledCountCurrent,
			SettledCountBaseline:    bucket.SettledCountBaseline,
			ObservedWinRateCurrent:  bucket.ObservedWinRateCurrent,
			ObservedWinRateBaseline: bucket.ObservedWinRateBaseline,
			ExpectedWinRateCurrent:  bucket.ExpectedWinRateCurrent,
			ExpectedWinRateBaseline: bucket.ExpectedWinRateBaseline,
			CalibrationGapCurrent:   bucket.CalibrationGapCurrent,
			CalibrationGapBaseline:  bucket.CalibrationGapBaseline,
			CalibrationGapDelta:     bucket.CalibrationGapDelta,
			BrierCurrent:            bucket.BrierCurrent,
			BrierBaseline:           bucket.BrierBaseline,
			BrierDelta:              bucket.BrierDelta,
		}
	}
	if err := a.persistRecommendationCalibrationAlertRun(
		c,
		recommendationCalibrationAlertRunInput{
			Sport:      sportFilter.DBSport,
			Mode:       decision.CalibrationAlertModePointInTime,
			WindowDays: nil,
			StepIndex:  0,
			StepCount:  1,
			Window: recommendationCalibrationAlertWindow{
				CurrentFrom:  currentFrom,
				CurrentTo:    currentTo,
				BaselineFrom: baselineFrom,
				BaselineTo:   baselineTo,
			},
			BucketCount:         bucketCount,
			Limit:               limit,
			MinSettledOverall:   minSettledOverall,
			MinSettledPerBucket: minSettledPerBucket,
			Thresholds:          thresholds,
			Current:             currentReport,
			Baseline:            baselineReport,
			Drift:               drift,
		},
	); err != nil {
		return err
	}

	return c.JSON(recommendationsCalibrationAlertsResponse{
		Filters: recommendationsCalibrationAlertsFilterEcho{
			Sport:               sportFilter.Key,
			Mode:                decision.CalibrationAlertModePointInTime,
			CurrentFrom:         nullableDateString(currentFrom),
			CurrentTo:           nullableDateString(currentTo),
			BaselineFrom:        nullableDateString(baselineFrom),
			BaselineTo:          nullableDateString(baselineTo),
			WindowDays:          nil,
			Steps:               nil,
			BucketCount:         bucketCount,
			Limit:               limit,
			MinSettledOverall:   minSettledOverall,
			MinSettledPerBucket: minSettledPerBucket,
			WarnECEDelta:        thresholds.WarnECEDelta,
			CriticalECEDelta:    thresholds.CriticalECEDelta,
			WarnBrierDelta:      thresholds.WarnBrierDelta,
			CriticalBrierDelta:  thresholds.CriticalBrierDelta,
		},
		Alert: recommendationsCalibrationAlertSummary{
			Level:   drift.Level,
			Reasons: drift.Reasons,
		},
		Samples: recommendationsCalibrationSampleSummary{
			MinSettledOverall:           drift.Samples.MinSettledOverall,
			MinSettledPerBucket:         drift.Samples.MinSettledPerBucket,
			CurrentSettledRows:          drift.Samples.CurrentSettledRows,
			BaselineSettledRows:         drift.Samples.BaselineSettledRows,
			InsufficientOverallWindows:  drift.Samples.InsufficientOverallWindows,
			CurrentInsufficientBuckets:  drift.Samples.CurrentInsufficientBuckets,
			BaselineInsufficientBuckets: drift.Samples.BaselineInsufficientBuckets,
		},
		Baseline: recommendationCalibrationSummary{
			TotalRows:              baselineReport.Summary.TotalRows,
			SettledRows:            baselineReport.Summary.SettledRows,
			ExcludedRows:           baselineReport.Summary.ExcludedRows,
			OverallObservedWinRate: baselineReport.Summary.OverallObservedWinRate,
			OverallExpectedWinRate: baselineReport.Summary.OverallExpectedWinRate,
			OverallBrier:           baselineReport.Summary.OverallBrier,
			OverallECE:             baselineReport.Summary.OverallECE,
			AverageCLV:             baselineReport.Summary.AverageCLV,
		},
		Current: recommendationCalibrationSummary{
			TotalRows:              currentReport.Summary.TotalRows,
			SettledRows:            currentReport.Summary.SettledRows,
			ExcludedRows:           currentReport.Summary.ExcludedRows,
			OverallObservedWinRate: currentReport.Summary.OverallObservedWinRate,
			OverallExpectedWinRate: currentReport.Summary.OverallExpectedWinRate,
			OverallBrier:           currentReport.Summary.OverallBrier,
			OverallECE:             currentReport.Summary.OverallECE,
			AverageCLV:             currentReport.Summary.AverageCLV,
		},
		Buckets: bucketRows,
		Trend:   nil,
	})
}

func parseRecommendationsMinSettled(raw string, field string, defaultValue int) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q; expected integer in [1,1000000]", field, raw)
	}
	if value < 1 || value > 1000000 {
		return 0, fmt.Errorf("invalid %s %q; expected integer in [1,1000000]", field, raw)
	}
	return value, nil
}

func parseRecommendationsCalibrationDriftThresholds(c fiber.Ctx) (decision.CalibrationDriftThresholds, error) {
	warnECE, err := parseRecommendationsDeltaThreshold(c.Query("warn_ece_delta"), "warn_ece_delta", decision.DefaultWarnECEDelta)
	if err != nil {
		return decision.CalibrationDriftThresholds{}, err
	}
	criticalECE, err := parseRecommendationsDeltaThreshold(c.Query("critical_ece_delta"), "critical_ece_delta", decision.DefaultCriticalECEDelta)
	if err != nil {
		return decision.CalibrationDriftThresholds{}, err
	}
	warnBrier, err := parseRecommendationsDeltaThreshold(c.Query("warn_brier_delta"), "warn_brier_delta", decision.DefaultWarnBrierDelta)
	if err != nil {
		return decision.CalibrationDriftThresholds{}, err
	}
	criticalBrier, err := parseRecommendationsDeltaThreshold(c.Query("critical_brier_delta"), "critical_brier_delta", decision.DefaultCriticalBrierDelta)
	if err != nil {
		return decision.CalibrationDriftThresholds{}, err
	}

	if warnECE > criticalECE {
		return decision.CalibrationDriftThresholds{}, fmt.Errorf("invalid thresholds: warn_ece_delta %.6f exceeds critical_ece_delta %.6f", warnECE, criticalECE)
	}
	if warnBrier > criticalBrier {
		return decision.CalibrationDriftThresholds{}, fmt.Errorf("invalid thresholds: warn_brier_delta %.6f exceeds critical_brier_delta %.6f", warnBrier, criticalBrier)
	}

	return decision.CalibrationDriftThresholds{
		WarnECEDelta:       warnECE,
		CriticalECEDelta:   criticalECE,
		WarnBrierDelta:     warnBrier,
		CriticalBrierDelta: criticalBrier,
	}, nil
}

func parseRecommendationsDeltaThreshold(raw string, field string, defaultValue float64) (float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q; expected decimal in [0,1]", field, raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0, fmt.Errorf("invalid %s %q; expected decimal in [0,1]", field, raw)
	}
	return value, nil
}

func nullableDateString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format("2006-01-02")
	return &formatted
}
