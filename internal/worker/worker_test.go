package worker

import (
	"reflect"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestActiveOddsPollArgsFiltersToActiveSports(t *testing.T) {
	registry := domain.DefaultSportRegistry()
	args := activeOddsPollArgs(
		time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		registry,
		[]string{"baseball_mlb", "basketball_nba", "icehockey_nhl", "americanfootball_nfl"},
	)

	want := []string{"baseball_mlb", "basketball_nba", "icehockey_nhl"}
	if !reflect.DeepEqual(args.Sports, want) {
		t.Fatalf("activeOddsPollArgs().Sports = %v, want %v", args.Sports, want)
	}
}

func TestActiveOddsPollArgsReturnsEmptyWhenNoConfiguredSportIsActive(t *testing.T) {
	registry := domain.DefaultSportRegistry()
	args := activeOddsPollArgs(
		time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
		registry,
		[]string{"americanfootball_nfl"},
	)

	if len(args.Sports) != 0 {
		t.Fatalf("expected no active sports, got %v", args.Sports)
	}
}
