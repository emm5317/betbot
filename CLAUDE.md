# CLAUDE.md — betbot

You are working on **betbot**, a quantitative sports betting trading system built in Go. This file defines the project context, conventions, and constraints for all Claude Code sessions.

---

## Project Identity

betbot is a five-layer pipeline system: Data Ingestion → Postgres Data Store → Modeling → Decision Engine → Execution. It treats sports betting as quantitative trading — finding mispriced probability, not predicting winners. The closing line is ground truth. CLV (Closing Line Value) is the only honest performance metric.

**This is a financial system.** Bet placement is a financial transaction. Idempotency, auditability, and exactly-once semantics are non-negotiable. When in doubt, fail safe — a missed bet costs nothing; a double-placed bet is a real loss.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24.x (current repo baseline) |
| HTTP | Target: Fiber v3 (`gofiber/fiber/v3`) |
| Database | Target: PostgreSQL 17; local dev currently uses Postgres 16 Alpine |
| SQL | sqlc (`sqlc-dev/sqlc`) — all queries are generated, never hand-written Go SQL |
| Job Queue | River (`riverqueue/river`) |
| Numerics | gonum (`gonum/gonum`) for Elo, regression, probability |
| Postgres Driver | pgx v5 (`jackc/pgx/v5`) |
| Logging | zerolog (`rs/zerolog`) — structured JSON, microsecond timestamps |
| ML (optional) | Python sidecar via gRPC for XGBoost/LightGBM ensemble models |
| Frontend | HTMX + Alpine.js for dashboard |

### Runtime Baseline

These values are the current repo baseline and should stay aligned with the actual checked-in runtime:

| Concern | Current baseline |
|---------|------------------|
| Module | `betbot` |
| Local app container | `betbot` |
| Local database container | `betbot-postgres` |
| Local host app port | `18080` |
| In-container app port | `8080` |
| Local health endpoint | `GET /health` |
| Local compose file | `deploy/docker/docker-compose.yml` |
| Current bootstrap server | Minimal `net/http` health service until Fiber dashboard work lands |

---

## Project Structure

```
betbot/
  cmd/
    server/         → main HTTP service (Fiber v3), serves dashboard
    worker/         → River worker process (all background jobs)
    backtest/       → CLI replay engine for model validation
  internal/
    domain/         → core types: Game, Odds, Bet, Bankroll, Prediction
    ingestion/      → odds poller, stats ETL, injury scraper
    modeling/       → EV calc, Elo system, feature engineering
    decision/       → Kelly sizer, correlation guard, bankroll manager
    execution/      → book adapters, placement, idempotency, audit
    store/          → sqlc-generated queries (DO NOT EDIT GENERATED FILES)
  proto/            → gRPC .proto definitions for Python sidecar
  migrations/       → SQL migrations (sequential, never modify deployed)
  sql/              → sqlc query definitions (.sql files you DO edit)
  config/           → environment config, feature flags
  templates/        → HTMX/Go templates for dashboard
  static/           → CSS, JS (Alpine.js), assets
```

---

## Code Conventions

### Go Style

- Follow standard `gofmt` and `go vet`. No exceptions.
- Use `context.Context` as first parameter on all functions that touch I/O (database, HTTP, gRPC).
- Error handling: wrap errors with `fmt.Errorf("operation: %w", err)` — never swallow errors silently.
- Domain types live in `internal/domain/`. They have zero dependencies on infrastructure packages.
- Interfaces are defined where they are consumed, not where they are implemented.
- Test files are colocated: `foo.go` → `foo_test.go`.

### SQL / sqlc

- All SQL queries are defined in `sql/` directory as `.sql` files with sqlc annotations.
- Run `sqlc generate` to regenerate `internal/store/`. **Never hand-edit generated files.**
- Migrations in `migrations/` are sequential and append-only. Never modify a deployed migration.
- Use `pgx` named parameters, not positional. Include explicit column lists — no `SELECT *`.

### River Jobs

- Each job type gets its own file in the appropriate `internal/` package.
- Job args structs must be serializable to JSON. Keep them small — reference IDs, not full payloads.
- All jobs must be idempotent. If a job runs twice with the same args, the second run is a no-op.
- Use `river.JobInsertOpts` for scheduling: `ScheduledAt` for future execution, `UniqueOpts` for dedup.

### Naming

- Package names: lowercase, single word (`ingestion`, `decision`, `execution`).
- Types: PascalCase. Exported types represent domain concepts (`Game`, `OddsSnapshot`, `BetTicket`).
- Database columns: snake_case. Go fields: PascalCase. sqlc handles the mapping.
- River job types: `PascalCase` + `Job` suffix (`OddsPollJob`, `SettlementJob`).
- Config keys: `BETBOT_` prefix for env vars (`BETBOT_KELLY_FRACTION`, `BETBOT_EV_THRESHOLD`).

---

## Critical Invariants

These rules are **never** violated. If a change would break an invariant, stop and flag it.

### Financial Safety

