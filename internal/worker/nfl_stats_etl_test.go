package worker

import (
	"context"
	"testing"
	"time"

	"betbot/internal/ingestion/statsetl"
)

func TestNFLStatsETLArgsInsertOpts(t *testing.T) {
	opts := (NFLStatsETLArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected nfl stats etl jobs to be unique by args")
	}
}

func TestEnqueueNFLStatsETLNormalizesArgs(t *testing.T) {
	inserter := &recordingJobInserter{}
	_, err := EnqueueNFLStatsETL(context.Background(), inserter, statsetl.NFLRequest{
		StatDate: time.Date(2026, time.January, 18, 23, 45, 0, 0, time.FixedZone("CST", -6*60*60)),
	})
	if err != nil {
		t.Fatalf("EnqueueNFLStatsETL() error = %v", err)
	}
	if inserter.opts != nil {
		t.Fatalf("expected nil explicit insert opts, got %+v", inserter.opts)
	}

	args, ok := inserter.args.(NFLStatsETLArgs)
	if !ok {
		t.Fatalf("inserted args type = %T, want NFLStatsETLArgs", inserter.args)
	}
	if args.Season != 2025 {
		t.Fatalf("season = %d, want 2025", args.Season)
	}
	if args.SeasonType != "regular" {
		t.Fatalf("season type = %q, want regular", args.SeasonType)
	}
	expectedDate := time.Date(2026, time.January, 19, 0, 0, 0, 0, time.UTC)
	if !args.StatDate.Equal(expectedDate) {
		t.Fatalf("stat date = %s, want %s", args.StatDate.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if !args.RequestedAt.Equal(expectedDate) {
		t.Fatalf("requested at = %s, want %s", args.RequestedAt.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
}
