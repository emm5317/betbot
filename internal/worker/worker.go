package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/config"
	"betbot/internal/domain"
	"betbot/internal/ingestion/oddspoller"
	"betbot/internal/ingestion/statsetl"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

const (
	QueueLatencySensitive = "latency-sensitive"
	QueueMaintenance      = "maintenance"
)

type OddsPollArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Sports      []string  `json:"sports,omitempty"`
}

func (OddsPollArgs) Kind() string { return "odds_poll" }

type OddsPollWorker struct {
	river.WorkerDefaults[OddsPollArgs]
	poller *oddspoller.Poller
	logger *slog.Logger
}

func (w *OddsPollWorker) Work(ctx context.Context, job *river.Job[OddsPollArgs]) error {
	if len(job.Args.Sports) == 0 {
		w.logger.InfoContext(ctx, "odds poll job skipped",
			slog.Time("requested_at", job.Args.RequestedAt),
			slog.String("reason", "no-active-sports"),
		)
		return nil
	}

	started := time.Now()
	metrics, err := w.poller.Run(ctx, job.Args.Sports)
	if err != nil {
		w.logger.ErrorContext(ctx, "odds poll job failed",
			slog.Time("requested_at", job.Args.RequestedAt),
			slog.Any("sports", job.Args.Sports),
			slog.Duration("duration", time.Since(started)),
			slog.String("error", err.Error()),
		)
		return err
	}

	w.logger.InfoContext(ctx, "odds poll job finished",
		slog.Time("requested_at", job.Args.RequestedAt),
		slog.Any("sports", job.Args.Sports),
		slog.Duration("duration", time.Since(started)),
		slog.Int("games_seen", metrics.GamesSeen),
		slog.Int("snapshots_seen", metrics.SnapshotsSeen),
		slog.Int("inserts", metrics.Inserts),
		slog.Int("dedup_skips", metrics.DedupSkips),
	)
	return nil
}

type App struct {
	cfg    config.Config
	logger *slog.Logger
	pool   interface{ Close() }
	client *river.Client[pgx.Tx]
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	driver := riverpgxv5.New(pool)
	migrator, err := rivermigrate.New(driver, &rivermigrate.Config{Schema: cfg.RiverSchema})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate river schema: %w", err)
	}

	sportRegistry := domain.DefaultSportRegistry()
	workers := river.NewWorkers()
	river.AddWorker(workers, &OddsPollWorker{poller: oddspoller.NewPoller(cfg, logger, pool), logger: logger})
	river.AddWorker(workers, NewMLBStatsETLWorker(pool, logger, statsetl.NewMLBStatsAPIProvider("", 0)))
	river.AddWorker(workers, NewNBAStatsETLWorker(pool, logger, statsetl.UnconfiguredNBAProvider{}))

	client, err := river.NewClient(driver, &river.Config{
		Logger: logger,
		Schema: cfg.RiverSchema,
		Queues: map[string]river.QueueConfig{
			QueueLatencySensitive: {MaxWorkers: 1},
			QueueMaintenance:      {MaxWorkers: 1},
		},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(cfg.OddsAPIPollInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					args := activeOddsPollArgs(time.Now().UTC(), sportRegistry, cfg.OddsAPISports)
					return args, &river.InsertOpts{Queue: QueueLatencySensitive}
				},
				&river.PeriodicJobOpts{ID: "odds-poll", RunOnStart: true},
			),
		},
		ReindexerSchedule: river.NeverSchedule(),
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create river client: %w", err)
	}

	return &App{cfg: cfg, logger: logger, pool: pool, client: client}, nil
}

func activeOddsPollArgs(at time.Time, registry domain.SportRegistry, configuredSports []string) OddsPollArgs {
	return OddsPollArgs{
		RequestedAt: at.UTC(),
		Sports:      registry.ActiveOddsAPISports(at, configuredSports),
	}
}

func (a *App) EnqueueMLBStatsETL(ctx context.Context, req statsetl.MLBRequest) (*rivertype.JobInsertResult, error) {
	return EnqueueMLBStatsETL(ctx, a.client, req)
}

func (a *App) Close() {
	a.pool.Close()
}

func (a *App) Run(ctx context.Context) error {
	if err := a.client.Start(context.Background()); err != nil {
		a.Close()
		return err
	}

	a.logger.Info("betbot worker started", slog.Duration("poll_interval", a.cfg.OddsAPIPollInterval))
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := a.client.Stop(stopCtx)
	a.Close()
	return err
}
