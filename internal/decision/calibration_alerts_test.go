package decision

import (
	"math"
	"testing"
)

func TestEvaluateCalibrationDriftInsufficientSample(t *testing.T) {
	current := calibrationReportFixture(2, 90, []int{15, 9}, 0.14, 0.23)
	baseline := calibrationReportFixture(2, 80, []int{8, 22}, 0.11, 0.21)

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "MLB",
		Current:  current,
		Baseline: baseline,
		Guardrails: CalibrationDriftGuardrails{
			MinSettledOverall:   100,
			MinSettledPerBucket: 20,
		},
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if result.Level != CalibrationDriftLevelInsufficientSample {
		t.Fatalf("Level = %q, want %q", result.Level, CalibrationDriftLevelInsufficientSample)
	}
	if result.Samples.InsufficientOverallWindows != 2 {
		t.Fatalf("InsufficientOverallWindows = %d, want 2", result.Samples.InsufficientOverallWindows)
	}
	if result.Samples.CurrentInsufficientBuckets != 2 {
		t.Fatalf("CurrentInsufficientBuckets = %d, want 2", result.Samples.CurrentInsufficientBuckets)
	}
	if result.Samples.BaselineInsufficientBuckets != 1 {
		t.Fatalf("BaselineInsufficientBuckets = %d, want 1", result.Samples.BaselineInsufficientBuckets)
	}
	wantReasons := []string{
		"current settled rows 90 below min_settled_overall 100",
		"baseline settled rows 80 below min_settled_overall 100",
		"current buckets below min_settled_per_bucket: 2",
		"baseline buckets below min_settled_per_bucket: 1",
	}
	if len(result.Reasons) != len(wantReasons) {
		t.Fatalf("len(reasons) = %d, want %d: %+v", len(result.Reasons), len(wantReasons), result.Reasons)
	}
	for i := range wantReasons {
		if result.Reasons[i] != wantReasons[i] {
			t.Fatalf("reason[%d] = %q, want %q", i, result.Reasons[i], wantReasons[i])
		}
	}
}

func TestEvaluateCalibrationDriftWarn(t *testing.T) {
	current := calibrationReportFixture(2, 180, []int{90, 90}, 0.09, 0.20)
	baseline := calibrationReportFixture(2, 160, []int{80, 80}, 0.06, 0.195)

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "NHL",
		Current:  current,
		Baseline: baseline,
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if result.Level != CalibrationDriftLevelWarn {
		t.Fatalf("Level = %q, want %q", result.Level, CalibrationDriftLevelWarn)
	}
	if len(result.Reasons) != 1 {
		t.Fatalf("len(reasons) = %d, want 1", len(result.Reasons))
	}
	if result.Reasons[0] != "ece delta 0.030000 exceeded warn threshold 0.020000" {
		t.Fatalf("reason = %q", result.Reasons[0])
	}
}

func TestEvaluateCalibrationDriftCritical(t *testing.T) {
	current := calibrationReportFixture(2, 220, []int{110, 110}, 0.04, 0.28)
	baseline := calibrationReportFixture(2, 210, []int{105, 105}, 0.03, 0.24)

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "NFL",
		Current:  current,
		Baseline: baseline,
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if result.Level != CalibrationDriftLevelCritical {
		t.Fatalf("Level = %q, want %q", result.Level, CalibrationDriftLevelCritical)
	}
	if len(result.Reasons) != 1 {
		t.Fatalf("len(reasons) = %d, want 1", len(result.Reasons))
	}
	if result.Reasons[0] != "brier delta 0.040000 exceeded critical threshold 0.020000" {
		t.Fatalf("reason = %q", result.Reasons[0])
	}
}

