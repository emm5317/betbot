package worker

import (
	"context"
	"testing"
	"time"

	"betbot/internal/ingestion/weather"
)

func TestWeatherSyncArgsInsertOpts(t *testing.T) {
	opts := (WeatherSyncArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected weather sync jobs to be unique by args")
	}
}

func TestEnqueueWeatherSyncNormalizesArgs(t *testing.T) {
	inserter := &recordingJobInserter{}
	_, err := EnqueueWeatherSync(context.Background(), inserter, weather.Request{
		ForecastDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Sport:        " nfl ",
	})
	if err != nil {
		t.Fatalf("EnqueueWeatherSync() error = %v", err)
	}
	if inserter.opts != nil {
		t.Fatalf("expected nil explicit insert opts, got %+v", inserter.opts)
	}

	args, ok := inserter.args.(WeatherSyncArgs)
	if !ok {
		t.Fatalf("inserted args type = %T, want WeatherSyncArgs", inserter.args)
	}
	expectedDate := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	if !args.ForecastDate.Equal(expectedDate) {
		t.Fatalf("forecast date = %s, want %s", args.ForecastDate.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if !args.RequestedAt.Equal(expectedDate) {
		t.Fatalf("requested at = %s, want %s", args.RequestedAt.Format(time.RFC3339), expectedDate.Format(time.RFC3339))
	}
	if args.Sport != "NFL" {
		t.Fatalf("sport = %q, want NFL", args.Sport)
	}
}
