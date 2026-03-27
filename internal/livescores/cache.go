package livescores

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"betbot/internal/ingestion/moneypuck"
)

const (
	pollIntervalLive    = 30 * time.Second
	pollIntervalIdle    = 2 * time.Minute
	pollIntervalNoGames = 10 * time.Minute
)

// Snapshot holds a point-in-time scoreboard fetch.
type Snapshot struct {
	Games     []LiveGame
	FetchedAt time.Time
}

// ScoreCache holds the latest NHL scoreboard in memory and
// runs a background goroutine to poll the NHL API.
type ScoreCache struct {
	client  *Client
	logger  *slog.Logger
	teamMap moneypuck.TeamMap

	mu       sync.RWMutex
	snapshot *Snapshot
}

// NewScoreCache creates a new score cache.
func NewScoreCache(client *Client, logger *slog.Logger) *ScoreCache {
	return &ScoreCache{
		client:  client,
		logger:  logger,
		teamMap: moneypuck.NewTeamMap(),
	}
}

// Start begins the background polling goroutine.
func (sc *ScoreCache) Start(ctx context.Context) {
	sc.logger.Info("live scores poller started")

	interval := pollIntervalIdle
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			sc.logger.Info("live scores poller stopped")
			return
		case <-timer.C:
			sc.fetch(ctx)
			interval = sc.nextInterval()
			timer.Reset(interval)
		}
	}
}

// Refresh forces an immediate fetch. Called on server startup.
func (sc *ScoreCache) Refresh(ctx context.Context) {
	sc.fetch(ctx)
}

// Latest returns the most recent snapshot, or nil if not yet fetched.
func (sc *ScoreCache) Latest() *Snapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.snapshot
}

func (sc *ScoreCache) fetch(ctx context.Context) {
	resp, err := sc.client.FetchScores(ctx)
	if err != nil {
		sc.logger.Warn("failed to fetch live scores", slog.String("error", err.Error()))
		return
	}

	games := make([]LiveGame, 0, len(resp.Games))
	for _, g := range resp.Games {
		if g.GameScheduleState != "OK" {
			continue
		}
		games = append(games, NormalizeNHLGame(g, sc.teamMap))
	}

	sc.mu.Lock()
	sc.snapshot = &Snapshot{
		Games:     games,
		FetchedAt: time.Now(),
	}
	sc.mu.Unlock()

	sc.logger.Info("live scores updated",
		slog.Int("games", len(games)),
		slog.String("date", resp.CurrentDate),
	)
}

func (sc *ScoreCache) nextInterval() time.Duration {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.snapshot == nil || len(sc.snapshot.Games) == 0 {
		return pollIntervalNoGames
	}

	for _, g := range sc.snapshot.Games {
		if g.IsLive() {
			return pollIntervalLive
		}
	}

	return pollIntervalIdle
}
