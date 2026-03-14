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
	"betbot/internal/decision"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	html "github.com/gofiber/template/html/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeReadQueries struct {
	listLatestOddsRows             []store.ListLatestOddsRow
	listLatestOddsErr              error
	listLatestOddsCalls            []store.ListLatestOddsParams
	listPerformanceRows            []store.ListRecommendationPerformanceSnapshotsRow
	listPerformanceRowsByRange     map[string][]store.ListRecommendationPerformanceSnapshotsRow
	listPerformanceErr             error
	listPerformanceCalls           []store.ListRecommendationPerformanceSnapshotsParams
	listCalibrationAlertRunsRows   []store.RecommendationCalibrationAlertRun
	listCalibrationAlertRunsErr    error
	listCalibrationAlertRunsCall   []store.ListRecommendationCalibrationAlertRunsParams
	modelPredictionsBySport        map[string][]store.ModelPrediction
	modelPredictionsCalls          []store.ListModelPredictionsForSportSeasonParams
	modelPredictionsErr            error
	bankrollBalanceCents           int64
	bankrollBalanceErr             error
	insertCalibrationAlertRunCalls []store.InsertRecommendationCalibrationAlertRunParams
	insertCalibrationAlertRunErr   error
	insertOutcomeCalls             []store.InsertRecommendationOutcomeIfChangedParams
	insertOutcomeErr               error
	insertSnapshotCalls            []store.InsertRecommendationSnapshotParams
	insertSnapshotErr              error
	latestPollRun                  store.PollRun
	latestPollRunErr               error
	oddsArchiveSummary             store.GetOddsArchiveSummaryRow
	oddsArchiveSummaryErr          error
	oddsArchiveSummarySportCalls   []*string
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
	if f.listPerformanceRowsByRange != nil {
		key := recommendationPerformanceRangeKey(arg.DateFrom, arg.DateTo)
		if rows, ok := f.listPerformanceRowsByRange[key]; ok {
			return rows, nil
		}
		return []store.ListRecommendationPerformanceSnapshotsRow{}, nil
	}
	return f.listPerformanceRows, nil
}

