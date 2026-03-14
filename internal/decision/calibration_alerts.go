package decision

import (
	"fmt"
	"math"
)

const (
	CalibrationDriftLevelOK                 = "ok"
	CalibrationDriftLevelWarn               = "warn"
	CalibrationDriftLevelCritical           = "critical"
	CalibrationDriftLevelInsufficientSample = "insufficient_sample"

	DefaultMinSettledOverall   = 100
	DefaultMinSettledPerBucket = 20

	DefaultWarnECEDelta       = 0.02
	DefaultCriticalECEDelta   = 0.05
	DefaultWarnBrierDelta     = 0.01
	DefaultCriticalBrierDelta = 0.02
)

type CalibrationDriftThresholds struct {
	WarnECEDelta       float64
	CriticalECEDelta   float64
	WarnBrierDelta     float64
	CriticalBrierDelta float64
}

type CalibrationDriftGuardrails struct {
	MinSettledOverall   int
	MinSettledPerBucket int
}

type CalibrationDriftInput struct {
	Sport      string
	Baseline   CalibrationReport
	Current    CalibrationReport
	Thresholds CalibrationDriftThresholds
	Guardrails CalibrationDriftGuardrails
}

type CalibrationDriftSummary struct {
	CurrentOverallECE    float64 `json:"current_overall_ece"`
	BaselineOverallECE   float64 `json:"baseline_overall_ece"`
	ECEDelta             float64 `json:"ece_delta"`
	CurrentOverallBrier  float64 `json:"current_overall_brier"`
	BaselineOverallBrier float64 `json:"baseline_overall_brier"`
	BrierDelta           float64 `json:"brier_delta"`
}

type CalibrationDriftBucket struct {
	BucketIndex              int     `json:"bucket_index"`
	SettledCountCurrent      int     `json:"settled_count_current"`
	SettledCountBaseline     int     `json:"settled_count_baseline"`
	ObservedWinRateCurrent   float64 `json:"observed_win_rate_current"`
	ObservedWinRateBaseline  float64 `json:"observed_win_rate_baseline"`
	ExpectedWinRateCurrent   float64 `json:"expected_win_rate_current"`
	ExpectedWinRateBaseline  float64 `json:"expected_win_rate_baseline"`
	CalibrationGapCurrent    float64 `json:"calibration_gap_current"`
	CalibrationGapBaseline   float64 `json:"calibration_gap_baseline"`
	CalibrationGapDelta      float64 `json:"calibration_gap_delta"`
	BrierCurrent             float64 `json:"brier_current"`
	BrierBaseline            float64 `json:"brier_baseline"`
	BrierDelta               float64 `json:"brier_delta"`
	InsufficientSampleBucket bool    `json:"insufficient_sample_bucket"`
}

type CalibrationDriftSampleSummary struct {
	MinSettledOverall           int `json:"min_settled_overall"`
	MinSettledPerBucket         int `json:"min_settled_per_bucket"`
	CurrentSettledRows          int `json:"current_settled_rows"`
	BaselineSettledRows         int `json:"baseline_settled_rows"`
	InsufficientOverallWindows  int `json:"insufficient_overall_windows"`
	CurrentInsufficientBuckets  int `json:"current_insufficient_buckets"`
	BaselineInsufficientBuckets int `json:"baseline_insufficient_buckets"`
}

type CalibrationDriftResult struct {
	Sport   string                        `json:"sport"`
	Level   string                        `json:"level"`
	Reasons []string                      `json:"reasons"`
	Summary CalibrationDriftSummary       `json:"summary"`
	Samples CalibrationDriftSampleSummary `json:"samples"`
	Buckets []CalibrationDriftBucket      `json:"buckets"`
}

