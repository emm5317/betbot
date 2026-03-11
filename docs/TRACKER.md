# TRACKER.md — betbot Progress Tracker

> Jira-style task tracking organized by implementation phase.
> Status: `⬜ TODO` · `🔵 IN PROGRESS` · `✅ DONE` · `🔴 BLOCKED` · `⏸️ DEFERRED`

**Last updated:** 2026-03-10
**Current phase:** Phase 1 — Data Foundation

---

## Current Repo State

This section tracks what exists in the repository now so the task table stays grounded in actual shipped artifacts.

- Project scaffold and directory layout are present
- Local Docker Compose stack exists for `betbot` and `betbot-postgres`
- Minimal HTTP server exists with `GET /health`
- Local health check is currently published on host port `18080`
- Remaining gaps: Fiber integration, pgxpool-backed data access, migrations, River workers, sqlc-generated store, dashboard, and model pipeline

---

## Phase 1: Data Foundation — Weeks 1–4

> **Goal:** Stand up the data pipeline and begin accumulating odds history. You can't improve what you can't measure.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P1-001 | Initialize Go module and project structure (`cmd/`, `internal/`, `migrations/`, `sql/`) | ✅ DONE | P0 | Scaffold exists; keep structure aligned with docs |
| P1-002 | Set up PostgreSQL 17 database and connection pooling (pgx v5) | 🔵 IN PROGRESS | P0 | Local Docker Postgres exists; pgxpool still needed |
| P1-003 | Write migration: `games` table | ⬜ TODO | P0 | UUID PK, sport, teams, start_time, status, result |
| P1-004 | Write migration: `odds_history` table (partitioned by month) | ⬜ TODO | P0 | Append-only; raw_json JSONB; snapshot_hash for dedup |
| P1-005 | Write migration: `bankroll_ledger` table | ⬜ TODO | P0 | Explicit ledger; event_type enum; balance_after |
| P1-006 | Write migration: `bets` table | ⬜ TODO | P1 | State machine: pending/placed/won/lost/push/cancelled |
| P1-007 | Write migration: `clv_log` table | ⬜ TODO | P1 | FK to bets; bet_implied, closing_implied, clv_pct |
| P1-008 | Write migration: `model_predictions` table | ⬜ TODO | P1 | Versioned; features_json for reproducibility |
| P1-009 | Configure sqlc and write initial query definitions | ⬜ TODO | P0 | `sql/` directory; generate `internal/store/` |
| P1-010 | Set up River job queue infrastructure | ⬜ TODO | P0 | River client, worker process in `cmd/worker/` |
| P1-011 | Implement The Odds API client (HTTP, rate-limited) | ⬜ TODO | P0 | Token bucket; configurable poll interval |
| P1-012 | Implement `OddsPollJob` River worker | ⬜ TODO | P0 | Fetch → store raw JSON → normalize → dedup → insert |
| P1-013 | Implement odds snapshot deduplication (hash-based) | ⬜ TODO | P1 | Hash on game_id+book+market+outcome+odds |
| P1-014 | Write `games` upsert logic (create games from odds data) | ⬜ TODO | P0 | Auto-create game records from first odds sighting |
| P1-015 | Implement `CLVCaptureJob` (archive closing odds at game start) | ⬜ TODO | P1 | Run at game start_time; capture last odds per bet |
| P1-016 | Implement `SettlementJob` (reconcile game results → settle bets) | ⬜ TODO | P1 | Update bet status; write bankroll_ledger entries |
| P1-017 | Set up Fiber v3 HTTP server (`cmd/server/`) | 🔵 IN PROGRESS | P1 | Minimal health endpoint exists; Fiber dashboard work remains |
| P1-018 | Build basic HTMX dashboard: live odds board | ⬜ TODO | P2 | Display current best-of-market odds per game |
| P1-019 | Build basic HTMX dashboard: pipeline health view | ⬜ TODO | P2 | Last poll time, rows inserted, error count |
| P1-020 | Write unit tests for odds normalization and implied prob calc | ⬜ TODO | P0 | American → decimal → implied; both directions |
| P1-021 | Write integration tests for odds_history insert/dedup | ⬜ TODO | P1 | Test with duplicate snapshots |
| P1-022 | Implement `PartitionMaintenanceJob` (auto-create monthly partitions) | ⬜ TODO | P2 | Runs monthly; creates next month's partition |
| P1-023 | Configure zerolog structured logging across all components | ⬜ TODO | P1 | JSON output; microsecond timestamps |
| P1-024 | Set up environment variable config with `BETBOT_` prefix | 🔵 IN PROGRESS | P0 | Baseline config exists; full tunables still pending |
| P1-025 | Seed initial bankroll via manual deposit ledger entry | ⬜ TODO | P2 | CLI command or admin endpoint |

