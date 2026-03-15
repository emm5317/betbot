package prediction

import (
	"testing"
	"time"
)

func TestNHLSeasonFromDate(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		expected int32
	}{
		{"October start", time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC), 2025},
		{"December mid-season", time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), 2025},
		{"January same season", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), 2025},
		{"March same season", time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), 2025},
		{"June playoffs", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), 2025},
		{"September pre-season", time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), 2025},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nhlSeasonFromDate(tt.date)
			if got != tt.expected {
				t.Errorf("nhlSeasonFromDate(%v) = %d, want %d", tt.date, got, tt.expected)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want float64
	}{
		{5.0, 0.0, 10.0, 5.0},
		{-1.0, 0.0, 10.0, 0.0},
		{15.0, 0.0, 10.0, 10.0},
		{0.0, 0.0, 10.0, 0.0},
		{10.0, 0.0, 10.0, 10.0},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}
