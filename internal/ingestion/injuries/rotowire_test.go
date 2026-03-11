package injuries

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRotowireProviderFetchMapsRows(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/football/tables/injury-report.php" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("team"); got != "ALL" {
			t.Fatalf("team query = %q, want ALL", got)
		}
		if got := r.URL.Query().Get("pos"); got != "ALL" {
			t.Fatalf("pos query = %q, want ALL", got)
		}
		_, _ = io.WriteString(w, `[{"ID":"12483","firstname":"Josh","lastname":"Allen","player":"Josh Allen","URL":"/football/player/josh-allen-12483","team":"BUF","position":"QB","injury":"Foot","status":"Questionable","rDate":"<i>Subscribers Only</i>"}]`)
	}))
	defer server.Close()

	provider := NewRotowireProvider(server.URL, time.Second)
	snapshot, err := provider.Fetch(context.Background(), Request{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		ReportDate:  time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Sport:       "nfl",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Source != defaultInjurySource {
		t.Fatalf("snapshot source = %q, want %q", snapshot.Source, defaultInjurySource)
	}
	if len(snapshot.Records) != 1 {
		t.Fatalf("record count = %d, want 1", len(snapshot.Records))
	}

	record := snapshot.Records[0]
	if record.ExternalID != "12483" || record.PlayerName != "Josh Allen" {
		t.Fatalf("unexpected record identity: %+v", record)
	}
	if record.TeamExternalID != "BUF" || record.Position != "QB" {
		t.Fatalf("unexpected team/position mapping: %+v", record)
	}
	if record.Injury != "Foot" || record.Status != "Questionable" {
		t.Fatalf("unexpected injury/status mapping: %+v", record)
	}
	if record.EstimatedReturn != nil {
		t.Fatalf("expected nil estimated return for subscribers-only value, got %#v", record.EstimatedReturn)
	}
	if record.PlayerURL != server.URL+"/football/player/josh-allen-12483" {
		t.Fatalf("unexpected player url %q", record.PlayerURL)
	}
	if len(record.RawJSON) == 0 {
		t.Fatal("expected raw json to be populated")
	}
}

func TestRotowireProviderRejectsUnsupportedSport(t *testing.T) {
	t.Parallel()

	provider := NewRotowireProvider("https://example.com", time.Second)
	_, err := provider.Fetch(context.Background(), Request{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		ReportDate:  time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Sport:       "nba",
	})
	if err == nil {
		t.Fatal("expected unsupported sport error")
	}
	if !strings.Contains(err.Error(), "only supports nfl") {
		t.Fatalf("unexpected error: %v", err)
	}
}
