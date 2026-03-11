# Architecture — betbot

> Technical architecture reference for the betbot sports betting trading system.

---

## System Overview

betbot is a five-layer pipeline architecture. Data flows unidirectionally from ingestion through to execution, with PostgreSQL as the central source of truth. Each layer has clear boundaries, explicit contracts, and independent scalability characteristics.

### Version & Runtime Baseline

The architecture target and the checked-in runtime are not identical yet. Document both explicitly to avoid drift:

| Concern | Current repo baseline | Target state |
|---------|------------------------|--------------|
| App module | `betbot` | `betbot` |
| Local app runtime | Standalone Docker container `betbot` | Container or droplet |
| Local DB runtime | Docker `betbot-postgres` | Managed Postgres 17 or equivalent |
| HTTP stack | Minimal `net/http` bootstrap for health | Fiber v3 dashboard + API |
| Host port | `18080` | Environment-specific |
| In-container app port | `8080` | `8080` unless overridden |
| Health endpoint | `/health` | `/health` plus `/metrics` |
| Queue | Not implemented yet | River with queue classes and uniqueness rules |

```
┌─────────────────────────────────────────────────────────────────┐
│                      DATA INGESTION LAYER                       │
│                                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐   │
│  │ Odds API │  │ Stats    │  │ Injury   │  │ Situational   │   │
│  │ Poller   │  │ ETL      │  │ Scraper  │  │ (weather etc) │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬────────┘   │
└───────┼──────────────┼─────────────┼───────────────┼────────────┘
        │              │             │               │
        ▼              ▼             ▼               ▼
┌─────────────────────────────────────────────────────────────────┐
│                     POSTGRESQL DATA STORE                        │
│                                                                 │
│  games │ odds_history │ team_stats │ injuries │ situational     │
│  model_predictions │ bets │ clv_log │ bankroll_ledger           │
│                                                                 │
└──────────────┬──────────────────────────────┬───────────────────┘
               │                              │
               ▼                              ▼
┌──────────────────────────┐   ┌──────────────────────────────────┐
│     MODELING LAYER       │   │        DASHBOARD (read-only)     │
│                          │   │                                  │
│  ┌────────┐ ┌─────────┐ │   │  Fiber v3 + HTMX + Alpine.js    │
│  │ Go     │ │ Python  │ │   │  Live odds │ PnL │ CLV │ Diag   │
│  │ gonum  │ │ sidecar │ │   │                                  │
│  │ Elo/LR │ │ gRPC    │ │   └──────────────────────────────────┘
│  └───┬────┘ └───┬─────┘ │
│      └─────┬────┘       │
└────────────┼────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      DECISION ENGINE                             │
│                                                                 │
│  EV Threshold    Kelly Sizer       Correlation Guard            │
│  Filter          (fractional)      (same-game dedup)            │
│                                                                 │
│  Bankroll Manager                  Circuit Breakers             │
│  (state machine)                   (daily/weekly/drawdown)      │
│                                                                 │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                      EXECUTION LAYER                             │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │              Book Adapter Interface                        │  │
│  │   PlaceBet() │ CheckStatus() │ GetBalance() │ Cancel()    │  │
│  └───────┬──────────┬──────────────┬─────────────┬───────────┘  │
│          │          │              │             │               │
│    ┌─────▼──┐ ┌─────▼──┐   ┌──────▼──┐  ┌──────▼──────┐       │
│    │ DK     │ │ FD     │   │ BetMGM  │  │ Pinnacle    │       │
│    │Adapter │ │Adapter │   │ Adapter │  │ Adapter     │       │
│    └────────┘ └────────┘   └─────────┘  └─────────────┘       │
│                                                                 │
│  Idempotency Keys │ Distributed Locks │ Audit Log              │
│  Read-back Verification │ Retry with Backoff                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Layer 1: Data Ingestion

### Odds Polling Worker

The odds poller is the most latency-sensitive component. It determines how quickly betbot can react to line movement and whether arbitrage or steam-following strategies are viable.

**Design:**

- Implemented as a River periodic job (`OddsPollJob`)
- Configurable poll interval: 60s for live games, 300s for pre-game, 15s for steam-detection mode
- Targets The Odds API as primary source; OddsJam as secondary for sharp book coverage
- Each poll cycle fetches all tracked markets for all active games

**Storage contract:**

Every API response is stored twice:
1. **Raw:** Full JSON response body in `odds_history.raw_json` (JSONB column). This is the source of truth for backtesting replay — it can always be reprocessed.
2. **Normalized:** Parsed into structured columns: `game_id`, `book`, `market_type`, `outcome`, `odds_american`, `odds_decimal`, `implied_prob`, `captured_at`.

**Deduplication:** Hash the normalized odds tuple (`game_id + book + market + outcome + odds`). If the hash matches the previous snapshot for that tuple, skip the insert. This prevents bloating `odds_history` during periods when lines aren't moving.

**Rate limiting:** Token bucket per API source, configured to stay within quota. If rate-limited, log the miss and retry on the next cycle — never block the worker.

```
OddsPollJob
  │
  ├─ Fetch from Odds API (HTTP GET)
  ├─ Store raw JSON → odds_history.raw_json
  ├─ Parse → normalize → dedup check
  ├─ INSERT normalized rows → odds_history
  ├─ If line changed → enqueue ModelRunJob for affected game_ids
  └─ Log: games polled, lines changed, API latency
