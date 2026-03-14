package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"betbot/internal/config"
	"betbot/internal/store"

	"github.com/gofiber/fiber/v3"
	fiberlogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	html "github.com/gofiber/template/html/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

type readQueries interface {
	GetLatestPollRun(ctx context.Context) (store.PollRun, error)
	GetOddsArchiveSummary(ctx context.Context, sport *string) (store.GetOddsArchiveSummaryRow, error)
	ListLatestOdds(ctx context.Context, arg store.ListLatestOddsParams) ([]store.ListLatestOddsRow, error)
	ListModelPredictionsForSportSeason(ctx context.Context, arg store.ListModelPredictionsForSportSeasonParams) ([]store.ModelPrediction, error)
	ListRecommendationPerformanceSnapshots(ctx context.Context, arg store.ListRecommendationPerformanceSnapshotsParams) ([]store.ListRecommendationPerformanceSnapshotsRow, error)
	GetBankrollBalanceCents(ctx context.Context) (int64, error)
	InsertRecommendationOutcomeIfChanged(ctx context.Context, arg store.InsertRecommendationOutcomeIfChangedParams) (int64, error)
	InsertRecommendationSnapshot(ctx context.Context, arg store.InsertRecommendationSnapshotParams) (store.RecommendationSnapshot, error)
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
	instance := &App{
		app:                       app,
		cfg:                       cfg,
		logger:                    appLogger,
		pool:                      pool,
		queries:                   store.New(pool),
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
	a.app.Get("/pipeline/health", a.handlePipelineHealth)
	a.app.Get("/partials/topbar-status", a.handlePartialTopbarStatus)
	a.app.Get("/partials/pipeline-status", a.handlePartialPipelineStatus)
	a.app.Get("/partials/odds-table", a.handlePartialOddsTable)
}

func (a *App) handleHome(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	pipeline, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Title":         "betbot",
		"ActiveNav":     "home",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"WorkerStatus":  overallStatus,
		"Pipeline":      pipeline,
		"Status":        overallStatus,
		"PageStatus":    overallStatus,
	}
	applySportFilterView(view, "/", sportFilter)
	if filterErr != nil {
		view["Alert"] = map[string]any{
			"Title":   "Invalid sport filter",
			"Message": filterErr.Error(),
		}
		return c.Status(fiber.StatusBadRequest).Render("pages/home", view, "layouts/base")
	}

	return c.Render("pages/home", view, "layouts/base")
}

func (a *App) handleOdds(c fiber.Ctx) error {
	sportFilter, filterErr := resolveSportFilter(c.Query("sport"))
	_, overallStatus := a.pipelineView(c.Context(), sportFilter)

	view := map[string]any{
		"Title":         "Latest Odds",
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
		"Title":         "Pipeline Health",
		"ActiveNav":     "pipeline",
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
