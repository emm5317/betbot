package worker

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"betbot/internal/execution"
	"betbot/internal/store"

	"github.com/riverqueue/river"
)

type fakeAutoPlacementQueries struct {
	rows     []store.ListPlaceableRecommendationSnapshotsRow
	err      error
	rowLimit int32
}

func (f *fakeAutoPlacementQueries) ListPlaceableRecommendationSnapshots(_ context.Context, rowLimit int32) ([]store.ListPlaceableRecommendationSnapshotsRow, error) {
	f.rowLimit = rowLimit
	return f.rows, f.err
}

type fakeAutoPlacementOrchestrator struct {
	inputs []execution.PlaceInput
	result execution.PlaceResult
	err    error
}

func (f *fakeAutoPlacementOrchestrator) Place(_ context.Context, input execution.PlaceInput) (execution.PlaceResult, error) {
	f.inputs = append(f.inputs, input)
	return f.result, f.err
}

func TestAutoPlacementArgsInsertOpts(t *testing.T) {
	opts := (AutoPlacementArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if opts.UniqueOpts.ByPeriod != autoPlacementInterval {
		t.Fatalf("expected unique period %s, got %s", autoPlacementInterval, opts.UniqueOpts.ByPeriod)
	}
}

func TestAutoPlacementWorkerSkipsAlreadyPlacedCandidates(t *testing.T) {
	// Already-linked snapshot rows are excluded by ListPlaceableRecommendationSnapshots.
	queries := &fakeAutoPlacementQueries{
		rows: []store.ListPlaceableRecommendationSnapshotsRow{},
	}
	orchestrator := &fakeAutoPlacementOrchestrator{}
	worker := &AutoPlacementWorker{
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
		readQueries:           queries,
		placementOrchestrator: orchestrator,
	}

	err := worker.Work(context.Background(), &river.Job[AutoPlacementArgs]{
		Args: AutoPlacementArgs{RequestedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("work error: %v", err)
	}
	if len(orchestrator.inputs) != 0 {
		t.Fatalf("placement attempts = %d, want 0", len(orchestrator.inputs))
	}
	if queries.rowLimit != autoPlacementRowLimit {
		t.Fatalf("row limit = %d, want %d", queries.rowLimit, autoPlacementRowLimit)
	}
}

func TestAutoPlacementWorkerDeterministicIdempotencyKey(t *testing.T) {
	key1 := autoPlacementIdempotencyKey(501, 2001, "H2H", "DraftKings")
	key2 := autoPlacementIdempotencyKey(501, 2001, "H2H", "DraftKings")
	if key1 != key2 {
		t.Fatalf("idempotency keys must be deterministic: %q != %q", key1, key2)
	}
	if !strings.Contains(key1, strconv.FormatInt(501, 10)) {
		t.Fatalf("idempotency key %q does not include snapshot id", key1)
	}

	key3 := autoPlacementIdempotencyKey(502, 2001, "H2H", "DraftKings")
	if key1 == key3 {
		t.Fatalf("keys for different snapshots must differ: %q", key1)
	}
}
