package decision

import (
	"math"
	"strings"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestBuildRecommendationsAppliesDecisionPipelineAndSortsDeterministically(t *testing.T) {
	generatedAt := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	recommendations, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportMLB,
			GameID:                101,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.58,
			MarketHomeProbability: 0.52,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 105, AwayAmerican: -120},
				{Book: "book-b", HomeAmerican: 112, AwayAmerican: -125},
			},
		},
		{
			Sport:                 domain.SportNFL,
			GameID:                102,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 17, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.60,
			MarketHomeProbability: 0.53,
			Quotes: []BookQuote{
				{Book: "book-c", HomeAmerican: 100, AwayAmerican: -115},
				{Book: "book-d", HomeAmerican: 108, AwayAmerican: -120},
			},
		},
	}, RecommendationBuildConfig{
		AvailableBankrollCents: 100000,
		GeneratedAt:            generatedAt,
	})
	if err != nil {
		t.Fatalf("BuildRecommendations() error = %v", err)
	}
	if len(recommendations) != 2 {
		t.Fatalf("len(recommendations) = %d, want 2", len(recommendations))
	}
	if recommendations[0].GameID != 102 {
		t.Fatalf("recommendations[0].GameID = %d, want 102 (higher edge first)", recommendations[0].GameID)
	}
	if recommendations[0].BestBook != "book-d" {
		t.Fatalf("recommendations[0].BestBook = %q, want book-d", recommendations[0].BestBook)
	}
	if recommendations[0].RawKellyFraction <= 0 {
		t.Fatalf("recommendations[0].RawKellyFraction = %.6f, want > 0", recommendations[0].RawKellyFraction)
	}
	if recommendations[0].PreBankrollStakeCents <= 0 {
		t.Fatalf("recommendations[0].PreBankrollStakeCents = %d, want > 0", recommendations[0].PreBankrollStakeCents)
	}
	if len(recommendations[0].SizingReasons) == 0 {
		t.Fatal("recommendations[0].SizingReasons should not be empty")
	}
	if recommendations[0].GeneratedAt != generatedAt {
		t.Fatalf("GeneratedAt = %s, want %s", recommendations[0].GeneratedAt, generatedAt)
	}
	if !recommendations[0].CorrelationCheckPass {
		t.Fatal("CorrelationCheckPass = false, want true")
	}
	if recommendations[0].CorrelationCheckReason != correlationReasonRetainedWithinLimits {
		t.Fatalf("CorrelationCheckReason = %q, want %q", recommendations[0].CorrelationCheckReason, correlationReasonRetainedWithinLimits)
	}
	if recommendations[0].CorrelationGroupKey == "" {
		t.Fatal("CorrelationGroupKey should not be empty")
	}
	if !recommendations[0].CircuitCheckPass {
		t.Fatal("CircuitCheckPass = false, want true")
	}
	if recommendations[0].CircuitCheckReason != circuitCheckReasonPass {
		t.Fatalf("CircuitCheckReason = %q, want %q", recommendations[0].CircuitCheckReason, circuitCheckReasonPass)
	}
}

func TestBuildRecommendationsFiltersNonPassingEV(t *testing.T) {
	recommendations, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportNBA,
			GameID:                201,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.52,
			MarketHomeProbability: 0.51,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 105, AwayAmerican: -120},
			},
		},
	}, RecommendationBuildConfig{
		AvailableBankrollCents: 50000,
		GeneratedAt:            time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildRecommendations() error = %v", err)
	}
	if len(recommendations) != 0 {
		t.Fatalf("len(recommendations) = %d, want 0", len(recommendations))
	}
}

func TestBuildRecommendationsAnnotatesInsufficientFunds(t *testing.T) {
	recommendations, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportMLB,
			GameID:                301,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.70,
			MarketHomeProbability: 0.55,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 110, AwayAmerican: -130},
			},
		},
	}, RecommendationBuildConfig{
		SizingBankrollCents:    100000,
		AvailableBankrollCents: 1000,
		GeneratedAt:            time.Date(2026, time.March, 14, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildRecommendations() error = %v", err)
	}
	if len(recommendations) != 1 {
		t.Fatalf("len(recommendations) = %d, want 1", len(recommendations))
	}
	if recommendations[0].BankrollCheckPass {
		t.Fatal("expected bankroll check to fail")
	}
	if recommendations[0].BankrollCheckReason != bankrollCheckReasonInsufficientFunds {
		t.Fatalf("BankrollCheckReason = %q, want %q", recommendations[0].BankrollCheckReason, bankrollCheckReasonInsufficientFunds)
	}
	if recommendations[0].SuggestedStakeCents != 1000 {
		t.Fatalf("SuggestedStakeCents = %d, want 1000 (reduced to available bankroll)", recommendations[0].SuggestedStakeCents)
	}
	expectedReasons := []string{
		stakeReasonCappedByMaxFraction,
		stakeReasonBankrollInsufficient,
		stakeReasonBankrollCapped,
		stakeReasonSized,
	}
	if strings.Join(recommendations[0].SizingReasons, ",") != strings.Join(expectedReasons, ",") {
		t.Fatalf("SizingReasons = %v, want %v", recommendations[0].SizingReasons, expectedReasons)
	}
}

