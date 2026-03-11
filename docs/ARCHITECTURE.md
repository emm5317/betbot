# Architecture — betbot

Technical architecture for the four-sport `MLB` / `NBA` / `NHL` / `NFL` betbot system.

---

## 1. Baseline and Scope

The repository is still early-stage, but it is no longer just a scaffold. Phase 1 shipped the data-foundation slice, and early Phase 2 sport-foundation work is now live. Many parts of the system described here remain target architecture rather than shipped implementation.

### Runtime Baseline vs Target

| Concern | Current repo baseline | Target state |
|--------|------------------------|--------------|
| HTTP server | Fiber v3 operational server | Fiber v3 server with broader operational HTML and API routes |
| Worker runtime | River-backed worker with periodic odds polling | River-backed worker process |
| Database | PostgreSQL 17 with live Phase 1 migrations and partitions | PostgreSQL 17 with broader sport-specific schema |
| DB access | `pgxpool` + `sqlc` store layer | `pgxpool` and `sqlc` as the standard path |
| Odds ingestion | scheduled external polling, normalization, dedup, persistence | broader source coverage and downstream ETL |
| Modeling | Placeholder packages | sport-specific model registry and feature builders |

### Architectural Invariants

- PostgreSQL is the source of truth.
- `odds_history` is append-only.
- CLV is the primary downstream validation metric.
- Shared infrastructure is cross-sport; data and model logic are sport-specific.
- Financial workflows must remain transaction-safe and auditable.

---

## 2. System Overview

```
┌──────────────────────────────────────────────────────────────┐
│                    DATA INGESTION LAYER                     │
│  Odds API poller │ sport ETL │ injuries │ lineups │ weather │
└──────────────────────────┬───────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────┐
│                    POSTGRESQL DATA STORE                    │
│  games │ odds_history │ sport stats │ situational │ ledger  │
└──────────────┬───────────────────────┬───────────────────────┘
               │                       │
               ▼                       ▼
┌──────────────────────────┐  ┌───────────────────────────────┐
│      MODELING LAYER      │  │     FIBER READ SURFACE        │
│  sport registry │ model  │  │  health │ odds │ diagnostics  │
│  features       │ EV     │  │  pipeline status │ later UI    │
└──────────────┬───────────┘  └───────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────────────────────┐
│                     DECISION ENGINE                         │
│ EV threshold │ line shopping │ Kelly │ exposure controls    │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                     EXECUTION LAYER                         │
│ adapters │ idempotency │ verification │ audit │ settlement  │
└──────────────────────────────────────────────────────────────┘
```

The shipped implementation covers the left edge of this diagram: DB foundation, odds ingestion, queue bootstrap, Fiber operational reads, and the first sport-aware scheduler policy. Sport-specific ETL, modeling, and execution layers remain later work.

---

## 3. Sport Specialization Model

### 3.1 Domain-Level Sport Registry

The domain model should treat sport selection as an explicit product boundary, not an open-ended enum. The core type is a `SportConfig` registry that captures:

- sport identity and display name
- key numbers or equivalent market anchors
- home advantage baseline
- season boundaries
- game volume expectations
- live and pregame polling cadence
- default Kelly range and modeling posture

This registry drives:

- scheduler behavior
- model selection
- feature-builder routing
- sport-aware configuration defaults
- dashboard filtering and operator mental model

### 3.2 Sport-Specific Responsibilities

| Sport | Data emphasis | Modeling emphasis | Operational nuance |
|------|---------------|-------------------|--------------------|
| `MLB` | Statcast, park factors, bullpen usage, lineups | starter-vs-lineup and run environment | daily game density and F5 market support |
| `NBA` | lineup status, player impact, pace, travel | lineup-adjusted spread and total models | back-to-back and travel-aware scheduling |
| `NHL` | goalie confirmations, xG, PDO, travel | xG plus goalie quality | late goalie confirmations and lower-scoring variance |
| `NFL` | EPA, DVOA, weather, QB status | spread/total model with key-number awareness | low game volume, high market efficiency |

---

## 4. Phase 1 Implementation Boundary

Phase 1 is a vertical slice, not the full sport-aware architecture.

### Implement Now

- PostgreSQL 17 runtime standardization
- `pgxpool` bootstrap
- `sqlc` query generation
- migrations for `games`, `odds_history`, `bankroll_ledger`
- River client, worker registration, and periodic odds polling
- The Odds API client
- normalization and deduplication
- Fiber health and operational read endpoints

### Design For Now, Implement Later

- sport-specific stat tables
- sport ETL jobs
- lineup, injury, and weather ingestion
- model registry and feature builders
- decision engine and execution semantics

This prevents the initial build from being blocked by sport-specific ETL breadth while still keeping the schema and scheduling model compatible with the four-sport target.

---

## 5. Layer 1: Ingestion

### 5.1 Odds Polling

The odds poller is the first real worker and the most latency-sensitive Phase 1 component.

Expected flow:

1. Poll The Odds API on a configured schedule.
2. Parse source payloads into canonical game and odds records.
3. Upsert `games`.
4. Compute snapshot hashes for normalized odds rows.
5. Skip unchanged rows.
6. Insert changed rows into `odds_history`.
7. Record operational stats for last poll health.

Phase 1 source policy:

- one external odds source
- config-driven polling cadence
- per-source timeout and rate-limiting
- raw JSON persisted with normalized rows for replay

### 5.2 Later Ingestion Expansion

The generic `StatsETLJob` concept is replaced by sport-specific workers:

- `MLBStatsETLJob`
- `NBAStatsETLJob`
- `NHLStatsETLJob`
- `NFLStatsETLJob`

Separate workers are intentional because source contracts, refresh cadence, and output tables differ materially by sport.

### 5.3 Injury, Lineup, and Weather Inputs

