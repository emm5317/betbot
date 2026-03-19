# betbot

Open-source sports betting infrastructure for `MLB`, `NBA`, `NHL`, and `NFL`.

betbot is a sports betting bot and sports betting analytics platform built for developers who care about `odds tracking`, `closing line value (CLV)`, `expected value (EV)`, `backtesting`, `line shopping`, and `risk-managed bet automation`. The system is being built in Go with PostgreSQL, River, Fiber, and sqlc, with a clear bias toward measurable edge instead of hype.

If you are looking for a `sports betting tracker`, `odds history database`, `CLV tracker`, `sports betting backtesting engine`, `Kelly Criterion bankroll manager`, or a serious open-source foundation for `MLB betting models`, `NBA betting models`, `NHL betting models`, and `NFL betting models`, this repository is aimed at that problem space.

## Why betbot

Most betting projects stop at picks, parlays, or dashboards. betbot is being built around a harder question:

How do you create a reproducible, measurable, open-source sports betting system that can:

- ingest and archive odds continuously
- compare model probability against market probability
- measure edge with CLV instead of short-term win rate
- backtest strategies against historical odds data
- apply bankroll controls before any execution path exists
- specialize deeply in the four highest-value North American leagues

The design principle is simple:

`measurement before modeling, modeling before execution`

## Supported Sports

betbot is intentionally specialized for:

- `MLB`
- `NBA`
- `NHL`
- `NFL`

This is not a generic all-sports repo. The architecture is reusable, but the product direction is focused on the leagues with the best mix of market depth, public data, and year-round operational value.

## Core Concepts

- `Odds ingestion`: poll sportsbooks and aggregators, normalize market data, and store a replayable odds history
- `CLV tracking`: use closing line value as the primary truth signal for whether the process has real edge
- `Expected value screening`: compare model probability to implied market probability
- `Line shopping`: identify the best available number across books
- `Backtesting`: replay historical odds and evaluate strategy quality before risking capital
- `Bankroll management`: apply Kelly-style sizing, exposure controls, and drawdown discipline
- `Sport-specific specialization`: separate data, features, and models for MLB, NBA, NHL, and NFL

## Tech Stack

- `Go 1.25`
- `PostgreSQL 17` target baseline
- `pgxpool` for database pooling
- `sqlc` for typed SQL access
- `River` for background jobs and scheduling
- `Fiber v3` for HTTP and operational views
- `HTMX` and server-rendered templates for the UI surface
- Python sidecar planned later for heavier ML workloads

## Current Status

The repository is still early-stage. What exists today:

- Go module and project scaffold
- local Docker Compose runtime
- Fiber operational server, River worker wiring, and PostgreSQL-backed Phase 1 slice
- sqlc-generated store layer backing the current app code
- Postgres-backed integration tests for dedup behavior and Phase 1 boot smoke
- live `SportConfig` registry for MLB/NBA/NHL/NFL
- worker scheduling that filters odds polling to sports active in the current season
- documentation set aligned to the four-sport roadmap
- manifest-backed `model_predictions` persistence with stable feature indexing
- end-to-end backtest CLI replay that consumes stored odds snapshots, persists predictions, and emits deterministic artifacts
- walk-forward validation plus unified CLV/calibration reporting in one pipeline output
- NHL xG + goalie and NFL EPA/DVOA situational baseline model packages with unit test coverage
- sport-specific Kelly baseline defaults (MLB/NBA/NHL/NFL) wired into replay stake recommendations and shared decision sizing
- decision-engine EV threshold filter with sport-aware defaults and pass/fail gating for candidate evaluation
- deterministic recommendation stake sizing with odds-aware Kelly math, hard cap enforcement, and ledger-backed bankroll availability gating
- recommendation-only pull and monitoring API surface:
  - `GET /recommendations`
  - `GET /recommendations/performance`
  - `GET /recommendations/calibration`
  - `GET /recommendations/calibration/alerts`
  - `GET /recommendations/calibration/alerts/history`
