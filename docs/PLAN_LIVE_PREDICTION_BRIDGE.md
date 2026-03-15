# Plan: Live Prediction Bridge

## Problem

The NHL model, feature pipeline, and decision engine all exist and work in the backtest CLI — but the live server can't produce predictions. `GET /recommendations` reads from the `model_predictions` table, which is only populated offline by the backtest pipeline. There is no path from "today's odds arrive" → "model runs" → "prediction persisted" → "recommendation served."

## Current Data Flow (Broken)

```
[The Odds API] → OddsPoller → odds_history     ──┐
                                                   ├─→ GET /recommendations → (empty, no predictions)
[Backtest CLI] → model_predictions (offline)   ──┘
```

`GET /recommendations` joins `model_predictions` (keyed by `game_id + market_key`) with `odds_history` (latest odds per game/book). If no prediction row exists for a game, that game produces zero candidates.

## Target Data Flow

```
[The Odds API] → OddsPoller → odds_history + games
                                       │
                          ┌─────────────┘
                          ▼
              LivePredictionService
              ├─ reads latest odds from odds_history
              ├─ maps game → team abbreviations (via games table)
              ├─ queries MoneyPuck rolling stats (current season)
              ├─ runs XGGoalieModel.Predict()
              ├─ persists to model_predictions (UpsertModelPrediction)
              └─────────────────────┐
                                    ▼
                          GET /recommendations → ranked candidates → JSON
```

## Prerequisites

### 1. Current-Season MoneyPuck Data — SATISFIED

The DB already has current-season data:

| Season | Team rows | Goalie rows | Latest game |
|--------|-----------|-------------|-------------|
| 2024 (2024-25) | 5,592 | 5,528 | 2025-06-17 |
| 2025 (2025-26) | 4,168 | 4,392 | 2026-03-13 |

Data is 2 days old (as of 2026-03-15). The team and goalie game-by-game stats needed by the model's rolling feature builder are present. Additional skaters/lines CSVs are available at `data/moneypuck/2024/` and `data/moneypuck/2025/` but are not required for the current model (it uses team-level aggregates + goalie GSAx, not individual skater stats).

**Ongoing concern:** MoneyPuck data needs periodic re-import to stay current. A future enhancement could automate this via a scheduled scrape or CSV download job.

### 2. Team Name Mapping: Odds API → MoneyPuck

The Odds API uses full names ("Tampa Bay Lightning"), MoneyPuck uses abbreviations ("T.B"). The existing `moneypuck.TeamMap` handles this mapping and is already used by `BuildNHLFeatures` in the backtest pipeline. The `games` table stores `home_team` / `away_team` using the Odds API full names. This mapping is available today.

## Implementation Steps

### Step 1: Extract Prediction Service (`internal/prediction/nhl.go`)

Create a new `internal/prediction` package with a `NHLPredictionService` that:
- Takes a `*pgxpool.Pool` (for MoneyPuck queries + model prediction persistence)
- Holds a `nhl.XGGoalieModel` instance
- Exposes `PredictGame(ctx, gameID, homeTeam, awayTeam, gameDate, season, openingHomeProb) → (predictedHomeProb, error)`

Internally:
1. Calls `BuildNHLFeatures(ctx, mpStore, homeTeam, awayTeam, gameDate, season, openingHomeProb, rollingWindow)` — this function already exists in `internal/backtest/nhlfeatures.go` but is coupled to the backtest package.
2. Runs `nhlModel.Predict(matchupInput)` to get `HomeWinProbability`.
3. Returns the prediction.

**Key refactor:** `BuildNHLFeatures` and its helpers (`computeRollingAverages`, `MoneyPuckStore` interface, `NHLFeatureResult`) currently live in `internal/backtest/nhlfeatures.go`. They need to be **extracted to a shared package** (e.g., `internal/prediction/nhlfeatures.go` or `internal/modeling/nhl/features.go`) so both the backtest CLI and the live server can use them. The backtest package would then import from the shared location.

**Alternative (less refactoring):** Keep the feature functions in `backtest` and have the prediction service import from `backtest`. This couples the live server to the backtest package, which is not ideal but works and avoids a large refactor.

### Step 2: Prediction Trigger — River Job or Request-Time

Two options for when predictions run:

**Option A: Prediction Job (recommended)**
Create a `PredictionJob` in `internal/worker/` that:
- Triggers after each `OddsPollJob` completes (or on a separate schedule, e.g., every 15 minutes)
- Scans `games` table for upcoming games (next 24-48 hours) that don't yet have a `model_predictions` row
- For each game: runs `NHLPredictionService.PredictGame`, persists via `UpsertModelPrediction`
- Uses `river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Minute}` to prevent duplicate runs

