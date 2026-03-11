# BETBOT — Four-Sport Trading System Plan

**Go · PostgreSQL 17 · River · Fiber · sqlc**
March 2026

---

## 1. Executive Summary

betbot is a quantitative sports betting trading system specialized for `MLB`, `NBA`, `NHL`, and `NFL`. The product direction is no longer "generic multi-sport." Every major planning decision now assumes these four leagues are the core operating surface for data ingestion, modeling, scheduling, and execution.

The immediate implementation goal is still narrow: ship a production-shaped data foundation before broad sport-specific modeling work. The repository today is an early scaffold with a minimal health server, Docker Compose, and placeholder package layout. The roadmap below separates that current baseline from the target system so implementation work can proceed without doc drift.

### Why These Four Sports

- `MLB`: highest game volume, richest free public analytics, pitcher-driven market structure
- `NBA`: player availability and schedule effects create measurable spread and total edges
- `NHL`: goalie confirmations, xG divergence, and PDO regression create distinct inefficiency windows
- `NFL`: deepest handle and most efficient game markets, but still exploitable through weather, rest, and key-number discipline

### Product Principles

- **Measurement before modeling.** Build the odds archive and monitoring path before advanced model work.
- **Market as prior.** Start from the market's probability estimate and model residual mispricing.
- **CLV is the primary truth signal.** Win/loss results matter less than whether the system beats the closing line.
- **Exactly-once matters.** Financial state changes and bet placement semantics must be auditable and idempotent.
- **Specialize where it pays.** Shared infrastructure is cross-sport; data, features, and models are not.

---

## 2. Current Baseline vs Target State

| Concern | Current repo baseline | Target state |
|--------|------------------------|--------------|
| HTTP stack | Minimal `net/http` bootstrap with `/health` | Fiber v3 server with operational views and APIs |
| Database runtime | Local Docker Postgres | Local and target runtime standardized on PostgreSQL 17 |
| DB access | No real pool-backed store wiring yet | `pgxpool` + `sqlc` everywhere |
| Queue | No live queue wiring | River-backed workers and periodic jobs |
| Odds ingestion | Not implemented | Continuous polling, normalization, dedup, append-only storage |
| Sport support | Planned in docs only | Explicitly specialized for MLB/NBA/NHL/NFL |
| Modeling | Placeholder packages | Sport-specific baseline models, then ML sidecar expansion |

This document is canonical for product direction and phase order. [ARCHITECTURE.md](ARCHITECTURE.md) carries the technical details, and [TRACKER.md](TRACKER.md) carries the executable work queue.

---

## 3. Four-Sport Product Direction

### 3.1 Shared Cross-Sport Thesis

All four sports follow the same analytical shape:

1. Start from the current market price.
2. Update that prior with sport-specific quality, situational, and injury inputs.
3. Compare the resulting posterior to the market.
4. Measure success using CLV and calibration before scaling capital.

### 3.2 Sport Profiles

| Sport | Primary edge surface | Core public data | Baseline model direction |
|------|-----------------------|------------------|--------------------------|
| `MLB` | pitcher matchups, park factors, bullpen fatigue, F5 markets | Baseball Savant, FanGraphs, Rotowire, weather APIs | pitcher and lineup adjusted run expectation |
| `NBA` | rest, travel, lineup absences, pace matchups | NBA Stats API, Dunks & Threes, DARKO, PBPStats, Rotowire | lineup-adjusted net rating and total model |
| `NHL` | goalie starts, xG divergence, PDO regression, travel | NHL API, MoneyPuck, Natural Stat Trick, Daily Faceoff | xG plus goalie quality model |
| `NFL` | key numbers, weather, short weeks, QB status | nflverse, PFR, Rotowire, weather APIs | EPA/DVOA plus situational spread model |

### 3.3 Shared Infrastructure, Specialized Inputs

These parts stay shared across all four sports:

- odds ingestion and deduplication
- append-only odds archive
- CLV measurement
- queueing, scheduling, observability, and configuration
- bankroll ledger and later execution semantics

These parts become sport-specific:

- ETL workers and source integrations
- feature builders
- calendar-aware polling and season activation
- model registry contents
- Kelly tuning and calibration expectations by sport

The supporting deep dive is in [SPORT_OPTIMIZATION.md](SPORT_OPTIMIZATION.md).

---

## 4. Phase 1: Data Foundation Vertical Slice

Phase 1 is intentionally narrower than the full four-sport end state. It establishes the one slice the system must get right before broader specialization work is worth doing.