- append-only recommendation calibration alert run persistence with deterministic rolling trend windows
- execution layer foundations for paper mode:
  - `POST /execution/place`
  - `GET /execution/bets`
  - idempotent placement orchestration, audit trail persistence, and settlement/CLV capture
- automated paper workers for recommendation auto-placement and auto-settlement
- execution runtime now uses explicit adapter selection via `BETBOT_EXECUTION_ADAPTER`; `BETBOT_PAPER_MODE=false` requires a non-paper adapter and defaults `BETBOT_AUTO_PLACEMENT_ENABLED=false` for conservative live rollout

What is built now:

- PostgreSQL 17 baseline
- `pgxpool` and `sqlc`
- River-backed odds polling
- append-only `odds_history`
- deduplicated market snapshots
- minimal Fiber operational views for health and current odds
- sport-aware registry and active-season polling policy
- recommendation calibration/drift observability with append-only historical alert runs
- paper-mode execution loop (recommendation -> placement -> settlement) with exactly-once controls

The current open build step is Phase 5 sustained paper-mode validation (`P5-006`): runbook validation, monitoring review cadence, and threshold tuning under paper traffic.

Recommendation mode is available through `GET /recommendations` for ranked bet suggestions. Live real-money execution is still deferred; paper-mode placement and settlement automation are active.

Recommendation performance monitoring is now available through `GET /recommendations/performance` for CLV/outcome audit rows plus aggregate operator summary metrics.

## Roadmap

### Phase 1: Data Foundation Vertical Slice

- Postgres 17 runtime baseline
- `games`, `odds_history`, and `bankroll_ledger`
- The Odds API client
- River-backed `OddsPollJob`
- odds normalization and deduplication
- minimal Fiber health and odds views

### Phase 2: Sport Foundation

- `SportConfig` registry
- sport-aware scheduling
- sport-specific stat tables
- lineup, injury, and weather ingestion
- MLB/NBA/NHL/NFL ETL workers

### Phase 3: Baseline Models and Backtesting

- MLB pitcher matchup model
- NBA lineup-adjusted rating model
- NHL xG plus goalie model
- NFL EPA/DVOA situational model
- walk-forward replay engine
- calibration and CLV reporting
- sport-specific Kelly baseline defaults

### Phase 4: Decision Engine

- EV thresholding
- line shopping
- Kelly sizing
- exposure controls
- circuit breakers

### Phase 5: Execution and Paper Validation

- adapter interface
- idempotency and audit flow
- paper trading mode
- settlement and CLV capture

### Phase 6: Live Validation and Expansion

- constrained live rollout
- sharper line sources
- richer props support
- ML sidecar and Bayesian refinement

## Architecture at a Glance

```text
Odds Sources -> Ingestion Workers -> PostgreSQL -> Models -> Decision Engine -> Execution
                     |                  |
                     |                  +-> Fiber operational views
                     +-> River jobs
```

Shared infrastructure:

- ingestion
- storage
- scheduling
- observability
- bankroll controls

Sport-specific layers:

- ETL sources
- feature engineering
- model families
- schedule and market heuristics

## Planned Feature Areas

- sports betting odds tracker
- sportsbook line history archive
- CLV dashboard
- EV screening engine
- bankroll ledger
- backtesting CLI
- sport-specific feature pipelines
- paper trading workflow
- operational alerts and runbooks

## Quick Start

The project is still under active build-out, but the local baseline is straightforward.

### Requirements

- Go `1.24+`
- Docker and Docker Compose
- PostgreSQL via the provided local stack

### Local setup

```bash
git clone <your-fork-or-repo-url>
cd betbot
docker compose -f deploy/docker/docker-compose.yml up -d --build
go test ./...
```


Local compose runs odds polling in explicit disabled mode so health is deterministic without a real Odds API key:

```bash
BETBOT_ODDS_POLLING_ENABLED=false
BETBOT_ODDS_API_KEY=TODO_SET_BETBOT_ODDS_API_KEY
```

