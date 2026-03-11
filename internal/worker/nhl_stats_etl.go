package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/ingestion/statsetl"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type NHLStatsETLArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Season      int32     `json:"season,omitempty" river:"unique"`
	SeasonType  string    `json:"season_type,omitempty" river:"unique"`
	StatDate    time.Time `json:"stat_date" river:"unique"`
}

func (NHLStatsETLArgs) Kind() string { return "nhl_stats_etl" }

func (NHLStatsETLArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewNHLStatsETLArgs(req statsetl.NHLRequest) (NHLStatsETLArgs, error) {
	normalized, err := statsetl.NormalizeNHLRequest(req)
	if err != nil {
		return NHLStatsETLArgs{}, err
	}
	return NHLStatsETLArgs{
		RequestedAt: normalized.RequestedAt,
		Season:      normalized.Season,
		SeasonType:  normalized.SeasonType,
		StatDate:    normalized.StatDate,
	}, nil
}

func EnqueueNHLStatsETL(ctx context.Context, inserter JobInserter, req statsetl.NHLRequest) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("nhl stats etl inserter is nil")
	}
	args, err := NewNHLStatsETLArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type NHLStatsETLWorker struct {
	river.WorkerDefaults[NHLStatsETLArgs]
	pool   *pgxpool.Pool
	etl    *statsetl.NHLETL
	logger *slog.Logger
}

func NewNHLStatsETLWorker(pool *pgxpool.Pool, logger *slog.Logger, provider statsetl.NHLProvider) *NHLStatsETLWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &NHLStatsETLWorker{
		pool:   pool,
		etl:    statsetl.NewNHLETL(provider, logger),
		logger: logger,
	}
}

func (w *NHLStatsETLWorker) Work(ctx context.Context, job *river.Job[NHLStatsETLArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin nhl stats etl tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.etl.Run(ctx, store.New(tx), statsetl.NHLRequest{
		RequestedAt: job.Args.RequestedAt,
		Season:      job.Args.Season,
		SeasonType:  job.Args.SeasonType,
		StatDate:    job.Args.StatDate,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "nhl stats etl job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.Int("season", int(job.Args.Season)),
			slog.String("season_type", job.Args.SeasonType),
			slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit nhl stats etl tx: %w", err)
	}

	w.logger.InfoContext(ctx, "nhl stats etl job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(job.Args.Season)),
		slog.String("season_type", job.Args.SeasonType),
		slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
	)
	return nil
}