func (f *fakeReadQueries) ListRecommendationCalibrationAlertRuns(_ context.Context, arg store.ListRecommendationCalibrationAlertRunsParams) ([]store.RecommendationCalibrationAlertRun, error) {
	f.listCalibrationAlertRunsCall = append(f.listCalibrationAlertRunsCall, arg)
	if f.listCalibrationAlertRunsErr != nil {
		return nil, f.listCalibrationAlertRunsErr
	}
	return f.listCalibrationAlertRunsRows, nil
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

func (f *fakeReadQueries) InsertRecommendationCalibrationAlertRun(_ context.Context, arg store.InsertRecommendationCalibrationAlertRunParams) (int64, error) {
	f.insertCalibrationAlertRunCalls = append(f.insertCalibrationAlertRunCalls, arg)
	if f.insertCalibrationAlertRunErr != nil {
		return 0, f.insertCalibrationAlertRunErr
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

func TestHandleRecommendationsCalibrationRejectsInvalidBucketCount(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration?bucket_count=0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration invalid bucket_count status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "expected integer in [1,20]")
}

func TestHandleRecommendationsCalibrationRejectsInvalidLimit(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration?limit=5001")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration invalid limit status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "expected integer in [1,5000]")
}

func TestHandleRecommendationsCalibrationReturnsSummaryAndBuckets(t *testing.T) {
	queries := &fakeReadQueries{
		listPerformanceRows: []store.ListRecommendationPerformanceSnapshotsRow{
			{
				SnapshotID:             1002,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 22,
				HomeTeam:               "Seattle Mariners",
				AwayTeam:               "Houston Astros",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "away",
				BestBook:               "book-b",
				BestAmericanOdds:       115,
				ModelProbability:       0.46,
				MarketProbability:      0.55,
				Edge:                   0.04,
				SuggestedStakeFraction: 0.01,
				SuggestedStakeCents:    1000,
				BankrollCheckPass:      true,
				BankrollCheckReason:    "ok",
				RankScore:              80,
				CloseLineID:            201,
				CloseAmericanOdds:      108,
				CloseProbability:       0.50,
				CloseCapturedAt:        store.Timestamptz(time.Date(2026, time.March, 15, 1, 0, 0, 0, time.UTC)),
				CloseRawJson:           json.RawMessage(`{"completed":true,"scores":[{"name":"Seattle Mariners","score":"2"},{"name":"Houston Astros","score":"4"}]}`),
			},
			{
				SnapshotID:             1005,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 25,
				HomeTeam:               "New York Mets",
				AwayTeam:               "Miami Marlins",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 5, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "away",
				BestBook:               "book-e",
				BestAmericanOdds:       120,
				ModelProbability:       0.45,
				MarketProbability:      0.40,
				Edge:                   0.05,
				SuggestedStakeFraction: 0.01,
				SuggestedStakeCents:    1000,
				BankrollCheckPass:      true,
				BankrollCheckReason:    "ok",
				RankScore:              50,
				CloseLineID:            0,
				CloseRawJson:           json.RawMessage(`{}`),
			},
			{
				SnapshotID:             1001,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 13, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 21,
				HomeTeam:               "Chicago Cubs",
				AwayTeam:               "St. Louis Cardinals",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "home",
				BestBook:               "book-a",
				BestAmericanOdds:       110,
				ModelProbability:       0.58,
				MarketProbability:      0.60,
				Edge:                   0.06,
				SuggestedStakeFraction: 0.02,
				SuggestedStakeCents:    2000,
				BankrollCheckPass:      true,
				BankrollCheckReason:    "ok",
				RankScore:              90,
				CloseLineID:            200,
				CloseAmericanOdds:      -105,
				CloseProbability:       0.65,
				CloseCapturedAt:        store.Timestamptz(time.Date(2026, time.March, 15, 1, 0, 0, 0, time.UTC)),
				CloseRawJson:           json.RawMessage(`{"completed":true,"scores":[{"name":"Chicago Cubs","score":"3"},{"name":"St. Louis Cardinals","score":"1"}]}`),
			},
			{
				SnapshotID:             1004,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 10, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 24,
				HomeTeam:               "San Diego Padres",
				AwayTeam:               "Los Angeles Dodgers",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 4, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "home",
				BestBook:               "book-d",
				BestAmericanOdds:       101,
				ModelProbability:       0.50,
				MarketProbability:      0.48,
				Edge:                   0.02,
				SuggestedStakeFraction: 0.01,
				SuggestedStakeCents:    800,
				BankrollCheckPass:      true,
				BankrollCheckReason:    "ok",
				RankScore:              60,
				CloseLineID:            202,
				CloseAmericanOdds:      -112,
				CloseProbability:       0.45,
				CloseCapturedAt:        store.Timestamptz(time.Date(2026, time.March, 15, 5, 0, 0, 0, time.UTC)),
				CloseRawJson:           json.RawMessage(`{"completed":true,"scores":[{"name":"San Diego Padres","score":"0"},{"name":"Los Angeles Dodgers","score":"1"}]}`),
			},
			{
				SnapshotID:             1003,
				GeneratedAt:            store.Timestamptz(time.Date(2026, time.March, 14, 11, 0, 0, 0, time.UTC)),
				Sport:                  "MLB",
				GameID:                 23,
				HomeTeam:               "Atlanta Braves",
				AwayTeam:               "Philadelphia Phillies",
				EventTime:              store.Timestamptz(time.Date(2026, time.March, 15, 2, 0, 0, 0, time.UTC)),
				EventDate:              pgtype.Date{Time: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), Valid: true},
				MarketKey:              "h2h",
				RecommendedSide:        "home",
				BestBook:               "book-c",
				BestAmericanOdds:       104,
				ModelProbability:       0.53,
				MarketProbability:      0.52,
				Edge:                   0.01,
				SuggestedStakeFraction: 0.005,
				SuggestedStakeCents:    500,
				BankrollCheckPass:      false,
				BankrollCheckReason:    "insufficient_funds",
				RankScore:              70,
				CloseLineID:            0,
				CloseRawJson:           json.RawMessage(`{}`),
			},
		},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations/calibration?sport=baseball_mlb&date_from=2026-03-10&date_to=2026-03-17&bucket_count=3&limit=5")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations/calibration status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Filters struct {
			Sport       string  `json:"sport"`
			DateFrom    *string `json:"date_from"`
			DateTo      *string `json:"date_to"`
			BucketCount int     `json:"bucket_count"`
			Limit       int     `json:"limit"`
		} `json:"filters"`
		Summary struct {
			TotalRows              int     `json:"total_rows"`
			SettledRows            int     `json:"settled_rows"`
			ExcludedRows           int     `json:"excluded_rows"`
			OverallObservedWinRate float64 `json:"overall_observed_win_rate"`
			OverallExpectedWinRate float64 `json:"overall_expected_win_rate"`
			OverallBrier           float64 `json:"overall_brier"`
			OverallECE             float64 `json:"overall_ece"`
			AvgCLV                 float64 `json:"avg_clv"`
		} `json:"summary"`
		Buckets []struct {
			BucketIndex  int      `json:"bucket_index"`
			RankMin      *float64 `json:"rank_min"`
			RankMax      *float64 `json:"rank_max"`
			Count        int      `json:"count"`
			SettledCount int      `json:"settled_count"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &payload); err != nil {
		t.Fatalf("decode /recommendations/calibration: %v", err)
	}

	if payload.Filters.Sport != "baseball_mlb" {
		t.Fatalf("filters.sport = %q, want baseball_mlb", payload.Filters.Sport)
	}
	if payload.Filters.DateFrom == nil || *payload.Filters.DateFrom != "2026-03-10" {
		t.Fatalf("filters.date_from = %v, want 2026-03-10", payload.Filters.DateFrom)
	}
	if payload.Filters.DateTo == nil || *payload.Filters.DateTo != "2026-03-17" {
		t.Fatalf("filters.date_to = %v, want 2026-03-17", payload.Filters.DateTo)
	}
	if payload.Filters.BucketCount != 3 || payload.Filters.Limit != 5 {
		t.Fatalf("filters = %+v, want bucket_count=3 limit=5", payload.Filters)
	}

	if payload.Summary.TotalRows != 5 || payload.Summary.SettledRows != 3 || payload.Summary.ExcludedRows != 2 {
		t.Fatalf("summary rows = %+v, want total=5 settled=3 excluded=2", payload.Summary)
	}
	if math.Abs(payload.Summary.OverallExpectedWinRate-0.51) > 1e-9 {
		t.Fatalf("summary.overall_expected_win_rate = %.6f, want 0.51", payload.Summary.OverallExpectedWinRate)
	}

	if len(payload.Buckets) != 3 {
		t.Fatalf("len(buckets) = %d, want 3", len(payload.Buckets))
	}
	for i, bucket := range payload.Buckets {
		if bucket.BucketIndex != i {
			t.Fatalf("bucket[%d].bucket_index = %d, want %d", i, bucket.BucketIndex, i)
		}
	}
	if payload.Buckets[2].Count != 1 || payload.Buckets[2].SettledCount != 0 {
		t.Fatalf("bucket[2] = %+v, want count=1 settled_count=0", payload.Buckets[2])
	}

	if len(queries.listPerformanceCalls) != 1 {
		t.Fatalf("ListRecommendationPerformanceSnapshots call count = %d, want 1", len(queries.listPerformanceCalls))
	}
	call := queries.listPerformanceCalls[0]
	if call.Sport == nil || *call.Sport != "MLB" {
		t.Fatalf("list performance sport = %v, want MLB", call.Sport)
	}
	if call.RowLimit != 5 {
		t.Fatalf("row_limit = %d, want 5", call.RowLimit)
	}
	if len(queries.insertOutcomeCalls) != 5 {
		t.Fatalf("InsertRecommendationOutcomeIfChanged call count = %d, want 5", len(queries.insertOutcomeCalls))
	}
}

func TestHandleRecommendationsCalibrationDeterministicBucketOrdering(t *testing.T) {
	rows := []store.ListRecommendationPerformanceSnapshotsRow{
		{
			SnapshotID:        2001,
			Sport:             "MLB",
			GameID:            31,
			HomeTeam:          "Team A",
			AwayTeam:          "Team B",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-a",
			BestAmericanOdds:  100,
			MarketProbability: 0.58,
			RankScore:         101,
			CloseLineID:       3001,
			CloseProbability:  0.60,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team A","score":"3"},{"name":"Team B","score":"1"}]}`),
		},
		{
			SnapshotID:        2002,
			Sport:             "MLB",
			GameID:            32,
			HomeTeam:          "Team C",
			AwayTeam:          "Team D",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 20, 1, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "away",
			BestBook:          "book-b",
			BestAmericanOdds:  105,
			MarketProbability: 0.53,
			RankScore:         101,
			CloseLineID:       3002,
			CloseProbability:  0.49,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team C","score":"2"},{"name":"Team D","score":"4"}]}`),
		},
		{
			SnapshotID:        2003,
			Sport:             "MLB",
			GameID:            33,
			HomeTeam:          "Team E",
			AwayTeam:          "Team F",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 20, 2, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-c",
			BestAmericanOdds:  111,
			MarketProbability: 0.52,
			RankScore:         95,
			CloseLineID:       0,
			CloseRawJson:      json.RawMessage(`{}`),
		},
	}
	reversed := []store.ListRecommendationPerformanceSnapshotsRow{rows[2], rows[1], rows[0]}

	appA := newTestServerApp(t, &fakeReadQueries{listPerformanceRows: rows})
	appB := newTestServerApp(t, &fakeReadQueries{listPerformanceRows: reversed})

	respA := doRequest(t, appA.app, "/recommendations/calibration?bucket_count=2&limit=3")
	respB := doRequest(t, appB.app, "/recommendations/calibration?bucket_count=2&limit=3")
	if respA.StatusCode != http.StatusOK || respB.StatusCode != http.StatusOK {
		t.Fatalf("status codes = (%d,%d), want (200,200)", respA.StatusCode, respB.StatusCode)
	}

	type bucket struct {
		BucketIndex  int      `json:"bucket_index"`
		RankMin      *float64 `json:"rank_min"`
		RankMax      *float64 `json:"rank_max"`
		Count        int      `json:"count"`
		SettledCount int      `json:"settled_count"`
	}
	var payloadA struct {
		Buckets []bucket `json:"buckets"`
	}
	var payloadB struct {
		Buckets []bucket `json:"buckets"`
	}
	if err := json.Unmarshal([]byte(readBody(t, respA)), &payloadA); err != nil {
		t.Fatalf("decode payloadA: %v", err)
	}
	if err := json.Unmarshal([]byte(readBody(t, respB)), &payloadB); err != nil {
		t.Fatalf("decode payloadB: %v", err)
	}

	if len(payloadA.Buckets) != len(payloadB.Buckets) {
		t.Fatalf("bucket lengths differ: %d vs %d", len(payloadA.Buckets), len(payloadB.Buckets))
	}
	for i := range payloadA.Buckets {
		left := payloadA.Buckets[i]
		right := payloadB.Buckets[i]
		if left.BucketIndex != right.BucketIndex || left.Count != right.Count || left.SettledCount != right.SettledCount {
			t.Fatalf("bucket[%d] mismatch: left=%+v right=%+v", i, left, right)
		}
		if !equalOptionalFloat(t, left.RankMin, right.RankMin) || !equalOptionalFloat(t, left.RankMax, right.RankMax) {
			t.Fatalf("bucket[%d] rank bounds mismatch: left=%+v right=%+v", i, left, right)
		}
	}
}

