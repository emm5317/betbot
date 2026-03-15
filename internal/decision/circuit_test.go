package decision

import (
	"math"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestEvaluateCircuitBreakersPassesWhenNoThresholdBreached(t *testing.T) {
	status, err := EvaluateCircuitBreakers(
		CircuitBreakerPolicy{DailyLossStop: 0.05, WeeklyLossStop: 0.10, DrawdownBreaker: 0.20},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   98000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 105000,
			PeakBalanceCents:      110000,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCircuitBreakers() error = %v", err)
	}
	if !status.Pass {
		t.Fatalf("Pass = %t, want true", status.Pass)
	}
	if status.Reason != circuitCheckReasonPass {
		t.Fatalf("Reason = %q, want %q", status.Reason, circuitCheckReasonPass)
	}
	if len(status.TriggeredReasonCode) != 0 {
		t.Fatalf("TriggeredReasonCode = %v, want []", status.TriggeredReasonCode)
	}
}

func TestEvaluateCircuitBreakersDailyLossStopTriggersOnEquality(t *testing.T) {
	status, err := EvaluateCircuitBreakers(
		CircuitBreakerPolicy{DailyLossStop: 0.05, WeeklyLossStop: 0.20, DrawdownBreaker: 0.20},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   95000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      100000,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCircuitBreakers() error = %v", err)
	}
	if status.Pass {
		t.Fatal("Pass = true, want false")
	}
	if status.Reason != circuitCheckReasonDroppedDailyLossStop {
		t.Fatalf("Reason = %q, want %q", status.Reason, circuitCheckReasonDroppedDailyLossStop)
	}
}

func TestEvaluateCircuitBreakersWeeklyLossStopTriggers(t *testing.T) {
	status, err := EvaluateCircuitBreakers(
		CircuitBreakerPolicy{DailyLossStop: 0.05, WeeklyLossStop: 0.10, DrawdownBreaker: 0.20},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   88000,
			DayStartBalanceCents:  90000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      100000,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCircuitBreakers() error = %v", err)
	}
	if status.Pass {
		t.Fatal("Pass = true, want false")
	}
	if status.Reason != circuitCheckReasonDroppedWeeklyLossStop {
		t.Fatalf("Reason = %q, want %q", status.Reason, circuitCheckReasonDroppedWeeklyLossStop)
	}
}

func TestEvaluateCircuitBreakersDrawdownTriggers(t *testing.T) {
	status, err := EvaluateCircuitBreakers(
		CircuitBreakerPolicy{DailyLossStop: 0.50, WeeklyLossStop: 0.50, DrawdownBreaker: 0.10},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   90000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      101000,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCircuitBreakers() error = %v", err)
	}
	if status.Pass {
		t.Fatal("Pass = true, want false")
	}
	if status.Reason != circuitCheckReasonDroppedDrawdownBreaker {
		t.Fatalf("Reason = %q, want %q", status.Reason, circuitCheckReasonDroppedDrawdownBreaker)
	}
}

func TestEvaluateCircuitBreakersDeterministicReasonPrecedence(t *testing.T) {
	status, err := EvaluateCircuitBreakers(
		CircuitBreakerPolicy{DailyLossStop: 0.01, WeeklyLossStop: 0.01, DrawdownBreaker: 0.01},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   90000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      120000,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateCircuitBreakers() error = %v", err)
	}
	expected := []string{
		circuitCheckReasonDroppedDailyLossStop,
		circuitCheckReasonDroppedWeeklyLossStop,
		circuitCheckReasonDroppedDrawdownBreaker,
	}
	if len(status.TriggeredReasonCode) != len(expected) {
		t.Fatalf("len(TriggeredReasonCode) = %d, want %d", len(status.TriggeredReasonCode), len(expected))
	}
	for i := range expected {
		if status.TriggeredReasonCode[i] != expected[i] {
			t.Fatalf("TriggeredReasonCode[%d] = %q, want %q", i, status.TriggeredReasonCode[i], expected[i])
		}
	}
	if status.Reason != circuitCheckReasonDroppedDailyLossStop {
		t.Fatalf("Reason = %q, want %q", status.Reason, circuitCheckReasonDroppedDailyLossStop)
	}
}

func TestApplyCircuitBreakerGuardDropsPositiveStakeAndRetainsZeroStake(t *testing.T) {
	result, err := ApplyCircuitBreakerGuard(
		[]Recommendation{
			{
				Sport:               domain.SportMLB,
				GameID:              1,
				Market:              "h2h",
				EventTime:           time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
				SuggestedStakeCents: 0,
			},
			{
				Sport:               domain.SportMLB,
				GameID:              2,
				Market:              "h2h",
				EventTime:           time.Date(2026, time.March, 16, 19, 0, 0, 0, time.UTC),
				SuggestedStakeCents: 1000,
			},
		},
		CircuitBreakerPolicy{DailyLossStop: 0.05, WeeklyLossStop: 0.10, DrawdownBreaker: 0.15},
		CircuitBreakerMetrics{
			CurrentBalanceCents:   80000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      100000,
		},
	)
	if err != nil {
		t.Fatalf("ApplyCircuitBreakerGuard() error = %v", err)
	}
	if len(result.Retained) != 1 || len(result.Dropped) != 1 {
		t.Fatalf("retained=%d dropped=%d, want 1 and 1", len(result.Retained), len(result.Dropped))
	}
	if result.Retained[0].CircuitCheckReason != circuitCheckReasonRetainedZeroStake {
		t.Fatalf("retained reason = %q, want %q", result.Retained[0].CircuitCheckReason, circuitCheckReasonRetainedZeroStake)
	}
	if result.Dropped[0].CircuitCheckReason != circuitCheckReasonDroppedDailyLossStop {
		t.Fatalf("dropped reason = %q, want %q", result.Dropped[0].CircuitCheckReason, circuitCheckReasonDroppedDailyLossStop)
	}
}

func TestResolveCircuitBreakerPolicyGuardrails(t *testing.T) {
	if _, err := ResolveCircuitBreakerPolicy(math.NaN(), 0, 0); err == nil {
		t.Fatal("expected invalid daily loss stop error")
	}
	if _, err := ResolveCircuitBreakerPolicy(0, -0.01, 0); err == nil {
		t.Fatal("expected invalid weekly loss stop error")
	}
	if _, err := ResolveCircuitBreakerPolicy(0, 0, 2); err == nil {
		t.Fatal("expected invalid drawdown breaker error")
	}
	policy, err := ResolveCircuitBreakerPolicy(0, 0, 0)
	if err != nil {
		t.Fatalf("ResolveCircuitBreakerPolicy() default error = %v", err)
	}
	if policy.DailyLossStop != defaultDailyLossStop || policy.WeeklyLossStop != defaultWeeklyLossStop || policy.DrawdownBreaker != defaultDrawdownBreaker {
		t.Fatalf("policy defaults = %+v", policy)
	}
}