Set `BETBOT_ODDS_POLLING_ENABLED=true` and provide a real `BETBOT_ODDS_API_KEY` when you want live odds ingestion.
If you are upgrading an older local Docker volume from PostgreSQL 16, recreate that local database volume before starting the PostgreSQL 17 container. The old data directory is not compatible with PostgreSQL 17.

To run the Postgres-backed integration package explicitly:

```bash
BETBOT_TEST_DATABASE_URL=postgres://betbot:betbot-dev-password@localhost:5432/betbot?sslmode=disable go test ./internal/integration -v
```

Current entrypoints:

- `cmd/server`
- `cmd/worker`
- `cmd/backtest`

### Free MLB Historical Backfill (2025-first)

The repository now includes free-source Python importers for MLB historical replay inputs:

- `scripts/import_historical_odds.py`
  - imports local workbook odds (default `C:\Users\Admin\Downloads\mlb-odds.xlsx`)
  - optional normalized scraper CSVs (`--scraped-csv`) for free-source gap fill
  - source precedence is deterministic: `mlb-odds.xlsx` first, then `sportsbookreview-scraper`, then `mlb-odds-scraper`
- `scripts/import_game_results.py`
  - imports final MLB game outcomes via `MLB-StatsAPI` into `game_results`
- `scripts/import_mlb_features_pybaseball.py`
  - imports season team/pitcher stat snapshots from `pybaseball`
- `scripts/import_scraped_odds.py`
  - wrapper for normalized outputs from `mlb-odds-scraper` / `sportsbookreview-scraper`

Install dependencies:

```bash
python -m pip install -r ml/requirements.txt
```

Typical flow:

```bash
# 1) odds snapshots (xlsx + optional scraped CSV)
python scripts/import_historical_odds.py --xlsx "C:\Users\Admin\Downloads\mlb-odds.xlsx"

# 2) final scores / outcomes
python scripts/import_game_results.py --season 2025

# 3) pybaseball stats snapshots (optional, model feature foundations)
python scripts/import_mlb_features_pybaseball.py --season 2025

# 4) run MLB replay
go run cmd/backtest/main.go --sport MLB --season 2025 --market h2h --mode odds
```

### Operator sport filters (read views)

Read views accept an optional `sport` query parameter for operator scoping. If omitted, views stay in all-sports mode.

Examples:

- `GET /odds`
- `GET /odds?sport=baseball_mlb`
- `GET /pipeline/health`
- `GET /pipeline/health?sport=icehockey_nhl`
- `GET /?sport=americanfootball_nfl`

Allowed `sport` values are exactly:

- `baseball_mlb`
- `basketball_nba`
- `icehockey_nhl`
- `americanfootball_nfl`

Invalid sport filters return `HTTP 400` and render an explicit operator-facing error message.

### Recommendation endpoints (recommendation-only)

Use the recommendation pull surface for ranked picks:

- `GET /recommendations`
- `GET /recommendations?sport=baseball_mlb&date=2026-03-16&limit=20`

`/recommendations` is recommendation-only and now returns auditable stake-sizing fields per row:

- `raw_kelly_fraction`, `applied_fractional_kelly`, `capped_fraction`
- `pre_bankroll_stake_dollars`, `pre_bankroll_stake_cents`
- `bankroll_available_cents`, `bankroll_check_pass`, `bankroll_check_reason`
- `suggested_stake_dollars`, `suggested_stake_cents`, `suggested_stake_fraction`
- `correlation_check_pass`, `correlation_check_reason`, `correlation_group_key`
- `circuit_check_pass`, `circuit_check_reason`
- deterministic `sizing_reasons`

Sizing semantics are deterministic and ordered:

- compute raw Kelly from model side probability + selected best-book odds
- apply fractional Kelly policy (sport defaults unless env override)
- enforce max bet fraction cap
- run ledger-backed bankroll gate against current `bankroll_ledger` balance
- if insufficient funds, cap final stake to available cents and emit explicit reasons

Correlation guard semantics are deterministic and recommendation-only:

