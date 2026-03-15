package decision

import (
	"math"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestApplyCorrelationGuardEnforcesMaxPicksPerGameDeterministically(t *testing.T) {
	policy := CorrelationPolicy{
		MaxPicksPerGame:         1,
		MaxStakeFractionPerGame: 0.10,
	}
	recs := []Recommendation{
		{
			Sport:                  domain.SportMLB,
			GameID:                 11,
			Market:                 "h2h",
			RecommendedSide:        "home",
			EventTime:              time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1000,
			SuggestedStakeFraction: 0.01,
			RankScore:              900,
		},
		{
			Sport:                  domain.SportMLB,
			GameID:                 11,
			Market:                 "spreads",
			RecommendedSide:        "away",
			EventTime:              time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1200,
			SuggestedStakeFraction: 0.012,
			RankScore:              800,
		},
	}

	result, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("ApplyCorrelationGuard() error = %v", err)
	}
	if len(result.Retained) != 1 || len(result.Dropped) != 1 {
		t.Fatalf("retained=%d dropped=%d, want 1 and 1", len(result.Retained), len(result.Dropped))
	}
	if result.Retained[0].CorrelationGroupKey != "MLB|11" {
		t.Fatalf("retained group key = %q, want MLB|11", result.Retained[0].CorrelationGroupKey)
	}
	if !result.Retained[0].CorrelationCheckPass {
		t.Fatal("retained row should pass correlation check")
	}
	if result.Retained[0].CorrelationCheckReason != correlationReasonRetainedWithinLimits {
		t.Fatalf("retained reason = %q, want %q", result.Retained[0].CorrelationCheckReason, correlationReasonRetainedWithinLimits)
	}
	if result.Dropped[0].CorrelationCheckPass {
		t.Fatal("dropped row should fail correlation check")
	}
	if result.Dropped[0].CorrelationCheckReason != correlationReasonDroppedMaxPicksPerGame {
		t.Fatalf("dropped reason = %q, want %q", result.Dropped[0].CorrelationCheckReason, correlationReasonDroppedMaxPicksPerGame)
	}
}