**Phase 1 exit criteria:**
- [ ] Odds data is being polled and stored continuously
- [ ] odds_history has 7+ days of accumulated data
- [ ] Dashboard shows live odds board
- [ ] CLV capture job is functional (tested with mock data)
- [ ] All P0 tests pass

---

## Phase 2: Modeling & Backtesting — Weeks 5–8

> **Goal:** Build a baseline probability model and validate it against historical data before risking capital.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P2-001 | Implement Elo rating system in Go (gonum) | ⬜ TODO | P0 | Margin-adjusted K-factor; home-field offset |
| P2-002 | Implement Elo → win probability conversion | ⬜ TODO | P0 | `1 / (1 + 10^((elo_B - elo_A + HFA) / 400))` |
| P2-003 | Build feature engineering pipeline | ⬜ TODO | P0 | Team quality, situational, injury, market signals |
| P2-004 | Write migration: `team_ratings` table (Elo history) | ⬜ TODO | P1 | Track rating changes per game for debugging |
| P2-005 | Implement `ModelRunJob` River worker | ⬜ TODO | P0 | Triggered by odds changes; emits model_predictions |
| P2-006 | Implement EV calculation: model_prob vs implied_prob | ⬜ TODO | P0 | `edge = model_prob - implied_prob` |
| P2-007 | Build backtesting CLI (`cmd/backtest/`) | ⬜ TODO | P0 | Walk-forward replay against odds_history |
| P2-008 | Implement virtual bankroll in backtester | ⬜ TODO | P0 | Same state machine as live; tracks PnL |
| P2-009 | Backtest output: cumulative PnL curve (CSV) | ⬜ TODO | P0 | Time-series of bankroll value |
| P2-010 | Backtest output: CLV distribution | ⬜ TODO | P0 | Mean, median, stddev of CLV across bets |
| P2-011 | Backtest output: calibration table (predicted vs actual by decile) | ⬜ TODO | P1 | Key validation: does 60% predicted = ~60% actual? |
| P2-012 | Backtest output: Sharpe-equivalent ratio | ⬜ TODO | P1 | `mean(returns) / std(returns)` |
| P2-013 | Backtest output: max drawdown and duration | ⬜ TODO | P1 | Peak-to-trough; time to recovery |
| P2-014 | Implement walk-forward cross-validation in backtester | ⬜ TODO | P0 | Train on weeks 1–N, predict N+1, roll forward |
| P2-015 | Historical data import: backfill odds_history from SBR or archive | ⬜ TODO | P1 | Need 1+ season of historical odds for meaningful backtest |
| P2-016 | Historical data import: game results for backtested seasons | ⬜ TODO | P1 | Scores and outcomes for settlement simulation |
| P2-017 | Implement look-ahead bias guard in backtester | ⬜ TODO | P0 | Verify no future data leaks into features |
| P2-018 | Write unit tests for Elo update logic | ⬜ TODO | P0 | Known input → expected output; margin multiplier |
| P2-019 | Write unit tests for EV calculation | ⬜ TODO | P0 | Edge positive/negative/zero scenarios |
| P2-020 | Run initial backtest and document results | ⬜ TODO | P0 | Record CLV, calibration, Sharpe; decision point |

