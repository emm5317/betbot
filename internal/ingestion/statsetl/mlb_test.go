package statsetl

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeMLBProvider struct {
	snapshot MLBSnapshot
	err      error
}

func (p fakeMLBProvider) Fetch(context.Context, MLBRequest) (MLBSnapshot, error) {
	return p.snapshot, p.err
}

type fakeMLBStore struct {
	teamUpserts    []store.UpsertMLBTeamStatsParams
	pitcherUpserts []store.UpsertMLBPitcherStatsParams
}

func (s *fakeMLBStore) UpsertMLBTeamStats(_ context.Context, arg store.UpsertMLBTeamStatsParams) error {
	s.teamUpserts = append(s.teamUpserts, arg)
	return nil
}

func (s *fakeMLBStore) UpsertMLBPitcherStats(_ context.Context, arg store.UpsertMLBPitcherStatsParams) error {
	s.pitcherUpserts = append(s.pitcherUpserts, arg)
	return nil
}

func TestMLBETLRunNormalizesAndMapsPayload(t *testing.T) {
	storeSink := &fakeMLBStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewMLBETL(fakeMLBProvider{snapshot: MLBSnapshot{
		Source:   " StatCast ",
		StatDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Teams: []MLBTeamStat{{
			ExternalID:  " BOS ",
			TeamName:    "  Boston   Red Sox  ",
			GamesPlayed: 12,
			Wins:        7,
			Losses:      5,
			RunsScored:  58,
			RunsAllowed: 49,
			BattingOPS:  float64Ptr(0.771),
			TeamERA:     float64Ptr(3.44),
		}},
		Pitchers: []MLBPitcherStat{{
			ExternalID:     " SALE ",
			PlayerName:     "  Chris   Sale  ",
			TeamExternalID: " ATL ",
			TeamName:       "  Atlanta   Braves ",
			GamesStarted:   3,
			InningsPitched: float64Ptr(18.2),
			Era:            float64Ptr(2.95),
			Fip:            float64Ptr(3.11),
			Whip:           float64Ptr(1.01),
			StrikeoutRate:  float64Ptr(0.317),
			WalkRate:       float64Ptr(0.068),
		}},
	}}, logger)

	metrics, err := etl.Run(context.Background(), storeSink, MLBRequest{
		RequestedAt: time.Date(2026, time.March, 12, 6, 0, 0, 0, time.UTC),
		Season:      2026,
	})
	if err != nil {
		t.Fatalf("run mlb etl: %v", err)
	}

	if metrics.TeamRows != 1 || metrics.PitcherRows != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if len(storeSink.teamUpserts) != 1 || len(storeSink.pitcherUpserts) != 1 {
		t.Fatalf("unexpected upsert counts: teams=%d pitchers=%d", len(storeSink.teamUpserts), len(storeSink.pitcherUpserts))
	}

	team := storeSink.teamUpserts[0]
	if team.Source != "statcast" {
		t.Fatalf("expected normalized source statcast, got %q", team.Source)
	}
	if team.ExternalID != "bos" {
		t.Fatalf("expected normalized team external id bos, got %q", team.ExternalID)
	}
	if team.TeamName != "Boston Red Sox" {
		t.Fatalf("expected normalized team name, got %q", team.TeamName)
	}
	if team.Season != 2026 {
		t.Fatalf("expected season 2026, got %d", team.Season)
	}
	if team.SeasonType != "regular" {
		t.Fatalf("expected default season type regular, got %q", team.SeasonType)
	}
	expectedDate := time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)
	if !team.StatDate.Valid || !team.StatDate.Time.Equal(expectedDate) {
		t.Fatalf("expected normalized stat date %s, got %+v", expectedDate.Format(time.RFC3339), team.StatDate)
	}

	pitcher := storeSink.pitcherUpserts[0]
	if pitcher.ExternalID != "sale" {
		t.Fatalf("expected normalized pitcher external id sale, got %q", pitcher.ExternalID)
	}
	if pitcher.PlayerName != "Chris Sale" {
		t.Fatalf("expected normalized pitcher name, got %q", pitcher.PlayerName)
	}
	if pitcher.TeamExternalID != "atl" {
		t.Fatalf("expected normalized team external id atl, got %q", pitcher.TeamExternalID)
	}
	if pitcher.TeamName != "Atlanta Braves" {
		t.Fatalf("expected normalized pitcher team name, got %q", pitcher.TeamName)
	}
}

func TestMLBETLRunRejectsMissingTeamIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewMLBETL(fakeMLBProvider{snapshot: MLBSnapshot{
		Source: "statcast",
		Teams: []MLBTeamStat{{
			TeamName: "Boston Red Sox",
		}},
	}}, logger)

	_, err := etl.Run(context.Background(), &fakeMLBStore{}, MLBRequest{
		RequestedAt: time.Date(2026, time.March, 11, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected validation error for missing team external id")
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}
