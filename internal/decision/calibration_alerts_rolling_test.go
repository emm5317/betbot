package decision

import (
	"testing"
	"time"
)

func TestBuildRollingCalibrationDriftWindows(t *testing.T) {
	anchor := time.Date(2026, time.March, 14, 18, 30, 0, 0, time.UTC)

	windows, err := BuildRollingCalibrationDriftWindows(anchor, 7, 3)
	if err != nil {
		t.Fatalf("BuildRollingCalibrationDriftWindows() error = %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("len(windows) = %d, want 3", len(windows))
	}

	if got := windows[0].CurrentFrom.Format("2006-01-02"); got != "2026-03-06" {
		t.Fatalf("windows[0].current_from = %s, want 2026-03-06", got)
	}
	if got := windows[0].CurrentTo.Format("2006-01-02"); got != "2026-03-12" {
		t.Fatalf("windows[0].current_to = %s, want 2026-03-12", got)
	}
	if got := windows[0].BaselineFrom.Format("2006-01-02"); got != "2026-02-27" {
		t.Fatalf("windows[0].baseline_from = %s, want 2026-02-27", got)
	}
	if got := windows[0].BaselineTo.Format("2006-01-02"); got != "2026-03-05" {
		t.Fatalf("windows[0].baseline_to = %s, want 2026-03-05", got)
	}

	if got := windows[2].CurrentFrom.Format("2006-01-02"); got != "2026-03-08" {
		t.Fatalf("windows[2].current_from = %s, want 2026-03-08", got)
	}
	if got := windows[2].CurrentTo.Format("2006-01-02"); got != "2026-03-14" {
		t.Fatalf("windows[2].current_to = %s, want 2026-03-14", got)
	}
}

func TestEvaluateRollingCalibrationDriftDeterministicWindowOrdering(t *testing.T) {
	windowA := CalibrationDriftWindow{
		CurrentFrom:  time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC),
		CurrentTo:    time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC),
		BaselineFrom: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		BaselineTo:   time.Date(2026, time.March, 7, 0, 0, 0, 0, time.UTC),
	}
	windowB := CalibrationDriftWindow{
		CurrentFrom:  time.Date(2026, time.March, 7, 0, 0, 0, 0, time.UTC),
		CurrentTo:    time.Date(2026, time.March, 13, 0, 0, 0, 0, time.UTC),
		BaselineFrom: time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
		BaselineTo:   time.Date(2026, time.March, 6, 0, 0, 0, 0, time.UTC),
	}

	stepA := RollingCalibrationDriftStepInput{
		Window:   windowA,
		Current:  calibrationReportFixture(2, 220, []int{110, 110}, 0.08, 0.25),
		Baseline: calibrationReportFixture(2, 220, []int{110, 110}, 0.04, 0.22),
	}
	stepB := RollingCalibrationDriftStepInput{
		Window:   windowB,
		Current:  calibrationReportFixture(2, 210, []int{105, 105}, 0.07, 0.23),
		Baseline: calibrationReportFixture(2, 210, []int{105, 105}, 0.05, 0.21),
	}

	orderedResult, err := EvaluateRollingCalibrationDrift(RollingCalibrationDriftInput{
		Sport: "MLB",
		Steps: []RollingCalibrationDriftStepInput{stepB, stepA},
	})
	if err != nil {
		t.Fatalf("EvaluateRollingCalibrationDrift(ordered) error = %v", err)
	}
	reversedResult, err := EvaluateRollingCalibrationDrift(RollingCalibrationDriftInput{
		Sport: "MLB",
		Steps: []RollingCalibrationDriftStepInput{stepA, stepB},
	})
	if err != nil {
		t.Fatalf("EvaluateRollingCalibrationDrift(reversed) error = %v", err)
	}

	if len(orderedResult.Steps) != 2 || len(reversedResult.Steps) != 2 {
		t.Fatalf("unexpected result lengths: %d and %d", len(orderedResult.Steps), len(reversedResult.Steps))
	}
	for i := range orderedResult.Steps {
		left := orderedResult.Steps[i]
		right := reversedResult.Steps[i]
		if !left.Window.CurrentTo.Equal(right.Window.CurrentTo) {
			t.Fatalf("step[%d] current_to mismatch: %s vs %s", i, left.Window.CurrentTo, right.Window.CurrentTo)
		}
		if left.Drift.Level != right.Drift.Level {
			t.Fatalf("step[%d] level mismatch: %q vs %q", i, left.Drift.Level, right.Drift.Level)
		}
	}
}

