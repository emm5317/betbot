# betbot

A quantitative sports betting system built in Go. betbot treats sports betting as trading: find mispriced probability, measure edge against the closing line, size positions with Kelly criterion, and enforce risk limits through circuit breakers.

Covers **MLB**, **NBA**, **NHL**, and **NFL** moneyline markets.

![Game Day Hub](docs/screenshots/gameday.png)

---

## What It Does

betbot is a five-layer pipeline:

```
Data Ingestion  ->  Postgres Data Store  ->  Modeling  ->  Decision Engine  ->  Execution
```

- **Ingestion** — Polls live odds from [The Odds API](https://the-odds-api.com/), normalizes across books, deduplicates unchanged snapshots, stores append-only to `odds_history`. Sport-specific stats ETL for team and player data.
- **Modeling** — Sport-specific baseline models: pitcher matchups (MLB), lineup-adjusted net ratings (NBA), xG + goalie (NHL), EPA/DVOA situational (NFL). Feature builders produce stable vectors for prediction.
- **Decision Engine** — EV thresholding, cross-book line shopping, fractional Kelly sizing, bankroll-aware stake capping, same-game correlation guards, and daily/weekly/drawdown circuit breakers.
- **Execution** — Two-transaction exactly-once placement protocol with idempotency keys, ledger-backed stake reservation, and paper/live adapter abstraction.
- **Monitoring** — CLV tracking, calibration by rank bucket, rolling drift alerts, and settlement accuracy verification.

### Current Status

The system is in **paper-trading validation**. Odds polling, predictions, recommendations, and bet placement run end-to-end through a paper adapter. No real capital is at risk. The pipeline must pass sustained validation criteria before a live book adapter is enabled.

See [`docs/TRACKER.md`](docs/TRACKER.md) for detailed phase-by-phase progress.

---

## Dashboard

A game-day surface built with HTMX + Alpine.js on Go HTML templates.

**Game Day Hub** — Recommendations, bankroll posture, open positions, and upcoming games. Click **Refresh Picks** to poll fresh odds and run predictions on demand. Click **Place Bet** on any qualified recommendation to place it through the paper adapter with full ledger tracking.

**Market Board** — Latest persisted prices across books, auto-refreshing.

![Market Board](docs/screenshots/market-board.png)

**Bankroll** — Ledger-backed balance, deposit form, and entry history. Every state transition (reserve, release, settle) is an append-only ledger entry.

![Bankroll](docs/screenshots/bankroll.png)

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24 |
| HTTP | [Fiber v3](https://github.com/gofiber/fiber) |
| Database | PostgreSQL 17 |
| SQL | [sqlc](https://github.com/sqlc-dev/sqlc) (generated, never hand-written) |
| Job Queue | [River](https://github.com/riverqueue/river) |
| Numerics | [gonum](https://github.com/gonum/gonum) |
| Frontend | HTMX 2 + Alpine.js + DaisyUI 5 / Tailwind CSS 4 |
| Driver | [pgx v5](https://github.com/jackc/pgx) |

---

## Project Structure

```
betbot/
  cmd/
    server/         HTTP service + dashboard
    worker/         River background jobs
    backtest/       CLI replay engine
  internal/
    domain/         Core types (Game, Odds, Bet, Bankroll, SportConfig)
    ingestion/      Odds poller, stats ETL, scores client
    modeling/       Sport-specific models + feature builders
    prediction/     Live prediction bridge (offline models -> live recommendations)
    decision/       Recommendations, sizing, correlation, circuit breakers
    execution/      Placement orchestrator, book adapters, settlement, audit
    store/          sqlc-generated queries (do not edit)
  migrations/       Sequential SQL migrations (append-only)
  sql/              sqlc query definitions (.sql files you edit)
  templates/        Go HTML templates (HTMX partials)
  static/           CSS, JS assets
  docs/             Architecture, runbooks, validation queries
```

---

## Quickstart

### Prerequisites

- Go 1.24+
- Docker (for PostgreSQL)
- An API key from [The Odds API](https://the-odds-api.com/)

### Setup

```bash
# Clone
git clone https://github.com/emm5317/betbot.git && cd betbot

# Create .env from template
cp .env.example .env
# Edit .env: set BETBOT_ODDS_API_KEY

# Start PostgreSQL (standalone, port 25432)
docker run -d --name betbot-postgres \
  -e POSTGRES_DB=betbot \
  -e POSTGRES_USER=betbot \
  -e POSTGRES_PASSWORD=betbot-dev-password \
  -p 25432:5432 \
  postgres:17-alpine

# Run migrations
docker run --rm \
  -v "$(pwd)/migrations:/migrations" \
  --network host \
  migrate/migrate \
  -path=/migrations \
  -database "postgres://betbot:betbot-dev-password@localhost:25432/betbot?sslmode=disable" \
  up

# Seed bankroll (paper money — $100,000)
docker exec betbot-postgres psql -U betbot -d betbot \
  -c "INSERT INTO bankroll_ledger (entry_type, amount_cents, currency, reference_type, reference_id, metadata) \
      VALUES ('deposit', 10000000, 'USD', 'manual', 'initial-seed', '{}'::jsonb);"

# Start the server
BETBOT_DATABASE_URL="postgres://betbot:betbot-dev-password@127.0.0.1:25432/betbot?sslmode=disable" \
BETBOT_ODDS_API_KEY="<your-key>" \
BETBOT_ODDS_API_MARKETS=h2h,spreads,totals \
BETBOT_ODDS_POLLING_ENABLED=false \
BETBOT_HTTP_ADDR=:18080 \
BETBOT_PAPER_MODE=true \
go run cmd/server/main.go
```

Open [http://localhost:18080](http://localhost:18080).

### Daily Workflow

1. Open the dashboard
2. Click **Refresh Picks** — pulls live odds from The Odds API and runs predictions
3. Review the decision rail: edge, stake, book, odds for each recommendation
4. Click **Place Bet** on the picks you want
5. Open Positions updates with bankroll deduction

---

## API Endpoints

### Pages

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/` | Game Day Hub |
| `GET` | `/odds` | Market board |
| `GET` | `/bets` | Bet ledger |
| `GET` | `/bankroll` | Bankroll + ledger |
| `GET` | `/pipeline/health` | System diagnostics |

### Data & Actions

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/health` | Health check (JSON) |
| `POST` | `/recommendations/refresh` | Poll odds + predict + build recommendations |
| `GET` | `/recommendations` | Ranked recommendations (JSON) |
| `GET` | `/recommendations/performance` | CLV and outcome metrics |
| `GET` | `/recommendations/calibration` | Calibration by rank bucket |
| `GET` | `/recommendations/calibration/alerts` | Drift alerts (point-in-time or rolling) |
| `GET` | `/recommendations/calibration/alerts/history` | Append-only drift run history |
| `POST` | `/execution/place` | Place a bet (JSON API) |
| `POST` | `/partials/place-bet` | Place a bet (HTMX, from dashboard) |
| `GET` | `/execution/bets` | List placed bets |
| `POST` | `/predictions/run` | Trigger predictions manually |

All pages accept an optional `?sport=` filter (`baseball_mlb`, `basketball_nba`, `icehockey_nhl`, `americanfootball_nfl`).

---

## Configuration

Key environment variables (see [`.env.example`](.env.example) for the full list):

| Variable | Purpose | Default |
|----------|---------|---------|
| `BETBOT_DATABASE_URL` | Postgres connection string | required |
| `BETBOT_ODDS_API_KEY` | [The Odds API](https://the-odds-api.com/) key | required |
| `BETBOT_PAPER_MODE` | Paper trading (no real money) | `true` |
| `BETBOT_EV_THRESHOLD` | Minimum EV edge to recommend | `0.02` |
| `BETBOT_KELLY_FRACTION` | Fractional Kelly override (`0` = sport defaults) | `0` |
| `BETBOT_DAILY_LOSS_STOP` | Daily loss halt threshold | `0.05` |
| `BETBOT_WEEKLY_LOSS_STOP` | Weekly loss halt threshold | `0.10` |
| `BETBOT_DRAWDOWN_BREAKER` | Peak drawdown halt threshold | `0.15` |
| `BETBOT_ODDS_POLLING_ENABLED` | Automatic odds polling | `false` |
| `BETBOT_AUTO_PLACEMENT_ENABLED` | Automatic bet placement | `false` |
| `BETBOT_EXECUTION_ADAPTER` | Book adapter (`paper`, `draftkings`, etc.) | `paper` |

---

## Key Design Decisions

**CLV as primary metric.** Win/loss over small samples is noise. The system tracks whether recommendations consistently beat the closing line.

**Append-only financial ledger.** Bankroll balance is computed from `bankroll_ledger` entries, never stored as a mutable field. Every stake reservation, release, and settlement is immutable.

**Two-transaction placement.** Tx1 inserts a pending bet and reserves stake in the ledger. The book adapter call happens outside any transaction. Tx2 finalizes (mark placed, or mark failed + release stake). Survives crashes mid-placement.

**Immutable recommendation snapshots.** Every recommendation is persisted with full metadata (sizing fractions, correlation check, circuit breaker state) for audit replay and drift detection.

**Models must pass backtesting.** No model touches capital without offline validation against historical odds data. The backtest CLI produces calibration reports, CLV metrics, and walk-forward validation artifacts.

---

## Testing

```bash
go test ./...                       # All tests
go test -race ./...                 # With race detector
go test -v ./internal/decision/...  # Specific package
```

---

## Documentation

| Document | Purpose |
|----------|---------|
| [`docs/TRACKER.md`](docs/TRACKER.md) | Phase-by-phase progress tracker |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Technical architecture |
| [`docs/SOUL.md`](docs/SOUL.md) | Project philosophy and boundaries |
| [`docs/runbooks/paper-validation.md`](docs/runbooks/paper-validation.md) | Daily/weekly validation checklist with exit criteria |
| [`docs/runbooks/threshold-tuning.md`](docs/runbooks/threshold-tuning.md) | Threshold inventory and tuning procedure |
| [`docs/sql/validation-queries.sql`](docs/sql/validation-queries.sql) | 12 diagnostic SQL queries for pipeline health |
| [`docs/SPORT_OPTIMIZATION.md`](docs/SPORT_OPTIMIZATION.md) | Four-sport specialization rationale |

---

## Compliance Note

This repository is for infrastructure, analytics, and research. Sports betting laws, operator terms of service, and automation rules vary by jurisdiction. Anyone using this code for real-money activity is responsible for their own legal and compliance review.

---

## License

Licensed under [Apache-2.0](LICENSE).