func TestApplyCorrelationGuardEnforcesMaxStakeFractionPerGame(t *testing.T) {
	policy := CorrelationPolicy{
		MaxPicksPerGame:         3,
		MaxStakeFractionPerGame: 0.03,
	}
	recs := []Recommendation{
		{
			Sport:                  domain.SportNBA,
			GameID:                 22,
			Market:                 "h2h",
			EventTime:              time.Date(2026, time.March, 16, 23, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    2000,
			SuggestedStakeFraction: 0.02,
			RankScore:              910,
		},
		{
			Sport:                  domain.SportNBA,
			GameID:                 22,
			Market:                 "totals",
			EventTime:              time.Date(2026, time.March, 16, 23, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1500,
			SuggestedStakeFraction: 0.015,
			RankScore:              900,
		},
	}

	result, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("ApplyCorrelationGuard() error = %v", err)
	}
	if len(result.Retained) != 1 || len(result.Dropped) != 1 {
		t.Fatalf("retained=%d dropped=%d, want 1 and 1", len(result.Retained), len(result.Dropped))
	}
	if result.Dropped[0].CorrelationCheckReason != correlationReasonDroppedMaxStakePerGame {
		t.Fatalf("dropped reason = %q, want %q", result.Dropped[0].CorrelationCheckReason, correlationReasonDroppedMaxStakePerGame)
	}
}

func TestApplyCorrelationGuardZeroStakeRetainedAndDoesNotConsumeGameCapacity(t *testing.T) {
	policy := CorrelationPolicy{
		MaxPicksPerGame:         1,
		MaxStakeFractionPerGame: 0.03,
	}
	recs := []Recommendation{
		{
			Sport:                  domain.SportNHL,
			GameID:                 33,
			Market:                 "h2h",
			EventTime:              time.Date(2026, time.March, 16, 1, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    0,
			SuggestedStakeFraction: 0,
			RankScore:              1000,
		},
		{
			Sport:                  domain.SportNHL,
			GameID:                 33,
			Market:                 "totals",
			EventTime:              time.Date(2026, time.March, 16, 1, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1200,
			SuggestedStakeFraction: 0.012,
			RankScore:              900,
		},
	}

	result, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("ApplyCorrelationGuard() error = %v", err)
	}
	if len(result.Retained) != 2 || len(result.Dropped) != 0 {
		t.Fatalf("retained=%d dropped=%d, want 2 and 0", len(result.Retained), len(result.Dropped))
	}
	if result.Retained[0].CorrelationCheckReason != correlationReasonRetainedZeroStake {
		t.Fatalf("zero-stake reason = %q, want %q", result.Retained[0].CorrelationCheckReason, correlationReasonRetainedZeroStake)
	}
	if result.Retained[1].CorrelationCheckReason != correlationReasonRetainedWithinLimits {
		t.Fatalf("second reason = %q, want %q", result.Retained[1].CorrelationCheckReason, correlationReasonRetainedWithinLimits)
	}
}

func TestApplyCorrelationGuardEnforcesOptionalSportDayLimit(t *testing.T) {
	policy := CorrelationPolicy{
		MaxPicksPerGame:         2,
		MaxStakeFractionPerGame: 0.05,
		MaxPicksPerSportDay:     1,
	}
	recs := []Recommendation{
		{
			Sport:                  domain.SportNFL,
			GameID:                 41,
			Market:                 "h2h",
			EventTime:              time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1200,
			SuggestedStakeFraction: 0.012,
			RankScore:              880,
		},
		{
			Sport:                  domain.SportNFL,
			GameID:                 42,
			Market:                 "h2h",
			EventTime:              time.Date(2026, time.September, 14, 21, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1300,
			SuggestedStakeFraction: 0.013,
			RankScore:              870,
		},
	}

	result, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("ApplyCorrelationGuard() error = %v", err)
	}
	if len(result.Retained) != 1 || len(result.Dropped) != 1 {
		t.Fatalf("retained=%d dropped=%d, want 1 and 1", len(result.Retained), len(result.Dropped))
	}
	if result.Dropped[0].CorrelationCheckReason != correlationReasonDroppedMaxPicksPerSportDay {
		t.Fatalf("dropped reason = %q, want %q", result.Dropped[0].CorrelationCheckReason, correlationReasonDroppedMaxPicksPerSportDay)
	}
}

func TestApplyCorrelationGuardDeterministicReasonOrderingForDroppedRows(t *testing.T) {
	policy := CorrelationPolicy{
		MaxPicksPerGame:         1,
		MaxStakeFractionPerGame: 0.02,
	}
	recs := []Recommendation{
		{
			Sport:                  domain.SportMLB,
			GameID:                 52,
			Market:                 "h2h",
			EventTime:              time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1500,
			SuggestedStakeFraction: 0.015,
			RankScore:              900,
		},
		{
			Sport:                  domain.SportMLB,
			GameID:                 52,
			Market:                 "totals",
			EventTime:              time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    1100,
			SuggestedStakeFraction: 0.011,
			RankScore:              890,
		},
		{
			Sport:                  domain.SportMLB,
			GameID:                 53,
			Market:                 "spreads",
			EventTime:              time.Date(2026, time.March, 16, 19, 0, 0, 0, time.UTC),
			SuggestedStakeCents:    900,
			SuggestedStakeFraction: math.NaN(),
			RankScore:              880,
		},
	}

	first, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("first ApplyCorrelationGuard() error = %v", err)
	}
	second, err := ApplyCorrelationGuard(recs, policy)
	if err != nil {
		t.Fatalf("second ApplyCorrelationGuard() error = %v", err)
	}
	if len(first.Dropped) != 2 || len(second.Dropped) != 2 {
		t.Fatalf("dropped lengths = (%d,%d), want (2,2)", len(first.Dropped), len(second.Dropped))
	}
	for i := range first.Dropped {
		if first.Dropped[i].Market != second.Dropped[i].Market {
			t.Fatalf("dropped order differs at %d: %q vs %q", i, first.Dropped[i].Market, second.Dropped[i].Market)
		}
		if first.Dropped[i].CorrelationCheckReason != second.Dropped[i].CorrelationCheckReason {
			t.Fatalf("dropped reason differs at %d: %q vs %q", i, first.Dropped[i].CorrelationCheckReason, second.Dropped[i].CorrelationCheckReason)
		}
	}
}

func TestResolveCorrelationPolicyGuardrails(t *testing.T) {
	if _, err := ResolveCorrelationPolicy(-1, 0, 0); err == nil {
		t.Fatal("ResolveCorrelationPolicy() expected error for negative max picks per game")
	}
	if _, err := ResolveCorrelationPolicy(0, math.NaN(), 0); err == nil {
		t.Fatal("ResolveCorrelationPolicy() expected error for NaN max stake fraction per game")
	}
	if _, err := ResolveCorrelationPolicy(0, 0, -1); err == nil {
		t.Fatal("ResolveCorrelationPolicy() expected error for negative max picks per sport/day")
	}
	if _, err := ResolveCorrelationPolicy(100, 0.05, 0); err == nil {
		t.Fatal("ResolveCorrelationPolicy() expected error for too-large max picks per game")
	}
	policy, err := ResolveCorrelationPolicy(0, 0, 0)
	if err != nil {
		t.Fatalf("ResolveCorrelationPolicy() default error = %v", err)
	}
	if policy.MaxPicksPerGame != defaultCorrelationMaxPicksPerGame {
		t.Fatalf("MaxPicksPerGame = %d, want %d", policy.MaxPicksPerGame, defaultCorrelationMaxPicksPerGame)
	}
	if policy.MaxStakeFractionPerGame != defaultCorrelationMaxStakeFractionPerGame {
		t.Fatalf("MaxStakeFractionPerGame = %.3f, want %.3f", policy.MaxStakeFractionPerGame, defaultCorrelationMaxStakeFractionPerGame)
	}
	if policy.MaxPicksPerSportDay != 0 {
		t.Fatalf("MaxPicksPerSportDay = %d, want 0", policy.MaxPicksPerSportDay)
	}
}
