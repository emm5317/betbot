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

type NFLStatsETLArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Season      int32     `json:"season,omitempty" river:"unique"`
	SeasonType  string    `json:"season_type,omitempty" river:"unique"`
	StatDate    time.Time `json:"stat_date" river:"unique"`
}

func (NFLStatsETLArgs) Kind() string { return "nfl_stats_etl" }

func (NFLStatsETLArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewNFLStatsETLArgs(req statsetl.NFLRequest) (NFLStatsETLArgs, error) {
	normalized, err := statsetl.NormalizeNFLRequest(req)
	if err != nil {
		return NFLStatsETLArgs{}, err
	}
	return NFLStatsETLArgs{
		RequestedAt: normalized.RequestedAt,
		Season:      normalized.Season,
		SeasonType:  normalized.SeasonType,
		StatDate:    normalized.StatDate,
	}, nil
}

func EnqueueNFLStatsETL(ctx context.Context, inserter JobInserter, req statsetl.NFLRequest) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("nfl stats etl inserter is nil")
	}
	args, err := NewNFLStatsETLArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type NFLStatsETLWorker struct {
	river.WorkerDefaults[NFLStatsETLArgs]
	pool   *pgxpool.Pool
	etl    *statsetl.NFLETL
	logger *slog.Logger
}

func NewNFLStatsETLWorker(pool *pgxpool.Pool, logger *slog.Logger, provider statsetl.NFLProvider) *NFLStatsETLWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &NFLStatsETLWorker{
		pool:   pool,
		etl:    statsetl.NewNFLETL(provider, logger),
		logger: logger,
	}
}

func (w *NFLStatsETLWorker) Work(ctx context.Context, job *river.Job[NFLStatsETLArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin nfl stats etl tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.etl.Run(ctx, store.New(tx), statsetl.NFLRequest{
		RequestedAt: job.Args.RequestedAt,
		Season:      job.Args.Season,
		SeasonType:  job.Args.SeasonType,
		StatDate:    job.Args.StatDate,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "nfl stats etl job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.Int("season", int(job.Args.Season)),
			slog.String("season_type", job.Args.SeasonType),
			slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit nfl stats etl tx: %w", err)
	}

	w.logger.InfoContext(ctx, "nfl stats etl job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(job.Args.Season)),
		slog.String("season_type", job.Args.SeasonType),
		slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
	)
	return nil
}
