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

func TestNBAStatsAPIProviderFetchMapsTeamEstimatedMetrics(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stats/teamestimatedmetrics" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("LeagueID"); got != nbaLeagueID {
			t.Fatalf("LeagueID = %q, want %q", got, nbaLeagueID)
		}
		if got := r.URL.Query().Get("Season"); got != "2025-26" {
			t.Fatalf("Season = %q, want 2025-26", got)
		}
		if got := r.URL.Query().Get("SeasonType"); got != "Regular Season" {
			t.Fatalf("SeasonType = %q, want Regular Season", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Mozilla/5.0") {
			t.Fatalf("unexpected User-Agent %q", got)
		}

		_, _ = io.WriteString(w, `{"resultSets":[{"name":"TeamEstimatedMetrics","headers":["TEAM_ID","TEAM_NAME","GP","W","L","E_OFF_RATING","E_DEF_RATING","E_NET_RATING","E_PACE"],"rowSet":[[1610612738,"Boston Celtics",65,48,17,119.4,110.2,9.2,98.7]]}]}`)
	}))
	defer server.Close()

	provider := NewNBAStatsAPIProvider(server.URL, time.Second)
	snapshot, err := provider.Fetch(context.Background(), NBARequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		SeasonType:  "regular",
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Source != defaultNBAProviderSource {
		t.Fatalf("snapshot source = %q, want %q", snapshot.Source, defaultNBAProviderSource)
	}
	if len(snapshot.Teams) != 1 {
		t.Fatalf("team count = %d, want 1", len(snapshot.Teams))
	}

	team := snapshot.Teams[0]
	if team.ExternalID != "1610612738" || team.TeamName != "Boston Celtics" {
		t.Fatalf("unexpected team identity: %+v", team)
	}
	if team.GamesPlayed != 65 || team.Wins != 48 || team.Losses != 17 {
		t.Fatalf("unexpected record: %+v", team)
	}
	if team.OffensiveRating == nil || *team.OffensiveRating != 119.4 {
		t.Fatalf("unexpected offensive rating: %#v", team.OffensiveRating)
	}
	if team.DefensiveRating == nil || *team.DefensiveRating != 110.2 {
		t.Fatalf("unexpected defensive rating: %#v", team.DefensiveRating)
	}
	if team.NetRating == nil || *team.NetRating != 9.2 {
		t.Fatalf("unexpected net rating: %#v", team.NetRating)
	}
	if team.Pace == nil || *team.Pace != 98.7 {
		t.Fatalf("unexpected pace: %#v", team.Pace)
	}
}

func TestNBAStatsAPIProviderFetchReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := NewNBAStatsAPIProvider(server.URL, time.Second)
	_, err := provider.Fetch(context.Background(), NBARequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}
