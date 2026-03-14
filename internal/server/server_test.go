package server

import (
	"context"
	"io"
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
)

type fakeReadQueries struct {
	listLatestOddsRows           []store.ListLatestOddsRow
	listLatestOddsErr            error
	listLatestOddsCalls          []store.ListLatestOddsParams
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