func TestBuildRecommendationsRejectsInvalidCandidate(t *testing.T) {
	_, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportMLB,
			GameID:                0,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.60,
			MarketHomeProbability: 0.50,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
			},
		},
	}, RecommendationBuildConfig{
		AvailableBankrollCents: 100000,
		GeneratedAt:            time.Date(2026, time.March, 14, 11, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected invalid candidate error")
	}
	if !strings.Contains(err.Error(), "game_id") {
		t.Fatalf("error %q missing game_id context", err)
	}
}

func TestBuildRecommendationsDeterministicTieBreakWithSameGameCorrelationGuard(t *testing.T) {
	generatedAt := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	recommendations, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportMLB,
			GameID:                501,
			Market:                "totals",
			EventTime:             time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.57,
			MarketHomeProbability: 0.50,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
			},
		},
		{
			Sport:                 domain.SportMLB,
			GameID:                501,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.57,
			MarketHomeProbability: 0.50,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
			},
		},
	}, RecommendationBuildConfig{
		AvailableBankrollCents:             100000,
		CorrelationMaxPicksPerGame:         1,
		CorrelationMaxStakeFractionPerGame: 0.10,
		GeneratedAt:                        generatedAt,
	})
	if err != nil {
		t.Fatalf("BuildRecommendations() error = %v", err)
	}
	if len(recommendations) != 1 {
		t.Fatalf("len(recommendations) = %d, want 1", len(recommendations))
	}
	if recommendations[0].Market != "h2h" {
		t.Fatalf("recommendations[0].Market = %q, want h2h (lexicographic tie-break)", recommendations[0].Market)
	}
	if recommendations[0].CorrelationCheckReason != correlationReasonRetainedWithinLimits {
		t.Fatalf("CorrelationCheckReason = %q, want %q", recommendations[0].CorrelationCheckReason, correlationReasonRetainedWithinLimits)
	}
}

