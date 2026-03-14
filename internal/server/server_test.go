package server

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"betbot/internal/config"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	html "github.com/gofiber/template/html/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeReadQueries struct {
	listLatestOddsRows           []store.ListLatestOddsRow
	listLatestOddsErr            error
	listLatestOddsCalls          []store.ListLatestOddsParams
	listPerformanceRows          []store.ListRecommendationPerformanceSnapshotsRow
	listPerformanceErr           error
	listPerformanceCalls         []store.ListRecommendationPerformanceSnapshotsParams
	modelPredictionsBySport      map[string][]store.ModelPrediction
	modelPredictionsCalls        []store.ListModelPredictionsForSportSeasonParams
	modelPredictionsErr          error
	bankrollBalanceCents         int64
	bankrollBalanceErr           error
	insertOutcomeCalls           []store.InsertRecommendationOutcomeIfChangedParams
	insertOutcomeErr             error
	insertSnapshotCalls          []store.InsertRecommendationSnapshotParams
	insertSnapshotErr            error
	latestPollRun                store.PollRun
	latestPollRunErr             error
	oddsArchiveSummary           store.GetOddsArchiveSummaryRow
	oddsArchiveSummaryErr        error
	oddsArchiveSummarySportCalls []*string
}

func (f *fakeReadQueries) GetLatestPollRun(context.Context) (store.PollRun, error) {
	if f.latestPollRunErr != nil {
		return store.PollRun{}, f.latestPollRunErr
	}
	return f.latestPollRun, nil
}

func (f *fakeReadQueries) GetOddsArchiveSummary(_ context.Context, sport *string) (store.GetOddsArchiveSummaryRow, error) {
	f.oddsArchiveSummarySportCalls = append(f.oddsArchiveSummarySportCalls, cloneStringPtr(sport))
	if f.oddsArchiveSummaryErr != nil {
		return store.GetOddsArchiveSummaryRow{}, f.oddsArchiveSummaryErr
	}
	return f.oddsArchiveSummary, nil
}

func (f *fakeReadQueries) ListLatestOdds(_ context.Context, arg store.ListLatestOddsParams) ([]store.ListLatestOddsRow, error) {
	f.listLatestOddsCalls = append(f.listLatestOddsCalls, store.ListLatestOddsParams{
		Sport:    cloneStringPtr(arg.Sport),
		RowLimit: arg.RowLimit,
	})
	if f.listLatestOddsErr != nil {
		return nil, f.listLatestOddsErr
	}
	return f.listLatestOddsRows, nil
}

func (f *fakeReadQueries) ListModelPredictionsForSportSeason(_ context.Context, arg store.ListModelPredictionsForSportSeasonParams) ([]store.ModelPrediction, error) {
	f.modelPredictionsCalls = append(f.modelPredictionsCalls, arg)
	if f.modelPredictionsErr != nil {
		return nil, f.modelPredictionsErr
	}
	if f.modelPredictionsBySport == nil {
		return nil, nil
	}
	return f.modelPredictionsBySport[arg.Sport], nil
}

func (f *fakeReadQueries) ListRecommendationPerformanceSnapshots(_ context.Context, arg store.ListRecommendationPerformanceSnapshotsParams) ([]store.ListRecommendationPerformanceSnapshotsRow, error) {
	f.listPerformanceCalls = append(f.listPerformanceCalls, arg)
	if f.listPerformanceErr != nil {
		return nil, f.listPerformanceErr
	}
	return f.listPerformanceRows, nil
}

func (f *fakeReadQueries) GetBankrollBalanceCents(context.Context) (int64, error) {
	if f.bankrollBalanceErr != nil {
		return 0, f.bankrollBalanceErr
	}
	return f.bankrollBalanceCents, nil
}

func (f *fakeReadQueries) InsertRecommendationOutcomeIfChanged(_ context.Context, arg store.InsertRecommendationOutcomeIfChangedParams) (int64, error) {
	f.insertOutcomeCalls = append(f.insertOutcomeCalls, arg)
	if f.insertOutcomeErr != nil {
		return 0, f.insertOutcomeErr
	}
	return 1, nil
}

