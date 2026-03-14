package backtest

type GuardrailConfig struct {
	MinimumSamples     int
	MinimumPositiveCLV float64
	MaximumCalibMAE    float64
	MaximumCalibBrier  float64
}

type GuardrailResult struct {
	Pass     bool     `json:"pass"`
	Findings []string `json:"findings"`
}

func EvaluateGuardrails(artifact PipelineArtifact, cfg GuardrailConfig) GuardrailResult {
	findings := make([]string, 0)
	if cfg.MinimumSamples > 0 && artifact.CLV.Samples < cfg.MinimumSamples {
		findings = append(findings, "insufficient_samples")
	}
	if cfg.MinimumPositiveCLV > 0 && artifact.CLV.PositiveCLVRate < cfg.MinimumPositiveCLV {
		findings = append(findings, "positive_clv_rate_below_threshold")
	}
	if cfg.MaximumCalibMAE > 0 && artifact.Calibration.MeanAbsoluteError > cfg.MaximumCalibMAE {
		findings = append(findings, "calibration_mae_above_threshold")
	}
	if cfg.MaximumCalibBrier > 0 && artifact.Calibration.BrierScore > cfg.MaximumCalibBrier {
		findings = append(findings, "calibration_brier_above_threshold")
	}

	return GuardrailResult{Pass: len(findings) == 0, Findings: findings}
}