```

### Stats ETL Pipeline

Sport-specific workers that pull team and player statistics from upstream sources and load into normalized Postgres tables. Each sport is an independent River job type.

| Sport | Source | Job | Schedule |
|-------|--------|-----|----------|
| NFL | nflverse / nflfastR datasets | `NFLStatsETLJob` | Daily 04:00 UTC |
| NBA | stats.nba.com unofficial API | `NBAStatsETLJob` | Daily 05:00 UTC |
| MLB | Baseball Savant Statcast API | `MLBStatsETLJob` | Daily 06:00 UTC |
| Soccer | Stats Perform / Opta (if licensed) | `SoccerStatsETLJob` | Daily 07:00 UTC |

**Principle:** ETL jobs are idempotent. Running the same job for the same date twice produces identical results. Use `ON CONFLICT DO UPDATE` for upserts.

### Injury & Situational Scraper

Monitors Rotowire API and structured feeds for:
- Injury reports (status changes: Questionable → Out)
- Lineup confirmations (starting pitcher, QB status)
- Weather conditions for outdoor venues

Updates `situational_factors` table keyed by `game_id`. These factors are consumed by the modeling layer as Bayesian update signals.

**Latency target:** Injury data should be available to the model within 5 minutes of source publication. For critical changes (starting QB ruled out), a manual override endpoint allows immediate injection.

---

## Layer 2: PostgreSQL Data Store

### Schema Design

PostgreSQL 17 is the single source of truth. All financial state, all historical data, and all model outputs live here. There is no authoritative in-memory state.

### Connection Management

All concurrent application traffic should use `pgxpool`, not single raw connections. Use a single process-wide pool per service with explicit defaults for:

- `MaxConns`
- `MinConns`
- `MaxConnLifetime`
- `MaxConnIdleTime`
- `HealthCheckPeriod`
- `AfterConnect` session setup such as `SET TIME ZONE 'UTC'`

For dedicated features such as `LISTEN/NOTIFY`, acquire a pool connection explicitly and release it after the operation completes.

#### Entity Relationship Summary

```
games (1) ──── (N) odds_history
games (1) ──── (N) model_predictions
games (1) ──── (N) situational_factors
games (1) ──── (N) bets
bets  (1) ──── (1) clv_log
bets  (1) ──── (N) bankroll_ledger entries
```

#### Table Definitions

**`games`** — Master registry of all tracked sporting events.

```sql
CREATE TABLE games (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sport           TEXT NOT NULL,           -- 'nfl', 'nba', 'mlb', 'soccer'
    external_id     TEXT UNIQUE,             -- upstream source ID
    home_team       TEXT NOT NULL,
    away_team       TEXT NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'scheduled',  -- scheduled/live/final/postponed
    home_score      INT,
    away_score      INT,
    result_metadata JSONB,                   -- sport-specific result data
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_games_sport_start ON games (sport, start_time);
CREATE INDEX idx_games_status ON games (status);
```

**`odds_history`** — Append-only time series of odds snapshots. The most important table in the system.

```sql
CREATE TABLE odds_history (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    game_id         UUID NOT NULL REFERENCES games(id),
    book            TEXT NOT NULL,            -- 'draftkings', 'fanduel', 'pinnacle'
    market_type     TEXT NOT NULL,            -- 'moneyline', 'spread', 'total', 'prop'
    outcome         TEXT NOT NULL,            -- 'home', 'away', 'over', 'under', player name
    odds_american   INT NOT NULL,
    odds_decimal    NUMERIC(8,4) NOT NULL,
    implied_prob    NUMERIC(6,5) NOT NULL,
    spread_value    NUMERIC(5,2),             -- for spread/total markets
    snapshot_hash   TEXT NOT NULL,            -- dedup hash
    raw_json        JSONB,                   -- full API response
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id, captured_at)            -- for partitioning
) PARTITION BY RANGE (captured_at);