func (f *fakeReadQueries) InsertRecommendationSnapshot(_ context.Context, arg store.InsertRecommendationSnapshotParams) (store.RecommendationSnapshot, error) {
	f.insertSnapshotCalls = append(f.insertSnapshotCalls, arg)
	if f.insertSnapshotErr != nil {
		return store.RecommendationSnapshot{}, f.insertSnapshotErr
	}
	return store.RecommendationSnapshot{}, nil
}

func TestHandleOddsWithoutSportFilterUsesAllSports(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:      pgx.ErrNoRows,
		oddsArchiveSummary:    store.GetOddsArchiveSummaryRow{},
		listLatestOddsRows:    []store.ListLatestOddsRow{},
		oddsArchiveSummaryErr: nil,
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/odds")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /odds status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.listLatestOddsCalls) != 1 {
		t.Fatalf("ListLatestOdds call count = %d, want 1", len(queries.listLatestOddsCalls))
	}
	if queries.listLatestOddsCalls[0].Sport != nil {
		t.Fatalf("ListLatestOdds sport = %v, want nil", queries.listLatestOddsCalls[0].Sport)
	}
	if queries.listLatestOddsCalls[0].RowLimit != 200 {
		t.Fatalf("ListLatestOdds row_limit = %d, want 200", queries.listLatestOddsCalls[0].RowLimit)
	}
}

func TestHandleOddsWithValidSportFilter(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:   pgx.ErrNoRows,
		oddsArchiveSummary: store.GetOddsArchiveSummaryRow{},
		listLatestOddsRows: []store.ListLatestOddsRow{},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/odds?sport=baseball_mlb")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /odds?sport=baseball_mlb status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.listLatestOddsCalls) != 1 {
		t.Fatalf("ListLatestOdds call count = %d, want 1", len(queries.listLatestOddsCalls))
	}
	sport := queries.listLatestOddsCalls[0].Sport
	if sport == nil || *sport != "MLB" {
		t.Fatalf("ListLatestOdds sport = %v, want MLB", sport)
	}
}

func TestHandleOddsWithInvalidSportFilterReturnsBadRequest(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:   pgx.ErrNoRows,
		oddsArchiveSummary: store.GetOddsArchiveSummaryRow{},
		listLatestOddsRows: []store.ListLatestOddsRow{},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/odds?sport=soccer_epl")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /odds?sport=soccer_epl status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if len(queries.listLatestOddsCalls) != 0 {
		t.Fatalf("ListLatestOdds call count = %d, want 0", len(queries.listLatestOddsCalls))
	}
	body := readBody(t, resp)
	assertContains(t, body, "invalid sport filter")
}

func TestHandlePipelineHealthSportFilterScopesSummary(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:      pgx.ErrNoRows,
		oddsArchiveSummary:    store.GetOddsArchiveSummaryRow{},
		oddsArchiveSummaryErr: nil,
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/pipeline/health?sport=icehockey_nhl")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /pipeline/health?sport=icehockey_nhl status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.oddsArchiveSummarySportCalls) == 0 {
		t.Fatal("GetOddsArchiveSummary was not called")
	}
	got := queries.oddsArchiveSummarySportCalls[0]
	if got == nil || *got != "NHL" {
		t.Fatalf("GetOddsArchiveSummary sport = %v, want NHL", got)
	}
}

func TestHandleHomeSportFilterScopesSummary(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:      pgx.ErrNoRows,
		oddsArchiveSummary:    store.GetOddsArchiveSummaryRow{},
		oddsArchiveSummaryErr: nil,
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/?sport=americanfootball_nfl")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /?sport=americanfootball_nfl status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.oddsArchiveSummarySportCalls) == 0 {
		t.Fatal("GetOddsArchiveSummary was not called")
	}
	got := queries.oddsArchiveSummarySportCalls[0]
	if got == nil || *got != "NFL" {
		t.Fatalf("GetOddsArchiveSummary sport = %v, want NFL", got)
	}
}