func EvaluateCalibrationDrift(input CalibrationDriftInput) (CalibrationDriftResult, error) {
	guardrails, err := resolveCalibrationDriftGuardrails(input.Guardrails)
	if err != nil {
		return CalibrationDriftResult{}, err
	}
	thresholds, err := resolveCalibrationDriftThresholds(input.Thresholds)
	if err != nil {
		return CalibrationDriftResult{}, err
	}
	if input.Current.BucketCount != input.Baseline.BucketCount {
		return CalibrationDriftResult{}, fmt.Errorf("bucket count mismatch: current=%d baseline=%d", input.Current.BucketCount, input.Baseline.BucketCount)
	}
	if len(input.Current.Buckets) != len(input.Baseline.Buckets) {
		return CalibrationDriftResult{}, fmt.Errorf("bucket rows mismatch: current=%d baseline=%d", len(input.Current.Buckets), len(input.Baseline.Buckets))
	}

	sampleSummary := CalibrationDriftSampleSummary{
		MinSettledOverall:   guardrails.MinSettledOverall,
		MinSettledPerBucket: guardrails.MinSettledPerBucket,
		CurrentSettledRows:  input.Current.Summary.SettledRows,
		BaselineSettledRows: input.Baseline.Summary.SettledRows,
	}

	buckets := make([]CalibrationDriftBucket, len(input.Current.Buckets))
	for i := range input.Current.Buckets {
		currentBucket := input.Current.Buckets[i]
		baselineBucket := input.Baseline.Buckets[i]
		if currentBucket.BucketIndex != i || baselineBucket.BucketIndex != i {
			return CalibrationDriftResult{}, fmt.Errorf("bucket ordering must be deterministic by index; expected %d got current=%d baseline=%d", i, currentBucket.BucketIndex, baselineBucket.BucketIndex)
		}

		insufficient := currentBucket.SettledCount < guardrails.MinSettledPerBucket || baselineBucket.SettledCount < guardrails.MinSettledPerBucket
		if currentBucket.SettledCount < guardrails.MinSettledPerBucket {
			sampleSummary.CurrentInsufficientBuckets++
		}
		if baselineBucket.SettledCount < guardrails.MinSettledPerBucket {
			sampleSummary.BaselineInsufficientBuckets++
		}

		buckets[i] = CalibrationDriftBucket{
			BucketIndex:              i,
			SettledCountCurrent:      currentBucket.SettledCount,
			SettledCountBaseline:     baselineBucket.SettledCount,
			ObservedWinRateCurrent:   currentBucket.ObservedWinRate,
			ObservedWinRateBaseline:  baselineBucket.ObservedWinRate,
			ExpectedWinRateCurrent:   currentBucket.ExpectedWinRate,
			ExpectedWinRateBaseline:  baselineBucket.ExpectedWinRate,
			CalibrationGapCurrent:    currentBucket.CalibrationGap,
			CalibrationGapBaseline:   baselineBucket.CalibrationGap,
			CalibrationGapDelta:      currentBucket.CalibrationGap - baselineBucket.CalibrationGap,
			BrierCurrent:             currentBucket.Brier,
			BrierBaseline:            baselineBucket.Brier,
			BrierDelta:               currentBucket.Brier - baselineBucket.Brier,
			InsufficientSampleBucket: insufficient,
		}
	}

	if input.Current.Summary.SettledRows < guardrails.MinSettledOverall {
		sampleSummary.InsufficientOverallWindows++
	}
	if input.Baseline.Summary.SettledRows < guardrails.MinSettledOverall {
		sampleSummary.InsufficientOverallWindows++
	}

	summary := CalibrationDriftSummary{
		CurrentOverallECE:    input.Current.Summary.OverallECE,
		BaselineOverallECE:   input.Baseline.Summary.OverallECE,
		ECEDelta:             input.Current.Summary.OverallECE - input.Baseline.Summary.OverallECE,
		CurrentOverallBrier:  input.Current.Summary.OverallBrier,
		BaselineOverallBrier: input.Baseline.Summary.OverallBrier,
		BrierDelta:           input.Current.Summary.OverallBrier - input.Baseline.Summary.OverallBrier,
	}

	level := CalibrationDriftLevelOK
	reasons := make([]string, 0, 6)
	if sampleSummary.InsufficientOverallWindows > 0 || sampleSummary.CurrentInsufficientBuckets > 0 || sampleSummary.BaselineInsufficientBuckets > 0 {
		level = CalibrationDriftLevelInsufficientSample
		if input.Current.Summary.SettledRows < guardrails.MinSettledOverall {
			reasons = append(reasons, fmt.Sprintf("current settled rows %d below min_settled_overall %d", input.Current.Summary.SettledRows, guardrails.MinSettledOverall))
		}
		if input.Baseline.Summary.SettledRows < guardrails.MinSettledOverall {
			reasons = append(reasons, fmt.Sprintf("baseline settled rows %d below min_settled_overall %d", input.Baseline.Summary.SettledRows, guardrails.MinSettledOverall))
		}
		if sampleSummary.CurrentInsufficientBuckets > 0 {
			reasons = append(reasons, fmt.Sprintf("current buckets below min_settled_per_bucket: %d", sampleSummary.CurrentInsufficientBuckets))
		}
		if sampleSummary.BaselineInsufficientBuckets > 0 {
			reasons = append(reasons, fmt.Sprintf("baseline buckets below min_settled_per_bucket: %d", sampleSummary.BaselineInsufficientBuckets))
		}
		return CalibrationDriftResult{
			Sport:   input.Sport,
			Level:   level,
			Reasons: reasons,
			Summary: summary,
			Samples: sampleSummary,
			Buckets: buckets,
		}, nil
	}

	absECEDelta := math.Abs(summary.ECEDelta)
	absBrierDelta := math.Abs(summary.BrierDelta)
	eceCritical := absECEDelta >= thresholds.CriticalECEDelta
	brierCritical := absBrierDelta >= thresholds.CriticalBrierDelta
	eceWarn := absECEDelta >= thresholds.WarnECEDelta
	brierWarn := absBrierDelta >= thresholds.WarnBrierDelta

	if eceCritical || brierCritical {
		level = CalibrationDriftLevelCritical
		if eceCritical {
			reasons = append(reasons, fmt.Sprintf("ece delta %.6f exceeded critical threshold %.6f", summary.ECEDelta, thresholds.CriticalECEDelta))
		}
		if brierCritical {
			reasons = append(reasons, fmt.Sprintf("brier delta %.6f exceeded critical threshold %.6f", summary.BrierDelta, thresholds.CriticalBrierDelta))
		}
	} else if eceWarn || brierWarn {
		level = CalibrationDriftLevelWarn
		if eceWarn {
			reasons = append(reasons, fmt.Sprintf("ece delta %.6f exceeded warn threshold %.6f", summary.ECEDelta, thresholds.WarnECEDelta))
		}
		if brierWarn {
			reasons = append(reasons, fmt.Sprintf("brier delta %.6f exceeded warn threshold %.6f", summary.BrierDelta, thresholds.WarnBrierDelta))
		}
	} else {
		reasons = append(reasons, "calibration drift within configured thresholds")
	}

	return CalibrationDriftResult{
		Sport:   input.Sport,
		Level:   level,
		Reasons: reasons,
		Summary: summary,
		Samples: sampleSummary,
		Buckets: buckets,
	}, nil
}

