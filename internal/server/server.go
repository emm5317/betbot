package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"betbot/internal/config"
	"betbot/internal/execution"
	executionadapters "betbot/internal/execution/adapters"
	"betbot/internal/ingestion/oddspoller"
	"betbot/internal/prediction"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	fiberlogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	html "github.com/gofiber/template/html/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type readQueries interface {
	GetLatestPollRun(ctx context.Context) (store.PollRun, error)
	GetOddsArchiveSummary(ctx context.Context, sport *string) (store.GetOddsArchiveSummaryRow, error)
	ListLatestOdds(ctx context.Context, arg store.ListLatestOddsParams) ([]store.ListLatestOddsRow, error)
	ListLatestOddsForUpcoming(ctx context.Context, arg store.ListLatestOddsForUpcomingParams) ([]store.ListLatestOddsForUpcomingRow, error)
	ListModelPredictionsForSportSeason(ctx context.Context, arg store.ListModelPredictionsForSportSeasonParams) ([]store.ModelPrediction, error)
	ListRecommendationCalibrationAlertRuns(ctx context.Context, arg store.ListRecommendationCalibrationAlertRunsParams) ([]store.RecommendationCalibrationAlertRun, error)
	ListRecommendationPerformanceSnapshots(ctx context.Context, arg store.ListRecommendationPerformanceSnapshotsParams) ([]store.ListRecommendationPerformanceSnapshotsRow, error)
	GetBankrollBalanceCents(ctx context.Context) (int64, error)
	GetBankrollCircuitMetrics(ctx context.Context) (store.GetBankrollCircuitMetricsRow, error)
	InsertRecommendationCalibrationAlertRun(ctx context.Context, arg store.InsertRecommendationCalibrationAlertRunParams) (int64, error)
	InsertRecommendationOutcomeIfChanged(ctx context.Context, arg store.InsertRecommendationOutcomeIfChangedParams) (int64, error)
	InsertRecommendationSnapshot(ctx context.Context, arg store.InsertRecommendationSnapshotParams) (store.RecommendationSnapshot, error)
	GetRecommendationSnapshotByID(ctx context.Context, id int64) (store.RecommendationSnapshot, error)
	ListBetsByStatus(ctx context.Context, arg store.ListBetsByStatusParams) ([]store.ListBetsByStatusRow, error)
	ListOpenBets(ctx context.Context) ([]store.ListOpenBetsRow, error)
	ListOpenBetsWithGame(ctx context.Context) ([]store.ListOpenBetsWithGameRow, error)
	GetBetByIdempotencyKey(ctx context.Context, idempotencyKey string) (store.GetBetByIdempotencyKeyRow, error)
	GetBetByID(ctx context.Context, id int64) (store.GetBetByIDRow, error)
	InsertManualBet(ctx context.Context, arg store.InsertManualBetParams) (store.InsertManualBetRow, error)
	ListBetsWithFilters(ctx context.Context, arg store.ListBetsWithFiltersParams) ([]store.ListBetsWithFiltersRow, error)
	GetBetPnLSummary(ctx context.Context, sport string) (store.GetBetPnLSummaryRow, error)
	VoidBet(ctx context.Context, id int64) error
	ListBankrollEntries(ctx context.Context, rowLimit int32) ([]store.BankrollLedger, error)
	InsertBankrollEntry(ctx context.Context, arg store.InsertBankrollEntryParams) (store.BankrollLedger, error)
	UpdateBetSettled(ctx context.Context, arg store.UpdateBetSettledParams) error
	ListUpcomingGames(ctx context.Context, limit int32) ([]store.Game, error)
	GetGameByID(ctx context.Context, id int64) (store.Game, error)
}

type App struct {
	app    *fiber.App
	cfg    config.Config
	logger *slog.Logger
	pool   interface {
		Close()
		Ping(context.Context) error
	}
	queries                   readQueries
	pgxPool                   *pgxpool.Pool
	placementOrchestrator     *execution.PlacementOrchestrator
	nhlPredictionService      *prediction.NHLPredictionService
	oddsPoller                *oddspoller.Poller
	oddsPollingEnabled        bool
	oddsPollingDisabledReason string
}

