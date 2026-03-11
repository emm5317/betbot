package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/ingestion/injuries"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type InjurySyncArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	ReportDate  time.Time `json:"report_date" river:"unique"`
	Sport       string    `json:"sport,omitempty" river:"unique"`
	Source      string    `json:"source,omitempty" river:"unique"`
}

func (InjurySyncArgs) Kind() string { return "injury_sync" }

func (InjurySyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

func NewInjurySyncArgs(req injuries.Request) (InjurySyncArgs, error) {
	normalized, err := injuries.NormalizeRequest(req)
	if err != nil {
		return InjurySyncArgs{}, err
	}
	return InjurySyncArgs{
		RequestedAt: normalized.RequestedAt,
		ReportDate:  normalized.ReportDate,
		Sport:       normalized.Sport,
		Source:      normalized.Source,
	}, nil
}

func EnqueueInjurySync(ctx context.Context, inserter JobInserter, req injuries.Request) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, errors.New("injury sync inserter is nil")
	}
	args, err := NewInjurySyncArgs(req)
	if err != nil {
		return nil, err
	}
	return inserter.Insert(ctx, args, nil)
}

type InjurySyncWorker struct {
	river.WorkerDefaults[InjurySyncArgs]
	pool    *pgxpool.Pool
	scraper *injuries.Scraper
	logger  *slog.Logger
}

func NewInjurySyncWorker(pool *pgxpool.Pool, logger *slog.Logger, provider injuries.Provider) *InjurySyncWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &InjurySyncWorker{
		pool:    pool,
		scraper: injuries.NewScraper(provider, logger),
		logger:  logger,
	}
}

func (w *InjurySyncWorker) Work(ctx context.Context, job *river.Job[InjurySyncArgs]) error {
	started := time.Now()

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin injury sync tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metrics, err := w.scraper.Run(ctx, store.New(tx), injuries.Request{
		RequestedAt: job.Args.RequestedAt,
		ReportDate:  job.Args.ReportDate,
		Sport:       job.Args.Sport,
		Source:      job.Args.Source,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "injury sync job failed",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(started)),
			slog.String("sport", job.Args.Sport),
			slog.String("source", job.Args.Source),
			slog.String("report_date", job.Args.ReportDate.UTC().Format(time.DateOnly)),
		)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit injury sync tx: %w", err)
	}

	w.logger.InfoContext(ctx, "injury sync job finished",
		slog.Duration("duration", time.Since(started)),
		slog.Int("record_rows", metrics.RecordRows),
		slog.String("sport", job.Args.Sport),
		slog.String("source", job.Args.Source),
		slog.String("report_date", job.Args.ReportDate.UTC().Format(time.DateOnly)),
	)
	return nil
}
