package decision

import (
	"fmt"
	"strings"
)

const (
	RecommendationResultWin     = "win"
	RecommendationResultLoss    = "loss"
	RecommendationResultPush    = "push"
	RecommendationResultUnknown = "unknown"

	RecommendationPerformanceStatusCloseUnavailable = "close_unavailable"
	RecommendationPerformanceStatusPendingOutcome   = "pending_outcome"
	RecommendationPerformanceStatusSettled          = "settled"
)

type RecommendationPerformanceInput struct {
	MarketKey                     string
	RecommendedSide               string
	RecommendationHomeProbability float64
	ClosingSideProbability        *float64
	HomeScore                     *int
	AwayScore                     *int
}

type RecommendationPerformanceResult struct {
	RecommendedSideProbability float64  `json:"recommended_side_probability"`
	ClosingSideProbability     *float64 `json:"closing_side_probability,omitempty"`
	CLVDelta                   *float64 `json:"clv_delta,omitempty"`
	RealizedResult             string   `json:"realized_result"`
	Status                     string   `json:"status"`
}

func ComputeRecommendationPerformance(input RecommendationPerformanceInput) (RecommendationPerformanceResult, error) {
	recommendedSideProbability, err := sideProbability(input.RecommendedSide, input.RecommendationHomeProbability)
	if err != nil {
		return RecommendationPerformanceResult{}, err
	}

	result := RecommendationPerformanceResult{
		RecommendedSideProbability: recommendedSideProbability,
		RealizedResult:             RecommendationResultUnknown,
		Status:                     RecommendationPerformanceStatusCloseUnavailable,
	}

	if input.ClosingSideProbability == nil {
		return result, nil
	}

	if err := validateProbability(*input.ClosingSideProbability, "closing side probability"); err != nil {
		return RecommendationPerformanceResult{}, err
	}

	closingSideProbability := *input.ClosingSideProbability
	clvDelta := closingSideProbability - recommendedSideProbability
	result.ClosingSideProbability = &closingSideProbability
	result.CLVDelta = &clvDelta
	result.Status = RecommendationPerformanceStatusPendingOutcome

	outcome, err := GradeRecommendationOutcome(OutcomeGradeInput{
		MarketKey:       input.MarketKey,
		RecommendedSide: input.RecommendedSide,
		HomeScore:       input.HomeScore,
		AwayScore:       input.AwayScore,
	})
	if err != nil {
		return RecommendationPerformanceResult{}, err
	}
	result.RealizedResult = outcome
	if outcome == RecommendationResultWin || outcome == RecommendationResultLoss || outcome == RecommendationResultPush {
		result.Status = RecommendationPerformanceStatusSettled
	}

	return result, nil
}

type OutcomeGradeInput struct {
	MarketKey       string
	RecommendedSide string
	HomeScore       *int
	AwayScore       *int
}

func GradeRecommendationOutcome(input OutcomeGradeInput) (string, error) {
	if input.HomeScore == nil || input.AwayScore == nil {
		return RecommendationResultUnknown, nil
	}
	if strings.TrimSpace(input.MarketKey) != "h2h" {
		return RecommendationResultUnknown, nil
	}

	switch strings.ToLower(strings.TrimSpace(input.RecommendedSide)) {
	case homeSide:
		return compareScores(*input.HomeScore, *input.AwayScore), nil
	case awaySide:
		outcome := compareScores(*input.HomeScore, *input.AwayScore)
		switch outcome {
		case RecommendationResultWin:
			return RecommendationResultLoss, nil
		case RecommendationResultLoss:
			return RecommendationResultWin, nil
		default:
			return outcome, nil
		}
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidRecommendedSide, input.RecommendedSide)
	}
}

func sideProbability(recommendedSide string, homeProbability float64) (float64, error) {
	if err := validateProbability(homeProbability, "home probability"); err != nil {
		return 0, err
	}

	switch strings.ToLower(strings.TrimSpace(recommendedSide)) {
	case homeSide:
		return homeProbability, nil
	case awaySide:
		return 1 - homeProbability, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidRecommendedSide, recommendedSide)
	}
}

func compareScores(homeScore int, awayScore int) string {
	switch {
	case homeScore > awayScore:
		return RecommendationResultWin
	case homeScore < awayScore:
		return RecommendationResultLoss
	default:
		return RecommendationResultPush
	}
}
