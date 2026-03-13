package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/ingestion/weather"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type WeatherSyncArgs struct {
	RequestedAt  time.Time `json:"requested_at"`
	ForecastDate time.Time `json:"forecast_date" river:"unique"`
	Sport        string    `json:"sport,omitempty" river:"unique"`
}

func (WeatherSyncArgs) Kind() string { return "weather_sync" }

func (WeatherSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewWeatherSyncArgs(req weather.Request) (WeatherSyncArgs, error) {
	normalized, err := weather.NormalizeRequest(req)
	if err != nil {
		return WeatherSyncArgs{}, err
	}
	return WeatherSyncArgs{
		RequestedAt:  normalized.RequestedAt,
		ForecastDate: normalized.ForecastDate,
		Sport:        normalized.Sport,
	}, nil
}

func EnqueueWeatherSync(ctx context.Context, inserter JobInserter, req weather.Request) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("weather sync inserter is nil")
	}
	args, err := NewWeatherSyncArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type WeatherSyncWorker struct {
	river.WorkerDefaults[WeatherSyncArgs]
	pool   *pgxpool.Pool
	syncer *weather.Syncer
	logger *slog.Logger
}

func NewWeatherSyncWorker(pool *pgxpool.Pool, logger *slog.Logger, provider weather.Provider) *WeatherSyncWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &WeatherSyncWorker{
		pool:   pool,
		syncer: weather.NewSyncer(provider, logger),
		logger: logger,
	}
}

func (w *WeatherSyncWorker) Work(ctx context.Context, job *river.Job[WeatherSyncArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin weather sync tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.syncer.Run(ctx, store.New(tx), weather.Request{
		RequestedAt:  job.Args.RequestedAt,
		ForecastDate: job.Args.ForecastDate,
		Sport:        job.Args.Sport,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "weather sync job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.String("forecast_date", job.Args.ForecastDate.UTC().Format(time.DateOnly)),
			slog.String("sport", job.Args.Sport),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit weather sync tx: %w", err)
	}

	w.logger.InfoContext(ctx, "weather sync job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("games_considered", metrics.GamesConsidered),
		slog.Int("persisted_rows", metrics.PersistedRows),
		slog.Int("outdoor_rows", metrics.OutdoorRows),
		slog.Int("indoor_rows", metrics.IndoorRows),
		slog.Int("dome_rows", metrics.DomeRows),
		slog.Int("retractable_rows", metrics.RetractableRows),
		slog.Int("missing_venue_games", metrics.MissingVenueGames),
		slog.String("forecast_date", job.Args.ForecastDate.UTC().Format(time.DateOnly)),
		slog.String("sport", job.Args.Sport),
	)
	return nil
}