func New(ctx context.Context, cfg config.Config, appLogger *slog.Logger) (*App, error) {
	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	engine := html.New("./templates", ".html")
	app := fiber.New(fiber.Config{
		AppName: "betbot",
		Views:   engine,
	})

	oddsPollingEnabled, oddsPollingDisabledReason := cfg.OddsPollingRuntime()
	// Always create the poller so on-demand refresh works regardless of
	// whether automatic periodic polling is enabled.
	poller := oddspoller.NewPoller(cfg, appLogger, pool)
	adapter, err := executionadapters.NewBookAdapter(cfg.ExecutionAdapter)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("configure execution adapter: %w", err)
	}

	instance := &App{
		app:                       app,
		cfg:                       cfg,
		logger:                    appLogger,
		pool:                      pool,
		queries:                   store.New(pool),
		pgxPool:                   pool,
		placementOrchestrator:     execution.NewPlacementOrchestrator(pool, adapter),
		nhlPredictionService:      prediction.NewNHLPredictionService(pool, appLogger),
		oddsPoller:                poller,
		oddsPollingEnabled:        oddsPollingEnabled,
		oddsPollingDisabledReason: oddsPollingDisabledReason,
	}
	instance.routes()
	return instance, nil
}

func (a *App) Close() {
	a.pool.Close()
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("betbot server listening", slog.String("addr", a.cfg.HTTPAddr))
		if err := a.app.Listen(a.cfg.HTTPAddr); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer a.Close()
		return a.app.ShutdownWithContext(shutdownCtx)
	case err := <-errCh:
		a.Close()
		return err
	}
}

func (a *App) routes() {
	a.app.Use(requestid.New())
	a.app.Use(recover.New())
	a.app.Use(fiberlogger.New(fiberlogger.Config{
		Format:     fiberlogger.JSONFormat,
		TimeFormat: time.RFC3339,
		TimeZone:   "UTC",
	}))
	a.app.Use("/static", static.New("./static"))

	a.app.Get("/", a.handleHome)
	a.app.Get("/health", a.handleHealth)
	a.app.Get("/odds", a.handleOdds)
	a.app.Get("/recommendations", a.handleRecommendations)
	a.app.Get("/recommendations/performance", a.handleRecommendationsPerformance)
	a.app.Get("/recommendations/performance/models", a.handleRecommendationsPerformanceModels)
	a.app.Get("/recommendations/calibration", a.handleRecommendationsCalibration)
	a.app.Get("/recommendations/calibration/alerts", a.handleRecommendationsCalibrationAlerts)
	a.app.Get("/recommendations/calibration/alerts/history", a.handleRecommendationsCalibrationAlertsHistory)
	a.app.Get("/pipeline/health", a.handlePipelineHealth)
	a.app.Get("/partials/topbar-status", a.handlePartialTopbarStatus)
	a.app.Get("/partials/pipeline-status", a.handlePartialPipelineStatus)
	a.app.Get("/partials/odds-table", a.handlePartialOddsTable)
	a.app.Get("/partials/home-recommendations", a.handlePartialHomeRecommendations)
	a.app.Get("/partials/home-open-bets", a.handlePartialHomeOpenBets)

	a.app.Post("/predictions/run", a.handlePredictionsRun)
	a.app.Post("/recommendations/refresh", a.handleRecommendationsRefresh)

	a.app.Post("/execution/place", a.handleExecutionPlace)
	a.app.Post("/partials/place-bet", a.handlePartialPlaceBet)
	a.app.Get("/execution/bets", a.handleExecutionBets)

	a.app.Get("/bets", a.handleBetsPage)
	a.app.Get("/bets/new", a.handleBetsNewPage)
	a.app.Post("/bets", a.handleBetsCreate)
	a.app.Post("/bets/:id/settle", a.handleBetsSettle)
	a.app.Post("/bets/:id/void", a.handleBetsVoid)

	a.app.Get("/bankroll", a.handleBankrollPage)
	a.app.Post("/bankroll/deposit", a.handleBankrollDeposit)
}