func resolveCalibrationDriftGuardrails(input CalibrationDriftGuardrails) (CalibrationDriftGuardrails, error) {
	minOverall := input.MinSettledOverall
	if minOverall == 0 {
		minOverall = DefaultMinSettledOverall
	}
	if minOverall < 1 {
		return CalibrationDriftGuardrails{}, fmt.Errorf("invalid min_settled_overall %d; expected integer >= 1", input.MinSettledOverall)
	}

	minPerBucket := input.MinSettledPerBucket
	if minPerBucket == 0 {
		minPerBucket = DefaultMinSettledPerBucket
	}
	if minPerBucket < 1 {
		return CalibrationDriftGuardrails{}, fmt.Errorf("invalid min_settled_per_bucket %d; expected integer >= 1", input.MinSettledPerBucket)
	}

	return CalibrationDriftGuardrails{
		MinSettledOverall:   minOverall,
		MinSettledPerBucket: minPerBucket,
	}, nil
}

func resolveCalibrationDriftThresholds(input CalibrationDriftThresholds) (CalibrationDriftThresholds, error) {
	thresholds := CalibrationDriftThresholds{
		WarnECEDelta:       input.WarnECEDelta,
		CriticalECEDelta:   input.CriticalECEDelta,
		WarnBrierDelta:     input.WarnBrierDelta,
		CriticalBrierDelta: input.CriticalBrierDelta,
	}
	if thresholds.WarnECEDelta == 0 {
		thresholds.WarnECEDelta = DefaultWarnECEDelta
	}
	if thresholds.CriticalECEDelta == 0 {
		thresholds.CriticalECEDelta = DefaultCriticalECEDelta
	}
	if thresholds.WarnBrierDelta == 0 {
		thresholds.WarnBrierDelta = DefaultWarnBrierDelta
	}
	if thresholds.CriticalBrierDelta == 0 {
		thresholds.CriticalBrierDelta = DefaultCriticalBrierDelta
	}

	values := map[string]float64{
		"warn_ece_delta":       thresholds.WarnECEDelta,
		"critical_ece_delta":   thresholds.CriticalECEDelta,
		"warn_brier_delta":     thresholds.WarnBrierDelta,
		"critical_brier_delta": thresholds.CriticalBrierDelta,
	}
	for name, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return CalibrationDriftThresholds{}, fmt.Errorf("invalid %s %.6f; expected finite value in [0,1]", name, value)
		}
	}

	if thresholds.WarnECEDelta > thresholds.CriticalECEDelta {
		return CalibrationDriftThresholds{}, fmt.Errorf("invalid thresholds: warn_ece_delta %.6f exceeds critical_ece_delta %.6f", thresholds.WarnECEDelta, thresholds.CriticalECEDelta)
	}
	if thresholds.WarnBrierDelta > thresholds.CriticalBrierDelta {
		return CalibrationDriftThresholds{}, fmt.Errorf("invalid thresholds: warn_brier_delta %.6f exceeds critical_brier_delta %.6f", thresholds.WarnBrierDelta, thresholds.CriticalBrierDelta)
	}

	return thresholds, nil
}