func TestHandleRecommendationsCalibrationAlertsRejectsInvalidThresholdOrdering(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts?warn_ece_delta=0.06&critical_ece_delta=0.05")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration/alerts invalid thresholds status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "warn_ece_delta")
}

func TestHandleRecommendationsCalibrationAlertsRejectsInvalidDateRange(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts?current_from=2026-03-20&current_to=2026-03-10")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration/alerts invalid date range status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "current_from")
}

func TestHandleRecommendationsCalibrationAlertsRejectsInvalidMinSettled(t *testing.T) {
	app := newTestServerApp(t, &fakeReadQueries{})

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts?min_settled_per_bucket=0")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /recommendations/calibration/alerts invalid min_settled_per_bucket status = %d, want 400", resp.StatusCode)
	}
	assertContains(t, readBody(t, resp), "min_settled_per_bucket")
}

func TestHandleRecommendationsCalibrationAlertsReturnsSummaryAndGuardrails(t *testing.T) {
	currentFrom := pgtype.Date{Time: time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC), Valid: true}
	currentTo := pgtype.Date{Time: time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC), Valid: true}
	baselineFrom := pgtype.Date{Time: time.Date(2026, time.February, 10, 0, 0, 0, 0, time.UTC), Valid: true}
	baselineTo := pgtype.Date{Time: time.Date(2026, time.February, 14, 0, 0, 0, 0, time.UTC), Valid: true}

	queries := &fakeReadQueries{
		listPerformanceRowsByRange: map[string][]store.ListRecommendationPerformanceSnapshotsRow{
			recommendationPerformanceRangeKey(currentFrom, currentTo): {
				{
					SnapshotID:        3001,
					Sport:             "MLB",
					GameID:            61,
					HomeTeam:          "Team A",
					AwayTeam:          "Team B",
					EventTime:         store.Timestamptz(time.Date(2026, time.March, 12, 1, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "home",
					BestBook:          "book-a",
					BestAmericanOdds:  101,
					MarketProbability: 0.58,
					RankScore:         120,
					CloseLineID:       9001,
					CloseProbability:  0.60,
					CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team A","score":"3"},{"name":"Team B","score":"1"}]}`),
				},
				{
					SnapshotID:        3002,
					Sport:             "MLB",
					GameID:            62,
					HomeTeam:          "Team C",
					AwayTeam:          "Team D",
					EventTime:         store.Timestamptz(time.Date(2026, time.March, 12, 2, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "away",
					BestBook:          "book-b",
					BestAmericanOdds:  102,
					MarketProbability: 0.57,
					RankScore:         110,
					CloseLineID:       9002,
					CloseProbability:  0.50,
					CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team C","score":"4"},{"name":"Team D","score":"2"}]}`),
				},
				{
					SnapshotID:        3003,
					Sport:             "MLB",
					GameID:            63,
					HomeTeam:          "Team E",
					AwayTeam:          "Team F",
					EventTime:         store.Timestamptz(time.Date(2026, time.March, 12, 3, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "home",
					BestBook:          "book-c",
					BestAmericanOdds:  103,
					MarketProbability: 0.55,
					RankScore:         100,
					CloseLineID:       0,
					CloseRawJson:      json.RawMessage(`{}`),
				},
			},
			recommendationPerformanceRangeKey(baselineFrom, baselineTo): {
				{
					SnapshotID:        3101,
					Sport:             "MLB",
					GameID:            71,
					HomeTeam:          "Team G",
					AwayTeam:          "Team H",
					EventTime:         store.Timestamptz(time.Date(2026, time.February, 12, 1, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "home",
					BestBook:          "book-a",
					BestAmericanOdds:  101,
					MarketProbability: 0.56,
					RankScore:         119,
					CloseLineID:       9101,
					CloseProbability:  0.57,
					CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team G","score":"2"},{"name":"Team H","score":"1"}]}`),
				},
				{
					SnapshotID:        3102,
					Sport:             "MLB",
					GameID:            72,
					HomeTeam:          "Team I",
					AwayTeam:          "Team J",
					EventTime:         store.Timestamptz(time.Date(2026, time.February, 12, 2, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "away",
					BestBook:          "book-b",
					BestAmericanOdds:  102,
					MarketProbability: 0.51,
					RankScore:         109,
					CloseLineID:       9102,
					CloseProbability:  0.52,
					CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team I","score":"0"},{"name":"Team J","score":"1"}]}`),
				},
				{
					SnapshotID:        3103,
					Sport:             "MLB",
					GameID:            73,
					HomeTeam:          "Team K",
					AwayTeam:          "Team L",
					EventTime:         store.Timestamptz(time.Date(2026, time.February, 12, 3, 0, 0, 0, time.UTC)),
					EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 12, 0, 0, 0, 0, time.UTC), Valid: true},
					MarketKey:         "h2h",
					RecommendedSide:   "home",
					BestBook:          "book-c",
					BestAmericanOdds:  103,
					MarketProbability: 0.54,
					RankScore:         99,
					CloseLineID:       9103,
					CloseProbability:  0.56,
					CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team K","score":"4"},{"name":"Team L","score":"3"}]}`),
				},
			},
		},
	}
	app := newTestServerApp(t, queries)

	resp := doRequest(t, app.app, "/recommendations/calibration/alerts?sport=baseball_mlb&current_from=2026-03-10&current_to=2026-03-14&baseline_from=2026-02-10&baseline_to=2026-02-14&bucket_count=2&limit=3&min_settled_overall=3&min_settled_per_bucket=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /recommendations/calibration/alerts status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Filters struct {
			Sport               string `json:"sport"`
			BucketCount         int    `json:"bucket_count"`
			Limit               int    `json:"limit"`
			MinSettledOverall   int    `json:"min_settled_overall"`
			MinSettledPerBucket int    `json:"min_settled_per_bucket"`
		} `json:"filters"`
		Alert struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		} `json:"alert"`
		Samples struct {
			CurrentSettledRows          int `json:"current_settled_rows"`
			BaselineSettledRows         int `json:"baseline_settled_rows"`
			InsufficientOverallWindows  int `json:"insufficient_overall_windows"`
			CurrentInsufficientBuckets  int `json:"current_insufficient_buckets"`
			BaselineInsufficientBuckets int `json:"baseline_insufficient_buckets"`
		} `json:"samples"`
		Buckets []struct {
			BucketIndex int `json:"bucket_index"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &payload); err != nil {
		t.Fatalf("decode /recommendations/calibration/alerts: %v", err)
	}

	if payload.Filters.Sport != "baseball_mlb" {
		t.Fatalf("filters.sport = %q, want baseball_mlb", payload.Filters.Sport)
	}
	if payload.Filters.BucketCount != 2 || payload.Filters.Limit != 3 {
		t.Fatalf("filters = %+v, want bucket_count=2 limit=3", payload.Filters)
	}
	if payload.Alert.Level != "insufficient_sample" {
		t.Fatalf("alert.level = %q, want insufficient_sample", payload.Alert.Level)
	}
	if len(payload.Alert.Reasons) == 0 {
		t.Fatal("alert.reasons should not be empty")
	}
	if payload.Samples.CurrentSettledRows != 2 || payload.Samples.BaselineSettledRows != 3 {
		t.Fatalf("samples settled rows = %+v, want current=2 baseline=3", payload.Samples)
	}
	if payload.Samples.InsufficientOverallWindows != 1 {
		t.Fatalf("insufficient_overall_windows = %d, want 1", payload.Samples.InsufficientOverallWindows)
	}
	if len(payload.Buckets) != 2 {
		t.Fatalf("len(buckets) = %d, want 2", len(payload.Buckets))
	}
	for i := range payload.Buckets {
		if payload.Buckets[i].BucketIndex != i {
			t.Fatalf("bucket[%d].bucket_index = %d, want %d", i, payload.Buckets[i].BucketIndex, i)
		}
	}

	if len(queries.listPerformanceCalls) != 2 {
		t.Fatalf("ListRecommendationPerformanceSnapshots call count = %d, want 2", len(queries.listPerformanceCalls))
	}
	if len(queries.insertOutcomeCalls) != 6 {
		t.Fatalf("InsertRecommendationOutcomeIfChanged call count = %d, want 6", len(queries.insertOutcomeCalls))
	}
	if len(queries.insertCalibrationAlertRunCalls) != 1 {
		t.Fatalf("InsertRecommendationCalibrationAlertRun call count = %d, want 1", len(queries.insertCalibrationAlertRunCalls))
	}
	if queries.insertCalibrationAlertRunCalls[0].Mode != decision.CalibrationAlertModePointInTime {
		t.Fatalf("InsertRecommendationCalibrationAlertRun mode = %q, want %q", queries.insertCalibrationAlertRunCalls[0].Mode, decision.CalibrationAlertModePointInTime)
	}
}

func TestHandleRecommendationsCalibrationAlertsDeterministicBucketOrdering(t *testing.T) {
	currentFrom := pgtype.Date{Time: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	currentTo := pgtype.Date{Time: time.Date(2026, time.March, 7, 0, 0, 0, 0, time.UTC), Valid: true}
	baselineFrom := pgtype.Date{Time: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	baselineTo := pgtype.Date{Time: time.Date(2026, time.February, 7, 0, 0, 0, 0, time.UTC), Valid: true}

	currentRows := []store.ListRecommendationPerformanceSnapshotsRow{
		{
			SnapshotID:        4001,
			Sport:             "MLB",
			GameID:            81,
			HomeTeam:          "Team A",
			AwayTeam:          "Team B",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 3, 1, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-a",
			BestAmericanOdds:  101,
			MarketProbability: 0.58,
			RankScore:         130,
			CloseLineID:       9201,
			CloseProbability:  0.60,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team A","score":"3"},{"name":"Team B","score":"1"}]}`),
		},
		{
			SnapshotID:        4002,
			Sport:             "MLB",
			GameID:            82,
			HomeTeam:          "Team C",
			AwayTeam:          "Team D",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 3, 2, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "away",
			BestBook:          "book-b",
			BestAmericanOdds:  102,
			MarketProbability: 0.57,
			RankScore:         130,
			CloseLineID:       9202,
			CloseProbability:  0.58,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team C","score":"2"},{"name":"Team D","score":"4"}]}`),
		},
		{
			SnapshotID:        4003,
			Sport:             "MLB",
			GameID:            83,
			HomeTeam:          "Team E",
			AwayTeam:          "Team F",
			EventTime:         store.Timestamptz(time.Date(2026, time.March, 3, 3, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-c",
			BestAmericanOdds:  103,
			MarketProbability: 0.56,
			RankScore:         110,
			CloseLineID:       0,
			CloseRawJson:      json.RawMessage(`{}`),
		},
	}
	baselineRows := []store.ListRecommendationPerformanceSnapshotsRow{
		{
			SnapshotID:        4101,
			Sport:             "MLB",
			GameID:            91,
			HomeTeam:          "Team G",
			AwayTeam:          "Team H",
			EventTime:         store.Timestamptz(time.Date(2026, time.February, 3, 1, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-a",
			BestAmericanOdds:  101,
			MarketProbability: 0.55,
			RankScore:         129,
			CloseLineID:       9301,
			CloseProbability:  0.56,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team G","score":"2"},{"name":"Team H","score":"1"}]}`),
		},
		{
			SnapshotID:        4102,
			Sport:             "MLB",
			GameID:            92,
			HomeTeam:          "Team I",
			AwayTeam:          "Team J",
			EventTime:         store.Timestamptz(time.Date(2026, time.February, 3, 2, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "away",
			BestBook:          "book-b",
			BestAmericanOdds:  102,
			MarketProbability: 0.53,
			RankScore:         129,
			CloseLineID:       9302,
			CloseProbability:  0.54,
			CloseRawJson:      json.RawMessage(`{"completed":true,"scores":[{"name":"Team I","score":"0"},{"name":"Team J","score":"3"}]}`),
		},
		{
			SnapshotID:        4103,
			Sport:             "MLB",
			GameID:            93,
			HomeTeam:          "Team K",
			AwayTeam:          "Team L",
			EventTime:         store.Timestamptz(time.Date(2026, time.February, 3, 3, 0, 0, 0, time.UTC)),
			EventDate:         pgtype.Date{Time: time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC), Valid: true},
			MarketKey:         "h2h",
			RecommendedSide:   "home",
			BestBook:          "book-c",
			BestAmericanOdds:  103,
			MarketProbability: 0.52,
			RankScore:         108,
			CloseLineID:       0,
			CloseRawJson:      json.RawMessage(`{}`),
		},
	}

	appA := newTestServerApp(t, &fakeReadQueries{
		listPerformanceRowsByRange: map[string][]store.ListRecommendationPerformanceSnapshotsRow{
			recommendationPerformanceRangeKey(currentFrom, currentTo):   currentRows,
			recommendationPerformanceRangeKey(baselineFrom, baselineTo): baselineRows,
		},
	})
	appB := newTestServerApp(t, &fakeReadQueries{
		listPerformanceRowsByRange: map[string][]store.ListRecommendationPerformanceSnapshotsRow{
			recommendationPerformanceRangeKey(currentFrom, currentTo):   {currentRows[2], currentRows[1], currentRows[0]},
			recommendationPerformanceRangeKey(baselineFrom, baselineTo): {baselineRows[2], baselineRows[1], baselineRows[0]},
		},
	})

	path := "/recommendations/calibration/alerts?current_from=2026-03-01&current_to=2026-03-07&baseline_from=2026-02-01&baseline_to=2026-02-07&bucket_count=2&limit=3&min_settled_overall=1&min_settled_per_bucket=1"
	respA := doRequest(t, appA.app, path)
	respB := doRequest(t, appB.app, path)
	if respA.StatusCode != http.StatusOK || respB.StatusCode != http.StatusOK {
		t.Fatalf("status codes = (%d,%d), want (200,200)", respA.StatusCode, respB.StatusCode)
	}

	type bucket struct {
		BucketIndex          int     `json:"bucket_index"`
		SettledCountCurrent  int     `json:"settled_count_current"`
		SettledCountBaseline int     `json:"settled_count_baseline"`
		CalibrationGapDelta  float64 `json:"calibration_gap_delta"`
		BrierDelta           float64 `json:"brier_delta"`
	}
	var payloadA struct {
		Buckets []bucket `json:"buckets"`
	}
	var payloadB struct {
		Buckets []bucket `json:"buckets"`
	}
	if err := json.Unmarshal([]byte(readBody(t, respA)), &payloadA); err != nil {
		t.Fatalf("decode payloadA: %v", err)
	}
	if err := json.Unmarshal([]byte(readBody(t, respB)), &payloadB); err != nil {
		t.Fatalf("decode payloadB: %v", err)
	}
	if len(payloadA.Buckets) != len(payloadB.Buckets) {
		t.Fatalf("bucket lengths differ: %d vs %d", len(payloadA.Buckets), len(payloadB.Buckets))
	}
	for i := range payloadA.Buckets {
		left := payloadA.Buckets[i]
		right := payloadB.Buckets[i]
		if left.BucketIndex != i || right.BucketIndex != i {
			t.Fatalf("bucket index mismatch at %d: left=%d right=%d", i, left.BucketIndex, right.BucketIndex)
		}
		if left.SettledCountCurrent != right.SettledCountCurrent || left.SettledCountBaseline != right.SettledCountBaseline {
			t.Fatalf("settled count mismatch at %d: left=%+v right=%+v", i, left, right)
		}
		if math.Abs(left.CalibrationGapDelta-right.CalibrationGapDelta) > 1e-9 {
			t.Fatalf("calibration gap delta mismatch at %d: left=%.6f right=%.6f", i, left.CalibrationGapDelta, right.CalibrationGapDelta)
		}
		if math.Abs(left.BrierDelta-right.BrierDelta) > 1e-9 {
			t.Fatalf("brier delta mismatch at %d: left=%.6f right=%.6f", i, left.BrierDelta, right.BrierDelta)
		}
	}
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

func recommendationPerformanceRangeKey(from pgtype.Date, to pgtype.Date) string {
	fromPart := ""
	toPart := ""
	if from.Valid {
		fromPart = from.Time.UTC().Format("2006-01-02")
	}
	if to.Valid {
		toPart = to.Time.UTC().Format("2006-01-02")
	}
	return fromPart + "|" + toPart
}

func assertContains(t *testing.T, body, substring string) {
	t.Helper()
	if !strings.Contains(body, substring) {
		t.Fatalf("response body missing %q: %q", substring, body)
	}
}

func equalOptionalFloat(t *testing.T, left, right *float64) bool {
	t.Helper()
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return math.Abs(*left-*right) < 1e-9
}
