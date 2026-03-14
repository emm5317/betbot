package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultRecommendationsCalibrationAlertHistoryLimit = 100
	maxRecommendationsCalibrationAlertHistoryLimit     = 500
)

type recommendationsCalibrationAlertsHistoryResponse struct {
	Filters recommendationsCalibrationAlertsHistoryFilterEcho `json:"filters"`
	Rows    []recommendationCalibrationAlertHistoryRow        `json:"rows"`
}

type recommendationsCalibrationAlertsHistoryFilterEcho struct {
	Sport    string  `json:"sport"`
	DateFrom *string `json:"date_from"`
	DateTo   *string `json:"date_to"`
	Limit    int     `json:"limit"`
}

type recommendationCalibrationAlertHistoryRow struct {
	ID                          int64     `json:"id"`
	CreatedAt                   time.Time `json:"created_at"`
	Sport                       string    `json:"sport"`
	RequestHash                 string    `json:"request_hash"`
	RunGroupHash                string    `json:"run_group_hash"`
	Mode                        string    `json:"mode"`
	StepIndex                   int       `json:"step_index"`
	StepCount                   int       `json:"step_count"`
	WindowDays                  *int      `json:"window_days,omitempty"`
	CurrentFrom                 *string   `json:"current_from"`
	CurrentTo                   *string   `json:"current_to"`
	BaselineFrom                *string   `json:"baseline_from"`
	BaselineTo                  *string   `json:"baseline_to"`
	BucketCount                 int       `json:"bucket_count"`
	Limit                       int       `json:"limit"`
	MinSettledOverall           int       `json:"min_settled_overall"`
	MinSettledPerBucket         int       `json:"min_settled_per_bucket"`
	WarnECEDelta                float64   `json:"warn_ece_delta"`
	CriticalECEDelta            float64   `json:"critical_ece_delta"`
	WarnBrierDelta              float64   `json:"warn_brier_delta"`
	CriticalBrierDelta          float64   `json:"critical_brier_delta"`
	AlertLevel                  string    `json:"alert_level"`
	Reasons                     []string  `json:"reasons"`
	CurrentOverallECE           float64   `json:"current_overall_ece"`
	BaselineOverallECE          float64   `json:"baseline_overall_ece"`
	ECEDelta                    float64   `json:"ece_delta"`
	CurrentOverallBrier         float64   `json:"current_overall_brier"`
	BaselineOverallBrier        float64   `json:"baseline_overall_brier"`
	BrierDelta                  float64   `json:"brier_delta"`
	CurrentSettledRows          int       `json:"current_settled_rows"`
	BaselineSettledRows         int       `json:"baseline_settled_rows"`
	InsufficientOverallWindows  int       `json:"insufficient_overall_windows"`
	CurrentInsufficientBuckets  int       `json:"current_insufficient_buckets"`
	BaselineInsufficientBuckets int       `json:"baseline_insufficient_buckets"`
	Payload                     any       `json:"payload,omitempty"`
}

type recommendationCalibrationAlertWindow struct {
	CurrentFrom  *time.Time
	CurrentTo    *time.Time
	BaselineFrom *time.Time
	BaselineTo   *time.Time
}

type recommendationCalibrationAlertRunInput struct {
	Sport               string
	Mode                string
	RequestHash         string
	RunGroupHash        string
	WindowDays          *int
	StepIndex           int
	StepCount           int
	Window              recommendationCalibrationAlertWindow
	BucketCount         int
	Limit               int
	MinSettledOverall   int
	MinSettledPerBucket int
	Thresholds          decision.CalibrationDriftThresholds
	Current             decision.CalibrationReport
	Baseline            decision.CalibrationReport
	Drift               decision.CalibrationDriftResult
}

type recommendationCalibrationAlertRollingStep struct {
	Window   recommendationCalibrationAlertWindow
	Current  decision.CalibrationReport
	Baseline decision.CalibrationReport
}