func TestBuildRecommendationsCircuitBreakerDropsPositiveStakeRows(t *testing.T) {
	recommendations, err := BuildRecommendations([]RecommendationCandidate{
		{
			Sport:                 domain.SportMLB,
			GameID:                7001,
			Market:                "h2h",
			EventTime:             time.Date(2026, time.March, 16, 0, 0, 0, 0, time.UTC),
			ModelHomeProbability:  0.60,
			MarketHomeProbability: 0.53,
			Quotes: []BookQuote{
				{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
			},
		},
	}, RecommendationBuildConfig{
		AvailableBankrollCents:             100000,
		CorrelationMaxPicksPerGame:         1,
		CorrelationMaxStakeFractionPerGame: 0.10,
		CircuitDailyLossStop:               0.05,
		CircuitWeeklyLossStop:              0.10,
		CircuitDrawdownBreaker:             0.15,
		CircuitMetrics: CircuitBreakerMetrics{
			CurrentBalanceCents:   80000,
			DayStartBalanceCents:  100000,
			WeekStartBalanceCents: 100000,
			PeakBalanceCents:      100000,
		},
		GeneratedAt: time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildRecommendations() error = %v", err)
	}
	if len(recommendations) != 0 {
		t.Fatalf("len(recommendations) = %d, want 0 when circuit breaker blocks positive stakes", len(recommendations))
	}
}

func TestBuildRecommendationsDeterministicIntegrationEdgeCases(t *testing.T) {
	generatedAt := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	type expectedRecommendation struct {
		GameID                 int64
		Market                 string
		BestBook               string
		SuggestedStakeCents    int64
		BankrollCheckReason    string
		CorrelationCheckReason string
		CircuitCheckReason     string
	}

	testCases := []struct {
		name               string
		candidates         []RecommendationCandidate
		cfg                RecommendationBuildConfig
		expected           []expectedRecommendation
		expectedReasonList []string
		assertExtra        func(t *testing.T, recommendations []Recommendation)
	}{
		{
			name: "ev_line_selection_and_bankroll_cap",
			candidates: []RecommendationCandidate{
				{
					Sport:                 domain.SportMLB,
					GameID:                8100,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.52,
					MarketHomeProbability: 0.51,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 105, AwayAmerican: -120},
					},
				},
				{
					Sport:                 domain.SportMLB,
					GameID:                8101,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.March, 16, 19, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 105, AwayAmerican: -120},
						{Book: "book-b", HomeAmerican: 112, AwayAmerican: -126},
					},
				},
			},
			cfg: RecommendationBuildConfig{
				SizingBankrollCents:    100000,
				AvailableBankrollCents: 1000,
				GeneratedAt:            generatedAt,
			},
			expected: []expectedRecommendation{
				{
					GameID:                 8101,
					Market:                 "h2h",
					BestBook:               "book-b",
					SuggestedStakeCents:    1000,
					BankrollCheckReason:    bankrollCheckReasonInsufficientFunds,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
			},
			expectedReasonList: []string{
				stakeReasonCappedByMaxFraction,
				stakeReasonBankrollInsufficient,
				stakeReasonBankrollCapped,
				stakeReasonSized,
			},
		},
		{
			name: "correlation_exact_threshold_with_mixed_same_game_markets",
			candidates: []RecommendationCandidate{
				{
					Sport:                 domain.SportNFL,
					GameID:                8200,
					Market:                "totals",
					EventTime:             time.Date(2026, time.September, 16, 18, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
				{
					Sport:                 domain.SportNFL,
					GameID:                8200,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.September, 16, 18, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
				{
					Sport:                 domain.SportNFL,
					GameID:                8200,
					Market:                "spreads",
					EventTime:             time.Date(2026, time.September, 16, 18, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
			},
			cfg: RecommendationBuildConfig{
				AvailableBankrollCents:             100000,
				CorrelationMaxPicksPerGame:         3,
				CorrelationMaxStakeFractionPerGame: 0.03,
				GeneratedAt:                        generatedAt,
			},
			expected: []expectedRecommendation{
				{
					GameID:                 8200,
					Market:                 "h2h",
					BestBook:               "book-a",
					SuggestedStakeCents:    1500,
					BankrollCheckReason:    bankrollCheckReasonOK,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
				{
					GameID:                 8200,
					Market:                 "spreads",
					BestBook:               "book-a",
					SuggestedStakeCents:    1500,
					BankrollCheckReason:    bankrollCheckReasonOK,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
			},
			assertExtra: func(t *testing.T, recommendations []Recommendation) {
				t.Helper()
				if math.Abs(recommendations[0].SuggestedStakeFraction-0.015) > 1e-9 {
					t.Fatalf("recommendations[0].SuggestedStakeFraction = %.9f, want 0.015", recommendations[0].SuggestedStakeFraction)
				}
				if math.Abs(recommendations[1].SuggestedStakeFraction-0.015) > 1e-9 {
					t.Fatalf("recommendations[1].SuggestedStakeFraction = %.9f, want 0.015", recommendations[1].SuggestedStakeFraction)
				}
			},
		},
		{
			name: "circuit_exact_threshold_drops_positive_stake_but_retains_zero_stake",
			candidates: []RecommendationCandidate{
				{
					Sport:                 domain.SportMLB,
					GameID:                8301,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.March, 16, 17, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.55,
					MarketHomeProbability: 0.52,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: -400, AwayAmerican: 350},
					},
				},
				{
					Sport:                 domain.SportMLB,
					GameID:                8302,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
			},
			cfg: RecommendationBuildConfig{
				AvailableBankrollCents: 100000,
				CircuitDailyLossStop:   0.05,
				CircuitWeeklyLossStop:  0.10,
				CircuitDrawdownBreaker: 0.15,
				CircuitMetrics: CircuitBreakerMetrics{
					CurrentBalanceCents:   95000,
					DayStartBalanceCents:  100000,
					WeekStartBalanceCents: 100000,
					PeakBalanceCents:      100000,
				},
				GeneratedAt: generatedAt,
			},
			expected: []expectedRecommendation{
				{
					GameID:                 8301,
					Market:                 "h2h",
					BestBook:               "book-a",
					SuggestedStakeCents:    0,
					BankrollCheckReason:    bankrollCheckReasonStakeNonPositive,
					CorrelationCheckReason: correlationReasonRetainedZeroStake,
					CircuitCheckReason:     circuitCheckReasonRetainedZeroStake,
				},
			},
		},
		{
			name: "identical_rank_ties_use_deterministic_game_and_market_tiebreakers",
			candidates: []RecommendationCandidate{
				{
					Sport:                 domain.SportMLB,
					GameID:                9902,
					Market:                "totals",
					EventTime:             time.Date(2026, time.March, 16, 20, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-b", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
				{
					Sport:                 domain.SportMLB,
					GameID:                9901,
					Market:                "totals",
					EventTime:             time.Date(2026, time.March, 16, 19, 0, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
				{
					Sport:                 domain.SportMLB,
					GameID:                9901,
					Market:                "h2h",
					EventTime:             time.Date(2026, time.March, 16, 19, 30, 0, 0, time.UTC),
					ModelHomeProbability:  0.60,
					MarketHomeProbability: 0.53,
					Quotes: []BookQuote{
						{Book: "book-c", HomeAmerican: 110, AwayAmerican: -120},
					},
				},
			},
			cfg: RecommendationBuildConfig{
				AvailableBankrollCents:             100000,
				CorrelationMaxPicksPerGame:         5,
				CorrelationMaxStakeFractionPerGame: 0.10,
				GeneratedAt:                        generatedAt,
			},
			expected: []expectedRecommendation{
				{
					GameID:                 9901,
					Market:                 "h2h",
					BestBook:               "book-c",
					SuggestedStakeCents:    3000,
					BankrollCheckReason:    bankrollCheckReasonOK,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
				{
					GameID:                 9901,
					Market:                 "totals",
					BestBook:               "book-a",
					SuggestedStakeCents:    3000,
					BankrollCheckReason:    bankrollCheckReasonOK,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
				{
					GameID:                 9902,
					Market:                 "totals",
					BestBook:               "book-b",
					SuggestedStakeCents:    3000,
					BankrollCheckReason:    bankrollCheckReasonOK,
					CorrelationCheckReason: correlationReasonRetainedWithinLimits,
					CircuitCheckReason:     circuitCheckReasonPass,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recommendations, err := BuildRecommendations(tc.candidates, tc.cfg)
			if err != nil {
				t.Fatalf("BuildRecommendations() error = %v", err)
			}
			if len(recommendations) != len(tc.expected) {
				t.Fatalf("len(recommendations) = %d, want %d", len(recommendations), len(tc.expected))
			}

			for i, expected := range tc.expected {
				got := recommendations[i]
				if got.GameID != expected.GameID {
					t.Fatalf("recommendations[%d].GameID = %d, want %d", i, got.GameID, expected.GameID)
				}
				if got.Market != expected.Market {
					t.Fatalf("recommendations[%d].Market = %q, want %q", i, got.Market, expected.Market)
				}
				if got.BestBook != expected.BestBook {
					t.Fatalf("recommendations[%d].BestBook = %q, want %q", i, got.BestBook, expected.BestBook)
				}
				if got.SuggestedStakeCents != expected.SuggestedStakeCents {
					t.Fatalf("recommendations[%d].SuggestedStakeCents = %d, want %d", i, got.SuggestedStakeCents, expected.SuggestedStakeCents)
				}
				if got.BankrollCheckReason != expected.BankrollCheckReason {
					t.Fatalf("recommendations[%d].BankrollCheckReason = %q, want %q", i, got.BankrollCheckReason, expected.BankrollCheckReason)
				}
				if got.CorrelationCheckReason != expected.CorrelationCheckReason {
					t.Fatalf("recommendations[%d].CorrelationCheckReason = %q, want %q", i, got.CorrelationCheckReason, expected.CorrelationCheckReason)
				}
				if got.CircuitCheckReason != expected.CircuitCheckReason {
					t.Fatalf("recommendations[%d].CircuitCheckReason = %q, want %q", i, got.CircuitCheckReason, expected.CircuitCheckReason)
				}
			}

			if tc.expectedReasonList != nil {
				if strings.Join(recommendations[0].SizingReasons, ",") != strings.Join(tc.expectedReasonList, ",") {
					t.Fatalf("recommendations[0].SizingReasons = %v, want %v", recommendations[0].SizingReasons, tc.expectedReasonList)
				}
			}

			if tc.assertExtra != nil {
				tc.assertExtra(t, recommendations)
			}
		})
	}
}