### Phase 1 Goal

Continuously ingest odds into PostgreSQL 17, preserve raw replayable data, expose operational visibility through Fiber, and build the wiring needed for later sport-specific expansion.

### In Scope

- Upgrade the local/runtime baseline to PostgreSQL 17
- Add `pgxpool` database bootstrap with explicit pool configuration
- Use `sqlc` as the application query layer
- Implement core schema for `games`, `odds_history`, and `bankroll_ledger`
- Implement a River-backed `OddsPollJob`
- Integrate The Odds API as the first external source
- Normalize and deduplicate odds snapshots before insert
- Upsert `games` from incoming odds payloads
- Preserve `raw_json` on `odds_history` rows for replayability
- Replace the bootstrap server with a minimal Fiber server
- Provide operational views:
  - `/health`
  - `/`
  - `/odds`
  - `/pipeline/health`

### Explicitly Out of Scope

- sport-specific stat ETL workers
- lineup, goalie, pitcher, and weather ingestion
- model execution and `model_predictions`
- CLV capture automation beyond future-ready schema and scheduling decisions
- bankroll state machine and settlement logic
- sportsbook execution and paper trading

### Phase 1 Deliverables

1. PostgreSQL 17 + `pgxpool` + `sqlc` foundation
2. Append-only `odds_history` with monthly partitioning
3. River queue topology for ingestion
4. The Odds API client and normalization pipeline
5. Deduplicated `OddsPollJob`
6. Fiber operational views backed by DB reads
7. Unit and integration coverage for normalization, dedup, and storage behavior

### Phase 1 Exit Criteria

- Odds polling runs on a schedule without manual intervention
- `games` and `odds_history` are populated continuously
- Duplicate snapshots do not bloat the odds archive
- Operational views expose latest odds and last poll health from stored data
- The repo runs cleanly against Postgres 17 with documented env/config defaults

---

## 5. Roadmap After Phase 1

### Phase 2: Sport Foundation

Build the shared specialization substrate:

- `SportConfig` registry
- season-aware scheduling
- sport-specific schema additions
- lineup and injury ingestion
- MLB/NBA/NHL/NFL ETL worker scaffolding
- weather ingestion for outdoor sports

### Phase 3: Baseline Models and Backtesting

- MLB pitcher matchup model
- NBA lineup-adjusted net rating model
- NHL xG plus goalie model
- NFL EPA/DVOA plus situational model
- walk-forward replay engine
- CLV and calibration reporting
- sport-specific Kelly defaults

### Phase 4: Decision Engine

- EV thresholding
- line shopping
- bankroll state machine
- correlation controls
- circuit breakers

### Phase 5: Execution and Paper Mode

- adapter interface
- first live/paper adapter
- idempotency and audit semantics
- paper-mode validation run

### Phase 6: Live Validation and Expansion

- constrained live rollout
- CLV and calibration monitoring by sport
- sharper data sources and richer props
- ML sidecar and Bayesian update refinement

---

## 6. Metrics and Validation Standards

| Metric | Why it matters | Phase it becomes operational |
|-------|-----------------|------------------------------|
| CLV | primary evidence of edge | foundation in Phase 1, operational in later execution phases |
| Calibration | verifies probabilities are honest | Phase 3 |
| Odds poll latency | tells whether the ingestion slice is credible | Phase 1 |
| Dedup skip rate | prevents silent archive bloat | Phase 1 |
| Queue delay | indicates worker pressure and missed windows | Phase 1 |
| Drawdown and exposure | protects capital | Phase 4 onward |

Sport-specific interpretation matters:

- `MLB`: CLV emphasis on moneyline and F5 markets
- `NBA`: spread and total efficiency dominate
- `NHL`: moneyline and goalie timing windows matter most
- `NFL`: spread CLV near key numbers carries outsized weight

---

## 7. Documentation Map

- [ARCHITECTURE.md](ARCHITECTURE.md): technical component contracts and implementation boundaries
- [TRACKER.md](TRACKER.md): current task list and phase sequencing
- [REPO_STRUCTURE.md](REPO_STRUCTURE.md): checked-in layout and documentation of planned package responsibilities
- [SPORT_OPTIMIZATION.md](SPORT_OPTIMIZATION.md): sport-by-sport modeling and data-source deep dive
- [007-four-sport-specialization.md](decisions/007-four-sport-specialization.md): why the product is specialized to these leagues
- [008-phase-1-vertical-slice-first.md](decisions/008-phase-1-vertical-slice-first.md): why the first implementation phase stays narrow
