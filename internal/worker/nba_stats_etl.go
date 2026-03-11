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

type NBAStatsETLArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Season      int32     `json:"season,omitempty" river:"unique"`
	SeasonType  string    `json:"season_type,omitempty" river:"unique"`
	StatDate    time.Time `json:"stat_date" river:"unique"`
}

func (NBAStatsETLArgs) Kind() string { return "nba_stats_etl" }

func (NBAStatsETLArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewNBAStatsETLArgs(req statsetl.NBARequest) (NBAStatsETLArgs, error) {
	normalized, err := statsetl.NormalizeNBARequest(req)
	if err != nil {
		return NBAStatsETLArgs{}, err
	}
	return NBAStatsETLArgs{
		RequestedAt: normalized.RequestedAt,
		Season:      normalized.Season,
		SeasonType:  normalized.SeasonType,
		StatDate:    normalized.StatDate,
	}, nil
}

func EnqueueNBAStatsETL(ctx context.Context, inserter JobInserter, req statsetl.NBARequest) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("nba stats etl inserter is nil")
	}
	args, err := NewNBAStatsETLArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type NBAStatsETLWorker struct {
	river.WorkerDefaults[NBAStatsETLArgs]
	pool   *pgxpool.Pool
	etl    *statsetl.NBAETL
	logger *slog.Logger
}

func NewNBAStatsETLWorker(pool *pgxpool.Pool, logger *slog.Logger, provider statsetl.NBAProvider) *NBAStatsETLWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &NBAStatsETLWorker{
		pool:   pool,
		etl:    statsetl.NewNBAETL(provider, logger),
		logger: logger,
	}
}

func (w *NBAStatsETLWorker) Work(ctx context.Context, job *river.Job[NBAStatsETLArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin nba stats etl tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.etl.Run(ctx, store.New(tx), statsetl.NBARequest{
		RequestedAt: job.Args.RequestedAt,
		Season:      job.Args.Season,
		SeasonType:  job.Args.SeasonType,
		StatDate:    job.Args.StatDate,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "nba stats etl job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.Int("season", int(job.Args.Season)),
			slog.String("season_type", job.Args.SeasonType),
			slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit nba stats etl tx: %w", err)
	}

	w.logger.InfoContext(ctx, "nba stats etl job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("team_rows", metrics.TeamRows),
		slog.Int("season", int(job.Args.Season)),
		slog.String("season_type", job.Args.SeasonType),
		slog.String("stat_date", job.Args.StatDate.UTC().Format(time.DateOnly)),
	)
	return nil
}
