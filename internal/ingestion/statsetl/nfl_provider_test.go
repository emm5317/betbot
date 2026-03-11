package statsetl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNFLverseProviderFetchMapsTeamStats(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/stats_team/stats_team_reg_2025.csv":
			_, _ = io.WriteString(w, strings.Join([]string{
				"season,team,season_type,games,attempts,sacks_suffered,carries,passing_epa,rushing_epa",
				"2025,BUF,REG,17,579,14,465,83.4,21.6",
			}, "\n"))
		case "/standings/division/2025/REG":
			_, _ = io.WriteString(w, `
<table>
  <tbody>
    <tr>
      <td scope="row" tabindex="0">
        <a class="d3-o-club-info" href="/teams/buffalo-bills/" aria-label="Go to Buffalo Bills info page.">
          <div class="d3-o-club-logo"><img src="https://static.www.nfl.com/t_q-best/league/api/clubs/logos/BUF" /></div>
          <div class="d3-o-club-fullname">Buffalo Bills</div>
        </a>
      </td>
      <td>13</td>
      <td>4</td>
      <td>0</td>
      <td>0.765</td>
      <td>525</td>
      <td>368</td>
      <td>157</td>
    </tr>
  </tbody>
</table>`)
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewNFLverseProvider(server.URL, server.URL, time.Second)
	snapshot, err := provider.Fetch(context.Background(), NFLRequest{
		RequestedAt: time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
		Season:      2025,
		SeasonType:  "regular",
		StatDate:    time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Source != defaultNFLSource {
		t.Fatalf("snapshot source = %q, want %q", snapshot.Source, defaultNFLSource)
	}
	if len(snapshot.Teams) != 1 {
		t.Fatalf("team count = %d, want 1", len(snapshot.Teams))
	}

	team := snapshot.Teams[0]
	if team.ExternalID != "buf" || team.TeamName != "Buffalo Bills" {
		t.Fatalf("unexpected team identity: %+v", team)
	}
	if team.GamesPlayed != 17 || team.Wins != 13 || team.Losses != 4 || team.Ties != 0 {
		t.Fatalf("unexpected record: %+v", team)
	}
	if team.PointsFor != 525 || team.PointsAgainst != 368 {
		t.Fatalf("unexpected point totals: %+v", team)
	}
	wantEPA := (83.4 + 21.6) / (579.0 + 14.0 + 465.0)
	if team.OffensiveEPAPerPlay == nil || fmt.Sprintf("%.6f", *team.OffensiveEPAPerPlay) != fmt.Sprintf("%.6f", wantEPA) {
		t.Fatalf("unexpected offensive epa/play: %#v want %.6f", team.OffensiveEPAPerPlay, wantEPA)
	}
	if team.DefensiveEPAPerPlay != nil {
		t.Fatalf("expected nil defensive epa/play, got %#v", team.DefensiveEPAPerPlay)
	}
	if team.OffensiveSuccessRate != nil || team.DefensiveSuccessRate != nil {
		t.Fatalf("expected nil success rates, got off=%#v def=%#v", team.OffensiveSuccessRate, team.DefensiveSuccessRate)
	}
}

func TestNFLverseProviderPropagatesHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/download/stats_team/stats_team_reg_2025.csv" {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewNFLverseProvider(server.URL, server.URL, time.Second)
	_, err := provider.Fetch(context.Background(), NFLRequest{
		RequestedAt: time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
		Season:      2025,
		SeasonType:  "regular",
		StatDate:    time.Date(2026, time.January, 19, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("unexpected error: %v", err)
	}
}
