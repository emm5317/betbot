package execution

import (
	"math"
	"testing"
)

func TestComputeCLVDelta(t *testing.T) {
	tests := []struct {
		name       string
		placed     float64
		closing    float64
		wantDelta  float64
	}{
		{"positive CLV", 0.60, 0.55, 0.05},
		{"negative CLV", 0.50, 0.55, -0.05},
		{"zero CLV", 0.50, 0.50, 0.00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCLVDelta(tt.placed, tt.closing)
			if math.Abs(got-tt.wantDelta) > 1e-9 {
				t.Fatalf("ComputeCLVDelta(%f, %f) = %f, want %f", tt.placed, tt.closing, got, tt.wantDelta)
			}
		})
	}
}

func TestAmericanOddsToImpliedProbability(t *testing.T) {
	tests := []struct {
		odds     int
		wantProb float64
	}{
		{-150, 0.6},    // 150/(150+100) = 0.6
		{+130, 100.0 / 230.0},
		{-100, 0.5},
		{+100, 0.5},
		{-200, 200.0 / 300.0},
	}

	for _, tt := range tests {
		prob := AmericanOddsToImpliedProbability(tt.odds)
		if math.Abs(prob-tt.wantProb) > 1e-6 {
			t.Fatalf("AmericanOddsToImpliedProbability(%d) = %f, want %f", tt.odds, prob, tt.wantProb)
		}
	}
}
