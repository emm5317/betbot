# Repository Structure — betbot

> Complete file and directory layout for the betbot sports betting trading system.
> This document is the canonical reference for where things live.

---

## Root

```
betbot/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                          # Go test + lint + sqlc diff on PR
│   │   ├── backtest-nightly.yml            # Nightly backtest regression against golden dataset
│   │   └── migration-check.yml             # Verify no deployed migration was modified
│   └── PULL_REQUEST_TEMPLATE.md
│
├── cmd/
│   ├── server/
│   │   └── main.go                         # HTTP server entry point (Fiber v3 + dashboard)
│   ├── worker/
│   │   └── main.go                         # River worker entry point (all background jobs)
│   └── backtest/
│       └── main.go                         # CLI replay engine for model validation
│
├── internal/
│   ├── domain/                             # Core business types — zero infrastructure deps
│   │   ├── game.go                         # Game, GameStatus, GameResult
│   │   ├── odds.go                         # OddsSnapshot, ImpliedProbability, MarketType
│   │   ├── bet.go                          # Bet, BetStatus, BetTicket, IdempotencyKey
│   │   ├── bankroll.go                     # BankrollEntry, EventType, BalanceQuery
│   │   ├── prediction.go                   # Prediction, Confidence, FeatureVector
│   │   ├── clv.go                          # CLVRecord, CLVCalculation
│   │   ├── book.go                         # BookName constants, BookConfig
│   │   ├── sport.go                        # Sport enum, SportConfig (key numbers, HFA, etc.)
│   │   └── errors.go                       # Domain-specific error types
│   │
│   ├── ingestion/                          # Layer 1: Data ingestion workers
│   │   ├── oddspoller/
│   │   │   ├── poller.go                   # OddsPollJob — fetch, normalize, dedup, store
│   │   │   ├── poller_test.go
│   │   │   ├── client.go                   # The Odds API HTTP client (rate-limited)
│   │   │   ├── client_test.go
│   │   │   ├── normalizer.go              # Raw JSON → domain.OddsSnapshot conversion
│   │   │   ├── normalizer_test.go
│   │   │   ├── dedup.go                    # Snapshot hash computation and duplicate detection
│   │   │   └── dedup_test.go
│   │   ├── statsetl/
│   │   │   ├── etl.go                      # Base ETL job logic (shared across sports)
│   │   │   ├── etl_test.go
│   │   │   ├── nfl.go                      # NFLStatsETLJob — nflverse data
│   │   │   ├── nfl_test.go
│   │   │   ├── nba.go                      # NBAStatsETLJob — stats.nba.com
│   │   │   ├── nba_test.go
│   │   │   ├── mlb.go                      # MLBStatsETLJob — Baseball Savant
│   │   │   └── mlb_test.go
│   │   ├── injuries/
│   │   │   ├── scraper.go                  # InjuryScanJob — Rotowire API + news feeds
│   │   │   ├── scraper_test.go
│   │   │   ├── rotowire.go                # Rotowire API client
│   │   │   └── rotowire_test.go
│   │   └── weather/
│   │       ├── weather.go                  # Weather fetch for outdoor venues
│   │       └── weather_test.go
│   │
│   ├── modeling/                           # Layer 3: Probability models and feature engineering
│   │   ├── elo/
│   │   │   ├── elo.go                      # Elo rating system (margin-adjusted, HFA)
│   │   │   ├── elo_test.go
│   │   │   ├── predictor.go               # Elo → win probability; implements Model interface
│   │   │   └── predictor_test.go
│   │   ├── features/
│   │   │   ├── builder.go                  # FeatureBuilder — assembles feature vector per game
│   │   │   ├── builder_test.go
│   │   │   ├── team_quality.go             # EPA, DVOA, net rating, xG features
│   │   │   ├── situational.go             # Rest days, travel, altitude, surface, indoor/outdoor
│   │   │   ├── injury_impact.go           # Weighted injury impact (WAR, usage rate)
│   │   │   ├── market_signals.go          # Opening line, current line, delta, public %
│   │   │   └── weather_features.go        # Temp, wind, precipitation
│   │   ├── ev.go                           # EV calculation: model_prob vs implied_prob
│   │   ├── ev_test.go
│   │   ├── calibration.go                 # Calibration verification utilities
│   │   ├── calibration_test.go
│   │   ├── model.go                        # Model interface definition
│   │   └── jobs.go                         # ModelRunJob, EVScreenJob River workers
│   │
│   ├── decision/                           # Layer 4: Risk management and bet sizing
│   │   ├── kelly.go                        # Kelly Criterion sizer (fractional)
│   │   ├── kelly_test.go
│   │   ├── threshold.go                   # EV threshold filter
│   │   ├── threshold_test.go
│   │   ├── correlation.go                 # Correlation guard (same-game exposure detection)
│   │   ├── correlation_test.go
│   │   ├── lineshopper.go                 # Best-of-market odds selection across books
│   │   ├── lineshopper_test.go
│   │   ├── bankroll.go                    # Bankroll manager (state machine, balance queries)
│   │   ├── bankroll_test.go
│   │   ├── circuit.go                     # Circuit breakers (daily/weekly/drawdown stops)
│   │   ├── circuit_test.go
│   │   ├── engine.go                      # Decision engine orchestrator (pipeline: filter → size → check → emit)
│   │   ├── engine_test.go
│   │   └── ticket.go                      # BetTicket construction, idempotency key generation
│   │
│   ├── execution/                          # Layer 5: Bet placement and settlement
│   │   ├── adapter.go                     # BookAdapter interface definition
│   │   ├── adapters/
│   │   │   ├── pinnacle/
│   │   │   │   ├── adapter.go             # Pinnacle BookAdapter implementation
│   │   │   │   ├── adapter_test.go
│   │   │   │   ├── client.go             # Pinnacle API HTTP client
│   │   │   │   └── types.go              # Pinnacle-specific request/response types
│   │   │   ├── draftkings/
│   │   │   │   ├── adapter.go
│   │   │   │   ├── adapter_test.go
│   │   │   │   ├── client.go
│   │   │   │   └── types.go
│   │   │   ├── fanduel/
│   │   │   │   ├── adapter.go
│   │   │   │   ├── adapter_test.go
│   │   │   │   ├── client.go
│   │   │   │   └── types.go
│   │   │   ├── betmgm/
│   │   │   │   ├── adapter.go
│   │   │   │   ├── adapter_test.go
│   │   │   │   ├── client.go
│   │   │   │   └── types.go
│   │   │   └── paper/
│   │   │       ├── adapter.go             # Paper trading adapter (simulated placement)
│   │   │       └── adapter_test.go
│   │   ├── placement.go                   # PlacementJob — lock, check, place, verify, audit
│   │   ├── placement_test.go
│   │   ├── settlement.go                 # SettlementJob — reconcile outcomes, update ledger
│   │   ├── settlement_test.go
│   │   ├── clvcapture.go                 # CLVCaptureJob — archive closing odds at game start
│   │   ├── clvcapture_test.go
│   │   ├── idempotency.go               # Idempotency key management, distributed locking
│   │   ├── idempotency_test.go
│   │   ├── retry.go                      # Retry with exponential backoff
│   │   ├── retry_test.go
│   │   └── audit.go                      # Audit log writer (append-only, redaction)
│   │
│   ├── store/                              # Layer 2: Database access (sqlc-generated)
│   │   ├── db.go                           # GENERATED — sqlc DBTX interface
│   │   ├── models.go                       # GENERATED — Go structs from schema
│   │   ├── querier.go                     # GENERATED — query interface
│   │   ├── games.sql.go                   # GENERATED — games queries
│   │   ├── odds_history.sql.go            # GENERATED — odds_history queries
│   │   ├── bets.sql.go                    # GENERATED — bets queries
│   │   ├── bankroll.sql.go               # GENERATED — bankroll_ledger queries
│   │   ├── predictions.sql.go            # GENERATED — model_predictions queries
│   │   ├── clv.sql.go                    # GENERATED — clv_log queries
│   │   ├── team_ratings.sql.go           # GENERATED — Elo rating queries
│   │   └── dashboard.sql.go             # GENERATED — dashboard aggregate queries
│   │
│   ├── server/                             # HTTP server setup and routes
│   │   ├── server.go                      # Fiber app initialization, middleware, route registration
│   │   ├── routes.go                      # Route definitions (dashboard, API, health)
│   │   ├── middleware.go                  # Auth, logging, recovery middleware
│   │   └── handlers/
│   │       ├── dashboard.go              # Dashboard page handler (serves HTMX views)
│   │       ├── odds.go                   # Live odds board handler
│   │       ├── bets.go                   # Pending/settled bets handler
│   │       ├── bankroll.go              # Bankroll chart handler
│   │       ├── clv.go                   # CLV tracker handler
│   │       ├── diagnostics.go           # Model diagnostics handler
│   │       ├── admin.go                 # Manual deposit, circuit breaker override
│   │       ├── health.go               # Health check endpoint
│   │       └── api.go                   # JSON API endpoints (for external consumers)
│   │
│   ├── worker/                             # River worker setup
│   │   ├── worker.go                      # River client init, job type registration
│   │   └── scheduler.go                  # Periodic job scheduling (cron-like intervals)
│   │
│   ├── backtest/                           # Backtesting engine
│   │   ├── engine.go                      # Replay engine — walks odds_history chronologically
│   │   ├── engine_test.go
│   │   ├── virtual_bankroll.go           # Virtual bankroll state machine
│   │   ├── virtual_bankroll_test.go
│   │   ├── reporter.go                   # Output generation: PnL CSV, CLV stats, calibration
│   │   ├── reporter_test.go
│   │   └── guardrails.go                # Look-ahead bias detection, survivorship checks
│   │
│   ├── alerts/                             # Alerting subsystem
│   │   ├── alerter.go                     # Alert dispatch (email, Slack webhook, stdout)
│   │   ├── alerter_test.go
│   │   └── jobs.go                       # AlertJob River worker
│   │
│   └── config/                             # Configuration loading
│       ├── config.go                      # Struct with all BETBOT_ env vars, defaults, validation
│       └── config_test.go
│
├── sql/                                    # sqlc query definitions (source of truth for store/)
│   ├── queries/
│   │   ├── games.sql                      # INSERT, SELECT, UPDATE for games
│   │   ├── odds_history.sql              # INSERT (append-only), SELECT by game/book/time range
│   │   ├── bets.sql                      # INSERT, UPDATE status, SELECT by game/status
│   │   ├── bankroll.sql                  # INSERT ledger entry, SELECT balance, SELECT history
│   │   ├── predictions.sql              # INSERT, SELECT by game/model
│   │   ├── clv.sql                      # INSERT, SELECT by bet, aggregate CLV stats
│   │   ├── team_ratings.sql             # INSERT/UPDATE ratings, SELECT current/historical
│   │   └── dashboard.sql               # Aggregate queries for dashboard views
│   └── sqlc.yaml                         # sqlc configuration (schema path, query path, output)
│
├── migrations/                             # Sequential SQL migrations (append-only)
│   ├── 001_create_games.up.sql
│   ├── 001_create_games.down.sql
│   ├── 002_create_odds_history.up.sql
│   ├── 002_create_odds_history.down.sql
│   ├── 003_create_model_predictions.up.sql
│   ├── 003_create_model_predictions.down.sql
│   ├── 004_create_bets.up.sql
│   ├── 004_create_bets.down.sql
│   ├── 005_create_clv_log.up.sql
│   ├── 005_create_clv_log.down.sql
│   ├── 006_create_bankroll_ledger.up.sql
│   ├── 006_create_bankroll_ledger.down.sql
│   ├── 007_create_team_ratings.up.sql
│   ├── 007_create_team_ratings.down.sql
│   ├── 008_create_situational_factors.up.sql
│   ├── 008_create_situational_factors.down.sql
│   ├── 009_create_river_tables.up.sql          # River queue schema
│   ├── 009_create_river_tables.down.sql
│   └── 010_add_odds_history_partitions.up.sql  # Initial monthly partitions
│
├── proto/                                  # gRPC definitions for Python ML sidecar
│   ├── model/
│   │   └── v1/
│   │       ├── model.proto                # Predict, BatchPredict, GetCalibration RPCs
│   │       └── model_grpc.pb.go           # GENERATED — Go gRPC stubs
│   └── buf.yaml                           # buf configuration for proto management
│
├── ml/                                     # Python ML sidecar (separate runtime)
│   ├── Dockerfile                         # Python container for ML service
│   ├── requirements.txt                   # scikit-learn, xgboost, lightgbm, grpcio
│   ├── pyproject.toml                     # Python project config
│   ├── server.py                          # gRPC server entry point
│   ├── models/
│   │   ├── ensemble.py                    # XGBoost / LightGBM ensemble model
│   │   ├── bayesian.py                    # Bayesian updating (market prior + signals)
│   │   └── calibration.py               # Platt scaling, isotonic regression
│   ├── training/
│   │   ├── train.py                      # Model training script (reads from Postgres)
│   │   ├── evaluate.py                   # Holdout evaluation, calibration curves
│   │   └── feature_importance.py         # Feature importance analysis
│   ├── artifacts/                         # Serialized model files (.joblib, .json)
│   │   └── .gitkeep
│   └── tests/
│       ├── test_ensemble.py
│       ├── test_bayesian.py
│       └── test_calibration.py
│
├── templates/                              # Go HTML templates for HTMX dashboard
│   ├── layouts/
│   │   ├── base.html                     # Base layout: head, nav, footer, Alpine.js init
│   │   └── dashboard.html               # Dashboard shell with sidebar nav
│   ├── pages/
│   │   ├── home.html                     # Dashboard home: summary cards, quick stats
│   │   ├── odds.html                     # Live odds board (full page)
│   │   ├── bets.html                     # Bet management (pending, history, filters)
│   │   ├── bankroll.html                # Bankroll chart and ledger history
│   │   ├── clv.html                     # CLV tracker with rolling averages
│   │   ├── diagnostics.html             # Model diagnostics, calibration, features
│   │   └── admin.html                   # Admin: deposit, circuit breaker controls
│   └── partials/                          # HTMX partial fragments (swapped into page)
│       ├── odds_table.html              # Odds board table (polled via hx-trigger)
│       ├── bet_row.html                 # Single bet row for list updates
│       ├── bankroll_chart.html          # Chart.js bankroll visualization
│       ├── clv_chart.html               # CLV rolling average chart
│       ├── pipeline_status.html         # Data pipeline health indicators
│       ├── alert_banner.html            # Circuit breaker / system alert banner
│       └── model_summary.html           # Latest model run summary card
│
├── static/                                 # Static assets served by Fiber
│   ├── css/
│   │   └── app.css                       # betbot styles (Soul.md design tokens)
│   ├── js/
│   │   ├── alpine.min.js                # Alpine.js (vendored)
│   │   └── app.js                       # Dashboard interactivity (Alpine components)
│   └── fonts/
│       ├── inter-variable.woff2         # Inter for headings/body
│       └── jetbrains-mono.woff2        # JetBrains Mono for data/numbers
│
├── testdata/                               # Test fixtures and golden datasets
│   ├── odds_snapshots/
│   │   ├── sample_odds_api_response.json # Example Odds API response for parser tests
│   │   └── dedup_test_cases.json        # Duplicate/unique snapshot pairs
│   ├── backtest/
│   │   ├── golden_nfl_2024.csv          # Historical odds + results for regression tests
│   │   ├── golden_nfl_2024_expected.csv # Expected backtest output for golden dataset
│   │   └── README.md                    # Dataset provenance and format documentation
│   ├── elo/
│   │   ├── nfl_elo_test_cases.json     # Known Elo update scenarios with expected outputs
│   │   └── margin_multiplier_cases.json # Margin-adjusted K-factor test vectors
│   └── placement/
│       ├── mock_pinnacle_response.json  # Mock sportsbook API responses
│       ├── mock_dk_response.json
│       └── mock_timeout_scenario.json   # Timeout + verification flow test data
│
├── scripts/                                # Utility scripts (not part of the Go build)
│   ├── seed_bankroll.sh                  # Insert initial deposit into bankroll_ledger
│   ├── create_partition.sh               # Manually create next month's odds_history partition
│   ├── import_historical_odds.py         # Import historical odds from SBR/CSV into odds_history
│   ├── import_game_results.py            # Import historical game results for backtesting
│   ├── export_backtest_report.sh         # Run backtest and format output for review
│   └── rotate_api_keys.sh               # Rotate Odds API / sportsbook credentials
│
├── deploy/                                 # Deployment configuration
│   ├── docker/
│   │   ├── Dockerfile                    # Multi-stage Go build (server + worker + backtest)
│   │   ├── Dockerfile.ml                 # Python ML sidecar
│   │   └── docker-compose.yml            # Local dev: Postgres + Go services + ML sidecar
│   ├── systemd/
│   │   ├── tradebot-server.service       # systemd unit for HTTP server
│   │   ├── tradebot-worker.service       # systemd unit for River worker
│   │   └── tradebot-ml.service           # systemd unit for Python ML sidecar
│   └── nginx/
│       └── tradebot.conf                 # Reverse proxy config (TLS, auth, rate limiting)
│
├── docs/                                   # Project documentation
│   ├── tradebot-plan.md                  # Comprehensive plan document
│   ├── ARCHITECTURE.md                   # Technical architecture deep-dive
│   ├── SOUL.md                           # Project identity, voice, design language
│   ├── TRACKER.md                        # Jira-style progress tracker
│   ├── decisions/                         # Architecture Decision Records (ADRs)
│   │   ├── 001-go-over-python.md        # Why Go as primary language
│   │   ├── 002-river-over-temporal.md   # Why River for job queue
│   │   ├── 003-sqlc-over-orm.md         # Why sqlc over GORM/ent
│   │   ├── 004-append-only-odds.md      # Why odds_history is append-only
│   │   ├── 005-fractional-kelly.md      # Why 25% Kelly default
│   │   └── 006-paper-mode-first.md      # Why paper trading before live
│   ├── runbooks/
│   │   ├── circuit-breaker-triggered.md # What to do when a breaker fires
│   │   ├── placement-failure.md         # Diagnosing and recovering from placement errors
│   │   ├── data-pipeline-outage.md      # Odds API down or ETL failure response
│   │   ├── account-limited.md           # Sportsbook limited or banned account
│   │   └── model-miscalibration.md      # Model calibration drift response
│   └── api/
│       └── openapi.yaml                  # OpenAPI spec for JSON API endpoints
│
├── .env.example                            # Example environment variables (no secrets)
├── .gitignore
├── .golangci.yml                           # golangci-lint configuration
├── CLAUDE.md                               # Claude Code system prompt
├── Makefile                                # Common tasks: build, test, lint, migrate, sqlc
├── README.md                               # Project overview, quickstart, contributing
├── go.mod
├── go.sum
└── LICENSE
```

