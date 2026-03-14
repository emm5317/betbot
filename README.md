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

What is built now:

- PostgreSQL 17 baseline
- `pgxpool` and `sqlc`
- River-backed odds polling
- append-only `odds_history`
- deduplicated market snapshots
- minimal Fiber operational views for health and current odds
- sport-aware registry and active-season polling policy

The current open build step is Phase 4 decision-engine implementation (`P4-002` onward).

Recommendation mode is now available through `GET /recommendations` for ranked bet suggestions. This flow is recommendation-only and does not invoke live placement adapters.

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








