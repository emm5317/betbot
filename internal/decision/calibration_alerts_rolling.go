package decision

import (
	"fmt"
	"sort"
	"time"
)

const (
	CalibrationAlertModePointInTime = "point_in_time"
	CalibrationAlertModeRolling     = "rolling"

	DefaultCalibrationRollingWindowDays = 14
	DefaultCalibrationRollingSteps      = 5
	MaxCalibrationRollingWindowDays     = 365
	MaxCalibrationRollingSteps          = 90
)

type CalibrationDriftWindow struct {
	CurrentFrom  time.Time
	CurrentTo    time.Time
	BaselineFrom time.Time
	BaselineTo   time.Time
}

type RollingCalibrationDriftStepInput struct {
	Window   CalibrationDriftWindow
	Current  CalibrationReport
	Baseline CalibrationReport
}

type RollingCalibrationDriftInput struct {
	Sport      string
	Thresholds CalibrationDriftThresholds
	Guardrails CalibrationDriftGuardrails
	Steps      []RollingCalibrationDriftStepInput
}

type RollingCalibrationDriftStepResult struct {
	StepIndex int
	Window    CalibrationDriftWindow
	Drift     CalibrationDriftResult
}

type RollingCalibrationDriftResult struct {
	Steps []RollingCalibrationDriftStepResult
}

func BuildRollingCalibrationDriftWindows(anchor time.Time, windowDays int, steps int) ([]CalibrationDriftWindow, error) {
	if anchor.IsZero() {
		return nil, fmt.Errorf("rolling window anchor date must be set")
	}
	if windowDays < 1 || windowDays > MaxCalibrationRollingWindowDays {
		return nil, fmt.Errorf("invalid rolling window_days %d; expected integer in [1,%d]", windowDays, MaxCalibrationRollingWindowDays)
	}
	if steps < 1 || steps > MaxCalibrationRollingSteps {
		return nil, fmt.Errorf("invalid rolling steps %d; expected integer in [1,%d]", steps, MaxCalibrationRollingSteps)
	}

	anchorDay := normalizeDriftWindowDate(anchor)
	windows := make([]CalibrationDriftWindow, steps)
	for i := 0; i < steps; i++ {
		offset := steps - 1 - i
		currentTo := anchorDay.AddDate(0, 0, -offset)
		currentFrom := currentTo.AddDate(0, 0, -(windowDays - 1))
		baselineTo := currentFrom.AddDate(0, 0, -1)
		baselineFrom := baselineTo.AddDate(0, 0, -(windowDays - 1))

		windows[i] = CalibrationDriftWindow{
			CurrentFrom:  currentFrom,
			CurrentTo:    currentTo,
			BaselineFrom: baselineFrom,
			BaselineTo:   baselineTo,
		}
	}
	return windows, nil
}

func EvaluateRollingCalibrationDrift(input RollingCalibrationDriftInput) (RollingCalibrationDriftResult, error) {
	if len(input.Steps) == 0 {
		return RollingCalibrationDriftResult{}, fmt.Errorf("rolling drift requires at least one step")
	}

	ordered := make([]RollingCalibrationDriftStepInput, len(input.Steps))
	copy(ordered, input.Steps)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].Window
		right := ordered[j].Window
		if !left.CurrentTo.Equal(right.CurrentTo) {
			return left.CurrentTo.Before(right.CurrentTo)
		}
		if !left.CurrentFrom.Equal(right.CurrentFrom) {
			return left.CurrentFrom.Before(right.CurrentFrom)
		}
		if !left.BaselineTo.Equal(right.BaselineTo) {
			return left.BaselineTo.Before(right.BaselineTo)
		}
		return left.BaselineFrom.Before(right.BaselineFrom)
	})

	result := RollingCalibrationDriftResult{
		Steps: make([]RollingCalibrationDriftStepResult, 0, len(ordered)),
	}
	for i := range ordered {
		stepInput := ordered[i]
		drift, err := EvaluateCalibrationDrift(CalibrationDriftInput{
			Sport:      input.Sport,
			Current:    stepInput.Current,
			Baseline:   stepInput.Baseline,
			Thresholds: input.Thresholds,
			Guardrails: input.Guardrails,
		})
		if err != nil {
			return RollingCalibrationDriftResult{}, fmt.Errorf("evaluate rolling drift step %d: %w", i, err)
		}
		result.Steps = append(result.Steps, RollingCalibrationDriftStepResult{
			StepIndex: i,
			Window: CalibrationDriftWindow{
				CurrentFrom:  normalizeDriftWindowDate(stepInput.Window.CurrentFrom),
				CurrentTo:    normalizeDriftWindowDate(stepInput.Window.CurrentTo),
				BaselineFrom: normalizeDriftWindowDate(stepInput.Window.BaselineFrom),
				BaselineTo:   normalizeDriftWindowDate(stepInput.Window.BaselineTo),
			},
			Drift: drift,
		})
	}
	return result, nil
}

func normalizeDriftWindowDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}