- runs after sizing/bankroll gate and before final `limit` truncation
- groups by `sport|game_id` across mixed markets (for example `h2h`, `spreads`, `totals`)
- applies same-game limits (`max picks`, `max summed stake fraction`) with fixed reason codes
- optional `sport|event_date` pick limit can also be enforced
- zero-stake rows are retained with explicit `retained_zero_stake` and do not consume exposure capacity

Example:

- `GET /recommendations?sport=baseball_mlb&date=2026-03-16&limit=10`
- If two MLB markets from the same game rank highly, only the highest-ranked retained row survives when `BETBOT_CORRELATION_MAX_PICKS_PER_GAME=1`, and the response order remains deterministic.

Circuit breaker semantics are deterministic and recommendation-only:

- runs after correlation guard and before final `limit` truncation
- uses persisted ledger-derived bankroll metrics (`current`, `day-start`, `week-start`, `peak`) from Postgres
- enforces `daily_loss_stop`, `weekly_loss_stop`, and `drawdown_breaker` with deterministic reason precedence
- exact threshold equality triggers (`>=`) a breaker
- zero-stake rows are retained with explicit `retained_zero_stake`

Example:

- `GET /recommendations?sport=baseball_mlb&date=2026-03-16&limit=20`
- If bankroll loss breaches `BETBOT_DAILY_LOSS_STOP`, positive-stake rows are dropped with `circuit_check_reason=dropped_daily_loss_stop`.

Use the performance surface for recommendation quality and CLV monitoring:

- `GET /recommendations/performance`
- `GET /recommendations/performance?sport=baseball_mlb&date_from=2026-03-01&date_to=2026-03-14&limit=100`

Performance rows are deterministic and include explicit status when close or result data is not yet available (`close_unavailable`, `pending_outcome`, `settled`).

Use the calibration surface for confidence alignment checks by rank bucket:

- `GET /recommendations/calibration`
- `GET /recommendations/calibration?sport=baseball_mlb&date_from=2026-03-01&date_to=2026-03-14&bucket_count=10&limit=500`

Calibration supports `bucket_count` in `[1,20]` (default `10`) and reports filter echo, settled/excluded counts, per-bucket observed vs expected win rates, calibration gaps, Brier scores, and overall ECE.

Use the drift-alert surface to compare current vs baseline calibration windows with sample guardrails:

- `GET /recommendations/calibration/alerts`
- `GET /recommendations/calibration/alerts?sport=baseball_mlb&current_from=2026-03-01&current_to=2026-03-14&baseline_from=2026-02-01&baseline_to=2026-02-14&bucket_count=10&min_settled_overall=100&min_settled_per_bucket=20`

Alert levels are `ok`, `warn`, `critical`, or `insufficient_sample`, with deterministic reasons and per-bucket calibration-gap/Brier deltas.

Use rolling mode on the same endpoint to evaluate deterministic multi-window drift trends:

- `GET /recommendations/calibration/alerts?mode=rolling&sport=baseball_mlb&current_to=2026-03-14&window_days=7&steps=5&bucket_count=10&limit=500`
- `GET /recommendations/calibration/alerts?mode=rolling&sport=baseball_mlb&current_to=2026-03-14&window_days=30&steps=10&min_settled_overall=200&min_settled_per_bucket=25`

Rolling mode returns the existing latest-window alert block plus a deterministic `trend` array ordered oldest-to-newest (`window_start`, `window_end`, `alert_level`, `ece_delta`, `brier_delta`, settled sample counts).

Use drift history for append-only alert run audit visibility:

- `GET /recommendations/calibration/alerts/history`
- `GET /recommendations/calibration/alerts/history?sport=baseball_mlb&date_from=2026-03-01&date_to=2026-03-14&limit=100`

History rows are ordered `created_at DESC, id DESC` and include run hashes, window bounds, thresholds/guardrails, alert level/reasons, summary deltas, and optional persisted payload snapshot.

### Recommendation sizing config overrides

Global env overrides for recommendation sizing are optional:

