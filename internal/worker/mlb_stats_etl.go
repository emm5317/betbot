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

type JobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type MLBStatsETLArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Season      int32     `json:"season,omitempty" river:"unique"`
	SeasonType  string    `json:"season_type,omitempty" river:"unique"`
	StatDate    time.Time `json:"stat_date" river:"unique"`
}

func (MLBStatsETLArgs) Kind() string { return "mlb_stats_etl" }

func (MLBStatsETLArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewMLBStatsETLArgs(req statsetl.MLBRequest) (MLBStatsETLArgs, error) {
	normalized, err := statsetl.NormalizeMLBRequest(req)
	if err != nil {
		return MLBStatsETLArgs{}, err
	}
	return MLBStatsETLArgs{
		RequestedAt: normalized.RequestedAt,
		Season:      normalized.Season,
		SeasonType:  normalized.SeasonType,
		StatDate:    normalized.StatDate,
	}, nil
}

func EnqueueMLBStatsETL(ctx context.Context, inserter JobInserter, req statsetl.MLBRequest) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("mlb stats etl inserter is nil")
	}
	args, err := NewMLBStatsETLArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type MLBStatsETLWorker struct {
	river.WorkerDefaults[MLBStatsETLArgs]
	pool   *pgxpool.Pool
	etl    *statsetl.MLBETL
	logger *slog.Logger
}

func NewMLBStatsETLWorker(pool *pgxpool.Pool, logger *slog.Logger, provider statsetl.MLBProvider) *MLBStatsETLWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &MLBStatsETLWorker{
		pool:   pool,
		etl:    statsetl.NewMLBETL(provider, logger),
		logger: logger,
	}
}

func (w *MLBStatsETLWorker) Work(ctx context.Context, job *river.Job[MLBStatsETLArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin mlb stats etl tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.etl.Run(ctx, store.New(tx), statsetl.MLBRequest{
		RequestedAt: job.Args.RequestedAt,
		Season:      job.Args.Season,
		SeasonType:  job.Args.SeasonType,
		StatDate:    job.Args.StatDate,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "mlb stats etl job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.Int("season", int(job.Args.Season)),
			slog.String("season_type", job.Args.SeasonType),
			slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit mlb stats etl tx: %w", err)
	}

	w.logger.InfoContext(ctx, "mlb stats etl job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("pitcher_rows", metrics.PitcherRows),
		slog.Int("season", int(job.Args.Season)),
		slog.String("season_type", job.Args.SeasonType),
		slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
	)
	return nil
}
