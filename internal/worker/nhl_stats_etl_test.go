package worker

import (
	"context"
	"testing"
	"time"

	"betbot/internal/ingestion/statsetl"
)

func TestNHLStatsETLArgsInsertOpts(t *testing.T) {
	opts := (NHLStatsETLArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected nhl stats etl jobs to be unique by args")
	}
}

func TestEnqueueNHLStatsETLNormalizesArgs(t *testing.T) {
	inserter := &recordingJobInserter{}
	_, err := EnqueueNHLStatsETL(context.Background(), inserter, statsetl.NHLRequest{
		StatDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
	})
	if err != nil {
		t.Fatalf("EnqueueNHLStatsETL() error = %v", err)
	}
	if inserter.opts != nil {
		t.Fatalf("expected nil explicit insert opts, got %+v", inserter.opts)
	}

	args, ok := inserter.args.(NHLStatsETLArgs)
	if !ok {
		t.Fatalf("inserted args type = %T, want NHLStatsETLArgs", inserter.args)
	}
	if args.Season != 2026 {
		t.Fatalf("season = %d, want 2026", args.Season)
	}
	if args.SeasonType != "regular" {
		t.Fatalf("season type = %q, want regular", args.SeasonType)
	}
	expectedDate := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	if !args.StatDate.Equal(expectedDate) {
		t.Fatalf("stat date = %s, want %s", args.StatDate.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if !args.RequestedAt.Equal(expectedDate) {
		t.Fatalf("requested at = %s, want %s", args.RequestedAt.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
}
