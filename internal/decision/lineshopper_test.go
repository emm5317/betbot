package decision

import (
	"errors"
	"testing"

	"betbot/internal/domain"
)

func TestShopBestLineSelectsHomeSideBestOdds(t *testing.T) {
	result, err := ShopBestLine(LineShoppingInput{
		Sport:           domain.SportMLB,
		RecommendedSide: homeSide,
		Quotes: []BookQuote{
			{Book: "book-a", HomeAmerican: 105, AwayAmerican: -120},
			{Book: "book-b", HomeAmerican: 115, AwayAmerican: -130},
			{Book: "book-c", HomeAmerican: 110, AwayAmerican: -125},
		},
	})
	if err != nil {
		t.Fatalf("ShopBestLine() error = %v", err)
	}
	if result.SelectedBook != "book-b" {
		t.Fatalf("SelectedBook = %q, want %q", result.SelectedBook, "book-b")
	}
	if result.SelectedOdds != 115 {
		t.Fatalf("SelectedOdds = %d, want %d", result.SelectedOdds, 115)
	}
}

func TestShopBestLineSelectsAwaySideBestOdds(t *testing.T) {
	result, err := ShopBestLine(LineShoppingInput{
		Sport:           domain.SportNBA,
		RecommendedSide: awaySide,
		Quotes: []BookQuote{
			{Book: "book-a", HomeAmerican: -110, AwayAmerican: 100},
			{Book: "book-b", HomeAmerican: -105, AwayAmerican: 120},
			{Book: "book-c", HomeAmerican: -115, AwayAmerican: 110},
		},
	})
	if err != nil {
		t.Fatalf("ShopBestLine() error = %v", err)
	}
	if result.SelectedBook != "book-b" {
		t.Fatalf("SelectedBook = %q, want %q", result.SelectedBook, "book-b")
	}
	if result.SelectedOdds != 120 {
		t.Fatalf("SelectedOdds = %d, want %d", result.SelectedOdds, 120)
	}
}

func TestShopBestLinePrefersHigherPositiveOdds(t *testing.T) {
	result, err := ShopBestLine(LineShoppingInput{
		RecommendedSide: homeSide,
		Quotes: []BookQuote{
			{Book: "book-a", HomeAmerican: 105, AwayAmerican: -125},
			{Book: "book-b", HomeAmerican: 120, AwayAmerican: -140},
			{Book: "book-c", HomeAmerican: 110, AwayAmerican: -130},
		},
	})
	if err != nil {
		t.Fatalf("ShopBestLine() error = %v", err)
	}
	if result.SelectedOdds != 120 {
		t.Fatalf("SelectedOdds = %d, want %d", result.SelectedOdds, 120)
	}
}

func TestShopBestLinePrefersLessNegativeOdds(t *testing.T) {
	result, err := ShopBestLine(LineShoppingInput{
		RecommendedSide: awaySide,
		Quotes: []BookQuote{
			{Book: "book-a", HomeAmerican: 120, AwayAmerican: -120},
			{Book: "book-b", HomeAmerican: 115, AwayAmerican: -105},
			{Book: "book-c", HomeAmerican: 110, AwayAmerican: -110},
		},
	})
	if err != nil {
		t.Fatalf("ShopBestLine() error = %v", err)
	}
	if result.SelectedBook != "book-b" {
		t.Fatalf("SelectedBook = %q, want %q", result.SelectedBook, "book-b")
	}
	if result.SelectedOdds != -105 {
		t.Fatalf("SelectedOdds = %d, want %d", result.SelectedOdds, -105)
	}
}

func TestShopBestLineTieBreakIsFirstQuote(t *testing.T) {
	result, err := ShopBestLine(LineShoppingInput{
		RecommendedSide: homeSide,
		Quotes: []BookQuote{
			{Book: "book-first", HomeAmerican: 110, AwayAmerican: -125},
			{Book: "book-second", HomeAmerican: 110, AwayAmerican: -130},
		},
	})
	if err != nil {
		t.Fatalf("ShopBestLine() error = %v", err)
	}
	if result.SelectedBook != "book-first" {
		t.Fatalf("SelectedBook = %q, want %q", result.SelectedBook, "book-first")
	}
}

func TestShopBestLineRejectsInvalidInputs(t *testing.T) {
	_, err := ShopBestLine(LineShoppingInput{
		RecommendedSide: "middle",
		Quotes:          []BookQuote{{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120}},
	})
	if !errors.Is(err, ErrInvalidRecommendedSide) {
		t.Fatalf("expected ErrInvalidRecommendedSide, got %v", err)
	}

	_, err = ShopBestLine(LineShoppingInput{
		RecommendedSide: homeSide,
	})
	if !errors.Is(err, ErrEmptyBookQuotes) {
		t.Fatalf("expected ErrEmptyBookQuotes, got %v", err)
	}

	_, err = ShopBestLine(LineShoppingInput{
		RecommendedSide: homeSide,
		Quotes:          []BookQuote{{Book: "", HomeAmerican: 110, AwayAmerican: -120}},
	})
	if !errors.Is(err, ErrMalformedBookQuote) {
		t.Fatalf("expected ErrMalformedBookQuote, got %v", err)
	}

	_, err = ShopBestLine(LineShoppingInput{
		RecommendedSide: awaySide,
		Quotes:          []BookQuote{{Book: "book-a", HomeAmerican: 50, AwayAmerican: -120}},
	})
	if !errors.Is(err, ErrInvalidAmericanOdds) {
		t.Fatalf("expected ErrInvalidAmericanOdds, got %v", err)
	}

	_, err = ShopBestLine(LineShoppingInput{
		Sport:           domain.Sport("soccer"),
		RecommendedSide: awaySide,
		Quotes:          []BookQuote{{Book: "book-a", HomeAmerican: 110, AwayAmerican: -120}},
	})
	if !errors.Is(err, ErrUnsupportedSport) {
		t.Fatalf("expected ErrUnsupportedSport, got %v", err)
	}
}
