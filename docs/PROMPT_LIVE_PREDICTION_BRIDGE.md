# Session Prompt: Build Live Prediction Bridge

## Task

Build the live prediction bridge so betbot can produce NHL betting recommendations from today's odds + current-season MoneyPuck stats. Read `docs/PLAN_LIVE_PREDICTION_BRIDGE.md` for the full design. Read `CLAUDE.md` and `docs/TRACKER.md` for project context.

## What Exists

- **NHL model** (`internal/modeling/nhl/predictor.go`): `XGGoalieModel` with 6 features (xG%, Corsi, GoalsFor, GoalsAgainst, GSAx, PDO). Tuned and backtested at Brier 0.2427 on 12,200 games.
- **Feature builder** (`internal/backtest/nhlfeatures.go`): `BuildNHLFeatures()` and `BuildNHLFeaturesFromAbbrev()` take team names + game date, query MoneyPuck rolling stats, return a `features.BuildRequest`. Also defines `MoneyPuckStore` interface.
- **Decision engine** (`internal/decision/recommendation.go`): `BuildRecommendations()` takes candidates with model probability + market odds → produces ranked, sized, risk-checked recommendations.
- **Recommendation endpoint** (`internal/server/recommendations.go`): `GET /recommendations` reads from `model_predictions` table + `odds_history` table, joins by `game_id + market_key`, feeds to decision engine.
- **MoneyPuck data in DB**: Team + goalie game-by-game stats through 2026-03-13 (season 2025). Goalie GSAx, rolling xG%, Corsi%, etc. all queryable via `store.Querier`.
- **Persistence**: `UpsertModelPrediction` query in `sql/queries/predictions.sql` already exists.
- **Team name mapping**: `moneypuck.TeamMap` converts Odds API names ↔ MoneyPuck abbreviations. Already used in backtest.
- **Execution layer**: Paper adapter, placement orchestrator, idempotency, settlement — all built in `internal/execution/`.

## The Gap

`model_predictions` is only populated by the offline backtest CLI. The server has no path to run the NHL model at serve time or on a schedule. When `GET /recommendations` scans `model_predictions` for upcoming games, it finds nothing.

## What to Build

### 1. Create `internal/prediction/nhl.go` — NHLPredictionService

A service that takes a game and produces a persisted prediction:

```go
type NHLPredictionService struct {
    pool         *pgxpool.Pool
    nhlModel     nhl.XGGoalieModel
    rollingWindow int  // use 40 (best backtest Brier)
}

func (s *NHLPredictionService) PredictGame(ctx, gameID int64, homeTeam, awayTeam string, gameDate time.Time, season int32, marketHomeProb float64) (float64, error)
```

Internally:
1. Create `store.New(s.pool)` as `MoneyPuckStore` (the `store.Querier` already satisfies the `MoneyPuckStore` interface — verify this by checking that `GetTeamRolling5on5Stats`, `GetStartingGoalie`, `GetGoalieSeasonGSAx`, `FindMoneypuckGameID` are all on `Querier`).
2. Call `backtest.BuildNHLFeatures(ctx, mpStore, homeTeam, awayTeam, gameDate, season, marketHomeProb, 40)` — import from backtest package directly (avoids large refactor).
3. Build `nhl.MatchupInput` from the result (same pattern as `engine.go:526-543` and `engine.go:787-804`).
4. Call `s.nhlModel.Predict(input)` → get `HomeWinProbability`.
5. Persist via `UpsertModelPrediction` with `source="live"`, `model_family="xg-goalie-quality"`.
6. Return the predicted probability.

### 2. Create `internal/prediction/service.go` — Batch Prediction Runner

A function that scans upcoming games and runs predictions for all that need them:

```go
func (s *NHLPredictionService) PredictUpcomingGames(ctx context.Context) (int, error)
```

1. Query `games` table for NHL games in the next 48 hours (`commence_time > NOW() AND commence_time < NOW() + interval '48 hours' AND sport = 'icehockey_nhl'`). You'll need a new sqlc query for this.
2. For each game, check if `model_predictions` already has a recent row (within last 15 min). Skip if so.
3. Get the latest market implied probability from `odds_history` for this game.
4. Call `PredictGame()`.
5. Return count of predictions made.

### 3. Create River Job (`internal/worker/prediction_job.go`)

Follow the existing River job pattern (see `internal/worker/mlb_stats_etl.go` for the canonical example):

```go
type PredictionJobArgs struct {
    Sport string `json:"sport"`
}
func (PredictionJobArgs) Kind() string { return "prediction" }
func (PredictionJobArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{
        UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Minute},
    }
}
```

The worker calls `NHLPredictionService.PredictUpcomingGames()`. Register in `internal/worker/worker.go`. Schedule to run every 15 minutes (or after each odds poll).

### 4. New SQL Queries

Add to `sql/queries/`:
- `ListUpcomingGamesForSport :many` — games where `commence_time` is in the next 48 hours for a given sport
- `GetLatestMarketProbabilityForGame :one` — most recent implied home probability from `odds_history` for h2h market

Run `sqlc generate -f sql/sqlc.yaml` after.

### 5. Wire into Server

Add `NHLPredictionService` to `App` struct in `internal/server/server.go`. Optionally add a `POST /predictions/run` endpoint that triggers prediction manually (useful for testing).

### 6. Validate

```bash
# Ensure odds exist for today's games (may need to poll first)
BETBOT_DATABASE_URL="postgres://betbot:betbot-dev-password@localhost:25432/betbot?sslmode=disable"

# Run predictions manually
go run cmd/server/main.go  # start server
curl http://127.0.0.1:18080/predictions/run  # trigger predictions
curl http://127.0.0.1:18080/recommendations?sport=NHL  # should now return candidates
```

## Key Decisions Already Made

- **Rolling window = 40** (best Brier 0.2427)
- **Import from backtest package directly** (avoid large refactor; `BuildNHLFeatures` stays in `internal/backtest`)
- **River job for async prediction** (keeps API fast, pre-computes results)
- **Paper mode only** for now (no real bet placement)
- **NHL only** for this bridge (other sports follow the same pattern later)

## Critical Files to Read First

1. `internal/backtest/nhlfeatures.go` — `BuildNHLFeatures`, `MoneyPuckStore` interface, `NHLFeatureResult`
2. `internal/backtest/engine.go:526-543` — how `MatchupInput` is built from `NHLFeatureResult`
3. `internal/modeling/nhl/predictor.go` — `XGGoalieModel.Predict()`, `TeamProfile` struct (includes `CorsiShare`)
4. `internal/server/recommendations.go:97-175` — `recommendationCandidates()` that reads `model_predictions` + `odds_history`
5. `sql/queries/predictions.sql` — `UpsertModelPrediction` query
6. `internal/worker/mlb_stats_etl.go` — canonical River job pattern to follow
7. `internal/store/querier.go` — verify MoneyPuck query methods are on the `Querier` interface

## DB Connection

Local Postgres is on port 25432 (not 5432):
```
BETBOT_DATABASE_URL="postgres://betbot:betbot-dev-password@localhost:25432/betbot?sslmode=disable"
```