---

## Directory Responsibilities

### `cmd/` — Entry Points

Three binaries, each with a single `main.go`. No business logic lives here — just wiring (config loading, dependency injection, service startup). Each binary can be built and deployed independently.

| Binary | Purpose | Runs As |
|--------|---------|---------|
| `cmd/server/` | Fiber HTTP server: dashboard UI, JSON API, health checks | Long-running service |
| `cmd/worker/` | River worker: all background jobs (polling, modeling, placement, settlement) | Long-running service |
| `cmd/backtest/` | Replay engine: offline model validation against historical data | CLI invocation |

### `internal/` — Business Logic

All business logic lives under `internal/` (Go convention: not importable by external packages). Organized by architectural layer, not by technical concern.

| Package | Layer | Depends On |
|---------|-------|-----------|
| `domain/` | — | Nothing (pure types) |
| `ingestion/` | Layer 1 | `domain/`, `store/` |
| `store/` | Layer 2 | `domain/` (sqlc-generated) |
| `modeling/` | Layer 3 | `domain/`, `store/` |
| `decision/` | Layer 4 | `domain/`, `store/`, `modeling/` |
| `execution/` | Layer 5 | `domain/`, `store/`, `decision/` |
| `server/` | Cross-cutting | `store/`, all layers (read-only) |
| `worker/` | Cross-cutting | All layers (job registration) |
| `backtest/` | Cross-cutting | `modeling/`, `decision/`, `store/` |
| `alerts/` | Cross-cutting | `store/`, `config/` |
| `config/` | Cross-cutting | Nothing |

