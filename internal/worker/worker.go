package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/config"
	"betbot/internal/domain"
	executionadapters "betbot/internal/execution/adapters"
	"betbot/internal/ingestion/injuries"
	"betbot/internal/ingestion/oddspoller"
	"betbot/internal/ingestion/scores"
	"betbot/internal/ingestion/statsetl"
	"betbot/internal/ingestion/weather"
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
	poller         *oddspoller.Poller
	logger         *slog.Logger
	enabled        bool
	disabledReason string
}

func (w *OddsPollWorker) Work(ctx context.Context, job *river.Job[OddsPollArgs]) error {
	if !w.enabled {
		reason := w.disabledReason
		if reason == "" {
			reason = "disabled"
		}
		w.logger.InfoContext(ctx, "odds poll job skipped",
			slog.Time("requested_at", job.Args.RequestedAt),
			slog.String("reason", reason),
		)
		return nil
	}

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
	cfg                        config.Config
	logger                     *slog.Logger
	pool                       interface{ Close() }
	client                     *river.Client[pgx.Tx]
	oddsPollingEnabled         bool
	oddsPollingDisableReason   string
	autoPlacementEnabled       bool
	autoPlacementDisableReason string
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
	oddsPollingEnabled, oddsPollingDisableReason := cfg.OddsPollingRuntime()
	autoPlacementEnabled, autoPlacementDisableReason := cfg.AutoPlacementRuntime()
	workers := river.NewWorkers()
	river.AddWorker(workers, &OddsPollWorker{
		poller:         oddspoller.NewPoller(cfg, logger, pool),
		logger:         logger,
		enabled:        oddsPollingEnabled,
		disabledReason: oddsPollingDisableReason,
	})
	river.AddWorker(workers, NewMLBStatsETLWorker(pool, logger, statsetl.NewMLBStatsAPIProvider("", 0)))
	river.AddWorker(workers, NewNBAStatsETLWorker(pool, logger, statsetl.NewNBAStatsAPIProvider("", 0)))
	river.AddWorker(workers, NewNHLStatsETLWorker(pool, logger, statsetl.NewNHLStatsAPIProvider("", 0)))
	river.AddWorker(workers, NewNFLStatsETLWorker(pool, logger, statsetl.NewNFLverseProvider("", "", 0)))
	river.AddWorker(workers, NewInjurySyncWorker(pool, logger, injuries.NewRotowireProvider("", 0)))
	river.AddWorker(workers, NewWeatherSyncWorker(pool, logger, weather.NewOpenMeteoProvider("", 0)))
	river.AddWorker(workers, NewPredictionWorker(pool, logger))
	river.AddWorker(workers, NewAutoSettlementWorker(
		pool,
		logger,
		scores.NewClient(cfg.OddsAPIKey, cfg.OddsAPIBaseURL, cfg.OddsAPITimeout, cfg.OddsAPIRateLimit),
		cfg.OddsAPISource,
	))
	if autoPlacementEnabled {
		adapter, err := executionadapters.NewBookAdapter(cfg.ExecutionAdapter)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("configure auto placement adapter: %w", err)
		}
		river.AddWorker(workers, NewAutoPlacementWorker(pool, logger, adapter))
	} else {
		logger.Info("auto placement disabled", slog.String("reason", autoPlacementDisableReason))
	}

	periodicJobs := []*river.PeriodicJob{}
	if oddsPollingEnabled {
		periodicJobs = append(periodicJobs,
			river.NewPeriodicJob(
				river.PeriodicInterval(cfg.OddsAPIPollInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					args := activeOddsPollArgs(time.Now().UTC(), sportRegistry, cfg.OddsAPISports)
					return args, &river.InsertOpts{Queue: QueueLatencySensitive}
				},
				&river.PeriodicJobOpts{ID: "odds-poll", RunOnStart: true},
			),
		)
	} else {
		reason := oddsPollingDisableReason
		if reason == "" {
			reason = "disabled"
		}
		logger.Info("odds polling disabled", slog.String("reason", reason))
	}

	// NHL prediction job runs every 15 minutes
	periodicJobs = append(periodicJobs,
		river.NewPeriodicJob(
			river.PeriodicInterval(15*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return PredictionArgs{
					RequestedAt: time.Now().UTC(),
					Sport:       "NHL",
				}, nil
			},
			&river.PeriodicJobOpts{ID: "nhl-prediction", RunOnStart: true},
		),
	)

	periodicJobs = append(periodicJobs,
		river.NewPeriodicJob(
			river.PeriodicInterval(autoSettlementInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return AutoSettlementArgs{
					RequestedAt: time.Now().UTC(),
				}, nil
			},
			&river.PeriodicJobOpts{ID: "auto-settlement", RunOnStart: true},
		),
	)

	if autoPlacementEnabled {
		periodicJobs = append(periodicJobs,
			river.NewPeriodicJob(
				river.PeriodicInterval(autoPlacementInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return AutoPlacementArgs{
						RequestedAt: time.Now().UTC(),
					}, nil
				},
				&river.PeriodicJobOpts{ID: "auto-placement", RunOnStart: true},
			),
		)
	}

	client, err := river.NewClient(driver, &river.Config{
		Logger: logger,
		Schema: cfg.RiverSchema,
		Queues: map[string]river.QueueConfig{
			QueueLatencySensitive: {MaxWorkers: 1},
			QueueMaintenance:      {MaxWorkers: 1},
		},
		Workers:           workers,
		PeriodicJobs:      periodicJobs,
		ReindexerSchedule: river.NeverSchedule(),
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create river client: %w", err)
	}

	return &App{
		cfg:                        cfg,
		logger:                     logger,
		pool:                       pool,
		client:                     client,
		oddsPollingEnabled:         oddsPollingEnabled,
		oddsPollingDisableReason:   oddsPollingDisableReason,
		autoPlacementEnabled:       autoPlacementEnabled,
		autoPlacementDisableReason: autoPlacementDisableReason,
	}, nil
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

func (a *App) EnqueueNBAStatsETL(ctx context.Context, req statsetl.NBARequest) (*rivertype.JobInsertResult, error) {
	return EnqueueNBAStatsETL(ctx, a.client, req)
}

func (a *App) EnqueueNHLStatsETL(ctx context.Context, req statsetl.NHLRequest) (*rivertype.JobInsertResult, error) {
	return EnqueueNHLStatsETL(ctx, a.client, req)
}

func (a *App) EnqueueNFLStatsETL(ctx context.Context, req statsetl.NFLRequest) (*rivertype.JobInsertResult, error) {
	return EnqueueNFLStatsETL(ctx, a.client, req)
}

func (a *App) EnqueueInjurySync(ctx context.Context, req injuries.Request) (*rivertype.JobInsertResult, error) {
	return EnqueueInjurySync(ctx, a.client, req)
}

func (a *App) EnqueueWeatherSync(ctx context.Context, req weather.Request) (*rivertype.JobInsertResult, error) {
	return EnqueueWeatherSync(ctx, a.client, req)
}

func (a *App) EnqueuePrediction(ctx context.Context, sport string) (*rivertype.JobInsertResult, error) {
	return EnqueuePrediction(ctx, a.client, sport)
}

func (a *App) Close() {
	a.pool.Close()
}

func (a *App) Run(ctx context.Context) error {
	if err := a.client.Start(context.Background()); err != nil {
		a.Close()
		return err
	}

	attrs := []any{
		slog.Duration("poll_interval", a.cfg.OddsAPIPollInterval),
		slog.Bool("odds_polling_enabled", a.oddsPollingEnabled),
		slog.Bool("auto_placement_enabled", a.autoPlacementEnabled),
		slog.String("execution_adapter", a.cfg.ExecutionAdapter),
		slog.Bool("paper_mode", a.cfg.PaperMode),
	}
	if !a.oddsPollingEnabled {
		reason := a.oddsPollingDisableReason
		if reason == "" {
			reason = "disabled"
		}
		attrs = append(attrs, slog.String("odds_polling_disabled_reason", reason))
	}
	if !a.autoPlacementEnabled {
		reason := a.autoPlacementDisableReason
		if reason == "" {
			reason = "disabled"
		}
		attrs = append(attrs, slog.String("auto_placement_disabled_reason", reason))
	}
	a.logger.Info("betbot worker started", attrs...)
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := a.client.Stop(stopCtx)
	a.Close()
	return err
}