These remain later-phase additions, but the architecture assumes they will eventually populate sport-aware situational data:

- `MLB`: starter confirmations, lineups, park and weather context
- `NBA`: player availability, rest, travel, and usage redistribution
- `NHL`: goalie starts and line combinations
- `NFL`: QB status, practice participation, and weather

---

## 6. Layer 2: PostgreSQL Data Store

### 6.1 Connection Management

`pgxpool` is the standard application access path.

Documented operational expectations:

- parse pool config from environment
- set `AfterConnect` session configuration such as `SET TIME ZONE 'UTC'`
- expose pool health/readiness through the server
- use pool methods for normal traffic
- acquire dedicated connections only for exceptional cases
- use transaction-scoped query handles for atomic workflows

### 6.2 Phase 1 Core Tables

| Table | Purpose | Phase |
|------|---------|-------|
| `games` | master game registry keyed to upstream identities | 1 |
| `odds_history` | append-only odds archive with normalized fields and raw payloads | 1 |
| `bankroll_ledger` | future capital ledger foundation | 1 |

### 6.3 Shared Later Tables

| Table | Purpose | Phase |
|------|---------|-------|
| `model_predictions` | persisted model output and feature vectors | later |
| `bets` | ticket and status state machine | later |
| `clv_log` | closing-line attribution | later |
| `situational_factors` | normalized cross-sport context and updates | later |

### 6.4 Sport-Specific Planned Tables

The four-sport direction adds dedicated stat tables after the ingestion slice is stable. Examples include:

- `mlb_pitcher_stats`, `mlb_batter_stats`, `mlb_park_factors`
- `nba_team_ratings`, `nba_player_impact`
- `nhl_team_analytics`, `nhl_goalie_stats`
- `nfl_team_epa`, `nfl_qb_metrics`

These are planned schema surfaces, not Phase 1 implementation requirements.

### 6.5 Partitioning and Read Models

`odds_history` remains partitioned by `captured_at`.

Read-path expectations:

- operational views should prefer latest-odds read models or aggregate queries
- the UI should not scan hot append-only partitions on every request
- partition maintenance can start simple in Phase 1 and become automated once retention pressure is real

---

## 7. Layer 3: Modeling

### 7.1 Registry Pattern

The model interface remains shared, but concrete model registration is sport-specific:

- `MLB`: starter and run environment models
- `NBA`: lineup-adjusted spread and total models
- `NHL`: xG and goalie models
- `NFL`: EPA/DVOA and key-number-aware spread models

### 7.2 Market-as-Prior Pattern

The recommended model structure across sports is:

1. start with the current market price
2. update with team quality
3. update with situational context
4. update with injury/personnel changes
5. compare posterior probability to the live market

This avoids the failure mode of building a fully standalone model when the market already encodes most available information.

### 7.3 Calibration and Kelly by Sport

The system should not assume one calibration cadence or Kelly fraction across all sports:

- `MLB`: faster sample accumulation, can tolerate somewhat higher Kelly ranges
- `NBA`: moderate Kelly and frequent recalibration
- `NHL`: more conservative Kelly because of scoring variance
- `NFL`: smallest Kelly and slowest validation cadence due to limited sample

---

## 8. Layer 4: Decision Engine

Later phases convert predictions into bet tickets through a shared pipeline:

- EV threshold
- line shopping
- Kelly sizing
- correlation checks
- bankroll checks
- circuit breakers

Even though decision logic is shared, the inputs are sport-aware:

- key-number treatment differs by sport
- same-game correlation risk differs by market type
- Kelly defaults vary by sport variance and volume

---

## 9. Layer 5: Execution

Execution remains book-centric rather than sport-centric:

- adapter interface
- idempotency
- verification
- audit logging
- settlement

Sport specialization affects ticket shape and market semantics, but not the need for exactly-once execution guarantees.

---

## 10. Queue Topology

River is the queueing system. The architecture should document queue classes explicitly rather than implying a single generic worker pool.

Recommended queues:

- `latency-sensitive`: odds polling and fast market reactions
- `compute`: feature building, model runs, calibration jobs
- `critical`: placement, verification, settlement, CLV capture
- `maintenance`: partition creation, cleanup, archival
- `alerting`: operator notifications

Operational expectations from River:

- register workers explicitly on startup
- use periodic jobs for schedule-driven polling
- define retry behavior per job type
- keep job uniqueness and queue boundaries intentional

Phase 1 only needs the ingestion-oriented subset of this topology.

---

## 11. Fiber Server Expectations

Fiber v3 is the target HTTP framework.

Documented server behavior:

- grouped routes for health, operational HTML, and later APIs
- template rendering for server-side operational pages
- static asset serving through Fiber's static middleware
- readiness semantics that include DB and worker dependencies
- middleware for logging, recovery, and later auth

Phase 1 route surface:

- `GET /`
- `GET /health`
- `GET /odds`
- `GET /pipeline/health`

---

## 12. Observability and Operational Safety

### Logging

- structured logs across server and worker paths
- poll counts, dedup skips, insert counts, API latency, and queue lag

### Metrics

- counters for jobs and placement attempts
- histograms for latency and run duration
- low-cardinality labels only

### Release Safety

- migrations run before queue consumers that depend on them
- schema and app changes must be documented for rollback compatibility
- financially sensitive features require paper-mode or dry-run validation before live use

---

## 13. Documentation Boundaries

- [betbot-plan.md](betbot-plan.md): product direction and roadmap
- [TRACKER.md](TRACKER.md): executable work queue
- [SPORT_OPTIMIZATION.md](SPORT_OPTIMIZATION.md): sport-by-sport specialization details
- [REPO_STRUCTURE.md](REPO_STRUCTURE.md): checked-in layout and target package responsibilities


