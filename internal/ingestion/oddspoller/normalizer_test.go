package oddspoller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeSamplePayload(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "odds_snapshots", "sample_odds_api_response.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var games []APIGame
	if err := json.Unmarshal(raw, &games); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	for i := range games {
		games[i].Raw = raw
	}

	payload, err := NewNormalizer("the-odds-api").Normalize(games, time.Date(2026, 3, 11, 23, 59, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if len(payload.Games) != 1 {
		t.Fatalf("games len = %d, want 1", len(payload.Games))
	}

	if payload.Games[0].Sport != "NBA" {
		t.Fatalf("sport = %q, want NBA", payload.Games[0].Sport)
	}

	if len(payload.Snapshots) != 4 {
		t.Fatalf("snapshots len = %d, want 4", len(payload.Snapshots))
	}

	first := payload.Snapshots[0]
	if first.OutcomeSide != "home" {
		t.Fatalf("first outcome side = %q, want home", first.OutcomeSide)
	}

	if first.SnapshotHash == "" {
		t.Fatal("expected snapshot hash")
	}
}

func TestImpliedProbability(t *testing.T) {
	if got := impliedProbability(-150); got < 0.59 || got > 0.61 {
		t.Fatalf("impliedProbability(-150) = %f, want about 0.60", got)
	}

	if got := impliedProbability(130); got < 0.43 || got > 0.44 {
		t.Fatalf("impliedProbability(130) = %f, want about 0.435", got)
	}
}
