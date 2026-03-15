package execution

import "math"

// ComputeCLVDelta calculates the Closing Line Value delta.
// Positive CLV means the bet was placed at better odds than the closing line.
// placedProb is the model's probability at placement time.
// closingProb is the market's implied probability at close.
func ComputeCLVDelta(placedProb, closingProb float64) float64 {
	return placedProb - closingProb
}

// AmericanOddsToImpliedProbability converts American odds to implied probability.
// Negative odds (e.g. -150): probability = |odds| / (|odds| + 100)
// Positive odds (e.g. +130): probability = 100 / (odds + 100)
func AmericanOddsToImpliedProbability(americanOdds int) float64 {
	if americanOdds < 0 {
		return math.Abs(float64(americanOdds)) / (math.Abs(float64(americanOdds)) + 100.0)
	}
	return 100.0 / (float64(americanOdds) + 100.0)
}
