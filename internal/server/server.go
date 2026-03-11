package server

import (
	"context"
	"errors"
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
)

type App struct {
	app    *fiber.App
	cfg    config.Config
	logger *slog.Logger
	pool   interface {
		Close()
		Ping(context.Context) error
	}
	queries *store.Queries
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

	instance := &App{
		app:     app,
		cfg:     cfg,
		logger:  appLogger,
		pool:    pool,
		queries: store.New(pool),
	}
	instance.routes()
	return instance, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("betbot server listening", slog.String("addr", a.cfg.HTTPAddr))
		if err := a.app.Listen(a.cfg.HTTPAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer a.pool.Close()
		return a.app.ShutdownWithContext(shutdownCtx)
	case err := <-errCh:
		a.pool.Close()
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
	a.app.Get("/pipeline/health", a.handlePipelineHealth)
}

func (a *App) handleHome(c fiber.Ctx) error {
	_, overallStatus := a.pipelineView(c.Context())
	return c.Render("pages/home", map[string]any{
		"Title":         "betbot",
		"ActiveNav":     "home",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"WorkerStatus":  overallStatus,
		"Pipeline":      first(a.pipelineView(c.Context())),
		"Status":        overallStatus,
	}, "layouts/base")
}

func (a *App) handleOdds(c fiber.Ctx) error {
	rows, err := a.queries.ListLatestOdds(c.Context(), 200)
	if err != nil {
		return err
	}

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
			"CapturedAt":    row.CapturedAt.Format(time.RFC3339),
		})
	}

	_, overallStatus := a.pipelineView(c.Context())
	return c.Render("pages/odds", map[string]any{
		"Title":         "Latest Odds",
		"ActiveNav":     "odds",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"Rows":          oddsRows,
		"Status":        overallStatus,
	}, "layouts/base")
}

func (a *App) handlePipelineHealth(c fiber.Ctx) error {
	pipeline, overallStatus := a.pipelineView(c.Context())
	return c.Render("pages/pipeline_health", map[string]any{
		"Title":         "Pipeline Health",
		"ActiveNav":     "pipeline",
		"OverallStatus": overallStatus,
		"Environment":   a.cfg.Env,
		"Pipeline":      pipeline,
		"Status":        overallStatus,
	}, "layouts/base")
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

	if run.FinishedAt != nil {
		response["last_poll_at"] = run.FinishedAt.Format(time.RFC3339)
	}
	if run.Status != "success" || run.FinishedAt == nil || time.Since(run.FinishedAt.UTC()) > a.cfg.RecentPollWindow {
		statusCode = fiber.StatusServiceUnavailable
		response["status"] = "degraded"
		response["worker"] = run.Status
		if run.FinishedAt == nil || time.Since(run.FinishedAt.UTC()) > a.cfg.RecentPollWindow {
			response["worker"] = "stale"
		}
	}

	return c.Status(statusCode).JSON(response)
}

func (a *App) pipelineView(ctx context.Context) (map[string]any, string) {
	run, err := a.queries.GetLatestPollRun(ctx)
	if err != nil {
		if store.IsNoRows(err) {
			pending := map[string]any{
				"Status":        "warming",
				"LastSuccessAt": "pending",
				"InsertCount":   "0",
				"DedupCount":    "0",
				"LastStartedAt": "pending",
				"Duration":      "n/a",
				"LastError":     "none",
			}
			return pending, "warming"
		}
		degraded := map[string]any{
			"Status":        "degraded",
			"LastSuccessAt": "pending",
			"InsertCount":   "0",
			"DedupCount":    "0",
			"LastStartedAt": "pending",
			"Duration":      "n/a",
			"LastError":     err.Error(),
		}
		return degraded, "degraded"
	}

	overall := run.Status
	lastSuccess := "pending"
	duration := "n/a"
	if run.FinishedAt != nil {
		lastSuccess = run.FinishedAt.Format(time.RFC3339)
		duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
		if time.Since(run.FinishedAt.UTC()) > a.cfg.RecentPollWindow {
			overall = "stale"
		}
	}

	return map[string]any{
		"Status":        overall,
		"LastSuccessAt": lastSuccess,
		"InsertCount":   fmt.Sprintf("%d", run.InsertsCount),
		"DedupCount":    fmt.Sprintf("%d", run.DedupSkips),
		"LastStartedAt": run.StartedAt.Format(time.RFC3339),
		"Duration":      duration,
		"LastError":     emptyAsNone(run.ErrorText),
	}, overall
}

func emptyAsNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func first(value map[string]any, _ string) map[string]any {
	return value
}