**Phase 2 exit criteria:**
- [ ] Elo model produces calibrated probabilities (verified on holdout)
- [ ] Backtester runs against 1+ season of historical data
- [ ] CLV is positive in backtest (model beats closing line)
- [ ] Calibration table shows reasonable alignment
- [ ] Results documented; go/no-go decision for Phase 3

---

## Phase 3: Decision Engine — Weeks 9–11

> **Goal:** Build the risk management layer that converts model outputs into sized, risk-checked bet tickets.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P3-001 | Implement Kelly Criterion sizer (fractional) | ⬜ TODO | P0 | `f = k * (b*p - q) / b`; k = BETBOT_KELLY_FRACTION |
| P3-002 | Implement max single-bet cap | ⬜ TODO | P0 | Hard limit: BETBOT_MAX_BET_FRACTION of bankroll |
| P3-003 | Implement EV threshold filter | ⬜ TODO | P0 | Reject if edge < BETBOT_EV_THRESHOLD |
| P3-004 | Implement line shopping: select best odds across books | ⬜ TODO | P0 | Query latest odds_history per book for target game |
| P3-005 | Implement correlation guard | ⬜ TODO | P0 | Detect same game_id in pending/placed bets |
| P3-006 | Implement aggregate exposure limit per game | ⬜ TODO | P0 | Reject if total exposure on game > 5% bankroll |
| P3-007 | Implement bankroll balance check (available, not pending) | ⬜ TODO | P0 | Available = total - sum(pending + placed stakes) |
| P3-008 | Implement daily loss stop | ⬜ TODO | P0 | Sum losses today; halt if > BETBOT_DAILY_LOSS_STOP |
| P3-009 | Implement weekly loss stop | ⬜ TODO | P0 | Sum losses this week; halt until manual review |
| P3-010 | Implement drawdown circuit breaker | ⬜ TODO | P0 | Track peak bankroll; halt if drawdown > threshold |
| P3-011 | Implement `EVScreenJob` River worker | ⬜ TODO | P0 | Runs after ModelRunJob; screens for +EV → emits tickets |
| P3-012 | Build BetTicket domain type with idempotency key generation | ⬜ TODO | P0 | `game_id + market + book + timestamp_bucket` |
| P3-013 | Dashboard: bankroll chart (cumulative PnL with drawdown) | ⬜ TODO | P1 | HTMX + Chart.js or D3 |
| P3-014 | Dashboard: pending bets view | ⬜ TODO | P1 | Status, odds, stake, estimated PnL |
| P3-015 | Dashboard: CLV tracker (rolling average) | ⬜ TODO | P1 | Rolling 50/100/500 bet CLV with CI |
| P3-016 | Write unit tests for Kelly sizer (edge cases: negative edge, zero odds) | ⬜ TODO | P0 | |
| P3-017 | Write unit tests for correlation guard | ⬜ TODO | P0 | Same-game, different-game, mixed scenarios |
| P3-018 | Write unit tests for circuit breakers | ⬜ TODO | P0 | Trigger at exact threshold; verify halt behavior |
| P3-019 | Write integration test: full pipeline model → decision → ticket | ⬜ TODO | P1 | End-to-end with test database |

**Phase 3 exit criteria:**
- [ ] Decision engine produces correctly sized bet tickets
- [ ] All circuit breakers tested and functional
- [ ] Correlation guard prevents over-exposure on same game
- [ ] Dashboard shows bankroll, bets, and CLV
- [ ] All P0 tests pass

---

## Phase 4: Execution — Weeks 12–14

