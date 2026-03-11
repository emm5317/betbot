package injuries

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeProvider struct {
	snapshot Snapshot
	err      error
}

func (p fakeProvider) Fetch(context.Context, Request) (Snapshot, error) {
	return p.snapshot, p.err
}

type fakeStore struct {
	upserts []store.UpsertPlayerInjuryReportParams
}

func (s *fakeStore) UpsertPlayerInjuryReport(_ context.Context, arg store.UpsertPlayerInjuryReportParams) error {
	s.upserts = append(s.upserts, arg)
	return nil
}

func TestScraperRunNormalizesAndMapsPayload(t *testing.T) {
	storeSink := &fakeStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scraper := NewScraper(fakeProvider{snapshot: Snapshot{
		Source:     " ROTOWIRE ",
		Sport:      " NFL ",
		ReportDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Records: []Record{{
			ExternalID:      " 12483 ",
			PlayerName:      "  Josh   Allen ",
			TeamExternalID:  " BUF ",
			Position:        " QB ",
			Injury:          " Foot ",
			Status:          " Questionable ",
			EstimatedReturn: strPtr(" Week 4 "),
			PlayerURL:       " https://www.rotowire.com/football/player/josh-allen-12483 ",
			RawJSON:         json.RawMessage(`{"ID":"12483"}`),
		}},
	}}, logger)

	metrics, err := scraper.Run(context.Background(), storeSink, Request{
		RequestedAt: time.Date(2026, time.March, 12, 6, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run injury scraper: %v", err)
	}

	if metrics.RecordRows != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if len(storeSink.upserts) != 1 {
		t.Fatalf("unexpected upserts: %d", len(storeSink.upserts))
	}

	record := storeSink.upserts[0]
	if record.Source != defaultInjurySource {
		t.Fatalf("expected normalized source %q, got %q", defaultInjurySource, record.Source)
	}
	if record.Sport != defaultInjurySport {
		t.Fatalf("expected normalized sport %q, got %q", defaultInjurySport, record.Sport)
	}
	if record.ExternalID != "12483" {
		t.Fatalf("expected normalized external id 12483, got %q", record.ExternalID)
	}
	if record.PlayerName != "Josh Allen" {
		t.Fatalf("expected normalized player name, got %q", record.PlayerName)
	}
	if record.TeamExternalID != "buf" {
		t.Fatalf("expected normalized team external id buf, got %q", record.TeamExternalID)
	}
	if !record.ReportDate.Valid || !record.ReportDate.Time.Equal(time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected normalized report date: %+v", record.ReportDate)
	}
	if record.EstimatedReturn == nil || *record.EstimatedReturn != "Week 4" {
		t.Fatalf("unexpected estimated return: %#v", record.EstimatedReturn)
	}
}

func TestScraperRunRejectsMissingIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scraper := NewScraper(fakeProvider{snapshot: Snapshot{
		Records: []Record{{
			PlayerName:     "Josh Allen",
			TeamExternalID: "buf",
			Position:       "QB",
			Injury:         "Foot",
			Status:         "Questionable",
			PlayerURL:      "https://www.rotowire.com/football/player/josh-allen-12483",
			RawJSON:        json.RawMessage(`{"ID":"12483"}`),
		}},
	}}, logger)

	_, err := scraper.Run(context.Background(), &fakeStore{}, Request{
		RequestedAt: time.Date(2026, time.March, 11, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected validation error for missing injury external id")
	}
}

func strPtr(value string) *string {
	return &value
}
