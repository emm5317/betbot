package decision

import (
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
	if recommendations[0].GeneratedAt != generatedAt {
		t.Fatalf("GeneratedAt = %s, want %s", recommendations[0].GeneratedAt, generatedAt)
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
