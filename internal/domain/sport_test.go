package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestSeasonWindowContains(t *testing.T) {
	tests := []struct {
		name   string
		window SeasonWindow
		at     time.Time
		want   bool
	}{
		{
			name:   "within year active",
			window: SeasonWindow{StartMonth: time.March, StartDay: 1, EndMonth: time.November, EndDay: 15},
			at:     time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC),
			want:   true,
		},
		{
			name:   "within year inactive",
			window: SeasonWindow{StartMonth: time.March, StartDay: 1, EndMonth: time.November, EndDay: 15},
			at:     time.Date(2026, time.January, 11, 0, 0, 0, 0, time.UTC),
			want:   false,
		},
		{
			name:   "cross year active in january",
			window: SeasonWindow{StartMonth: time.October, StartDay: 1, EndMonth: time.June, EndDay: 30},
			at:     time.Date(2026, time.January, 11, 0, 0, 0, 0, time.UTC),
			want:   true,
		},
		{
			name:   "cross year inactive in july",
			window: SeasonWindow{StartMonth: time.October, StartDay: 1, EndMonth: time.June, EndDay: 30},
			at:     time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.window.Contains(tt.at); got != tt.want {
				t.Fatalf("Contains(%s) = %v, want %v", tt.at.Format(time.DateOnly), got, tt.want)
			}
		})
	}
}

func TestDefaultSportRegistryGetByOddsAPIKey(t *testing.T) {
	registry := DefaultSportRegistry()
	config, ok := registry.GetByOddsAPIKey("basketball_nba")
	if !ok {
		t.Fatal("expected basketball_nba to resolve")
	}
	if config.ID != SportNBA {
		t.Fatalf("expected NBA config, got %s", config.ID)
	}
	if config.PollCadence.Pregame <= 0 || config.PollCadence.Live <= 0 {
		t.Fatalf("expected positive poll cadence, got %+v", config.PollCadence)
	}
}

func TestDefaultSportRegistryActiveOddsAPISports(t *testing.T) {
	registry := DefaultSportRegistry()
	at := time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC)
	got := registry.ActiveOddsAPISports(at, []string{"basketball_nba", "icehockey_nhl", "americanfootball_nfl"})
	want := []string{"basketball_nba", "icehockey_nhl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveOddsAPISports() = %v, want %v", got, want)
	}
}
