package statsetl

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"betbot/internal/store"
)

type fakeNFLProvider struct {
	snapshot NFLSnapshot
	err      error
}

func (p fakeNFLProvider) Fetch(context.Context, NFLRequest) (NFLSnapshot, error) {
	return p.snapshot, p.err
}

type fakeNFLStore struct {
	teamUpserts []store.UpsertNFLTeamStatsParams
}

func (s *fakeNFLStore) UpsertNFLTeamStats(_ context.Context, arg store.UpsertNFLTeamStatsParams) error {
	s.teamUpserts = append(s.teamUpserts, arg)
	return nil
}

func TestNFLETLRunNormalizesAndMapsPayload(t *testing.T) {
	storeSink := &fakeNFLStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNFLETL(fakeNFLProvider{snapshot: NFLSnapshot{
		Source:   " NFLVERSE-NFL-COM ",
		StatDate: time.Date(2026, time.January, 18, 23, 45, 0, 0, time.FixedZone("CST", -6*60*60)),
		Teams: []NFLTeamStat{{
			ExternalID:           " BUF ",
			TeamName:             "  Buffalo   Bills ",
			GamesPlayed:          17,
			Wins:                 13,
			Losses:               4,
			Ties:                 0,
			PointsFor:            525,
			PointsAgainst:        368,
			OffensiveEPAPerPlay:  float64Ptr(0.099),
			DefensiveEPAPerPlay:  nil,
			OffensiveSuccessRate: nil,
			DefensiveSuccessRate: nil,
		}},
	}}, logger)

	metrics, err := etl.Run(context.Background(), storeSink, NFLRequest{
		RequestedAt: time.Date(2026, time.January, 19, 6, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run nfl etl: %v", err)
	}

	if metrics.TeamRows != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if len(storeSink.teamUpserts) != 1 {
		t.Fatalf("unexpected team upserts: %d", len(storeSink.teamUpserts))
	}

	team := storeSink.teamUpserts[0]
	if team.Source != defaultNFLSource {
		t.Fatalf("expected normalized source %q, got %q", defaultNFLSource, team.Source)
	}
	if team.ExternalID != "buf" {
		t.Fatalf("expected normalized team external id buf, got %q", team.ExternalID)
	}
	if team.TeamName != "Buffalo Bills" {
		t.Fatalf("expected normalized team name, got %q", team.TeamName)
	}
	if team.Season != 2025 {
		t.Fatalf("expected season 2025, got %d", team.Season)
	}
	if team.SeasonType != "regular" {
		t.Fatalf("expected default season type regular, got %q", team.SeasonType)
	}
	expectedDate := time.Date(2026, time.January, 19, 0, 0, 0, 0, time.UTC)
	if !team.StatDate.Valid || !team.StatDate.Time.Equal(expectedDate) {
		t.Fatalf("expected normalized stat date %s, got %+v", expectedDate.Format(time.RFC3339), team.StatDate)
	}
}

func TestNFLETLRunRejectsMissingTeamIdentifier(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	etl := NewNFLETL(fakeNFLProvider{snapshot: NFLSnapshot{
		Source: "nflverse-nfl-com",
		Teams: []NFLTeamStat{{
			TeamName: "Buffalo Bills",
		}},
	}}, logger)

	_, err := etl.Run(context.Background(), &fakeNFLStore{}, NFLRequest{
		RequestedAt: time.Date(2026, time.January, 19, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected validation error for missing team external id")
	}
}