> **Goal:** Build the book adapters and placement infrastructure with exactly-once semantics.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P4-001 | Define `BookAdapter` interface in Go | ⬜ TODO | P0 | PlaceBet, CheckStatus, GetBalance, Cancel |
| P4-002 | Implement first book adapter (Pinnacle or primary target) | ⬜ TODO | P0 | Auth, bet slip construction, response parsing |
| P4-003 | Implement idempotency key check before placement | ⬜ TODO | P0 | SELECT before INSERT; skip if already processed |
| P4-004 | Implement distributed lock (Postgres advisory lock) | ⬜ TODO | P0 | Lock on idempotency_key; prevent concurrent placement |
| P4-005 | Implement read-back verification after placement | ⬜ TODO | P0 | CheckStatus to confirm bet was actually placed |
| P4-006 | Implement placement audit logging (full request/response) | ⬜ TODO | P0 | Store in bets.audit_json; redact sensitive fields |
| P4-007 | Implement retry with exponential backoff (transient failures) | ⬜ TODO | P1 | 1s, 4s, 16s; max 3 attempts |
| P4-008 | Implement `pending_verification` state for timeout scenarios | ⬜ TODO | P1 | Schedule VerifyBetJob after 60s |
| P4-009 | Implement `PlacementJob` River worker | ⬜ TODO | P0 | Receives BetTicket; executes via adapter |
| P4-010 | Implement paper trading mode (`BETBOT_PAPER_MODE=true`) | ⬜ TODO | P0 | Simulated placement; all other logic real |
| P4-011 | Run full pipeline in paper mode for 1+ week | ⬜ TODO | P0 | Validate: data → model → decision → paper placement |
| P4-012 | Implement second book adapter (DraftKings or FanDuel) | ⬜ TODO | P2 | Expand coverage for line shopping |
| P4-013 | Write unit tests for idempotency (double-submit produces one bet) | ⬜ TODO | P0 | |
| P4-014 | Write integration test: placement → verification → settlement | ⬜ TODO | P1 | Full lifecycle with mock adapter |
| P4-015 | Write integration test: retry behavior on transient failure | ⬜ TODO | P1 | Mock adapter returns 5xx then success |

**Phase 4 exit criteria:**
- [ ] Paper trading running end-to-end for 7+ days
- [ ] Zero double-placement bugs in paper mode
- [ ] Audit log captures all placement attempts
- [ ] Idempotency proven under concurrent load test
- [ ] Settlement reconciliation matches expected outcomes

---

## Phase 5: Live Deployment — Weeks 15–16

> **Goal:** Deploy with minimal capital, validate edge with real money over a statistically meaningful sample.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P5-001 | Fund bankroll with minimal validation capital | ⬜ TODO | P0 | Amount TBD; enough for 100+ bets at target stake |
| P5-002 | Switch `BETBOT_PAPER_MODE` to `false` | ⬜ TODO | P0 | The moment of truth |
| P5-003 | Monitor CLV daily for first 2 weeks | ⬜ TODO | P0 | Positive CLV = model has live edge |
| P5-004 | Monitor calibration: predicted vs actual win rate | ⬜ TODO | P0 | Flag if divergence exceeds 5pp by decile |
| P5-005 | Monitor bankroll PnL vs backtest expectations | ⬜ TODO | P0 | Should track within 2σ of backtested range |
| P5-006 | Verify circuit breakers function under real conditions | ⬜ TODO | P0 | Intentionally test daily loss stop |
| P5-007 | Document live results: first 50 bets | ⬜ TODO | P1 | CLV, win rate, PnL, any anomalies |
| P5-008 | Document live results: first 100 bets | ⬜ TODO | P1 | Statistical significance check |
| P5-009 | Gradual bankroll increase (if CLV confirms edge) | ⬜ TODO | P1 | Scale after 100+ bet validation |
| P5-010 | Add additional book adapters for live execution | ⬜ TODO | P2 | Expand line shopping coverage |
| P5-011 | Set up automated backup: daily pg_dump to object store | ⬜ TODO | P1 | Backblaze B2 or equivalent |
| P5-012 | Set up monitoring alerts (pipeline failure, placement failure) | ⬜ TODO | P1 | Email or Slack webhook |

**Phase 5 exit criteria:**
- [ ] 100+ live bets placed and settled
- [ ] CLV is positive over the sample
- [ ] Calibration holds within acceptable tolerance
- [ ] No double-placement or financial integrity bugs
- [ ] Circuit breakers triggered correctly (if applicable)
- [ ] Go/no-go decision for bankroll scaling

---

