package backtest

import (
	"context"
	"reflect"
	"strconv"
	"testing"
	"time"

	"betbot/internal/domain"
	"betbot/internal/store"
)

type fakeReplayStore struct {
	rows      []store.ListBacktestReplayRowsRow
	upserts   []store.UpsertModelPredictionParams
	listCalls int
}

func (f *fakeReplayStore) ListBacktestReplayRows(_ context.Context, _ store.ListBacktestReplayRowsParams) ([]store.ListBacktestReplayRowsRow, error) {
	f.listCalls++
	out := make([]store.ListBacktestReplayRowsRow, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeReplayStore) UpsertModelPrediction(_ context.Context, arg store.UpsertModelPredictionParams) (store.ModelPrediction, error) {
	f.upserts = append(f.upserts, arg)
	return store.ModelPrediction{ID: int64(len(f.upserts)), GameID: arg.GameID, ModelFamily: arg.ModelFamily, ModelVersion: arg.ModelVersion, EventTime: arg.EventTime}, nil
}

func TestEngineRunPersistsPredictionsAndBuildsArtifacts(t *testing.T) {
	rows := makeReplayRows(domain.SportNFL, 18)
	fake := &fakeReplayStore{rows: rows}
	engine, err := NewEngine(fake)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	sport := domain.SportNFL
	season := 2025
	artifact, err := engine.Run(context.Background(), RunConfig{
		Sport:                 &sport,
		Season:                &season,
		ModelVersion:          "test-v1",
		WalkForwardTrain:      8,
		WalkForwardValidation: 4,
		WalkForwardStep:       2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if fake.listCalls != 1 {
		t.Fatalf("list call count = %d, want 1", fake.listCalls)
	}
	if len(fake.upserts) != len(rows) {
		t.Fatalf("upsert count = %d, want %d", len(fake.upserts), len(rows))
	}
	if artifact.PersistedPredictionRows != len(rows) {
		t.Fatalf("persisted rows = %d, want %d", artifact.PersistedPredictionRows, len(rows))
	}
	if len(artifact.Outcomes) != len(rows) {
		t.Fatalf("outcome count = %d, want %d", len(artifact.Outcomes), len(rows))
	}
	if len(artifact.WalkForward) == 0 {
		t.Fatal("expected non-empty walk-forward output")
	}
	if artifact.CLV.Samples != len(rows) {
		t.Fatalf("clv samples = %d, want %d", artifact.CLV.Samples, len(rows))
	}
	if artifact.Calibration.Samples != len(rows) {
		t.Fatalf("calibration samples = %d, want %d", artifact.Calibration.Samples, len(rows))
	}
	if len(artifact.SportCalibrations) == 0 {
		t.Fatal("expected sport calibration artifacts")
	}
	for i, outcome := range artifact.Outcomes {
		if outcome.KellyFraction != 0.10 {
			t.Fatalf("outcome[%d] KellyFraction = %.3f, want 0.10", i, outcome.KellyFraction)
		}
		if outcome.MaxStakeFraction != 0.015 {
			t.Fatalf("outcome[%d] MaxStakeFraction = %.3f, want 0.015", i, outcome.MaxStakeFraction)
		}
	}
}

func TestEngineRunDeterministic(t *testing.T) {
	rows := makeReplayRows(domain.SportNHL, 16)
	fake := &fakeReplayStore{rows: rows}
	engine, err := NewEngine(fake)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	sport := domain.SportNHL
	cfg := RunConfig{Sport: &sport, ModelVersion: "det-v1", WalkForwardTrain: 8, WalkForwardValidation: 4, WalkForwardStep: 2}

	first, err := engine.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	fake.upserts = nil
	second, err := engine.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if !reflect.DeepEqual(first.Outcomes, second.Outcomes) {
		t.Fatal("outcomes are not deterministic")
	}
	if !reflect.DeepEqual(first.WalkForward, second.WalkForward) {
		t.Fatal("walk-forward results are not deterministic")
	}
	if !reflect.DeepEqual(first.Calibration, second.Calibration) {
		t.Fatal("calibration results are not deterministic")
	}
}

func TestEngineRunWithoutSufficientSamplesOmitsWalkForward(t *testing.T) {
	rows := makeReplayRows(domain.SportMLB, 6)
	fake := &fakeReplayStore{rows: rows}
	engine, err := NewEngine(fake)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	sport := domain.SportMLB
	artifact, err := engine.Run(context.Background(), RunConfig{Sport: &sport, WalkForwardTrain: 8, WalkForwardValidation: 4, WalkForwardStep: 2})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(artifact.WalkForward) != 0 {
		t.Fatalf("expected no walk-forward splits, got %d", len(artifact.WalkForward))
	}
}

func makeReplayRows(sport domain.Sport, n int) []store.ListBacktestReplayRowsRow {
	rows := make([]store.ListBacktestReplayRowsRow, 0, n)
	start := time.Date(2025, time.September, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		open := start.Add(time.Duration(i) * 24 * time.Hour)
		close := open.Add(2 * time.Hour)
		commence := open.Add(4 * time.Hour)
		openProb := 0.45 + float64(i%9)*0.03
		if openProb > 0.75 {
			openProb = 0.75
		}
		closeProb := openProb + (float64((i%5)-2) * 0.01)
		if closeProb < 0.2 {
			closeProb = 0.2
		}
		if closeProb > 0.8 {
			closeProb = 0.8
		}

		rows = append(rows, store.ListBacktestReplayRowsRow{
			GameID:                        int64(i + 1),
			Source:                        "the-odds-api",
			ExternalID:                    "game-" + string(sport) + "-" + strconv.Itoa(i),
			Sport:                         string(sport),
			HomeTeam:                      "Home " + strconv.Itoa(i),
			AwayTeam:                      "Away " + strconv.Itoa(i),
			CommenceTime:                  store.Timestamptz(commence),
			BookKey:                       "draftkings",
			MarketKey:                     "h2h",
			OpeningHomeImpliedProbability: openProb,
			ClosingHomeImpliedProbability: closeProb,
			OpeningCapturedAt:             store.Timestamptz(open),
			ClosingCapturedAt:             store.Timestamptz(close),
		})
	}
	return rows
}

func TestResolveRunConfigDefaultsBankrollPolicy(t *testing.T) {
	resolved, err := resolveRunConfig(RunConfig{})
	if err != nil {
		t.Fatalf("resolveRunConfig() error = %v", err)
	}
	if resolved.StartingBankroll != defaultStartingBankroll {
		t.Fatalf("StartingBankroll = %.2f, want %.2f", resolved.StartingBankroll, defaultStartingBankroll)
	}
	if resolved.MinimumStakeDollars != defaultMinimumStakeDollars {
		t.Fatalf("MinimumStakeDollars = %.2f, want %.2f", resolved.MinimumStakeDollars, defaultMinimumStakeDollars)
	}
}

func TestResolveRunConfigRejectsInvalidBankrollFractions(t *testing.T) {
	if _, err := resolveRunConfig(RunConfig{KellyFraction: 1.1}); err == nil {
		t.Fatal("expected invalid kelly fraction error")
	}
	if _, err := resolveRunConfig(RunConfig{MaxStakeFraction: -0.1}); err == nil {
		t.Fatal("expected invalid max stake fraction error")
	}
}
