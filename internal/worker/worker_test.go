package worker

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"betbot/internal/domain"

	"github.com/riverqueue/river"
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

func TestOddsPollWorkerSkipsWhenDisabled(t *testing.T) {
	worker := &OddsPollWorker{
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		enabled:        false,
		disabledReason: "disabled-by-config",
	}
	job := &river.Job[OddsPollArgs]{
		Args: OddsPollArgs{RequestedAt: time.Date(2026, time.March, 12, 8, 0, 0, 0, time.UTC)},
	}

	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work() error = %v, want nil", err)
	}
}