- `BETBOT_KELLY_FRACTION` in `[0,1]` (`0` = use sport defaults)
- `BETBOT_MAX_BET_FRACTION` in `[0,1]` (`0` = use sport defaults)
- `BETBOT_CORRELATION_MAX_PICKS_PER_GAME` in `[1,25]` (default `1`)
- `BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME` in `(0,1]` (default `0.03`)
- `BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY` in `[0,500]` (default `0`, disabled)
- `BETBOT_DAILY_LOSS_STOP` in `[0,1]` (default `0.05`)
- `BETBOT_WEEKLY_LOSS_STOP` in `[0,1]` (default `0.10`)
- `BETBOT_DRAWDOWN_BREAKER` in `[0,1]` (default `0.15`)
- `BETBOT_PAPER_MODE` toggles paper vs live execution posture (default `true`)
- `BETBOT_EXECUTION_ADAPTER` selects the execution adapter; defaults to `paper` in paper mode and must be explicitly set to a live adapter when `BETBOT_PAPER_MODE=false`
- `BETBOT_AUTO_PLACEMENT_ENABLED` controls the recommendation auto-placement worker; defaults to `true` in paper mode and `false` in live mode

When sizing overrides are unset/zero, the decision layer uses sport-specific baseline sizing policy values (MLB/NBA/NHL/NFL). Correlation guard settings use the deterministic global defaults shown above.

## Documentation

Start here if you want the full project picture:

- [Implementation plan](docs/betbot-plan.md)
- [Technical architecture](docs/ARCHITECTURE.md)
- [Progress tracker](docs/TRACKER.md)
- [Repository structure](docs/REPO_STRUCTURE.md)
- [Four-sport specialization deep dive](docs/SPORT_OPTIMIZATION.md)
- [Product voice and operator posture](docs/SOUL.md)

Key ADRs:

- [Four-sport specialization](docs/decisions/007-four-sport-specialization.md)
- [Phase 1 vertical slice first](docs/decisions/008-phase-1-vertical-slice-first.md)

## Open Source Position

betbot is intended to remain open source.

### License

This project is licensed under `Apache-2.0`.

Why that is the best fit here:

- permissive for commercial and hobby use
- clearer patent grant than MIT
- widely understood in infrastructure and developer-tooling projects
- friendly to contributors who may later build hosted or managed derivatives

The full license text is in [LICENSE](LICENSE).

## Contributing

Contributions are welcome, especially around:

- odds ingestion
- PostgreSQL schema design
- sqlc query design
- River worker patterns
- sports data integrations
- CLV and backtesting workflows
- MLB, NBA, NHL, and NFL feature engineering
- documentation and runbooks

Before opening a large PR, read:

- [betbot-plan.md](docs/betbot-plan.md)
- [ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [TRACKER.md](docs/TRACKER.md)

## Suggested GitHub Topics

If this repo is published publicly, these topics will help discovery:

- `sports-betting`
- `sports-analytics`
- `sports-betting-bot`
- `clv`
- `expected-value`
- `backtesting`
- `odds-api`
- `postgresql`
- `golang`
- `fiber`
- `river`
- `sqlc`
- `mlb`
- `nba`
- `nhl`
- `nfl`

## Recommended Community Setup

For better GitHub discovery and contributor flow, enable:

- GitHub Discussions for modeling, data sources, and roadmap questions
- issue labels by area: `ingestion`, `db`, `worker`, `ui`, `docs`, `mlb`, `nba`, `nhl`, `nfl`
- a public roadmap pinned from [TRACKER.md](docs/TRACKER.md)
- a CONTRIBUTING guide in a follow-up change

## Compliance and Risk Note

This repository is for infrastructure, analytics, and research. Sports betting laws, operator terms of service, and automation rules vary by jurisdiction and book. Anyone using this code for real-money workflows is responsible for their own legal, compliance, and operational review.

## Keywords

Open-source sports betting bot, sports betting analytics, sports betting tracker, odds tracking, closing line value, CLV tracker, expected value betting, Kelly Criterion bankroll management, sportsbook odds history, line shopping, sports betting backtesting, MLB betting model, NBA betting model, NHL betting model, NFL betting model, Go sports betting project, PostgreSQL odds database.








