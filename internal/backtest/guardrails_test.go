package backtest

import "testing"

func TestEvaluateGuardrails(t *testing.T) {
	artifact := PipelineArtifact{
		CLV: CLVReport{Samples: 10, PositiveCLVRate: 0.45},
		Calibration: CalibrationReport{
			MeanAbsoluteError: 0.06,
			BrierScore:        0.01,
		},
	}

	result := EvaluateGuardrails(artifact, GuardrailConfig{
		MinimumSamples:     20,
		MinimumPositiveCLV: 0.50,
		MaximumCalibMAE:    0.05,
	})
	if result.Pass {
		t.Fatal("expected guardrail failure")
	}
	if len(result.Findings) != 3 {
		t.Fatalf("finding count = %d, want 3", len(result.Findings))
	}
}