func TestPartialPipelineStatusValidSportFilter(t *testing.T) {
	queries := &fakeReadQueries{
		latestPollRunErr:      pgx.ErrNoRows,
		oddsArchiveSummary:    store.GetOddsArchiveSummaryRow{},
		oddsArchiveSummaryErr: nil,
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/partials/pipeline-status?sport=icehockey_nhl")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /partials/pipeline-status?sport=icehockey_nhl status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.oddsArchiveSummarySportCalls) == 0 {
		t.Fatal("GetOddsArchiveSummary was not called")
	}
	got := queries.oddsArchiveSummarySportCalls[0]
	if got == nil || *got != "NHL" {
		t.Fatalf("GetOddsArchiveSummary sport = %v, want NHL", got)
	}
	body := readBody(t, resp)
	assertContains(t, body, "id=\"pipeline-status-block\"")
}

func TestPartialPipelineStatusInvalidSportFilterReturnsBadRequest(t *testing.T) {
	queries := &fakeReadQueries{latestPollRunErr: pgx.ErrNoRows}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/partials/pipeline-status?sport=soccer_epl")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /partials/pipeline-status?sport=soccer_epl status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body := readBody(t, resp)
	assertContains(t, body, "invalid sport filter")
}

func TestPartialOddsTableValidSportFilter(t *testing.T) {
	queries := &fakeReadQueries{listLatestOddsRows: []store.ListLatestOddsRow{}}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/partials/odds-table?sport=baseball_mlb")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /partials/odds-table?sport=baseball_mlb status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(queries.listLatestOddsCalls) != 1 {
		t.Fatalf("ListLatestOdds call count = %d, want 1", len(queries.listLatestOddsCalls))
	}
	sport := queries.listLatestOddsCalls[0].Sport
	if sport == nil || *sport != "MLB" {
		t.Fatalf("ListLatestOdds sport = %v, want MLB", sport)
	}
	body := readBody(t, resp)
	assertContains(t, body, "id=\"odds-table-block\"")
}

func TestPartialOddsTableInvalidSportFilterReturnsBadRequest(t *testing.T) {
	queries := &fakeReadQueries{listLatestOddsRows: []store.ListLatestOddsRow{}}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/partials/odds-table?sport=soccer_epl")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /partials/odds-table?sport=soccer_epl status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if len(queries.listLatestOddsCalls) != 0 {
		t.Fatalf("ListLatestOdds call count = %d, want 0", len(queries.listLatestOddsCalls))
	}
	body := readBody(t, resp)
	assertContains(t, body, "invalid sport filter")
}

func TestPartialTopbarStatusRendersLiveFragment(t *testing.T) {
	queries := &fakeReadQueries{latestPollRunErr: pgx.ErrNoRows}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/partials/topbar-status?sport=basketball_nba")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /partials/topbar-status?sport=basketball_nba status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body := readBody(t, resp)
	assertContains(t, body, "id=\"topbar-status-region\"")
	assertContains(t, body, "/partials/topbar-status?sport=basketball_nba")
}

func TestOverviewPageContainsHTMXRefreshTargets(t *testing.T) {
	queries := &fakeReadQueries{latestPollRunErr: pgx.ErrNoRows}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/?sport=basketball_nba")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /?sport=basketball_nba status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body := readBody(t, resp)
	assertContains(t, body, "/partials/topbar-status?sport=basketball_nba")
	assertContains(t, body, "/partials/pipeline-status?sport=basketball_nba")
}

func TestOddsPageContainsHTMXRefreshTargets(t *testing.T) {
	queries := &fakeReadQueries{latestPollRunErr: pgx.ErrNoRows}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/odds?sport=americanfootball_nfl")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /odds?sport=americanfootball_nfl status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body := readBody(t, resp)
	assertContains(t, body, "/partials/topbar-status?sport=americanfootball_nfl")
	assertContains(t, body, "/partials/odds-table?sport=americanfootball_nfl")
}

