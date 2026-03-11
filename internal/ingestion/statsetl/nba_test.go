package statsetl

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeNBAProvider struct {
	snapshot NBASnapshot
	err      error
}

func (p fakeNBAProvider) Fetch(context.Context, NBARequest) (NBASnapshot, error) {
	return p.snapshot, p.err
}

type fakeNBAStore struct {
	teamUpserts []store.UpsertNBATeamStatsParams
}

func (s *fakeNBAStore) UpsertNBATeamStats(_ context.Context, arg store.UpsertNBATeamStatsParams) error {
	s.teamUpserts = append(s.teamUpserts, arg)
	return nil
}

func TestNBAETLRunNormalizesAndMapsPayload(t *testing.T) {
	storeSink := &fakeNBAStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNBAETL(fakeNBAProvider{snapshot: NBASnapshot{
		Source:   " NBA-Stats-API ",
		StatDate: time.Date(2026, time.March, 11, 23, 45, 0, 0, time.FixedZone("CDT", -5*60*60)),
		Teams: []NBATeamStat{{
			ExternalID:      " BOS ",
			TeamName:        "  Boston   Celtics ",
			GamesPlayed:     65,
			Wins:            48,
			Losses:          17,
			OffensiveRating: float64Ptr(119.4),
			DefensiveRating: float64Ptr(110.2),
			NetRating:       float64Ptr(9.2),
			Pace:            float64Ptr(98.7),
		}},
	}}, logger)

	metrics, err := etl.Run(context.Background(), storeSink, NBARequest{
		RequestedAt: time.Date(2026, time.March, 12, 6, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run nba etl: %v", err)
	}

	if metrics.TeamRows != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if len(storeSink.teamUpserts) != 1 {
		t.Fatalf("unexpected team upserts: %d", len(storeSink.teamUpserts))
	}

	team := storeSink.teamUpserts[0]
	if team.Source != "nba-stats-api" {
		t.Fatalf("expected normalized source nba-stats-api, got %q", team.Source)
	}
	if team.ExternalID != "bos" {
		t.Fatalf("expected normalized team external id bos, got %q", team.ExternalID)
	}
	if team.TeamName != "Boston Celtics" {
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

func TestNBAETLRunRejectsMissingTeamIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNBAETL(fakeNBAProvider{snapshot: NBASnapshot{
		Source: "nba-stats-api",
		Teams: []NBATeamStat{{
			TeamName: "Boston Celtics",
		}},
	}}, logger)

	_, err := etl.Run(context.Background(), &fakeNBAStore{}, NBARequest{
		RequestedAt: time.Date(2026, time.March, 11, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected validation error for missing team external id")
	}
}