1. **Bet placement is exactly-once.** Every placement attempt generates an idempotency key (`game_id + market + book + timestamp_bucket`). Distributed lock acquired before placement. Read-back verification after.
2. **Bankroll is an explicit ledger.** Balance is computed from the `bankroll_ledger` table, not inferred. Every state transition (Available → Pending → Placed → Settled) creates a ledger entry.
3. **No in-memory-only financial state.** If the process crashes mid-operation, recovery reads from Postgres. All financial state survives restart.
4. **Hard limits are enforced in the decision engine, not the execution layer.** The execution layer trusts the ticket it receives. The decision engine is the gatekeeper.

### Data Integrity

5. **`odds_history` is append-only.** Never UPDATE or DELETE. Always INSERT. This table is the source of truth for backtesting.
6. **Raw JSON is stored alongside normalized data.** Every API response is preserved verbatim for reprocessing.
7. **Timestamps use database-side `NOW()`.** Not application time. NTP-synced workers, but Postgres is canonical.

### Modeling Discipline

8. **No model touches real capital without backtesting.** The backtest CLI must validate any model against historical odds data before the model is eligible for live execution.
9. **CLV is the primary performance metric.** Win/loss record over small samples is noise. Track CLV over 500+ bets.
10. **Calibration is monitored.** If a model says 60%, it must win ~60% of the time on holdout data. Miscalibrated models are disabled.

---

## Testing Strategy

- **Unit tests** for all domain logic (`internal/domain/`, `internal/modeling/`, `internal/decision/`). These are pure functions — no database, no I/O.
- **Integration tests** for `internal/store/` using a test Postgres database. Use `testcontainers-go` or a dedicated test database.
- **Job tests** for River workers: verify idempotency by running the same job twice and asserting no side effects on the second run.
- **Backtest regression tests**: a known historical dataset with expected outputs. If model changes alter backtest results, the diff is reviewed explicitly.

Run all tests: `go test ./...`
Run with race detector: `go test -race ./...`

---

## Common Operations

```bash
# Generate sqlc code after editing sql/ files
sqlc generate

# Run migrations
migrate -path migrations -database "$BETBOT_DATABASE_URL" up

# Start the HTTP server (dashboard + API)
go run cmd/server/main.go

# Start the River worker
go run cmd/worker/main.go

# Run backtester against historical data
go run cmd/backtest/main.go --sport nfl --season 2025 --model elo-v1

# Run tests
go test ./...
go test -race ./...
go test -v ./internal/decision/...    # specific package
```

### Local Dev Quickstart

```bash
# Start local app + Postgres
docker compose -f deploy/docker/docker-compose.yml up -d --build

# Verify local health
curl http://127.0.0.1:18080/health

# Tail logs
docker compose -f deploy/docker/docker-compose.yml logs -f betbot postgres

# Stop local stack
docker compose -f deploy/docker/docker-compose.yml down
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BETBOT_DATABASE_URL` | Postgres connection string | required |
| `BETBOT_HTTP_ADDR` | HTTP listen address | `:8080` |
| `BETBOT_DB_CONNECT_TIMEOUT` | Database reachability timeout | `5s` |
| `BETBOT_ODDS_API_KEY` | The Odds API key | required for ingestion |
| `BETBOT_KELLY_FRACTION` | Fractional Kelly multiplier | `0.25` |
| `BETBOT_EV_THRESHOLD` | Minimum EV edge to place bet | `0.02` |
| `BETBOT_MAX_BET_FRACTION` | Max single bet as fraction of bankroll | `0.03` |
| `BETBOT_DAILY_LOSS_STOP` | Daily loss halt threshold | `0.05` |
| `BETBOT_WEEKLY_LOSS_STOP` | Weekly loss halt threshold | `0.10` |
| `BETBOT_DRAWDOWN_BREAKER` | Drawdown % from peak to halt | `0.15` |
| `BETBOT_POLL_INTERVAL_LIVE` | Live game poll interval | `60s` |
| `BETBOT_POLL_INTERVAL_PRE` | Pre-game poll interval | `300s` |
| `BETBOT_LOG_LEVEL` | zerolog level | `info` |
| `BETBOT_PAPER_MODE` | Paper trading (no real placement) | `true` |

---

## Session Workflow

When starting a Claude Code session on betbot:

1. **Orient:** Read this file. Check which phase of the roadmap the project is in by reviewing `TRACKER.md`.
2. **Scope:** Confirm the task scope before writing code. Ask if unclear.
3. **Branch:** Work in a feature branch. Name: `feat/<phase>-<short-description>` or `fix/<description>`.
4. **Test:** Write or update tests for all changes. Run `go test ./...` before committing.
5. **Wrapup:** Summarize what was done, what's next, and any decisions made. Update `TRACKER.md` if a task was completed.

---

## What NOT To Do

- Do not use an ORM. sqlc is the data access layer.
- Do not store financial state in memory only. Postgres is the ledger.
- Do not hand-edit files in `internal/store/` — they are sqlc-generated.
- Do not modify deployed migrations. Create new ones.
- Do not skip idempotency on any job that creates side effects.
- Do not use `time.Now()` for financial timestamps. Use Postgres `NOW()`.
- Do not deploy a model without backtesting. Period.
- Do not log sensitive credentials. zerolog fields are reviewed for PII.
