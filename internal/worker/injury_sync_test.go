package worker

import (
	"context"
	"testing"
	"time"

	"betbot/internal/ingestion/injuries"
)

func TestInjurySyncArgsInsertOpts(t *testing.T) {
	opts := (InjurySyncArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected injury sync jobs to be unique by args")
	}
}

func TestEnqueueInjurySyncNormalizesArgs(t *testing.T) {
	inserter := &recordingJobInserter{}
	_, err := EnqueueInjurySync(context.Background(), inserter, injuries.Request{
		ReportDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
	})
	if err != nil {
		t.Fatalf("EnqueueInjurySync() error = %v", err)
	}
	if inserter.opts != nil {
		t.Fatalf("expected nil explicit insert opts, got %+v", inserter.opts)
	}

	args, ok := inserter.args.(InjurySyncArgs)
	if !ok {
		t.Fatalf("inserted args type = %T, want InjurySyncArgs", inserter.args)
	}
	if args.Sport != "nfl" {
		t.Fatalf("sport = %q, want nfl", args.Sport)
	}
	if args.Source != "rotowire" {
		t.Fatalf("source = %q, want rotowire", args.Source)
	}
	expectedDate := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	if !args.ReportDate.Equal(expectedDate) {
		t.Fatalf("report date = %s, want %s", args.ReportDate.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if !args.RequestedAt.Equal(expectedDate) {
		t.Fatalf("requested at = %s, want %s", args.RequestedAt.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
}
