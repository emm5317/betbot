package statsetl

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeNHLProvider struct {
	snapshot NHLSnapshot
	err      error
}

func (p fakeNHLProvider) Fetch(context.Context, NHLRequest) (NHLSnapshot, error) {
	return p.snapshot, p.err
}

type fakeNHLStore struct {
	teamUpserts []store.UpsertNHLTeamStatsParams
}

func (s *fakeNHLStore) UpsertNHLTeamStats(_ context.Context, arg store.UpsertNHLTeamStatsParams) error {
	s.teamUpserts = append(s.teamUpserts, arg)
	return nil
}

func TestNHLETLRunNormalizesAndMapsPayload(t *testing.T) {
	storeSink := &fakeNHLStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNHLETL(fakeNHLProvider{snapshot: NHLSnapshot{
		Source:   " NHL-WEB-API ",
		StatDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Teams: []NHLTeamStat{{
			ExternalID:          " BOS ",
			TeamName:            "  Boston   Bruins ",
			GamesPlayed:         64,
			Wins:                39,
			Losses:              18,
			OTLosses:            7,
			GoalsForPerGame:     float64Ptr(3.41),
			GoalsAgainstPerGame: float64Ptr(2.67),
			ExpectedGoalsShare:  float64Ptr(0.534),
			SavePercentage:      float64Ptr(0.912),
		}},
	}}, logger)

	metrics, err := etl.Run(context.Background(), storeSink, NHLRequest{
		RequestedAt: time.Date(2026, time.March, 12, 6, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run nhl etl: %v", err)
	}

	if metrics.TeamRows != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if len(storeSink.teamUpserts) != 1 {
		t.Fatalf("unexpected team upserts: %d", len(storeSink.teamUpserts))
	}

	team := storeSink.teamUpserts[0]
	if team.Source != defaultNHLSource {
		t.Fatalf("expected normalized source %q, got %q", defaultNHLSource, team.Source)
	}
	if team.ExternalID != "bos" {
		t.Fatalf("expected normalized team external id bos, got %q", team.ExternalID)
	}
	if team.TeamName != "Boston Bruins" {
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
}

func TestNHLETLRunRejectsMissingTeamIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNHLETL(fakeNHLProvider{snapshot: NHLSnapshot{
		Source: "nhl-web-api",
		Teams: []NHLTeamStat{{
			TeamName: "Boston Bruins",
		}},
	}}, logger)

	_, err := etl.Run(context.Background(), &fakeNHLStore{}, NHLRequest{
		RequestedAt: time.Date(2026, time.March, 11, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected validation error for missing team external id")
	}
}