**Option B: Request-Time Prediction**
When `GET /recommendations` is called, before assembling candidates:
- Query upcoming games from `odds_history` that lack `model_predictions` rows
- Run predictions inline and persist them
- Then proceed with the existing `recommendationCandidates` flow

**Recommendation:** Option A. Separating prediction from serving keeps the API fast and allows predictions to be pre-computed. The `GET /recommendations` endpoint then reads from a warm cache of predictions.

### Step 3: Wire Prediction Job into Worker

In `internal/worker/worker.go`:
- Register `PredictionJobArgs` and `PredictionWorker`
- The worker calls `NHLPredictionService.PredictGame` for each game
- Schedule prediction runs after odds polling completes, or on a fixed interval

```go
type PredictionJobArgs struct {
    Sport  string `json:"sport"`
    GameID int64  `json:"game_id"`
}
func (PredictionJobArgs) Kind() string { return "prediction" }
func (PredictionJobArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{
        UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Minute},
    }
}
```

### Step 4: Game-to-Team Resolution

The prediction job needs to know which teams are playing. The `games` table stores `home_team` and `away_team` (Odds API names). The prediction service needs:
- A query: `GetGameByID(ctx, gameID) → (homeTeam, awayTeam, commenceTime, sport)` — this may already exist or needs a simple sqlc query.
- The `moneypuck.TeamMap` to convert Odds API names to MoneyPuck abbreviations (already exists in `internal/ingestion/moneypuck`).

### Step 5: Persist Predictions

The `UpsertModelPrediction` query already exists in `sql/queries/predictions.sql`. The prediction service calls it with:
```
source:               "live"
sport:                "NHL"
book_key:             best available book from odds
market_key:           "h2h"
model_family:         "xg-goalie-quality"
model_version:        "v2" (post-retune with corsi+offense)
predicted_probability: model output
market_probability:    implied from opening odds
feature_vector:        encoded via manifest
```

### Step 6: Validate End-to-End

1. Import current-season MoneyPuck data
2. Ensure odds_history has today's NHL games (via polling or manual insert)
3. Trigger prediction job (or call service directly)
4. Call `GET /recommendations?sport=NHL` and verify predictions appear

## File Changes Summary

| Action | File | Description |
|--------|------|-------------|
| **Create** | `internal/prediction/nhl.go` | `NHLPredictionService` — feature build + model predict + persist |
| **Create** | `internal/prediction/nhl_test.go` | Unit tests for prediction service |
| **Refactor** | `internal/backtest/nhlfeatures.go` | Extract `BuildNHLFeatures` + `MoneyPuckStore` interface to shared location (or import from backtest) |
| **Create** | `internal/worker/prediction_job.go` | River job that scans upcoming games and runs predictions |
| **Modify** | `internal/worker/worker.go` | Register prediction job worker |
| **Create** | `sql/queries/games_upcoming.sql` | Query: list upcoming games without predictions |
| **Modify** | `internal/server/server.go` | Add `NHLPredictionService` to App (for optional request-time prediction) |
| **Run** | `sqlc generate` | Regenerate store after new queries |

## Estimated Effort

- Step 1 (prediction service): ~1 hour — straightforward extraction and wiring
- Step 2-3 (River job): ~30 min — follows existing job patterns exactly
- Step 4 (game resolution): ~15 min — likely just a new sqlc query
- Step 5 (persist): ~15 min — `UpsertModelPrediction` already exists
- Step 6 (validation): ~30 min — import data + end-to-end test

**Total: ~2.5 hours of implementation**, assuming MoneyPuck 2024-25 data is available.

## Open Questions

1. ~~**Is 2024-25 MoneyPuck data available?**~~ **RESOLVED** — Data through 2026-03-13 is in the DB (both seasons 2024 and 2025).

2. **Rolling window size:** The backtest shows window=40 produces the best Brier score (0.2427). Should the live service use 40-game windows? This means early-season games (< 20 games played) fall back to defaults.

3. **Prediction frequency:** How often should predictions refresh? Every odds poll (5 min)? Every 15 min? Once per game at a fixed pre-game window (e.g., 2 hours before)?

4. **Multi-sport:** This plan covers NHL only. MLB/NBA/NFL would need equivalent prediction services. The architecture is the same — only the feature builder and model differ per sport.

5. **MoneyPuck data freshness:** Current data is 2 days old. Need a strategy for keeping it current — manual re-import before each betting session, or automated scrape.
