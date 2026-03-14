package decision

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"betbot/internal/domain"
)

type RecommendationCandidate struct {
	Sport                 domain.Sport
	GameID                int64
	Market                string
	EventTime             time.Time
	ModelHomeProbability  float64
	MarketHomeProbability float64
	Quotes                []BookQuote
}

type RecommendationBuildConfig struct {
	EVThreshold            float64
	KellyFraction          float64
	MaxBetFraction         float64
	SizingBankrollCents    int64
	AvailableBankrollCents int64
	GeneratedAt            time.Time
}

type Recommendation struct {
	Sport                  domain.Sport `json:"sport"`
	GameID                 int64        `json:"game_id"`
	Market                 string       `json:"market"`
	RecommendedSide        string       `json:"recommended_side"`
	BestBook               string       `json:"best_book"`
	BestAmericanOdds       int          `json:"best_american_odds"`
	EventTime              time.Time    `json:"event_time"`
	ModelProbability       float64      `json:"model_probability"`
	MarketProbability      float64      `json:"market_probability"`
	Edge                   float64      `json:"edge"`
	SuggestedStakeDollars  float64      `json:"suggested_stake_dollars"`
	SuggestedStakeCents    int64        `json:"suggested_stake_cents"`
	SuggestedStakeFraction float64      `json:"suggested_stake_fraction"`
	BankrollCheckPass      bool         `json:"bankroll_check_pass"`
	BankrollCheckReason    string       `json:"bankroll_check_reason"`
	RankScore              float64      `json:"rank_score"`
	GeneratedAt            time.Time    `json:"generated_at"`
}

func BuildRecommendations(candidates []RecommendationCandidate, cfg RecommendationBuildConfig) ([]Recommendation, error) {
	if cfg.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("generated_at must be set")
	}
	if cfg.AvailableBankrollCents < 0 {
		return nil, fmt.Errorf("%w: must be >= 0", ErrInvalidBankrollCents)
	}
	if cfg.SizingBankrollCents < 0 {
		return nil, fmt.Errorf("%w: sizing bankroll must be >= 0", ErrInvalidBankrollCents)
	}

	sizingBankrollCents := cfg.SizingBankrollCents
	if sizingBankrollCents == 0 {
		sizingBankrollCents = cfg.AvailableBankrollCents
	}
	bankrollDollars := float64(sizingBankrollCents) / 100.0
	recommendations := make([]Recommendation, 0, len(candidates))
	for i := range candidates {
		candidate := candidates[i]
		if candidate.GameID <= 0 {
			return nil, fmt.Errorf("candidate %d: game_id must be > 0", i)
		}
		if strings.TrimSpace(candidate.Market) == "" {
			return nil, fmt.Errorf("candidate %d: market must not be empty", i)
		}
		if candidate.EventTime.IsZero() {
			return nil, fmt.Errorf("candidate %d: event_time must be set", i)
		}

		evDecision, err := EvaluateEVThreshold(EVThresholdInput{
			Sport:                 candidate.Sport,
			ModelHomeProbability:  candidate.ModelHomeProbability,
			MarketHomeProbability: candidate.MarketHomeProbability,
			MinEdge:               cfg.EVThreshold,
		})
		if err != nil {
			return nil, fmt.Errorf("candidate %d: evaluate ev threshold: %w", i, err)
		}
		if !evDecision.Pass {
			continue
		}

		line, err := ShopBestLine(LineShoppingInput{
			Sport:           candidate.Sport,
			RecommendedSide: evDecision.RecommendedSide,
			Quotes:          candidate.Quotes,
		})
		if err != nil {
			return nil, fmt.Errorf("candidate %d: line shopping: %w", i, err)
		}

		sizing, err := RecommendStake(SizingRequest{
			Sport:          candidate.Sport,
			Bankroll:       bankrollDollars,
			ModelEdge:      evDecision.ModelEdge,
			KellyFraction:  cfg.KellyFraction,
			MaxBetFraction: cfg.MaxBetFraction,
		})
		if err != nil {
			return nil, fmt.Errorf("candidate %d: sizing: %w", i, err)
		}

		stakeCents := int64(math.Round(sizing.StakeDollars * 100))
		bankrollCheck, err := CheckBankrollAvailability(BankrollAvailabilityInput{
			AvailableCents: cfg.AvailableBankrollCents,
			StakeCents:     stakeCents,
		})
		if err != nil {
			return nil, fmt.Errorf("candidate %d: bankroll availability: %w", i, err)
		}

		rank := recommendationRank(evDecision.ModelEdge, sizing.StakeFraction, line.SelectedOdds)
		recommendations = append(recommendations, Recommendation{
			Sport:                  candidate.Sport,
			GameID:                 candidate.GameID,
			Market:                 candidate.Market,
			RecommendedSide:        evDecision.RecommendedSide,
			BestBook:               line.SelectedBook,
			BestAmericanOdds:       line.SelectedOdds,
			EventTime:              candidate.EventTime.UTC(),
			ModelProbability:       candidate.ModelHomeProbability,
			MarketProbability:      candidate.MarketHomeProbability,
			Edge:                   evDecision.ModelEdge,
			SuggestedStakeDollars:  sizing.StakeDollars,
			SuggestedStakeCents:    stakeCents,
			SuggestedStakeFraction: sizing.StakeFraction,
			BankrollCheckPass:      bankrollCheck.Pass,
			BankrollCheckReason:    bankrollCheck.Reason,
			RankScore:              rank,
			GeneratedAt:            cfg.GeneratedAt.UTC(),
		})
	}

	sort.SliceStable(recommendations, func(i, j int) bool {
		left := recommendations[i]
		right := recommendations[j]
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if left.Edge != right.Edge {
			return left.Edge > right.Edge
		}
		if left.SuggestedStakeFraction != right.SuggestedStakeFraction {
			return left.SuggestedStakeFraction > right.SuggestedStakeFraction
		}
		if left.BestAmericanOdds != right.BestAmericanOdds {
			return left.BestAmericanOdds > right.BestAmericanOdds
		}
		if left.GameID != right.GameID {
			return left.GameID < right.GameID
		}
		if left.Market != right.Market {
			return left.Market < right.Market
		}
		if left.BestBook != right.BestBook {
			return left.BestBook < right.BestBook
		}
		return left.RecommendedSide < right.RecommendedSide
	})

	return recommendations, nil
}

func recommendationRank(edge float64, stakeFraction float64, bestOdds int) float64 {
	return (edge * 10000.0) + (stakeFraction * 1000.0) + (float64(bestOdds) / 100000.0)
}