func TestEvaluateCalibrationDriftNoAlert(t *testing.T) {
	current := calibrationReportFixture(3, 240, []int{80, 80, 80}, 0.07, 0.18)
	baseline := calibrationReportFixture(3, 230, []int{78, 76, 76}, 0.065, 0.175)

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "NBA",
		Current:  current,
		Baseline: baseline,
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if result.Level != CalibrationDriftLevelOK {
		t.Fatalf("Level = %q, want %q", result.Level, CalibrationDriftLevelOK)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "calibration drift within configured thresholds" {
		t.Fatalf("reasons = %+v, want in-threshold message", result.Reasons)
	}
}

func TestEvaluateCalibrationDriftDeterministicReasonOrdering(t *testing.T) {
	current := calibrationReportFixture(2, 300, []int{150, 150}, 0.18, 0.31)
	baseline := calibrationReportFixture(2, 320, []int{160, 160}, 0.10, 0.26)

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "MLB",
		Current:  current,
		Baseline: baseline,
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if result.Level != CalibrationDriftLevelCritical {
		t.Fatalf("Level = %q, want %q", result.Level, CalibrationDriftLevelCritical)
	}
	wantReasons := []string{
		"ece delta 0.080000 exceeded critical threshold 0.050000",
		"brier delta 0.050000 exceeded critical threshold 0.020000",
	}
	if len(result.Reasons) != len(wantReasons) {
		t.Fatalf("len(reasons) = %d, want %d", len(result.Reasons), len(wantReasons))
	}
	for i := range wantReasons {
		if result.Reasons[i] != wantReasons[i] {
			t.Fatalf("reason[%d] = %q, want %q", i, result.Reasons[i], wantReasons[i])
		}
	}
}

func TestEvaluateCalibrationDriftDeterministicBucketOrdering(t *testing.T) {
	current := CalibrationReport{
		BucketCount: 3,
		Summary: CalibrationSummary{
			SettledRows:  300,
			OverallECE:   0.09,
			OverallBrier: 0.22,
		},
		Buckets: []CalibrationBucket{
			{BucketIndex: 0, SettledCount: 100, ObservedWinRate: 0.61, ExpectedWinRate: 0.58, CalibrationGap: 0.03, Brier: 0.21},
			{BucketIndex: 1, SettledCount: 100, ObservedWinRate: 0.57, ExpectedWinRate: 0.55, CalibrationGap: 0.02, Brier: 0.22},
			{BucketIndex: 2, SettledCount: 100, ObservedWinRate: 0.54, ExpectedWinRate: 0.53, CalibrationGap: 0.01, Brier: 0.23},
		},
	}
	baseline := CalibrationReport{
		BucketCount: 3,
		Summary: CalibrationSummary{
			SettledRows:  300,
			OverallECE:   0.07,
			OverallBrier: 0.20,
		},
		Buckets: []CalibrationBucket{
			{BucketIndex: 0, SettledCount: 100, ObservedWinRate: 0.59, ExpectedWinRate: 0.58, CalibrationGap: 0.01, Brier: 0.19},
			{BucketIndex: 1, SettledCount: 100, ObservedWinRate: 0.56, ExpectedWinRate: 0.55, CalibrationGap: 0.01, Brier: 0.20},
			{BucketIndex: 2, SettledCount: 100, ObservedWinRate: 0.53, ExpectedWinRate: 0.53, CalibrationGap: 0.00, Brier: 0.21},
		},
	}

	result, err := EvaluateCalibrationDrift(CalibrationDriftInput{
		Sport:    "MLB",
		Current:  current,
		Baseline: baseline,
	})
	if err != nil {
		t.Fatalf("EvaluateCalibrationDrift() error = %v", err)
	}

	if len(result.Buckets) != 3 {
		t.Fatalf("len(buckets) = %d, want 3", len(result.Buckets))
	}
	for i := range result.Buckets {
		if result.Buckets[i].BucketIndex != i {
			t.Fatalf("bucket[%d].BucketIndex = %d, want %d", i, result.Buckets[i].BucketIndex, i)
		}
	}
	if math.Abs(result.Buckets[0].CalibrationGapDelta-0.02) > 1e-9 {
		t.Fatalf("bucket[0].CalibrationGapDelta = %.6f, want 0.020000", result.Buckets[0].CalibrationGapDelta)
	}
	if math.Abs(result.Buckets[0].BrierDelta-0.02) > 1e-9 {
		t.Fatalf("bucket[0].BrierDelta = %.6f, want 0.020000", result.Buckets[0].BrierDelta)
	}
}

func calibrationReportFixture(bucketCount int, settledRows int, perBucketSettled []int, overallECE float64, overallBrier float64) CalibrationReport {
	buckets := make([]CalibrationBucket, bucketCount)
	for i := 0; i < bucketCount; i++ {
		settled := 0
		if i < len(perBucketSettled) {
			settled = perBucketSettled[i]
		}
		buckets[i] = CalibrationBucket{
			BucketIndex:     i,
			SettledCount:    settled,
			ObservedWinRate: 0.5,
			ExpectedWinRate: 0.5,
			CalibrationGap:  0.0,
			Brier:           overallBrier,
		}
	}

	return CalibrationReport{
		BucketCount: bucketCount,
		Summary: CalibrationSummary{
			SettledRows:  settledRows,
			OverallECE:   overallECE,
			OverallBrier: overallBrier,
		},
		Buckets: buckets,
	}
}
