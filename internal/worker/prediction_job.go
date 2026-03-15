package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/prediction"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type PredictionArgs struct {
	RequestedAt time.Time `json:"requested_at"`
	Sport       string    `json:"sport" river:"unique"`
}

func (PredictionArgs) Kind() string { return "prediction" }

func (PredictionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: 15 * time.Minute,
		},
	}
}

type PredictionWorker struct {
	river.WorkerDefaults[PredictionArgs]
	nhlService *prediction.NHLPredictionService
	logger     *slog.Logger
}

func NewPredictionWorker(pool *pgxpool.Pool, logger *slog.Logger) *PredictionWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &PredictionWorker{
		nhlService: prediction.NewNHLPredictionService(pool, logger),
		logger:     logger,
	}
}

func (w *PredictionWorker) Work(ctx context.Context, job *river.Job[PredictionArgs]) error {
	started := time.Now()

	var predicted int
	var err error

	switch job.Args.Sport {
	case "NHL":
		predicted, err = w.nhlService.PredictUpcomingGames(ctx)
	default:
		w.logger.InfoContext(ctx, "prediction job skipped: unsupported sport",
			slog.String("sport", job.Args.Sport),
		)
		return nil
	}

	if err != nil {
		w.logger.ErrorContext(ctx, "prediction job failed",
			slog.String("sport", job.Args.Sport),
			slog.Duration("duration", time.Since(started)),
			slog.String("error", err.Error()),
		)
		return err
	}

	w.logger.InfoContext(ctx, "prediction job finished",
		slog.String("sport", job.Args.Sport),
		slog.Duration("duration", time.Since(started)),
		slog.Int("predictions_made", predicted),
	)
	return nil
}

func EnqueuePrediction(ctx context.Context, inserter JobInserter, sport string) (*rivertype.JobInsertResult, error) {
	if inserter == nil {
		return nil, fmt.Errorf("prediction inserter is nil")
	}
	return inserter.Insert(ctx, PredictionArgs{
		RequestedAt: time.Now().UTC(),
		Sport:       sport,
	}, nil)
}
