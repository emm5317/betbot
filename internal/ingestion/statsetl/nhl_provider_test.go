package statsetl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNHLStatsAPIProviderFetchMapsStandings(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/standings/2026-03-11" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"standings":[{"gameTypeId":2,"seasonId":20252026,"gamesPlayed":64,"wins":39,"losses":18,"otLosses":7,"goalAgainst":171,"goalsForPctg":3.40625,"teamName":{"default":"Boston Bruins"},"teamAbbrev":{"default":"BOS"}}]}`)
	}))
	defer server.Close()

	provider := NewNHLStatsAPIProvider(server.URL+"/v1", time.Second)
	snapshot, err := provider.Fetch(context.Background(), NHLRequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		SeasonType:  "regular",
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Source != defaultNHLSource {
		t.Fatalf("snapshot source = %q, want %q", snapshot.Source, defaultNHLSource)
	}
	if len(snapshot.Teams) != 1 {
		t.Fatalf("team count = %d, want 1", len(snapshot.Teams))
	}

	team := snapshot.Teams[0]
	if team.ExternalID != "BOS" || team.TeamName != "Boston Bruins" {
		t.Fatalf("unexpected team identity: %+v", team)
	}
	if team.GamesPlayed != 64 || team.Wins != 39 || team.Losses != 18 || team.OTLosses != 7 {
		t.Fatalf("unexpected record: %+v", team)
	}
	if team.GoalsForPerGame == nil || *team.GoalsForPerGame != 3.40625 {
		t.Fatalf("unexpected goals for per game: %#v", team.GoalsForPerGame)
	}
	if team.GoalsAgainstPerGame == nil || *team.GoalsAgainstPerGame != 171.0/64.0 {
		t.Fatalf("unexpected goals against per game: %#v", team.GoalsAgainstPerGame)
	}
	if team.ExpectedGoalsShare != nil {
		t.Fatalf("expected nil xG share, got %#v", team.ExpectedGoalsShare)
	}
	if team.SavePercentage != nil {
		t.Fatalf("expected nil save percentage, got %#v", team.SavePercentage)
	}
}

func TestNHLStatsAPIProviderRejectsUnsupportedSeasonType(t *testing.T) {
	t.Parallel()

	provider := NewNHLStatsAPIProvider("https://example.com", time.Second)
	_, err := provider.Fetch(context.Background(), NHLRequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		SeasonType:  "playoffs",
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected unsupported season type error")
	}
	if !strings.Contains(err.Error(), "only supports regular") {
		t.Fatalf("unexpected error: %v", err)
	}
}