## Phase 6: Advanced Models — Ongoing

> **Goal:** Iterate on model sophistication, expand sports coverage, and explore new market types.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P6-001 | Set up Python ML sidecar with gRPC | ⬜ TODO | P1 | Docker container; proto definitions |
| P6-002 | Implement XGBoost/LightGBM ensemble model | ⬜ TODO | P1 | Feature matrix → calibrated probability |
| P6-003 | Implement Platt scaling / isotonic regression for calibration | ⬜ TODO | P1 | Post-processing on ML model outputs |
| P6-004 | Implement Bayesian updating (market prior + situational signals) | ⬜ TODO | P2 | Start from closing line; adjust for injuries/weather |
| P6-005 | Build player prop modeling pipeline | ⬜ TODO | P2 | Less efficient market; potentially higher edge |
| P6-006 | Implement steam move detection | ⬜ TODO | P2 | Rapid line movement → alert or auto-follow |
| P6-007 | Implement reverse line movement (RLM) detection | ⬜ TODO | P2 | Public % vs line direction divergence |
| P6-008 | Add NFL stats ETL (`NFLStatsETLJob`) | ⬜ TODO | P1 | nflverse data; EPA, CPOE, DVOA proxies |
| P6-009 | Add NBA stats ETL (`NBAStatsETLJob`) | ⬜ TODO | P2 | stats.nba.com; net rating, tracking data |
| P6-010 | Add MLB stats ETL (`MLBStatsETLJob`) | ⬜ TODO | P2 | Baseball Savant; Statcast |
| P6-011 | Implement injury impact weighting (WAR/usage-based) | ⬜ TODO | P2 | Injury scraper → quantified impact on model |
| P6-012 | Implement weather integration for outdoor sport models | ⬜ TODO | P2 | OpenWeatherMap API → situational_factors |
| P6-013 | Build arbitrage detection engine | ⬜ TODO | P3 | Cross-book spread divergence → alert |
| P6-014 | Implement A/B model comparison framework | ⬜ TODO | P2 | Run multiple models simultaneously; compare CLV |
| P6-015 | Build model performance monitoring dashboard | ⬜ TODO | P2 | Calibration curves, CLV by model, feature importance |
| P6-016 | Implement account longevity strategies (bet timing randomization) | ⬜ TODO | P3 | Jitter placement timing; vary stake rounding |
| P6-017 | Multi-sport expansion: second sport go-live | ⬜ TODO | P2 | Full pipeline for sport #2 |
| P6-018 | Evaluate OddsJam integration for sharp book coverage | ⬜ TODO | P3 | Pinnacle, Circa, BetOnline lines |

**Phase 6 is ongoing — no fixed exit criteria. Prioritize by expected CLV impact.**

---

## Backlog / Future Ideas

| ID | Idea | Priority | Notes |
|----|------|----------|-------|
| BL-001 | Mobile alert app (bet placed, circuit breaker, CLV update) | P3 | Push notifications |
| BL-002 | Automated tax reporting from bankroll_ledger | P3 | Annual P&L summary |
| BL-003 | Multi-user support (multiple operators, separate bankrolls) | P3 | Not needed for solo operation |
| BL-004 | Historical model performance report generator | P2 | Monthly PDF/HTML reports |
| BL-005 | Line movement visualization (odds over time per game) | P2 | Dashboard chart |
| BL-006 | Correlated parlay optimizer (when correlation is modeled) | P3 | Only if correlation model is validated |
| BL-007 | Live in-game betting model | P3 | Requires sub-second data; different architecture |
| BL-008 | Prediction market expansion (PredictIt, Polymarket) | P3 | Same framework, different data sources |

---

## How to Update This File

When completing a task:
1. Change status from `⬜ TODO` to `✅ DONE`
2. Add completion date in Notes column: `Done 2026-03-15`
3. If a task is blocked, change to `🔴 BLOCKED` and note the blocker
4. If deferring, change to `⏸️ DEFERRED` and note the reason
5. Update "Last updated" date at top of file
6. Update "Current phase" if advancing to next phase