func TestHandleRecommendationsReturnsRankedJSONAndPersistsSnapshots(t *testing.T) {
	queries := &fakeReadQueries{
		bankrollBalanceCents: 100000,
		modelPredictionsBySport: map[string][]store.ModelPrediction{
			"MLB": {
				{
					ID:                   1,
					GameID:               42,
					Sport:                "MLB",
					MarketKey:            "h2h",
					PredictedProbability: 0.58,
					MarketProbability:    0.52,
					EventTime:            store.Timestamptz(time.Date(2026, time.March, 16, 18, 0, 0, 0, time.UTC)),
				},
				{
					ID:                   2,
					GameID:               43,
					Sport:                "MLB",
					MarketKey:            "h2h",
					PredictedProbability: 0.62,
					MarketProbability:    0.53,
					EventTime:            store.Timestamptz(time.Date(2026, time.March, 16, 20, 0, 0, 0, time.UTC)),
				},
			},
		},
		listLatestOddsRows: []store.ListLatestOddsRow{
			{GameID: 42, BookKey: "book-a", BookName: "book-a", MarketKey: "h2h", OutcomeSide: "home", PriceAmerican: 105},
			{GameID: 42, BookKey: "book-a", BookName: "book-a", MarketKey: "h2h", OutcomeSide: "away", PriceAmerican: -120},
			{GameID: 42, BookKey: "book-b", BookName: "book-b", MarketKey: "h2h", OutcomeSide: "home", PriceAmerican: 110},
			{GameID: 42, BookKey: "book-b", BookName: "book-b", MarketKey: "h2h", OutcomeSide: "away", PriceAmerican: -125},
			{GameID: 43, BookKey: "book-a", BookName: "book-a", MarketKey: "h2h", OutcomeSide: "home", PriceAmerican: 102},
			{GameID: 43, BookKey: "book-a", BookName: "book-a", MarketKey: "h2h", OutcomeSide: "away", PriceAmerican: -115},
			{GameID: 43, BookKey: "book-c", BookName: "book-c", MarketKey: "h2h", OutcomeSide: "home", PriceAmerican: 108},
			{GameID: 43, BookKey: "book-c", BookName: "book-c", MarketKey: "h2h", OutcomeSide: "away", PriceAmerican: -121},
		},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations?sport=baseball_mlb&date=2026-03-16&limit=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations status = %d, want 200", resp.StatusCode)
	}

	var body []map[string]any
	if err := json.Unmarshal([]byte(readBody(t, resp)), &body); err != nil {
		t.Fatalf("decode recommendations: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("len(body) = %d, want 2", len(body))
	}
	if got := int(body[0]["game_id"].(float64)); got != 43 {
		t.Fatalf("body[0].game_id = %d, want 43", got)
	}
	if got := body[0]["best_book"].(string); got != "book-c" {
		t.Fatalf("body[0].best_book = %q, want book-c", got)
	}
	if len(queries.insertSnapshotCalls) != 2 {
		t.Fatalf("snapshot inserts = %d, want 2", len(queries.insertSnapshotCalls))
	}
	if len(queries.modelPredictionsCalls) != 1 || queries.modelPredictionsCalls[0].Sport != "MLB" {
		t.Fatalf("model predictions calls = %+v, want one MLB call", queries.modelPredictionsCalls)
	}
}

func TestHandleRecommendationsRejectsInvalidDate(t *testing.T) {
	queries := &fakeReadQueries{bankrollBalanceCents: 100000}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations?date=03-16-2026")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations?date=03-16-2026 status = %d, want 400", resp.StatusCode)
	}
	body := readBody(t, resp)
	assertContains(t, body, "expected YYYY-MM-DD")
}

func TestHandleRecommendationsRejectsInvalidSport(t *testing.T) {
	queries := &fakeReadQueries{bankrollBalanceCents: 100000}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations?sport=soccer_epl")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations?sport=soccer_epl status = %d, want 400", resp.StatusCode)
	}
	body := readBody(t, resp)
	assertContains(t, body, "invalid sport filter")
}

