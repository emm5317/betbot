package decision

import (
	"fmt"
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
	EVThreshold                          float64
	KellyFraction                        float64
	MaxBetFraction                       float64
	CorrelationMaxPicksPerGame           int
	CorrelationMaxStakeFractionPerGame   float64
	CorrelationMaxPicksPerSportDayWindow int
	CircuitDailyLossStop                 float64
	CircuitWeeklyLossStop                float64
	CircuitDrawdownBreaker               float64
	CircuitMetrics                       CircuitBreakerMetrics
	SizingBankrollCents                  int64
	AvailableBankrollCents               int64
	GeneratedAt                          time.Time
}

type Recommendation struct {
	Sport                   domain.Sport `json:"sport"`
	GameID                  int64        `json:"game_id"`
	Market                  string       `json:"market"`
	RecommendedSide         string       `json:"recommended_side"`
	BestBook                string       `json:"best_book"`
	BestAmericanOdds        int          `json:"best_american_odds"`
	EventTime               time.Time    `json:"event_time"`
	ModelProbability        float64      `json:"model_probability"`
	MarketProbability       float64      `json:"market_probability"`
	Edge                    float64      `json:"edge"`
	SuggestedStakeDollars   float64      `json:"suggested_stake_dollars"`
	SuggestedStakeCents     int64        `json:"suggested_stake_cents"`
	SuggestedStakeFraction  float64      `json:"suggested_stake_fraction"`
	RawKellyFraction        float64      `json:"raw_kelly_fraction"`
	AppliedFractionalKelly  float64      `json:"applied_fractional_kelly"`
	CappedFraction          float64      `json:"capped_fraction"`
	PreBankrollStakeDollars float64      `json:"pre_bankroll_stake_dollars"`
	PreBankrollStakeCents   int64        `json:"pre_bankroll_stake_cents"`
	BankrollAvailableCents  int64        `json:"bankroll_available_cents"`
	SizingReasons           []string     `json:"sizing_reasons"`
	BankrollCheckPass       bool         `json:"bankroll_check_pass"`
	BankrollCheckReason     string       `json:"bankroll_check_reason"`
	CorrelationCheckPass    bool         `json:"correlation_check_pass"`
	CorrelationCheckReason  string       `json:"correlation_check_reason"`
	CorrelationGroupKey     string       `json:"correlation_group_key"`
	CircuitCheckPass        bool         `json:"circuit_check_pass"`
	CircuitCheckReason      string       `json:"circuit_check_reason"`
	RankScore               float64      `json:"rank_score"`
	GeneratedAt             time.Time    `json:"generated_at"`
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
	correlationPolicy, err := ResolveCorrelationPolicy(
		cfg.CorrelationMaxPicksPerGame,
		cfg.CorrelationMaxStakeFractionPerGame,
		cfg.CorrelationMaxPicksPerSportDayWindow,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve correlation policy: %w", err)
	}
	circuitPolicy, err := ResolveCircuitBreakerPolicy(
		cfg.CircuitDailyLossStop,
		cfg.CircuitWeeklyLossStop,
		cfg.CircuitDrawdownBreaker,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve circuit policy: %w", err)
	}
	circuitMetrics := cfg.CircuitMetrics
	if circuitMetrics == (CircuitBreakerMetrics{}) {
		circuitMetrics = CircuitBreakerMetrics{
			CurrentBalanceCents:   cfg.AvailableBankrollCents,
			DayStartBalanceCents:  cfg.AvailableBankrollCents,
			WeekStartBalanceCents: cfg.AvailableBankrollCents,
			PeakBalanceCents:      cfg.AvailableBankrollCents,
		}
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

		modelProbability := candidate.ModelHomeProbability
		if evDecision.RecommendedSide == awaySide {
			modelProbability = 1 - candidate.ModelHomeProbability
		}

		sizing, err := EvaluateRecommendationStake(RecommendationStakeRequest{
			Sport:                  candidate.Sport,
			ModelProbability:       modelProbability,
			SelectedAmericanOdds:   line.SelectedOdds,
			Bankroll:               bankrollDollars,
			AvailableBankrollCents: cfg.AvailableBankrollCents,
			KellyFraction:          cfg.KellyFraction,
			MaxBetFraction:         cfg.MaxBetFraction,
		})
		if err != nil {
			return nil, fmt.Errorf("candidate %d: sizing: %w", i, err)
		}

		rank := recommendationRank(evDecision.ModelEdge, sizing.RecommendedStakeFraction, line.SelectedOdds)
		recommendations = append(recommendations, Recommendation{
			Sport:                   candidate.Sport,
			GameID:                  candidate.GameID,
			Market:                  candidate.Market,
			RecommendedSide:         evDecision.RecommendedSide,
			BestBook:                line.SelectedBook,
			BestAmericanOdds:        line.SelectedOdds,
			EventTime:               candidate.EventTime.UTC(),
			ModelProbability:        candidate.ModelHomeProbability,
			MarketProbability:       candidate.MarketHomeProbability,
			Edge:                    evDecision.ModelEdge,
			SuggestedStakeDollars:   sizing.RecommendedStakeDollars,
			SuggestedStakeCents:     sizing.RecommendedStakeCents,
			SuggestedStakeFraction:  sizing.RecommendedStakeFraction,
			RawKellyFraction:        sizing.RawKellyFraction,
			AppliedFractionalKelly:  sizing.AppliedFractionalKelly,
			CappedFraction:          sizing.CappedFraction,
			PreBankrollStakeDollars: sizing.PreBankrollStakeDollars,
			PreBankrollStakeCents:   sizing.PreBankrollStakeCents,
			BankrollAvailableCents:  sizing.BankrollAvailableCents,
			SizingReasons:           append([]string(nil), sizing.Reasons...),
			BankrollCheckPass:       sizing.BankrollCheckPass,
			BankrollCheckReason:     sizing.BankrollCheckReason,
			RankScore:               rank,
			GeneratedAt:             cfg.GeneratedAt.UTC(),
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

	correlationResult, err := ApplyCorrelationGuard(recommendations, correlationPolicy)
	if err != nil {
		return nil, fmt.Errorf("apply correlation guard: %w", err)
	}

	circuitResult, err := ApplyCircuitBreakerGuard(correlationResult.Retained, circuitPolicy, circuitMetrics)
	if err != nil {
		return nil, fmt.Errorf("apply circuit breaker guard: %w", err)
	}

	return circuitResult.Retained, nil
}

func recommendationRank(edge float64, stakeFraction float64, bestOdds int) float64 {
	return (edge * 10000.0) + (stakeFraction * 1000.0) + (float64(bestOdds) / 100000.0)
}