**Dependency rule:** Layers only depend downward or on `domain/`. The execution layer never imports from `ingestion/`. The modeling layer never imports from `execution/`. `domain/` imports nothing.

### `sql/` — Query Definitions

The **source of truth** for all database queries. Edit files here; run `sqlc generate` to regenerate `internal/store/`. Never hand-edit generated files.

### `migrations/` — Schema Migrations

Sequential, append-only. Each migration has an `up.sql` and `down.sql`. Never modify a migration that has been applied to a live database — create a new one.

### `proto/` — gRPC Definitions

Protobuf definitions for communication between the Go system and the Python ML sidecar. Generated Go stubs are committed to the repo. Python stubs are generated at build time inside the `ml/` container.

### `ml/` — Python ML Sidecar

Entirely self-contained Python project. Has its own Dockerfile, dependencies, and test suite. Communicates with Go exclusively via gRPC. No shared filesystem, no shared database connections — only protobuf messages.

### `templates/` + `static/` — Dashboard UI

Server-rendered HTML using Go's `html/template` package. HTMX for dynamic updates (polling, partial swaps). Alpine.js for client-side interactivity (toggle states, charts). Fonts and styles follow the Soul.md design system.

### `testdata/` — Test Fixtures

Golden datasets, mock API responses, and known-good test vectors. Committed to the repo. The backtest golden dataset (`golden_nfl_2024.csv`) is the regression test baseline — if a model change alters backtest output, the diff is reviewed explicitly.