func (a *App) handleHome(c fiber.Ctx) error {
	return a.renderHome(c)
}

func (a *App) handleOdds(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	_, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Title":         "Market Board",
		"ActiveNav":     "odds",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"OddsRows":      []map[string]any{},
		"Status":        overallStatus,
		"PageStatus":    overallStatus,
	}
	applySportFilterView(view, "/odds", sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("pages/odds", view, "layouts/base")
	}

	rows, err := a.queries.ListLatestOdds(c.Context(), store.ListLatestOddsParams{
		RowLimit: 200,
		Sport:    sportFilter.storeParam(),
	})
	if err != nil {
		return err
	}

	view["OddsRows"] = mapLatestOddsRows(rows)
	return c.Render("pages/odds", view, "layouts/base")
}

func (a *App) handlePipelineHealth(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	pipeline, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Title":         "System Pulse",
		"ActiveNav":     "system",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"Pipeline":      pipeline,
		"Status":        overallStatus,
		"PageStatus":    overallStatus,
	}
	applySportFilterView(view, "/pipeline/health", sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("pages/pipeline_health", view, "layouts/base")
	}

	return c.Render("pages/pipeline_health", view, "layouts/base")
}

func (a *App) handlePartialTopbarStatus(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	_, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"Status":        overallStatus,
		"PageStatus":    overallStatus,
	}
	applySportFilterView(view, c.Path(), sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("partials/fragment_error", view)
	}

	return c.Render("partials/topbar_status", view)
}

func (a *App) handlePartialPipelineStatus(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	pipeline, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Pipeline":      pipeline,
		"OverallStatus": overallStatus,
		"Status":        overallStatus,
	}
	applySportFilterView(view, c.Path(), sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("partials/fragment_error", view)
	}

	return c.Render("partials/pipeline_status_block", view)
}

func (a *App) handlePartialOddsTable(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	view := map[string]any{
		"OddsRows": []map[string]any{},
	}
	applySportFilterView(view, c.Path(), sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("partials/fragment_error", view)
	}

	rows, err := a.queries.ListLatestOdds(c.Context(), store.ListLatestOddsParams{
		RowLimit: 200,
		Sport:    sportFilter.storeParam(),
	})
	if err != nil {
		return err
	}

	view["OddsRows"] = mapLatestOddsRows(rows)
	return c.Render("partials/odds_table_block", view)
}

func (a *App) handleHealth(c fiber.Ctx) error {
	statusCode := fiber.StatusOK
	response := map[string]any{
		"service":     "betbot",
		"status":      "ok",
		"environment": a.cfg.Env,
		"database":    "ok",
		"worker":      "ready",
	}

	if err := a.pool.Ping(c.Context()); err != nil {
		statusCode = fiber.StatusServiceUnavailable
		response["status"] = "degraded"
		response["database"] = err.Error()
	}

	if !a.oddsPollingEnabled {
		reason := a.oddsPollingDisabledReason
		if reason == "" {
			reason = "disabled"
		}
		response["worker"] = "odds-poll-disabled"
		response["odds_polling_reason"] = reason
		return c.Status(statusCode).JSON(response)
	}

	run, err := a.queries.GetLatestPollRun(c.Context())
	if err != nil {
		if store.IsNoRows(err) {
			statusCode = fiber.StatusServiceUnavailable
			response["status"] = "degraded"
			response["worker"] = "waiting-for-first-poll"
			return c.Status(statusCode).JSON(response)
		}
		return fmt.Errorf("latest poll run: %w", err)
	}

	if run.FinishedAt.Valid {
		response["last_poll_at"] = run.FinishedAt.Time.UTC().Format(time.RFC3339)
	}
	if run.Status != "success" || !run.FinishedAt.Valid || time.Since(run.FinishedAt.Time.UTC()) > a.cfg.RecentPollWindow {
		statusCode = fiber.StatusServiceUnavailable
		response["status"] = "degraded"
		response["worker"] = run.Status
		if !run.FinishedAt.Valid || time.Since(run.FinishedAt.Time.UTC()) > a.cfg.RecentPollWindow {
			response["worker"] = "stale"
		}
	}

	return c.Status(statusCode).JSON(response)
}