func (a *App) handleRecommendationsCalibrationAlertsRolling(c fiber.Ctx, sportFilter sportFilterSelection) error {
	currentFrom, err := parseRecommendationsRangeDate(c.Query("current_from"), "current_from")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	currentTo, err := parseRecommendationsRangeDate(c.Query("current_to"), "current_to")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	baselineFrom, err := parseRecommendationsRangeDate(c.Query("baseline_from"), "baseline_from")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	baselineTo, err := parseRecommendationsRangeDate(c.Query("baseline_to"), "baseline_to")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	if currentFrom != nil || baselineFrom != nil || baselineTo != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{
			"error": "rolling mode only accepts current_to with window_days and steps; current_from, baseline_from, and baseline_to must be omitted",
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
	windowDays, err := parseRecommendationsRollingWindowDays(c.Query("window_days"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	stepsCount, err := parseRecommendationsRollingSteps(c.Query("steps"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	thresholds, err := parseRecommendationsCalibrationDriftThresholds(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	anchorDate := time.Now().UTC()
	if currentTo != nil {
		anchorDate = *currentTo
	}
	windows, err := decision.BuildRollingCalibrationDriftWindows(anchorDate, windowDays, stepsCount)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	steps := make([]recommendationCalibrationAlertRollingStep, 0, len(windows))
	for i := range windows {
		window := recommendationCalibrationAlertWindow{
			CurrentFrom:  timePtr(windows[i].CurrentFrom),
			CurrentTo:    timePtr(windows[i].CurrentTo),
			BaselineFrom: timePtr(windows[i].BaselineFrom),
			BaselineTo:   timePtr(windows[i].BaselineTo),
		}
		currentRows, loadErr := a.loadRecommendationCalibrationRows(c, sportFilter.storeParam(), window.CurrentFrom, window.CurrentTo, limit)
		if loadErr != nil {
			return loadErr
		}
		baselineRows, loadErr := a.loadRecommendationCalibrationRows(c, sportFilter.storeParam(), window.BaselineFrom, window.BaselineTo, limit)
		if loadErr != nil {
			return loadErr
		}

		currentReport, calcErr := decision.ComputeCalibrationReport(currentRows, decision.CalibrationOptions{BucketCount: bucketCount})
		if calcErr != nil {
			return fmt.Errorf("compute current calibration report: %w", calcErr)
		}
		baselineReport, calcErr := decision.ComputeCalibrationReport(baselineRows, decision.CalibrationOptions{BucketCount: bucketCount})
		if calcErr != nil {
			return fmt.Errorf("compute baseline calibration report: %w", calcErr)
		}
		steps = append(steps, recommendationCalibrationAlertRollingStep{
			Window:   window,
			Current:  currentReport,
			Baseline: baselineReport,
		})
	}
	sort.SliceStable(steps, func(i, j int) bool {
		left := steps[i].Window
		right := steps[j].Window
		if !left.CurrentTo.Equal(*right.CurrentTo) {
			return left.CurrentTo.Before(*right.CurrentTo)
		}
		if !left.CurrentFrom.Equal(*right.CurrentFrom) {
			return left.CurrentFrom.Before(*right.CurrentFrom)
		}
		if !left.BaselineTo.Equal(*right.BaselineTo) {
			return left.BaselineTo.Before(*right.BaselineTo)
		}
		return left.BaselineFrom.Before(*right.BaselineFrom)
	})

	rollingInput := decision.RollingCalibrationDriftInput{
		Sport:      sportFilter.DBSport,
		Thresholds: thresholds,
		Guardrails: decision.CalibrationDriftGuardrails{
			MinSettledOverall:   minSettledOverall,
			MinSettledPerBucket: minSettledPerBucket,
		},
		Steps: make([]decision.RollingCalibrationDriftStepInput, 0, len(steps)),
	}
	for i := range steps {
		rollingInput.Steps = append(rollingInput.Steps, decision.RollingCalibrationDriftStepInput{
			Window: decision.CalibrationDriftWindow{
				CurrentFrom:  *steps[i].Window.CurrentFrom,
				CurrentTo:    *steps[i].Window.CurrentTo,
				BaselineFrom: *steps[i].Window.BaselineFrom,
				BaselineTo:   *steps[i].Window.BaselineTo,
			},
			Current:  steps[i].Current,
			Baseline: steps[i].Baseline,
		})
	}
	rollingResult, err := decision.EvaluateRollingCalibrationDrift(rollingInput)
	if err != nil {
		return fmt.Errorf("evaluate calibration drift (rolling): %w", err)
	}
	if len(rollingResult.Steps) == 0 {
		return fmt.Errorf("evaluate calibration drift (rolling): no steps returned")
	}

	latest := rollingResult.Steps[len(rollingResult.Steps)-1]
	latestStep := steps[len(steps)-1]
	requestHash := buildRecommendationCalibrationAlertRequestHash(
		sportFilter.DBSport,
		decision.CalibrationAlertModeRolling,
		recommendationCalibrationAlertWindow{
			CurrentFrom:  nil,
			CurrentTo:    timePtr(anchorDate),
			BaselineFrom: nil,
			BaselineTo:   nil,
		},
		bucketCount,
		limit,
		minSettledOverall,
		minSettledPerBucket,
		thresholds,
		&windowDays,
		&stepsCount,
	)
	runGroupHash := buildRecommendationCalibrationAlertRunGroupHash(requestHash, steps)
	for i := range rollingResult.Steps {
		if err := a.persistRecommendationCalibrationAlertRun(c, recommendationCalibrationAlertRunInput{
			Sport:               sportFilter.DBSport,
			Mode:                decision.CalibrationAlertModeRolling,
			RequestHash:         requestHash,
			RunGroupHash:        runGroupHash,
			WindowDays:          &windowDays,
			StepIndex:           i,
			StepCount:           len(rollingResult.Steps),
			Window:              steps[i].Window,
			BucketCount:         bucketCount,
			Limit:               limit,
			MinSettledOverall:   minSettledOverall,
			MinSettledPerBucket: minSettledPerBucket,
			Thresholds:          thresholds,
			Current:             steps[i].Current,
			Baseline:            steps[i].Baseline,
			Drift:               rollingResult.Steps[i].Drift,
		}); err != nil {
			return err
		}
	}

	buckets := make([]recommendationCalibrationAlertBucketRow, len(latest.Drift.Buckets))
	for i := range latest.Drift.Buckets {
		bucket := latest.Drift.Buckets[i]
		buckets[i] = recommendationCalibrationAlertBucketRow{
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
	trend := make([]recommendationCalibrationAlertTrendRow, 0, len(rollingResult.Steps))
	for i := range rollingResult.Steps {
		step := rollingResult.Steps[i]
		trend = append(trend, recommendationCalibrationAlertTrendRow{
			WindowStart:     step.Window.CurrentFrom.UTC().Format("2006-01-02"),
			WindowEnd:       step.Window.CurrentTo.UTC().Format("2006-01-02"),
			AlertLevel:      step.Drift.Level,
			ECEDelta:        step.Drift.Summary.ECEDelta,
			BrierDelta:      step.Drift.Summary.BrierDelta,
			SettledCurrent:  step.Drift.Samples.CurrentSettledRows,
			SettledBaseline: step.Drift.Samples.BaselineSettledRows,
		})
	}

	return c.JSON(recommendationsCalibrationAlertsResponse{
		Filters: recommendationsCalibrationAlertsFilterEcho{
			Sport:               sportFilter.Key,
			Mode:                decision.CalibrationAlertModeRolling,
			CurrentFrom:         nullableDateString(latestStep.Window.CurrentFrom),
			CurrentTo:           nullableDateString(latestStep.Window.CurrentTo),
			BaselineFrom:        nullableDateString(latestStep.Window.BaselineFrom),
			BaselineTo:          nullableDateString(latestStep.Window.BaselineTo),
			WindowDays:          &windowDays,
			Steps:               &stepsCount,
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
			Level:   latest.Drift.Level,
			Reasons: latest.Drift.Reasons,
		},
		Samples: recommendationsCalibrationSampleSummary{
			MinSettledOverall:           latest.Drift.Samples.MinSettledOverall,
			MinSettledPerBucket:         latest.Drift.Samples.MinSettledPerBucket,
			CurrentSettledRows:          latest.Drift.Samples.CurrentSettledRows,
			BaselineSettledRows:         latest.Drift.Samples.BaselineSettledRows,
			InsufficientOverallWindows:  latest.Drift.Samples.InsufficientOverallWindows,
			CurrentInsufficientBuckets:  latest.Drift.Samples.CurrentInsufficientBuckets,
			BaselineInsufficientBuckets: latest.Drift.Samples.BaselineInsufficientBuckets,
		},
		Baseline: recommendationCalibrationSummary{
			TotalRows:              latestStep.Baseline.Summary.TotalRows,
			SettledRows:            latestStep.Baseline.Summary.SettledRows,
			ExcludedRows:           latestStep.Baseline.Summary.ExcludedRows,
			OverallObservedWinRate: latestStep.Baseline.Summary.OverallObservedWinRate,
			OverallExpectedWinRate: latestStep.Baseline.Summary.OverallExpectedWinRate,
			OverallBrier:           latestStep.Baseline.Summary.OverallBrier,
			OverallECE:             latestStep.Baseline.Summary.OverallECE,
			AverageCLV:             latestStep.Baseline.Summary.AverageCLV,
		},
		Current: recommendationCalibrationSummary{
			TotalRows:              latestStep.Current.Summary.TotalRows,
			SettledRows:            latestStep.Current.Summary.SettledRows,
			ExcludedRows:           latestStep.Current.Summary.ExcludedRows,
			OverallObservedWinRate: latestStep.Current.Summary.OverallObservedWinRate,
			OverallExpectedWinRate: latestStep.Current.Summary.OverallExpectedWinRate,
			OverallBrier:           latestStep.Current.Summary.OverallBrier,
			OverallECE:             latestStep.Current.Summary.OverallECE,
			AverageCLV:             latestStep.Current.Summary.AverageCLV,
		},
		Buckets: buckets,
		Trend:   trend,
	})
}

func (a *App) handleRecommendationsCalibrationAlertsHistory(c fiber.Ctx) error {
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
	limit, err := parseRecommendationsCalibrationAlertHistoryLimit(c.Query("limit"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}
	rows, err := a.queries.ListRecommendationCalibrationAlertRuns(c.Context(), store.ListRecommendationCalibrationAlertRunsParams{
		Sport:    sportFilter.storeParam(),
		DateFrom: optionalDateParam(dateFrom),
		DateTo:   optionalDateParam(dateTo),
		RowLimit: int32(limit),
	})
	if err != nil {
		return fmt.Errorf("list recommendation calibration alert runs: %w", err)
	}

	responseRows := make([]recommendationCalibrationAlertHistoryRow, 0, len(rows))
	for i := range rows {
		var reasons []string
		if len(rows[i].Reasons) > 0 {
			if err := json.Unmarshal(rows[i].Reasons, &reasons); err != nil {
				return fmt.Errorf("decode recommendation calibration alert reasons id=%d: %w", rows[i].ID, err)
			}
		}
		var payload any
		if len(rows[i].Payload) > 0 {
			if err := json.Unmarshal(rows[i].Payload, &payload); err != nil {
				return fmt.Errorf("decode recommendation calibration alert payload id=%d: %w", rows[i].ID, err)
			}
		}
		responseRows = append(responseRows, recommendationCalibrationAlertHistoryRow{
			ID:                          rows[i].ID,
			CreatedAt:                   rows[i].CreatedAt.Time.UTC(),
			Sport:                       rows[i].Sport,
			RequestHash:                 rows[i].RequestHash,
			RunGroupHash:                rows[i].RunGroupHash,
			Mode:                        rows[i].Mode,
			StepIndex:                   int(rows[i].StepIndex),
			StepCount:                   int(rows[i].StepCount),
			WindowDays:                  intPtrFromInt32(rows[i].WindowDays),
			CurrentFrom:                 nullableDateStringFromPg(rows[i].CurrentFrom),
			CurrentTo:                   nullableDateStringFromPg(rows[i].CurrentTo),
			BaselineFrom:                nullableDateStringFromPg(rows[i].BaselineFrom),
			BaselineTo:                  nullableDateStringFromPg(rows[i].BaselineTo),
			BucketCount:                 int(rows[i].BucketCount),
			Limit:                       int(rows[i].RowLimit),
			MinSettledOverall:           int(rows[i].MinSettledOverall),
			MinSettledPerBucket:         int(rows[i].MinSettledPerBucket),
			WarnECEDelta:                rows[i].WarnEceDelta,
			CriticalECEDelta:            rows[i].CriticalEceDelta,
			WarnBrierDelta:              rows[i].WarnBrierDelta,
			CriticalBrierDelta:          rows[i].CriticalBrierDelta,
			AlertLevel:                  rows[i].AlertLevel,
			Reasons:                     reasons,
			CurrentOverallECE:           rows[i].CurrentOverallEce,
			BaselineOverallECE:          rows[i].BaselineOverallEce,
			ECEDelta:                    rows[i].EceDelta,
			CurrentOverallBrier:         rows[i].CurrentOverallBrier,
			BaselineOverallBrier:        rows[i].BaselineOverallBrier,
			BrierDelta:                  rows[i].BrierDelta,
			CurrentSettledRows:          int(rows[i].CurrentSettledRows),
			BaselineSettledRows:         int(rows[i].BaselineSettledRows),
			InsufficientOverallWindows:  int(rows[i].InsufficientOverallWindows),
			CurrentInsufficientBuckets:  int(rows[i].CurrentInsufficientBuckets),
			BaselineInsufficientBuckets: int(rows[i].BaselineInsufficientBuckets),
			Payload:                     payload,
		})
	}

	return c.JSON(recommendationsCalibrationAlertsHistoryResponse{
		Filters: recommendationsCalibrationAlertsHistoryFilterEcho{
			Sport:    sportFilter.Key,
			DateFrom: nullableDateString(dateFrom),
			DateTo:   nullableDateString(dateTo),
			Limit:    limit,
		},
		Rows: responseRows,
	})
}

func (a *App) persistRecommendationCalibrationAlertRun(c fiber.Ctx, input recommendationCalibrationAlertRunInput) error {
	requestHash := input.RequestHash
	if requestHash == "" {
		requestHash = buildRecommendationCalibrationAlertRequestHash(
			input.Sport,
			input.Mode,
			input.Window,
			input.BucketCount,
			input.Limit,
			input.MinSettledOverall,
			input.MinSettledPerBucket,
			input.Thresholds,
			input.WindowDays,
			nil,
		)
	}
	runGroupHash := input.RunGroupHash
	if runGroupHash == "" {
		runGroupHash = buildRecommendationCalibrationAlertRunGroupHash(requestHash, []recommendationCalibrationAlertRollingStep{
			{Window: input.Window},
		})
	}
	reasonsJSON, err := json.Marshal(input.Drift.Reasons)
	if err != nil {
		return fmt.Errorf("marshal calibration alert reasons: %w", err)
	}
	payloadJSON, err := json.Marshal(struct {
		Sport           string                              `json:"sport"`
		Mode            string                              `json:"mode"`
		StepIndex       int                                 `json:"step_index"`
		StepCount       int                                 `json:"step_count"`
		WindowDays      *int                                `json:"window_days,omitempty"`
		Window          recommendationCalibrationWindow     `json:"window"`
		Thresholds      decision.CalibrationDriftThresholds `json:"thresholds"`
		Guardrails      decision.CalibrationDriftGuardrails `json:"guardrails"`
		CurrentSummary  decision.CalibrationSummary         `json:"current_summary"`
		BaselineSummary decision.CalibrationSummary         `json:"baseline_summary"`
		Drift           decision.CalibrationDriftResult     `json:"drift"`
	}{
		Sport:      input.Sport,
		Mode:       input.Mode,
		StepIndex:  input.StepIndex,
		StepCount:  input.StepCount,
		WindowDays: input.WindowDays,
		Window: recommendationCalibrationWindow{
			CurrentFrom:  nullableDateString(input.Window.CurrentFrom),
			CurrentTo:    nullableDateString(input.Window.CurrentTo),
			BaselineFrom: nullableDateString(input.Window.BaselineFrom),
			BaselineTo:   nullableDateString(input.Window.BaselineTo),
		},
		Thresholds: input.Thresholds,
		Guardrails: decision.CalibrationDriftGuardrails{
			MinSettledOverall:   input.MinSettledOverall,
			MinSettledPerBucket: input.MinSettledPerBucket,
		},
		CurrentSummary:  input.Current.Summary,
		BaselineSummary: input.Baseline.Summary,
		Drift:           input.Drift,
	})
	if err != nil {
		return fmt.Errorf("marshal calibration alert payload: %w", err)
	}

	var windowDays *int32
	if input.WindowDays != nil {
		value := int32(*input.WindowDays)
		windowDays = &value
	}
	if _, err := a.queries.InsertRecommendationCalibrationAlertRun(c.Context(), store.InsertRecommendationCalibrationAlertRunParams{
		Sport:                       input.Sport,
		RequestHash:                 requestHash,
		RunGroupHash:                runGroupHash,
		Mode:                        input.Mode,
		StepIndex:                   int32(input.StepIndex),
		StepCount:                   int32(input.StepCount),
		WindowDays:                  windowDays,
		CurrentFrom:                 optionalDateParam(input.Window.CurrentFrom),
		CurrentTo:                   optionalDateParam(input.Window.CurrentTo),
		BaselineFrom:                optionalDateParam(input.Window.BaselineFrom),
		BaselineTo:                  optionalDateParam(input.Window.BaselineTo),
		BucketCount:                 int32(input.BucketCount),
		RowLimit:                    int32(input.Limit),
		MinSettledOverall:           int32(input.MinSettledOverall),
		MinSettledPerBucket:         int32(input.MinSettledPerBucket),
		WarnEceDelta:                input.Thresholds.WarnECEDelta,
		CriticalEceDelta:            input.Thresholds.CriticalECEDelta,
		WarnBrierDelta:              input.Thresholds.WarnBrierDelta,
		CriticalBrierDelta:          input.Thresholds.CriticalBrierDelta,
		AlertLevel:                  input.Drift.Level,
		Reasons:                     reasonsJSON,
		CurrentOverallEce:           input.Drift.Summary.CurrentOverallECE,
		BaselineOverallEce:          input.Drift.Summary.BaselineOverallECE,
		EceDelta:                    input.Drift.Summary.ECEDelta,
		CurrentOverallBrier:         input.Drift.Summary.CurrentOverallBrier,
		BaselineOverallBrier:        input.Drift.Summary.BaselineOverallBrier,
		BrierDelta:                  input.Drift.Summary.BrierDelta,
		CurrentSettledRows:          int32(input.Drift.Samples.CurrentSettledRows),
		BaselineSettledRows:         int32(input.Drift.Samples.BaselineSettledRows),
		InsufficientOverallWindows:  int32(input.Drift.Samples.InsufficientOverallWindows),
		CurrentInsufficientBuckets:  int32(input.Drift.Samples.CurrentInsufficientBuckets),
		BaselineInsufficientBuckets: int32(input.Drift.Samples.BaselineInsufficientBuckets),
		Payload:                     payloadJSON,
	}); err != nil {
		return fmt.Errorf("insert recommendation calibration alert run: %w", err)
	}
	return nil
}

type recommendationCalibrationWindow struct {
	CurrentFrom  *string `json:"current_from"`
	CurrentTo    *string `json:"current_to"`
	BaselineFrom *string `json:"baseline_from"`
	BaselineTo   *string `json:"baseline_to"`
}

func buildRecommendationCalibrationAlertRequestHash(
	sport string,
	mode string,
	window recommendationCalibrationAlertWindow,
	bucketCount int,
	limit int,
	minSettledOverall int,
	minSettledPerBucket int,
	thresholds decision.CalibrationDriftThresholds,
	windowDays *int,
	steps *int,
) string {
	payload, _ := json.Marshal(struct {
		Sport               string  `json:"sport"`
		Mode                string  `json:"mode"`
		CurrentFrom         *string `json:"current_from"`
		CurrentTo           *string `json:"current_to"`
		BaselineFrom        *string `json:"baseline_from"`
		BaselineTo          *string `json:"baseline_to"`
		BucketCount         int     `json:"bucket_count"`
		Limit               int     `json:"limit"`
		MinSettledOverall   int     `json:"min_settled_overall"`
		MinSettledPerBucket int     `json:"min_settled_per_bucket"`
		WarnECEDelta        float64 `json:"warn_ece_delta"`
		CriticalECEDelta    float64 `json:"critical_ece_delta"`
		WarnBrierDelta      float64 `json:"warn_brier_delta"`
		CriticalBrierDelta  float64 `json:"critical_brier_delta"`
		WindowDays          *int    `json:"window_days,omitempty"`
		Steps               *int    `json:"steps,omitempty"`
	}{
		Sport:               sport,
		Mode:                mode,
		CurrentFrom:         nullableDateString(window.CurrentFrom),
		CurrentTo:           nullableDateString(window.CurrentTo),
		BaselineFrom:        nullableDateString(window.BaselineFrom),
		BaselineTo:          nullableDateString(window.BaselineTo),
		BucketCount:         bucketCount,
		Limit:               limit,
		MinSettledOverall:   minSettledOverall,
		MinSettledPerBucket: minSettledPerBucket,
		WarnECEDelta:        thresholds.WarnECEDelta,
		CriticalECEDelta:    thresholds.CriticalECEDelta,
		WarnBrierDelta:      thresholds.WarnBrierDelta,
		CriticalBrierDelta:  thresholds.CriticalBrierDelta,
		WindowDays:          windowDays,
		Steps:               steps,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func buildRecommendationCalibrationAlertRunGroupHash(requestHash string, steps []recommendationCalibrationAlertRollingStep) string {
	if len(steps) == 0 {
		return requestHash
	}
	first := steps[0].Window
	last := steps[len(steps)-1].Window
	payload, _ := json.Marshal(struct {
		RequestHash      string  `json:"request_hash"`
		FirstCurrentFrom *string `json:"first_current_from"`
		FirstCurrentTo   *string `json:"first_current_to"`
		LastCurrentFrom  *string `json:"last_current_from"`
		LastCurrentTo    *string `json:"last_current_to"`
		StepCount        int     `json:"step_count"`
	}{
		RequestHash:      requestHash,
		FirstCurrentFrom: nullableDateString(first.CurrentFrom),
		FirstCurrentTo:   nullableDateString(first.CurrentTo),
		LastCurrentFrom:  nullableDateString(last.CurrentFrom),
		LastCurrentTo:    nullableDateString(last.CurrentTo),
		StepCount:        len(steps),
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func parseRecommendationsCalibrationAlertMode(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decision.CalibrationAlertModePointInTime, nil
	}
	switch trimmed {
	case decision.CalibrationAlertModePointInTime, decision.CalibrationAlertModeRolling:
		return trimmed, nil
	default:
		return "", fmt.Errorf("invalid mode %q; expected one of point_in_time|rolling", raw)
	}
}

func parseRecommendationsRollingWindowDays(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decision.DefaultCalibrationRollingWindowDays, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid window_days %q; expected integer in [1,%d]", raw, decision.MaxCalibrationRollingWindowDays)
	}
	if value < 1 || value > decision.MaxCalibrationRollingWindowDays {
		return 0, fmt.Errorf("invalid window_days %q; expected integer in [1,%d]", raw, decision.MaxCalibrationRollingWindowDays)
	}
	return value, nil
}

func parseRecommendationsRollingSteps(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decision.DefaultCalibrationRollingSteps, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid steps %q; expected integer in [1,%d]", raw, decision.MaxCalibrationRollingSteps)
	}
	if value < 1 || value > decision.MaxCalibrationRollingSteps {
		return 0, fmt.Errorf("invalid steps %q; expected integer in [1,%d]", raw, decision.MaxCalibrationRollingSteps)
	}
	return value, nil
}

func parseRecommendationsCalibrationAlertHistoryLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRecommendationsCalibrationAlertHistoryLimit, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsCalibrationAlertHistoryLimit)
	}
	if value < 1 || value > maxRecommendationsCalibrationAlertHistoryLimit {
		return 0, fmt.Errorf("invalid limit %q; expected integer in [1,%d]", raw, maxRecommendationsCalibrationAlertHistoryLimit)
	}
	return value, nil
}

func intPtrFromInt32(value *int32) *int {
	if value == nil {
		return nil
	}
	copied := int(*value)
	return &copied
}

func nullableDateStringFromPg(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format("2006-01-02")
	return &formatted
}

func timePtr(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}
