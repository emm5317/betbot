package decision

import (
	"fmt"
	"math"
)

const (
	defaultDailyLossStop   = 0.05
	defaultWeeklyLossStop  = 0.10
	defaultDrawdownBreaker = 0.15

	circuitCheckReasonPass                   = "pass"
	circuitCheckReasonRetainedZeroStake      = "retained_zero_stake"
	circuitCheckReasonDroppedDailyLossStop   = "dropped_daily_loss_stop"
	circuitCheckReasonDroppedWeeklyLossStop  = "dropped_weekly_loss_stop"
	circuitCheckReasonDroppedDrawdownBreaker = "dropped_drawdown_breaker"
)

type CircuitBreakerPolicy struct {
	DailyLossStop   float64 `json:"daily_loss_stop"`
	WeeklyLossStop  float64 `json:"weekly_loss_stop"`
	DrawdownBreaker float64 `json:"drawdown_breaker"`
}

type CircuitBreakerMetrics struct {
	CurrentBalanceCents   int64 `json:"current_balance_cents"`
	DayStartBalanceCents  int64 `json:"day_start_balance_cents"`
	WeekStartBalanceCents int64 `json:"week_start_balance_cents"`
	PeakBalanceCents      int64 `json:"peak_balance_cents"`
}

type CircuitBreakerStatus struct {
	Pass                bool     `json:"pass"`
	Reason              string   `json:"reason"`
	TriggeredReasonCode []string `json:"triggered_reason_code"`
	DailyLossFraction   float64  `json:"daily_loss_fraction"`
	WeeklyLossFraction  float64  `json:"weekly_loss_fraction"`
	DrawdownFraction    float64  `json:"drawdown_fraction"`
}

type CircuitBreakerResult struct {
	Status   CircuitBreakerStatus `json:"status"`
	Retained []Recommendation     `json:"retained"`
	Dropped  []Recommendation     `json:"dropped"`
}

func DefaultCircuitBreakerPolicy() CircuitBreakerPolicy {
	return CircuitBreakerPolicy{
		DailyLossStop:   defaultDailyLossStop,
		WeeklyLossStop:  defaultWeeklyLossStop,
		DrawdownBreaker: defaultDrawdownBreaker,
	}
}

func ResolveCircuitBreakerPolicy(dailyLossStop, weeklyLossStop, drawdownBreaker float64) (CircuitBreakerPolicy, error) {
	policy := DefaultCircuitBreakerPolicy()
	if err := validateCircuitBreakerOverrideValue(dailyLossStop, "daily loss stop"); err != nil {
		return CircuitBreakerPolicy{}, err
	}
	if err := validateCircuitBreakerOverrideValue(weeklyLossStop, "weekly loss stop"); err != nil {
		return CircuitBreakerPolicy{}, err
	}
	if err := validateCircuitBreakerOverrideValue(drawdownBreaker, "drawdown breaker"); err != nil {
		return CircuitBreakerPolicy{}, err
	}

	if dailyLossStop > 0 {
		policy.DailyLossStop = dailyLossStop
	}
	if weeklyLossStop > 0 {
		policy.WeeklyLossStop = weeklyLossStop
	}
	if drawdownBreaker > 0 {
		policy.DrawdownBreaker = drawdownBreaker
	}
	return policy, nil
}

func EvaluateCircuitBreakers(policy CircuitBreakerPolicy, metrics CircuitBreakerMetrics) (CircuitBreakerStatus, error) {
	if err := validateCircuitBreakerPolicy(policy); err != nil {
		return CircuitBreakerStatus{}, err
	}

	dailyLossFraction := bankrollLossFraction(metrics.DayStartBalanceCents, metrics.CurrentBalanceCents)
	weeklyLossFraction := bankrollLossFraction(metrics.WeekStartBalanceCents, metrics.CurrentBalanceCents)
	drawdownFraction := bankrollLossFraction(metrics.PeakBalanceCents, metrics.CurrentBalanceCents)

	status := CircuitBreakerStatus{
		Pass:               true,
		Reason:             circuitCheckReasonPass,
		DailyLossFraction:  dailyLossFraction,
		WeeklyLossFraction: weeklyLossFraction,
		DrawdownFraction:   drawdownFraction,
	}
	if policy.DailyLossStop > 0 && dailyLossFraction >= policy.DailyLossStop {
		status.TriggeredReasonCode = append(status.TriggeredReasonCode, circuitCheckReasonDroppedDailyLossStop)
	}
	if policy.WeeklyLossStop > 0 && weeklyLossFraction >= policy.WeeklyLossStop {
		status.TriggeredReasonCode = append(status.TriggeredReasonCode, circuitCheckReasonDroppedWeeklyLossStop)
	}
	if policy.DrawdownBreaker > 0 && drawdownFraction >= policy.DrawdownBreaker {
		status.TriggeredReasonCode = append(status.TriggeredReasonCode, circuitCheckReasonDroppedDrawdownBreaker)
	}
	if len(status.TriggeredReasonCode) > 0 {
		status.Pass = false
		status.Reason = status.TriggeredReasonCode[0]
	}

	return status, nil
}

func ApplyCircuitBreakerGuard(recommendations []Recommendation, policy CircuitBreakerPolicy, metrics CircuitBreakerMetrics) (CircuitBreakerResult, error) {
	status, err := EvaluateCircuitBreakers(policy, metrics)
	if err != nil {
		return CircuitBreakerResult{}, err
	}

	retained := make([]Recommendation, 0, len(recommendations))
	dropped := make([]Recommendation, 0, len(recommendations))
	for i := range recommendations {
		rec := recommendations[i]
		rec.CircuitCheckPass = true
		rec.CircuitCheckReason = circuitCheckReasonPass

		if rec.SuggestedStakeCents <= 0 {
			rec.CircuitCheckReason = circuitCheckReasonRetainedZeroStake
			retained = append(retained, rec)
			continue
		}

		if status.Pass {
			retained = append(retained, rec)
			continue
		}

		rec.CircuitCheckPass = false
		rec.CircuitCheckReason = status.Reason
		dropped = append(dropped, rec)
	}

	return CircuitBreakerResult{
		Status:   status,
		Retained: retained,
		Dropped:  dropped,
	}, nil
}

func validateCircuitBreakerOverrideValue(value float64, name string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be finite in [0,1]", name)
	}
	return nil
}

func validateCircuitBreakerPolicy(policy CircuitBreakerPolicy) error {
	if policy.DailyLossStop < 0 || policy.DailyLossStop > 1 || math.IsNaN(policy.DailyLossStop) || math.IsInf(policy.DailyLossStop, 0) {
		return fmt.Errorf("daily loss stop must be finite in [0,1]")
	}
	if policy.WeeklyLossStop < 0 || policy.WeeklyLossStop > 1 || math.IsNaN(policy.WeeklyLossStop) || math.IsInf(policy.WeeklyLossStop, 0) {
		return fmt.Errorf("weekly loss stop must be finite in [0,1]")
	}
	if policy.DrawdownBreaker < 0 || policy.DrawdownBreaker > 1 || math.IsNaN(policy.DrawdownBreaker) || math.IsInf(policy.DrawdownBreaker, 0) {
		return fmt.Errorf("drawdown breaker must be finite in [0,1]")
	}
	return nil
}

func bankrollLossFraction(startBalanceCents int64, currentBalanceCents int64) float64 {
	if currentBalanceCents >= startBalanceCents {
		return 0
	}
	if startBalanceCents <= 0 {
		return 1
	}
	lossCents := startBalanceCents - currentBalanceCents
	return float64(lossCents) / float64(startBalanceCents)
}
