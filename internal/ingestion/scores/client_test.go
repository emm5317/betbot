package scores

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchSportFiltersToCompletedScores(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/sports/baseball_mlb/scores" {
			t.Fatalf("path = %q, want /v4/sports/baseball_mlb/scores", r.URL.Path)
		}
		if got := r.URL.Query().Get("apiKey"); got != "test-key" {
			t.Fatalf("apiKey = %q, want test-key", got)
		}
		if got := r.URL.Query().Get("daysFrom"); got != "3" {
			t.Fatalf("daysFrom = %q, want 3", got)
		}
		_, _ = w.Write([]byte(`[
			{"id":"skip-not-completed","home_team":"A","away_team":"B","completed":false,"scores":[{"name":"A","score":"1"},{"name":"B","score":"0"}]},
			{"id":"skip-missing-scores","home_team":"C","away_team":"D","completed":true,"scores":null},
			{"id":"keep","sport_key":"baseball_mlb","home_team":"Home","away_team":"Away","completed":true,"scores":[{"name":"Home","score":"5"},{"name":"Away","score":"3"}]}
		]`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL+"/v4", 2*time.Second, 0)
	rows, err := client.FetchSport(context.Background(), "baseball_mlb")
	if err != nil {
		t.Fatalf("FetchSport() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ExternalID != "keep" {
		t.Fatalf("rows[0].ExternalID = %q, want keep", rows[0].ExternalID)
	}
	if rows[0].HomeScore != 5 || rows[0].AwayScore != 3 {
		t.Fatalf("scores = %d-%d, want 5-3", rows[0].HomeScore, rows[0].AwayScore)
	}
}

func TestClientFetchSportReturnsErrorOnHTTPFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failure", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL+"/v4", 2*time.Second, 0)
	if _, err := client.FetchSport(context.Background(), "baseball_mlb"); err == nil {
		t.Fatal("expected FetchSport() to return error for non-200 status")
	}
}

func TestToGameScoreSkipsInvalidTeamScores(t *testing.T) {
	event := apiScoreEvent{
		ID:        "bad-score",
		HomeTeam:  "Home",
		AwayTeam:  "Away",
		Completed: true,
		Scores: []apiTeamScore{
			{Name: "Home", Score: "not-a-number"},
			{Name: "Away", Score: "3"},
		},
	}

	if _, ok := toGameScore(event); ok {
		t.Fatal("expected toGameScore() to skip invalid score rows")
	}
}