func (a *App) pipelineView(ctx context.Context, sportFilter sportFilterSelection) (map[string]any, string) {
	summary := map[string]any{
		"ScopeSportLabel":     sportFilter.Label,
		"ScopeSnapshotsCount": "0",
		"ScopeLastSnapshotAt": "pending",
		"ScopeError":          "none",
	}

	oddsSummary, err := a.queries.GetOddsArchiveSummary(ctx, sportFilter.storeParam())
	if err != nil {
		summary["ScopeError"] = err.Error()
	} else {
		summary["ScopeSnapshotsCount"] = fmt.Sprintf("%d", oddsSummary.SnapshotsCount)
		summary["ScopeLastSnapshotAt"] = formatTimestamp(oddsSummary.LastSnapshotAt, "pending")
	}

	if !a.oddsPollingEnabled {
		reason := a.oddsPollingDisabledReason
		if reason == "" {
			reason = "disabled"
		}
		summary["Status"] = "disabled"
		summary["LastSuccessAt"] = "disabled"
		summary["InsertCount"] = "0"
		summary["DedupCount"] = "0"
		summary["LastStartedAt"] = "disabled"
		summary["Duration"] = "n/a"
		summary["LastError"] = "odds polling disabled (" + reason + ")"
		return summary, "ok"
	}

	run, err := a.queries.GetLatestPollRun(ctx)
	if err != nil {
		if store.IsNoRows(err) {
			summary["Status"] = "warming"
			summary["LastSuccessAt"] = "pending"
			summary["InsertCount"] = "0"
			summary["DedupCount"] = "0"
			summary["LastStartedAt"] = "pending"
			summary["Duration"] = "n/a"
			summary["LastError"] = "none"
			return summary, "warming"
		}
		summary["Status"] = "degraded"
		summary["LastSuccessAt"] = "pending"
		summary["InsertCount"] = "0"
		summary["DedupCount"] = "0"
		summary["LastStartedAt"] = "pending"
		summary["Duration"] = "n/a"
		summary["LastError"] = err.Error()
		return summary, "degraded"
	}

	overall := run.Status
	lastSuccess := "pending"
	duration := "n/a"
	if run.FinishedAt.Valid {
		lastSuccess = run.FinishedAt.Time.UTC().Format(time.RFC3339)
		duration = run.FinishedAt.Time.Sub(run.StartedAt.Time).Round(time.Second).String()
		if time.Since(run.FinishedAt.Time.UTC()) > a.cfg.RecentPollWindow {
			overall = "stale"
		}
	}

	lastStartedAt := "pending"
	if run.StartedAt.Valid {
		lastStartedAt = run.StartedAt.Time.UTC().Format(time.RFC3339)
	}

	summary["Status"] = overall
	summary["LastSuccessAt"] = lastSuccess
	summary["InsertCount"] = fmt.Sprintf("%d", run.InsertsCount)
	summary["DedupCount"] = fmt.Sprintf("%d", run.DedupSkips)
	summary["LastStartedAt"] = lastStartedAt
	summary["Duration"] = duration
	summary["LastError"] = emptyAsNone(run.ErrorText)
	return summary, overall
}

func emptyAsNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func mapLatestOddsRows(rows []store.ListLatestOddsRow) []map[string]any {
	oddsRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		oddsRows = append(oddsRows, map[string]any{
			"Sport":         row.Sport,
			"HomeTeam":      row.HomeTeam,
			"AwayTeam":      row.AwayTeam,
			"Book":          row.BookName,
			"BookKey":       row.BookKey,
			"Market":        row.MarketName,
			"MarketKey":     row.MarketKey,
			"Outcome":       row.OutcomeName,
			"Side":          row.OutcomeSide,
			"Point":         row.Point,
			"PriceAmerican": row.PriceAmerican,
			"CapturedAt":    formatTimestamp(row.CapturedAt, "pending"),
		})
	}
	return oddsRows
}

func formatTimestamp(value pgtype.Timestamptz, empty string) string {
	if !value.Valid {
		return empty
	}
	return value.Time.UTC().Format(time.RFC3339)
}
