package integration_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"betbot/internal/execution"
	"betbot/internal/ingestion/scores"
	"betbot/internal/store"
	"betbot/internal/worker"

	"github.com/riverqueue/river"
)

func TestAutoSettlementWorkerSettlesPlacedBetFromScores(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	queries := store.New(pool)
	game, err := queries.UpsertGame(ctx, store.UpsertGameParams{
		Source:       "the-odds-api",
		ExternalID:   "score-event-1",
		Sport:        "MLB",
		HomeTeam:     "Boston Red Sox",
		AwayTeam:     "New York Yankees",
		CommenceTime: store.Timestamptz(time.Date(2026, time.March, 11, 18, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("upsert game: %v", err)
	}

	bet, err := queries.InsertManualBet(ctx, store.InsertManualBetParams{
		IdempotencyKey:  "manual-bet-auto-settle-1",
		GameID:          game.ID,
		Sport:           "MLB",
		MarketKey:       "h2h",
		RecommendedSide: "home",
		BookKey:         "draftkings",
		AmericanOdds:    110,
		StakeCents:      10000,
	})
	if err != nil {
		t.Fatalf("insert manual bet: %v", err)
	}

	if _, err := queries.InsertBankrollEntry(ctx, store.InsertBankrollEntryParams{
		EntryType:     "bet_stake_reserved",
		AmountCents:   -bet.StakeCents,
		Currency:      "USD",
		ReferenceType: "bet",
		ReferenceID:   bet.IdempotencyKey,
		Metadata:      []byte(`{"bet_id":1}`),
	}); err != nil {
		t.Fatalf("insert stake reservation ledger entry: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/sports/baseball_mlb/scores" {
			t.Fatalf("scores path = %q, want /v4/sports/baseball_mlb/scores", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"id":"score-event-1",
				"sport_key":"baseball_mlb",
				"home_team":"Boston Red Sox",
				"away_team":"New York Yankees",
				"completed":true,
				"scores":[
					{"name":"Boston Red Sox","score":"6"},
					{"name":"New York Yankees","score":"2"}
				]
			}
		]`))
	}))
	defer server.Close()

	scoreClient := scores.NewClient("integration-key", server.URL+"/v4", 2*time.Second, 0)
	settlementWorker := worker.NewAutoSettlementWorker(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		scoreClient,
		"the-odds-api",
	)

	job := &river.Job[worker.AutoSettlementArgs]{
		Args: worker.AutoSettlementArgs{RequestedAt: time.Now().UTC()},
	}
	if err := settlementWorker.Work(ctx, job); err != nil {
		t.Fatalf("auto settlement worker work: %v", err)
	}

	settledBet, err := queries.GetBetByID(ctx, bet.ID)
	if err != nil {
		t.Fatalf("get settled bet: %v", err)
	}
	if settledBet.Status != store.BetStatusSettled {
		t.Fatalf("bet status = %q, want settled", settledBet.Status)
	}
	if settledBet.SettlementResult == nil || *settledBet.SettlementResult != "win" {
		t.Fatalf("settlement result = %v, want win", settledBet.SettlementResult)
	}
	if settledBet.PayoutCents == nil {
		t.Fatal("payout_cents is nil, want value")
	}
	expectedPayout := bet.StakeCents + execution.CalculateWinnings(bet.StakeCents, int(bet.AmericanOdds))
	if *settledBet.PayoutCents != expectedPayout {
		t.Fatalf("payout_cents = %d, want %d", *settledBet.PayoutCents, expectedPayout)
	}

	entries, err := queries.ListBankrollEntries(ctx, 20)
	if err != nil {
		t.Fatalf("list bankroll entries: %v", err)
	}
	var found bool
	for _, entry := range entries {
		if entry.ReferenceID == bet.IdempotencyKey && entry.EntryType == "bet_settlement_win" {
			if entry.AmountCents != expectedPayout {
				t.Fatalf("settlement ledger amount = %d, want %d", entry.AmountCents, expectedPayout)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected settlement ledger entry for bet %q", bet.IdempotencyKey)
	}
}