func TestHandleRecommendationsRejectsInvalidLimit(t *testing.T) {
	queries := &fakeReadQueries{bankrollBalanceCents: 100000}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations?limit=0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations?limit=0 status = %d, want 400", resp.StatusCode)
	}
	body := readBody(t, resp)
	assertContains(t, body, "expected integer in [1,200]")
}

func TestHandleRecommendationsReturnsServiceUnavailableWhenBankrollUnavailable(t *testing.T) {
	queries := &fakeReadQueries{bankrollBalanceErr: pgx.ErrNoRows}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /recommendations status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	body := readBody(t, resp)
	assertContains(t, body, "bankroll balance unavailable")
}

func TestHandleRecommendationsPerformanceReturnsRowsAndSummary(t *testing.T) {
	queries := &fakeReadQueries{
		listPerformanceRows: []store.ListRecommendationPerformanceSnapshotsRow{
			{
				SnapshotID:             701,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 16, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 77,
				HomeTeam:               "Boston Red Sox",
				AwayTeam:               "New York Yankees",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 1, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "home",
				BestBook:               "book-a",
				BestAmericanOdds:       110,
				ModelProbability:       0.58,
				MarketProbability:      0.52,
				Edge:                   0.06,
				SuggestedStakeFraction: 0.02,
				SuggestedStakeCents:    2000,
				BankrollCheckPass:      true,
				BankrollCheckReason:    "ok",
				RankScore:              602.01,
				SnapshotMetadata:       json.RawMessage(`{"mode":"recommendation-only"}`),
				CloseLineID:            9001,
				CloseAmericanOdds:      -105,
				CloseProbability:       0.55,
				CloseCapturedAt:        store.Timestamptz(time.Date(2026, time.March, 15, 4, 0, 0, 0, time.UTC)),
				CloseRawJson:           json.RawMessage(`{"completed":true,"scores":[{"name":"Boston Red Sox","score":"5"},{"name":"New York Yankees","score":"2"}]}`),
				PersistedOutcomeID:     0,
				PersistedStatus:        "",
				PersistedNotes:         "",
				PersistedMetadata:      json.RawMessage(`{}`),
				PersistedCreatedAt:     store.Timestamptz(time.Date(2026, time.March, 14, 16, 0, 0, 0, time.UTC)),
			},
			{
				SnapshotID:             700,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 15, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 78,
				HomeTeam:               "Chicago Cubs",
				AwayTeam:               "St. Louis Cardinals",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 3, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "away",
				BestBook:               "book-b",
				BestAmericanOdds:       118,
				ModelProbability:       0.44,
				MarketProbability:      0.49,
				Edge:                   0.04,
				SuggestedStakeFraction: 0.01,
				SuggestedStakeCents:    1000,
				BankrollCheckPass:      false,
				BankrollCheckReason:    "insufficient_funds",
				RankScore:              401.01,
				SnapshotMetadata:       json.RawMessage(`{"mode":"recommendation-only"}`),
				CloseLineID:            0,
				CloseAmericanOdds:      0,
				CloseProbability:       0,
				CloseCapturedAt:        store.Timestamptz(time.Date(2026, time.March, 15, 3, 0, 0, 0, time.UTC)),
				CloseRawJson:           json.RawMessage(`{}`),
				PersistedOutcomeID:     0,
				PersistedStatus:        "",
				PersistedNotes:         "",
				PersistedMetadata:      json.RawMessage(`{}`),
				PersistedCreatedAt:     store.Timestamptz(time.Date(2026, time.March, 14, 15, 0, 0, 0, time.UTC)),
			},
		},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations/performance?sport=baseball_mlb&date_from=2026-03-10&date_to=2026-03-17&limit=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations/performance status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Rows    []map[string]any `json:"rows"`
		Summary map[string]any   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &payload); err != nil {
		t.Fatalf("decode recommendations/performance: %v", err)
	}
	if len(payload.Rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(payload.Rows))
	}

	if got := int(payload.Rows[0]["snapshot_id"].(float64)); got != 701 {
		t.Fatalf("rows[0].snapshot_id = %d, want 701", got)
	}
	if got := payload.Rows[0]["status"].(string); got != "settled" {
		t.Fatalf("rows[0].status = %q, want settled", got)
	}
	if got := payload.Rows[0]["realized_result"].(string); got != "win" {
		t.Fatalf("rows[0].realized_result = %q, want win", got)
	}
	if got := payload.Rows[1]["status"].(string); got != "close_unavailable" {
		t.Fatalf("rows[1].status = %q, want close_unavailable", got)
	}

	if got := int(payload.Summary["count"].(float64)); got != 2 {
		t.Fatalf("summary.count = %d, want 2", got)
	}
	if got := int(payload.Summary["settled_count"].(float64)); got != 1 {
		t.Fatalf("summary.settled_count = %d, want 1", got)
	}
	if got := payload.Summary["avg_clv"].(float64); math.Abs(got-0.03) > 1e-9 {
		t.Fatalf("summary.avg_clv = %.6f, want 0.03", got)
	}

	if len(queries.listPerformanceCalls) != 1 {
		t.Fatalf("ListRecommendationPerformanceSnapshots call count = %d, want 1", len(queries.listPerformanceCalls))
	}
	call := queries.listPerformanceCalls[0]
	if call.Sport == nil || *call.Sport != "MLB" {
		t.Fatalf("list performance sport = %v, want MLB", call.Sport)
	}
	if !call.DateFrom.Valid || call.DateFrom.Time.Format("2006-01-02") != "2026-03-10" {
		t.Fatalf("date_from = %+v, want 2026-03-10", call.DateFrom)
	}
	if !call.DateTo.Valid || call.DateTo.Time.Format("2006-01-02") != "2026-03-17" {
		t.Fatalf("date_to = %+v, want 2026-03-17", call.DateTo)
	}
	if call.RowLimit != 2 {
		t.Fatalf("row_limit = %d, want 2", call.RowLimit)
	}

	if len(queries.insertOutcomeCalls) != 2 {
		t.Fatalf("InsertRecommendationOutcomeIfChanged call count = %d, want 2", len(queries.insertOutcomeCalls))
	}
	firstResult := queries.insertOutcomeCalls[0].RealizedResult
	if firstResult == nil || *firstResult != "win" {
		t.Fatalf("first realized_result = %v, want win", firstResult)
	}
	if queries.insertOutcomeCalls[1].RealizedResult != nil {
		t.Fatalf("second realized_result = %v, want nil", queries.insertOutcomeCalls[1].RealizedResult)
	}
}

func TestHandleRecommendationsPerformanceRejectsInvalidDate(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/performance?date_from=03-10-2026")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/performance invalid date status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "expected YYYY-MM-DD")
}

func TestHandleRecommendationsPerformanceRejectsInvalidDateRange(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/performance?date_from=2026-03-20&date_to=2026-03-10")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/performance invalid date range status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "date_from")
}

func TestHandleRecommendationsPerformanceRejectsInvalidLimit(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/performance?limit=0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/performance invalid limit status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "expected integer in [1,500]")
}

func newTestServerApp(t *testing.T, queries readQueries) *App {
	t.Helper()
	engine := html.New("../../templates", ".html")
	fiberApp := fiber.New(fiber.Config{Views: engine})

	app := &App{
		app:                fiberApp,
		cfg:                config.Config{Env: "test", RecentPollWindow: 20 * time.Minute},
		queries:            queries,
		oddsPollingEnabled: true,
	}
	app.routes()
	return app
}

func doRequest(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("app.Test(%s): %v", path, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func assertContains(t *testing.T, body, substring string) {
	t.Helper()
	if !strings.Contains(body, substring) {
		t.Fatalf("response body missing %q: %q", substring, body)
	}
}
