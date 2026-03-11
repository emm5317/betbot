package worker

import (
	"context"
	"testing"
	"time"

	"betbot/internal/ingestion/statsetl"
)

func TestNBAStatsETLArgsInsertOpts(t *testing.T) {
	opts := (NBAStatsETLArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected nba stats etl jobs to be unique by args")
	}
}

func TestEnqueueNBAStatsETLNormalizesArgs(t *testing.T) {
	inserter := &recordingJobInserter{}
	_, err := EnqueueNBAStatsETL(context.Background(), inserter, statsetl.NBARequest{
		StatDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
	})
	if err != nil {
		t.Fatalf("EnqueueNBAStatsETL() error = %v", err)
	}
	if inserter.opts != nil {
		t.Fatalf("expected nil explicit insert opts, got %+v", inserter.opts)
	}

	args, ok := inserter.args.(NBAStatsETLArgs)
	if !ok {
		t.Fatalf("inserted args type = %T, want NBAStatsETLArgs", inserter.args)
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