func TestEvaluateRollingCalibrationDriftPropagatesGuardrailsPerStep(t *testing.T) {
	windows, err := BuildRollingCalibrationDriftWindows(time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC), 7, 2)
	if err != nil {
		t.Fatalf("BuildRollingCalibrationDriftWindows() error = %v", err)
	}

	steps := []RollingCalibrationDriftStepInput{
		{
			Window:   windows[0],
			Current:  calibrationReportFixture(2, 80, []int{10, 15}, 0.11, 0.22),
			Baseline: calibrationReportFixture(2, 90, []int{12, 9}, 0.09, 0.20),
		},
		{
			Window:   windows[1],
			Current:  calibrationReportFixture(2, 70, []int{8, 14}, 0.12, 0.23),
			Baseline: calibrationReportFixture(2, 95, []int{18, 11}, 0.10, 0.21),
		},
	}

	result, err := EvaluateRollingCalibrationDrift(RollingCalibrationDriftInput{
		Sport: "NBA",
		Steps: steps,
		Guardrails: CalibrationDriftGuardrails{
			MinSettledOverall:   100,
			MinSettledPerBucket: 20,
		},
	})
	if err != nil {
		t.Fatalf("EvaluateRollingCalibrationDrift() error = %v", err)
	}

	for i := range result.Steps {
		step := result.Steps[i]
		if step.Drift.Level != CalibrationDriftLevelInsufficientSample {
			t.Fatalf("step[%d].level = %q, want %q", i, step.Drift.Level, CalibrationDriftLevelInsufficientSample)
		}
		if len(step.Drift.Reasons) == 0 {
			t.Fatalf("step[%d].reasons should not be empty", i)
		}
	}
}

func TestEvaluateRollingCalibrationDriftDeterministicReasonsPerStep(t *testing.T) {
	windows, err := BuildRollingCalibrationDriftWindows(time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC), 7, 1)
	if err != nil {
		t.Fatalf("BuildRollingCalibrationDriftWindows() error = %v", err)
	}

	step := RollingCalibrationDriftStepInput{
		Window:   windows[0],
		Current:  calibrationReportFixture(2, 220, []int{110, 110}, 0.14, 0.27),
		Baseline: calibrationReportFixture(2, 220, []int{110, 110}, 0.07, 0.23),
	}

	result, err := EvaluateRollingCalibrationDrift(RollingCalibrationDriftInput{
		Sport: "NHL",
		Steps: []RollingCalibrationDriftStepInput{step},
	})
	if err != nil {
		t.Fatalf("EvaluateRollingCalibrationDrift() error = %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(result.Steps))
	}
	wantReasons := []string{
		"ece delta 0.070000 exceeded critical threshold 0.050000",
		"brier delta 0.040000 exceeded critical threshold 0.020000",
	}
	gotReasons := result.Steps[0].Drift.Reasons
	if len(gotReasons) != len(wantReasons) {
		t.Fatalf("len(reasons) = %d, want %d", len(gotReasons), len(wantReasons))
	}
	for i := range wantReasons {
		if gotReasons[i] != wantReasons[i] {
			t.Fatalf("reason[%d] = %q, want %q", i, gotReasons[i], wantReasons[i])
		}
	}
}
