package statsetl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMLBStatsAPIProviderFetchMapsTeamAndPitcherStats(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/teams/stats":
			if got := r.URL.Query().Get("stats"); got != "season" {
				t.Fatalf("team stats query stats = %q, want season", got)
			}
			if got := r.URL.Query().Get("group"); got != "hitting,pitching" {
				t.Fatalf("team stats query group = %q, want hitting,pitching", got)
			}
			if got := r.URL.Query().Get("gameType"); got != "R" {
				t.Fatalf("team stats query gameType = %q, want R", got)
			}
			_, _ = io.WriteString(w, `{"stats":[{"group":{"displayName":"hitting"},"splits":[{"team":{"id":111,"name":"Boston Red Sox"},"stat":{"gamesPlayed":12,"runs":58,"ops":"0.771"}}]},{"group":{"displayName":"pitching"},"splits":[{"team":{"id":111,"name":"Boston Red Sox"},"stat":{"gamesPlayed":12,"wins":7,"losses":5,"runs":49,"era":"3.44"}}]}]}`)
		case "/api/v1/stats":
			if got := r.URL.Query().Get("playerPool"); got != "ALL" {
				t.Fatalf("pitcher stats query playerPool = %q, want ALL", got)
			}
			if got := r.URL.Query().Get("limit"); got != mlbPitcherQueryResultLimit {
				t.Fatalf("pitcher stats query limit = %q, want %s", got, mlbPitcherQueryResultLimit)
			}
			_, _ = io.WriteString(w, `{"stats":[{"splits":[{"team":{"id":111,"name":"Boston Red Sox"},"player":{"id":12345,"fullName":"Chris Sale"},"stat":{"gamesStarted":3,"strikeOuts":31,"baseOnBalls":6,"battersFaced":100,"inningsPitched":"18.2","era":"2.95","whip":"1.01"}}]}]}`)
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewMLBStatsAPIProvider(server.URL+"/api/v1", time.Second)
	snapshot, err := provider.Fetch(context.Background(), MLBRequest{
		RequestedAt: time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
		Season:      2026,
		SeasonType:  "regular",
		StatDate:    time.Date(2026, time.March, 11, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Source != defaultMLBProviderSource {
		t.Fatalf("snapshot source = %q, want %q", snapshot.Source, defaultMLBProviderSource)
	}
	if len(snapshot.Teams) != 1 {
		t.Fatalf("team count = %d, want 1", len(snapshot.Teams))
	}
	if len(snapshot.Pitchers) != 1 {
		t.Fatalf("pitcher count = %d, want 1", len(snapshot.Pitchers))
	}

	team := snapshot.Teams[0]
	if team.ExternalID != "111" || team.TeamName != "Boston Red Sox" {
		t.Fatalf("unexpected team identity: %+v", team)
	}
	if team.GamesPlayed != 12 || team.Wins != 7 || team.Losses != 5 || team.RunsScored != 58 || team.RunsAllowed != 49 {
		t.Fatalf("unexpected team totals: %+v", team)
	}
	if team.BattingOPS == nil || *team.BattingOPS != 0.771 {
		t.Fatalf("unexpected batting ops: %#v", team.BattingOPS)
	}
	if team.TeamERA == nil || *team.TeamERA != 3.44 {
		t.Fatalf("unexpected team era: %#v", team.TeamERA)
	}

	pitcher := snapshot.Pitchers[0]
	if pitcher.ExternalID != "12345" || pitcher.PlayerName != "Chris Sale" {
		t.Fatalf("unexpected pitcher identity: %+v", pitcher)
	}
	if pitcher.TeamExternalID != "111" || pitcher.TeamName != "Boston Red Sox" {
		t.Fatalf("unexpected pitcher team identity: %+v", pitcher)
	}
	if pitcher.GamesStarted != 3 {
		t.Fatalf("games started = %d, want 3", pitcher.GamesStarted)
	}
	if pitcher.InningsPitched == nil || *pitcher.InningsPitched != 18.2 {
		t.Fatalf("unexpected innings pitched: %#v", pitcher.InningsPitched)
	}
	if pitcher.Era == nil || *pitcher.Era != 2.95 {
		t.Fatalf("unexpected era: %#v", pitcher.Era)
	}
	if pitcher.Whip == nil || *pitcher.Whip != 1.01 {
		t.Fatalf("unexpected whip: %#v", pitcher.Whip)
	}
	if pitcher.StrikeoutRate == nil || *pitcher.StrikeoutRate != 0.31 {
		t.Fatalf("unexpected strikeout rate: %#v", pitcher.StrikeoutRate)
	}
	if pitcher.WalkRate == nil || *pitcher.WalkRate != 0.06 {
		t.Fatalf("unexpected walk rate: %#v", pitcher.WalkRate)
	}
	if pitcher.Fip != nil {
		t.Fatalf("expected nil fip, got %#v", pitcher.Fip)
	}
}