### `docs/` — Documentation

All project documentation lives here, alongside the repo root files (`CLAUDE.md`, `README.md`). Includes Architecture Decision Records (ADRs) for significant technical choices and operational runbooks for common failure scenarios.

### `deploy/` — Deployment

Docker, systemd, and nginx configurations. `docker-compose.yml` provides a complete local development environment (Postgres, Go services, ML sidecar). Production deployment uses systemd units on DigitalOcean droplets.

---

## Key Files at Root

| File | Purpose |
|------|---------|
| `CLAUDE.md` | Claude Code system prompt — read before every session |
| `README.md` | Project overview, quickstart, architecture summary |
| `Makefile` | Common commands: `make build`, `make test`, `make lint`, `make migrate`, `make sqlc` |
| `.env.example` | Template for required environment variables (no real secrets) |
| `.golangci.yml` | Linter config: `gofmt`, `govet`, `errcheck`, `staticcheck`, `gosec` |
| `go.mod` / `go.sum` | Go module definition and dependency lock |

---

## Makefile Targets

```makefile
.PHONY: build test lint migrate sqlc proto dev clean

build:                  ## Build all binaries
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker
	go build -o bin/backtest ./cmd/backtest

test:                   ## Run all tests
	go test ./...

test-race:              ## Run tests with race detector
	go test -race ./...

test-integration:       ## Run integration tests (requires Postgres)
	go test -tags=integration ./...

lint:                   ## Run linters
	golangci-lint run ./...

sqlc:                   ## Regenerate sqlc code
	sqlc generate

migrate-up:             ## Apply all pending migrations
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:           ## Rollback last migration
	migrate -path migrations -database "$$DATABASE_URL" down 1

migrate-create:         ## Create new migration (usage: make migrate-create NAME=add_foo)
	migrate create -ext sql -dir migrations -seq $(NAME)

proto:                  ## Generate gRPC Go stubs from proto definitions
	buf generate

dev:                    ## Start local development environment
	docker compose -f deploy/docker/docker-compose.yml up -d

dev-down:               ## Stop local development environment
	docker compose -f deploy/docker/docker-compose.yml down

backtest:               ## Run backtest against golden dataset
	go run ./cmd/backtest --sport nfl --season 2024 --model elo-v1

clean:                  ## Remove build artifacts
	rm -rf bin/
```