-- Create monthly partitions
CREATE TABLE odds_history_2026_01 PARTITION OF odds_history
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
-- ... additional months

CREATE INDEX idx_odds_game_book ON odds_history (game_id, book, market_type);
CREATE INDEX idx_odds_captured ON odds_history (captured_at);
CREATE INDEX idx_odds_hash ON odds_history (snapshot_hash);
```

**`model_predictions`** — Model outputs per game, versioned.

```sql
CREATE TABLE model_predictions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id         UUID NOT NULL REFERENCES games(id),
    model_name      TEXT NOT NULL,            -- 'elo-v1', 'xgb-nfl-v2'
    model_version   TEXT NOT NULL,
    predicted_prob  NUMERIC(6,5) NOT NULL,   -- model's estimated probability
    confidence_low  NUMERIC(6,5),            -- bootstrap lower bound
    confidence_high NUMERIC(6,5),            -- bootstrap upper bound
    features_json   JSONB NOT NULL,          -- full feature vector for reproducibility
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_predictions_game ON model_predictions (game_id, model_name);
```

**`bets`** — The bet ledger. State machine: pending → placed → settled.

```sql
CREATE TABLE bets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id         UUID NOT NULL REFERENCES games(id),
    prediction_id   BIGINT REFERENCES model_predictions(id),
    book            TEXT NOT NULL,
    market_type     TEXT NOT NULL,
    outcome         TEXT NOT NULL,
    odds_american   INT NOT NULL,
    odds_decimal    NUMERIC(8,4) NOT NULL,
    implied_prob    NUMERIC(6,5) NOT NULL,
    model_prob      NUMERIC(6,5) NOT NULL,   -- model's probability at time of bet
    edge            NUMERIC(6,5) NOT NULL,   -- model_prob - implied_prob
    kelly_fraction  NUMERIC(6,5) NOT NULL,   -- computed Kelly %
    stake           NUMERIC(12,2) NOT NULL,
    potential_payout NUMERIC(12,2) NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending/placed/won/lost/push/cancelled
    idempotency_key TEXT UNIQUE NOT NULL,
    placed_at       TIMESTAMPTZ,
    settled_at      TIMESTAMPTZ,
    settlement_pnl  NUMERIC(12,2),
    book_bet_id     TEXT,                    -- sportsbook's confirmation ID
    audit_json      JSONB,                   -- placement attempt log
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bets_game ON bets (game_id);
CREATE INDEX idx_bets_status ON bets (status);
CREATE INDEX idx_bets_placed ON bets (placed_at);
```

**`clv_log`** — CLV attribution computed at game start.

```sql
CREATE TABLE clv_log (
    bet_id          UUID PRIMARY KEY REFERENCES bets(id),
    bet_implied     NUMERIC(6,5) NOT NULL,
    closing_implied NUMERIC(6,5) NOT NULL,
    clv_pct         NUMERIC(8,5) NOT NULL,   -- (closing - bet) / bet
    closing_odds    INT NOT NULL,
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**`bankroll_ledger`** — Explicit financial ledger. Balance is always the sum of this table.

```sql
CREATE TABLE bankroll_ledger (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type      TEXT NOT NULL,            -- 'deposit', 'withdrawal', 'bet_placed', 'bet_won', 'bet_lost', 'bet_push'
    amount          NUMERIC(12,2) NOT NULL,  -- positive for inflows, negative for outflows
    balance_after   NUMERIC(12,2) NOT NULL,
    bet_id          UUID REFERENCES bets(id),
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_created ON bankroll_ledger (created_at);
```

### Partitioning Strategy

`odds_history` is partitioned by month on `captured_at`. At sustained polling, this table will grow by millions of rows per month. Monthly partitions allow:
- Efficient pruning of old data (detach and archive partitions)
- Partition-pruned queries for backtest date ranges
- Parallel sequential scans within partition boundaries

**Automation:** A River job (`PartitionMaintenanceJob`) runs monthly to create the next month's partition and optionally archive partitions older than the configured retention window.

**Index strategy:** Use partitioned-table indexes for indexes that must exist on every partition. When rolling out new indexes on a hot system, create them concurrently on child partitions and attach them to the parent definition.

**Retention rule:** Partitioning is worthwhile when `odds_history` is large enough that pruning, archival, and partition-local scans materially improve operations. Keep the maintenance path simple until that threshold is real.

**Runbook expectation:** Document the create-next-partition flow, detach/archive flow, and recovery steps if the next partition is missing at rollover.

### Backup & Recovery

- **WAL streaming** to a secondary for point-in-time recovery
- **Daily pg_dump** of the full database to Backblaze B2 (or configured object store)
- **odds_history raw_json** is the ultimate recovery artifact — all normalized data can be reconstructed from it

---

## Layer 3: Modeling

### Model Interface

All models implement a common Go interface:

```go
type Model interface {
    Name() string
    Version() string
    Predict(ctx context.Context, game domain.Game, features Features) (Prediction, error)
}

type Prediction struct {
    Probability   float64  // estimated win probability [0, 1]
    ConfidenceLow float64  // bootstrap lower bound
    ConfidenceHigh float64 // bootstrap upper bound
    Features      Features // full feature vector (stored for reproducibility)
}

type Features map[string]float64
```

### Go-Native: Elo System

The Elo model is the baseline. It's simple, interpretable, and runs entirely in Go using gonum.

**Update rule (margin-adjusted):**

```
K = base_K * margin_multiplier(actual_margin)
new_rating = old_rating + K * (actual - expected)
expected = 1 / (1 + 10^((rating_B - rating_A) / 400))
```

Margin multiplier scales K-factor based on margin of victory — a 30-point blowout is more informative than a 1-point win. Home-field advantage is a constant offset (~3 points NFL, ~3.5 points NBA) subtracted from the away team's effective rating.

**Win probability from Elo:**

```
P(A wins) = 1 / (1 + 10^((elo_B - elo_A + HFA) / 400))
```

### Python Sidecar: Ensemble ML

For models requiring scikit-learn, XGBoost, or LightGBM, a Python microservice runs alongside the Go system. Communication is via gRPC with protobuf-defined schemas.

```protobuf
service ModelService {
    rpc Predict(PredictRequest) returns (PredictResponse);
    rpc BatchPredict(BatchPredictRequest) returns (BatchPredictResponse);
    rpc GetCalibration(CalibrationRequest) returns (CalibrationResponse);
}

message PredictRequest {
    string game_id = 1;
    string model_name = 2;
    map<string, double> features = 3;
}

message PredictResponse {
    double probability = 1;
    double confidence_low = 2;
    double confidence_high = 3;
}
```

**Calibration requirement:** All ML models must pass calibration verification before deployment. Platt scaling (logistic regression on model outputs) or isotonic regression is applied as a post-processing step. The `GetCalibration` endpoint returns the calibration curve for monitoring.

#### Operational Contract

The sidecar is a bounded dependency, not an implicit always-on requirement:

- Every RPC has a timeout budget
- Batch inference is preferred when the worker can accumulate requests cheaply
- Failure behavior is explicit: fallback to a Go-native model or fail closed for that workflow
- Repeated inference errors trip a circuit breaker instead of stalling workers indefinitely
- Prediction rows persist `model_name` and `model_version`
- Worker concurrency must stay within sidecar throughput limits

### Feature Engineering

Features are computed per game and cached in `model_predictions.features_json` for reproducibility:

| Category | Features | Source |
|----------|----------|--------|
| Team Quality | EPA/play, DVOA, net rating, xG, Elo | Stats ETL |
| Situational | Rest days, travel distance, altitude, surface, indoor/outdoor | Computed |
| Injury Impact | Weighted by player WAR / usage rate / snap % | Injury scraper |
| Market Signals | Opening line, current line, line delta, public bet % | Odds poller |
| Weather | Temperature, wind mph, precipitation probability | Weather API |
| Historical | H2H record, venue-specific performance | Stats ETL |

---

## Layer 4: Decision Engine

The decision engine transforms model predictions into actionable bet tickets. It is the system's **risk management layer** — the gatekeeper between quantitative output and real capital deployment.

### Pipeline

```
Model Prediction
  │
  ├─ EV Threshold Filter
  │   └─ Reject if edge < BETBOT_EV_THRESHOLD (default 2%)
  │
  ├─ Line Shopping
  │   └─ Select best available odds across all tracked books
  │
  ├─ Kelly Sizer
  │   └─ f = kelly_fraction * (b*p - q) / b
  │   └─ Cap at BETBOT_MAX_BET_FRACTION
  │
  ├─ Correlation Guard
  │   └─ Check pending/placed bets on same game_id
  │   └─ Reduce effective Kelly if correlated exposure exists
  │   └─ Reject if aggregate exposure on game > 5% of bankroll
  │
  ├─ Bankroll Check
  │   └─ Verify sufficient available balance (not pending)
  │   └─ Check daily/weekly loss stop thresholds
  │   └─ Check drawdown circuit breaker
  │
  └─ Emit BetTicket → Execution Layer
```

### Bankroll State Machine

```
                 ┌──────────┐
    deposit ───► │ Available │ ◄──── bet_push / bet_won
                 └────┬─────┘
                      │ bet_placed (stake deducted)
                      ▼
                 ┌──────────┐
                 │ Pending   │     (bet submitted, awaiting confirmation)
                 └────┬─────┘
                      │ placement confirmed
                      ▼
                 ┌──────────┐
                 │ Placed    │     (confirmed with sportsbook)
                 └────┬─────┘
                      │ game settles
              ┌───────┼───────┐
              ▼       ▼       ▼
          ┌──────┐ ┌──────┐ ┌──────┐
          │ Won  │ │ Lost │ │ Push │
          └──────┘ └──────┘ └──────┘
```

Every state transition creates a `bankroll_ledger` entry. The current balance is always:

```sql
SELECT balance_after FROM bankroll_ledger ORDER BY created_at DESC LIMIT 1;
```

---

## Layer 5: Execution

### Book Adapter Interface

```go
type BookAdapter interface {
    Name() string
    PlaceBet(ctx context.Context, ticket BetTicket) (PlacementResult, error)
    CheckStatus(ctx context.Context, bookBetID string) (BetStatus, error)
    GetBalance(ctx context.Context) (Balance, error)
    Cancel(ctx context.Context, bookBetID string) error
}

type BetTicket struct {
    IdempotencyKey string
    GameID         uuid.UUID
    Book           string
    MarketType     string
    Outcome        string
    OddsAmerican   int
    Stake          decimal.Decimal
}

type PlacementResult struct {
    BookBetID    string
    Confirmed    bool
    ActualOdds   int        // may differ from requested if line moved
    ErrorMessage string
}
```

### Placement Flow

```
BetTicket arrives from Decision Engine
  │
  ├─ 1. Acquire distributed lock (Postgres advisory lock on idempotency_key)
  │      └─ If lock held → skip (another worker is handling this bet)
  │
  ├─ 2. Check idempotency: SELECT FROM bets WHERE idempotency_key = ?
  │      └─ If exists and status != 'pending' → skip (already processed)
  │
  ├─ 3. INSERT bet record with status = 'pending'
  │
  ├─ 4. Deduct stake from bankroll_ledger (Available → Pending)
  │
  ├─ 5. Call BookAdapter.PlaceBet()
  │      └─ Timeout: 10 seconds
  │      └─ On timeout → mark bet as 'pending_verification'
  │
  ├─ 6. Read-back verification: BookAdapter.CheckStatus()
  │      └─ Confirm the bet was actually placed
  │
  ├─ 7. UPDATE bet status = 'placed', book_bet_id = result.BookBetID
  │
  ├─ 8. Log full audit record (request, response, timing)
  │
  └─ 9. Release distributed lock
```

**Paper trading mode:** When `BETBOT_PAPER_MODE=true`, step 5 is replaced with a simulated placement that always succeeds. All other steps (ledger entries, audit logging, CLV tracking) execute normally. This allows full pipeline validation without real capital.

### Retry Strategy

- On transient failure (timeout, 5xx): retry up to 3 times with exponential backoff (1s, 4s, 16s)
- On rejection (odds changed, insufficient funds): do NOT retry — re-evaluate from the decision engine
- On unknown state (timeout without confirmation): enter `pending_verification` state, schedule a `VerifyBetJob` to check status after 60 seconds

---

## Cross-Cutting Concerns

### Dashboard Read Models

The dashboard should not query the hottest append-only tables directly for every request. `odds_history` remains the system of record, but UI handlers should prefer read models optimized for read-heavy paths:

- `current_odds`: latest normalized market snapshot per game/book/market/outcome
- `best_market_odds`: best-of-market view for dashboard and screening
- Dashboard aggregate queries or materialized views for bankroll, CLV, and pipeline summaries

### Observability

| Signal | Implementation |
|--------|---------------|
| Structured logging | zerolog → JSON to stdout → collected by log aggregator |
| Metrics | Prometheus-compatible `/metrics` endpoint on Fiber server |
| Key metrics | odds_poll_latency, model_run_duration, bet_placement_latency, clv_rolling_avg |
| Alerting | River-driven alert jobs on critical thresholds |
| Tracing | OpenTelemetry spans on cross-layer calls (ingestion → model → decision → execution) |

**Metric rules:**

- Use counters for irreversible events such as jobs completed, bets attempted, bets placed, and placement failures
- Use histograms for latency and duration signals
- Keep labels low-cardinality; do not attach raw `game_id`, `bet_id`, or sportsbook ticket IDs to hot metrics
- Expose pool stats, queue depth, and model inference latency as first-class operational metrics

**Tracing rules:**

- Start spans at HTTP entrypoints, worker job execution boundaries, and external dependency calls
- Propagate trace context into downstream HTTP and gRPC requests
- Record errors and timeout causes on spans, not only in logs

### Queue Topology & Backpressure

River usage should be documented as distinct queue classes rather than one undifferentiated worker pool:

- `critical`: placement, settlement, CLV capture
- `latency-sensitive`: odds polling and line movement reactions
- `compute`: model runs, feature builds, calibration jobs
- `maintenance`: partition creation, cleanup, archival
- `alerting`: notifications and operator-facing signals

Each queue class should define concurrency ceilings, retry policy, uniqueness windows, and whether backlog in that class is allowed to delay other work.

### Configuration Management

All tunable parameters use environment variables with a `BETBOT_` prefix. Defaults are sane for paper trading. Production overrides are applied via environment-specific config files loaded at startup.

No hot-reload of financial parameters (Kelly fraction, exposure limits, circuit breakers). Changing these requires a worker restart to prevent mid-cycle inconsistency.

### Deployment

| Component | Deployment |
|-----------|-----------|
| `cmd/server` | Single DigitalOcean droplet (or container) — serves dashboard + API |
| `cmd/worker` | Same or separate droplet — runs all River jobs |
| PostgreSQL | Managed Postgres (DO Managed Database) or self-hosted with WAL streaming |
| Python sidecar | Containerized, deployed alongside worker; gRPC on localhost |

**Scaling consideration:** The worker is the bottleneck during high-volume polling (e.g., NFL Sunday with 14 simultaneous games). River supports multiple worker processes reading from the same queue — horizontal scaling is adding worker instances.

### Release & Rollback

Release safety is part of the architecture:

- Migrations run before workers that depend on new schema
- Readiness checks gate traffic and queue consumption
- Rollback procedures specify whether a release is application-only or schema-compatible
- Financially sensitive changes require a paper-mode or dry-run path before live enablement

---

## Backtesting Architecture

The backtester (`cmd/backtest`) is a **replay engine** that walks through historical `odds_history` data and simulates the full pipeline offline.

```
Historical odds_history
  │
  ├─ For each game in chronological order:
  │   ├─ Reconstruct odds timeline from stored snapshots
  │   ├─ At each snapshot, run the model (same code as live)
  │   ├─ Feed prediction through decision engine (same code as live)
  │   ├─ If bet triggered, record virtual placement at available odds
  │   ├─ At game start, capture closing odds (same CLV logic as live)
  │   ├─ At game end, settle virtual bet
  │   └─ Update virtual bankroll
  │
  └─ Output:
      ├─ Cumulative PnL curve (CSV + optional chart)
      ├─ CLV distribution (mean, median, stddev)
      ├─ Calibration table (predicted vs actual by decile)
      ├─ Sharpe-equivalent ratio
      ├─ Max drawdown and duration
      └─ Bet-level detail log (CSV)
```

**Critical design decision:** The backtester uses **the same model and decision engine code** as the live system. It does not have a separate implementation. This eliminates the risk of backtest/live divergence (one of the most common quant pitfalls).

The only difference: the execution layer is replaced with a virtual placement recorder that logs the bet without calling any external API.

---

## Security Considerations

- **Sportsbook credentials** stored in environment variables or a secrets manager, never in code or config files committed to version control.
- **Database credentials** via connection string in environment, not hardcoded.
- **API keys** (Odds API, stats sources) rotate on a schedule; stored outside the codebase.
- **Audit log** is append-only and includes full request/response payloads for placement attempts — but sensitive fields (passwords, tokens) are redacted before logging.
- **Dashboard access** is localhost-only or behind authentication (basic auth minimum; OAuth if exposed to network).
