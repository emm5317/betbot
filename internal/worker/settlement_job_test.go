package worker

import (
	"testing"

	"betbot/internal/ingestion/scores"
	"betbot/internal/store"
)

func TestAutoSettlementArgsInsertOpts(t *testing.T) {
	opts := (AutoSettlementArgs{}).InsertOpts()
	if opts.Queue != QueueMaintenance {
		t.Fatalf("expected queue %q, got %q", QueueMaintenance, opts.Queue)
	}
	if opts.UniqueOpts.ByPeriod != autoSettlementInterval {
		t.Fatalf("expected unique period %s, got %s", autoSettlementInterval, opts.UniqueOpts.ByPeriod)
	}
}

func TestDetermineAutoSettlementResult(t *testing.T) {
	tests := []struct {
		name       string
		bet        store.ListOpenBetsWithGameRow
		score      scores.GameScore
		wantResult string
		wantPayout int64
		wantOK     bool
	}{
		{
			name: "home win",
			bet: store.ListOpenBetsWithGameRow{
				MarketKey:       "h2h",
				RecommendedSide: "home",
				AmericanOdds:    110,
				StakeCents:      10000,
				GameSport:       "MLB",
			},
			score:      scores.GameScore{HomeScore: 5, AwayScore: 3, Completed: true},
			wantResult: "win",
			wantPayout: 21000,
			wantOK:     true,
		},
		{
			name: "away loss",
			bet: store.ListOpenBetsWithGameRow{
				MarketKey:       "h2h",
				RecommendedSide: "away",
				AmericanOdds:    -120,
				StakeCents:      10000,
				GameSport:       "MLB",
			},
			score:      scores.GameScore{HomeScore: 2, AwayScore: 1, Completed: true},
			wantResult: "loss",
			wantPayout: 0,
			wantOK:     true,
		},
		{
			name: "tie push for mlb",
			bet: store.ListOpenBetsWithGameRow{
				MarketKey:       "h2h",
				RecommendedSide: "home",
				AmericanOdds:    100,
				StakeCents:      7500,
				GameSport:       "MLB",
			},
			score:      scores.GameScore{HomeScore: 3, AwayScore: 3, Completed: true},
			wantResult: "push",
			wantPayout: 7500,
			wantOK:     true,
		},
		{
			name: "tie skipped for nhl",
			bet: store.ListOpenBetsWithGameRow{
				MarketKey:       "h2h",
				RecommendedSide: "away",
				AmericanOdds:    100,
				StakeCents:      5000,
				GameSport:       "NHL",
			},
			score:      scores.GameScore{HomeScore: 2, AwayScore: 2, Completed: true},
			wantResult: "",
			wantPayout: 0,
			wantOK:     false,
		},
		{
			name: "non h2h market skipped",
			bet: store.ListOpenBetsWithGameRow{
				MarketKey:       "spreads",
				RecommendedSide: "home",
				AmericanOdds:    100,
				StakeCents:      5000,
				GameSport:       "NBA",
			},
			score:      scores.GameScore{HomeScore: 100, AwayScore: 98, Completed: true},
			wantResult: "",
			wantPayout: 0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult, gotPayout, gotOK := determineAutoSettlementResult(tt.bet, tt.score)
			if gotOK != tt.wantOK {
				t.Fatalf("determineAutoSettlementResult() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotResult != tt.wantResult {
				t.Fatalf("determineAutoSettlementResult() result = %q, want %q", gotResult, tt.wantResult)
			}
			if gotPayout != tt.wantPayout {
				t.Fatalf("determineAutoSettlementResult() payout = %d, want %d", gotPayout, tt.wantPayout)
			}
		})
	}
}

func TestAutoSettlementWorkerToOddsAPISportKey(t *testing.T) {
	worker := NewAutoSettlementWorker(nil, nil, nil, "the-odds-api")

	got, ok := worker.toOddsAPISportKey("MLB")
	if !ok {
		t.Fatal("expected MLB to resolve to odds api key")
	}
	if got != "baseball_mlb" {
		t.Fatalf("MLB sport key = %q, want baseball_mlb", got)
	}

	got, ok = worker.toOddsAPISportKey("icehockey_nhl")
	if !ok {
		t.Fatal("expected odds api key passthrough to resolve")
	}
	if got != "icehockey_nhl" {
		t.Fatalf("icehockey_nhl sport key = %q, want icehockey_nhl", got)
	}
}