---

## Naming Conventions

| Scope | Convention | Example |
|-------|-----------|---------|
| Go packages | Lowercase, single word | `ingestion`, `decision`, `execution` |
| Go types | PascalCase, domain nouns | `Game`, `OddsSnapshot`, `BetTicket` |
| Go interfaces | PascalCase, `-er` suffix where natural | `BookAdapter`, `Model`, `Alerter` |
| Go test files | Colocated, `_test.go` suffix | `kelly.go` → `kelly_test.go` |
| SQL migrations | Sequential number + snake_case | `001_create_games.up.sql` |
| SQL queries | snake_case verb + noun | `insert_odds_snapshot.sql` |
| sqlc query names | PascalCase matching Go method | `-- name: InsertOddsSnapshot :exec` |
| River job types | PascalCase + `Job` suffix | `OddsPollJob`, `SettlementJob` |
| Environment vars | `BETBOT_` + SCREAMING_SNAKE | `BETBOT_KELLY_FRACTION` |
| Proto messages | PascalCase | `PredictRequest`, `PredictResponse` |
| ADR files | Sequential number + kebab-case | `001-go-over-python.md` |
| Runbook files | kebab-case descriptive | `circuit-breaker-triggered.md` |
| Git branches | `feat/<phase>-<desc>` or `fix/<desc>` | `feat/p1-odds-poller`, `fix/dedup-hash` |
