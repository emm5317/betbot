package decision

import (
	"errors"
	"fmt"
	"strings"

	"betbot/internal/domain"
)

var (
	ErrInvalidRecommendedSide = errors.New("invalid recommended side")
	ErrEmptyBookQuotes        = errors.New("book quotes required")
	ErrMalformedBookQuote     = errors.New("malformed book quote")
	ErrInvalidAmericanOdds    = errors.New("invalid american odds")
)

type BookQuote struct {
	Book         string `json:"book"`
	HomeAmerican int    `json:"home_american"`
	AwayAmerican int    `json:"away_american"`
}

type LineShoppingInput struct {
	Sport           domain.Sport
	RecommendedSide string
	Quotes          []BookQuote
}

type LineShoppingResult struct {
	Sport           domain.Sport `json:"sport,omitempty"`
	RecommendedSide string       `json:"recommended_side"`
	SelectedBook    string       `json:"selected_book"`
	SelectedOdds    int          `json:"selected_odds"`
	QuotesEvaluated int          `json:"quotes_evaluated"`
}

func ShopBestLine(input LineShoppingInput) (LineShoppingResult, error) {
	if input.Sport != "" {
		if _, err := DefaultEVThresholdPolicy(input.Sport); err != nil {
			return LineShoppingResult{}, err
		}
	}

	if input.RecommendedSide != homeSide && input.RecommendedSide != awaySide {
		return LineShoppingResult{}, fmt.Errorf("%w: %q", ErrInvalidRecommendedSide, input.RecommendedSide)
	}
	if len(input.Quotes) == 0 {
		return LineShoppingResult{}, ErrEmptyBookQuotes
	}

	result := LineShoppingResult{
		Sport:           input.Sport,
		RecommendedSide: input.RecommendedSide,
		QuotesEvaluated: len(input.Quotes),
	}

	bestIdx := -1
	for i, quote := range input.Quotes {
		book := strings.TrimSpace(quote.Book)
		if book == "" {
			return LineShoppingResult{}, fmt.Errorf("%w: quote %d book is empty", ErrMalformedBookQuote, i)
		}
		if err := validateAmericanOdds(quote.HomeAmerican, "home_american"); err != nil {
			return LineShoppingResult{}, fmt.Errorf("quote %d: %w", i, err)
		}
		if err := validateAmericanOdds(quote.AwayAmerican, "away_american"); err != nil {
			return LineShoppingResult{}, fmt.Errorf("quote %d: %w", i, err)
		}

		candidateOdds := quote.HomeAmerican
		if input.RecommendedSide == awaySide {
			candidateOdds = quote.AwayAmerican
		}

		if bestIdx == -1 || candidateOdds > result.SelectedOdds {
			result.SelectedBook = book
			result.SelectedOdds = candidateOdds
			bestIdx = i
		}
		// Deterministic tie-break: first matching quote wins.
	}

	return result, nil
}

func validateAmericanOdds(value int, field string) error {
	if value > -100 && value < 100 {
		return fmt.Errorf("%w: %s must be <= -100 or >= 100", ErrInvalidAmericanOdds, field)
	}
	return nil
}
