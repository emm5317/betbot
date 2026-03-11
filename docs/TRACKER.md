# TRACKER.md — betbot Progress Tracker

Status: `⬜ TODO` · `🔵 IN PROGRESS` · `✅ DONE` · `🔴 BLOCKED` · `⏸️ DEFERRED`

**Last updated:** 2026-03-11
**Current phase:** Phase 1 — Data Foundation Vertical Slice

---

## Current Repo State

- Project scaffold and package layout exist
- Local Docker Compose exists for `betbot` and `betbot-postgres`
- `cmd/server` runs a minimal health-checked bootstrap service
- `cmd/worker`, migrations, sqlc queries, and most ingestion/modeling packages are still placeholders
- Documentation is now aligned to the four-sport direction: `MLB`, `NBA`, `NHL`, `NFL`

The current implementation target is intentionally narrower than the full product architecture. Phase 1 focuses on one end-to-end ingestion slice before sport-specific ETL and modeling breadth.

---

## Phase 1 — Data Foundation Vertical Slice

Goal: ship a working ingestion slice with PostgreSQL 17, `pgxpool`, `sqlc`, River, The Odds API polling, append-only odds storage, and minimal Fiber operational visibility.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P1-001 | Keep scaffold aligned with canonical docs | ✅ DONE | P0 | Repo structure is in place |
| P1-002 | Upgrade local/runtime baseline to PostgreSQL 17 | ⬜ TODO | P0 | Compose, docs, and env examples must agree |
| P1-003 | Add `pgxpool` bootstrap and pool config env vars | ⬜ TODO | P0 | `AfterConnect` should set UTC |
| P1-004 | Implement `games` migration | ⬜ TODO | P0 | Include sport and external ID |
| P1-005 | Implement `odds_history` migration with partitions | ⬜ TODO | P0 | Append-only with `raw_json` and `snapshot_hash` |
| P1-006 | Implement `bankroll_ledger` migration | ⬜ TODO | P1 | Foundation only in this phase |
| P1-007 | Configure `sqlc` for PostgreSQL + `pgx/v5` | ⬜ TODO | P0 | Generate `internal/store` from migrations and queries |
| P1-008 | Write `games`, `odds_history`, `bankroll`, and dashboard queries | ⬜ TODO | P0 | Keep Phase 1 query set minimal |
| P1-009 | Wire River client and worker registration | ⬜ TODO | P0 | Phase 1 queues: ingestion and maintenance |
| P1-010 | Implement The Odds API client | ⬜ TODO | P0 | Timeout, API key config, rate limiting |
| P1-011 | Implement odds normalization | ⬜ TODO | P0 | Canonical game and market records |
| P1-012 | Implement snapshot deduplication | ⬜ TODO | P0 | Skip unchanged market rows |
| P1-013 | Implement `games` upsert from incoming odds data | ⬜ TODO | P0 | Idempotent external ID handling |
| P1-014 | Implement `OddsPollJob` | ⬜ TODO | P0 | Poll, normalize, dedup, insert, log |
| P1-015 | Replace bootstrap server with Fiber v3 | ⬜ TODO | P1 | Keep route surface intentionally small |
| P1-016 | Implement `/health` readiness semantics | ⬜ TODO | P0 | Include DB and worker dependencies |
| P1-017 | Build minimal `/odds` operational view | ⬜ TODO | P1 | Read from stored latest-odds query path |
| P1-018 | Build minimal `/pipeline/health` view | ⬜ TODO | P1 | Last successful poll, insert count, errors |
| P1-019 | Add structured logging for server and worker | ⬜ TODO | P1 | Poll metrics, dedup skips, latencies |
| P1-020 | Add unit tests for normalization and implied probability | ⬜ TODO | P0 | Include malformed payload handling |
| P1-021 | Add integration tests for insert and dedup behavior | ⬜ TODO | P1 | Duplicate snapshots should not bloat storage |
| P1-022 | Add migration/boot smoke test against Postgres 17 | ⬜ TODO | P1 | Catch schema and startup regressions early |
| P1-023 | Documentation refresh for four-sport direction | ✅ DONE | P1 | Done 2026-03-11 |

Phase 1 exit criteria:

- odds polling runs on a schedule
- `games` and `odds_history` populate continuously
- duplicate snapshots are skipped reliably
- Fiber operational views read from persisted data
- Postgres 17 + `pgxpool` + River + `sqlc` are the live baseline

Out of scope for Phase 1:

- sport-specific stats ETL
- injury/lineup/weather ingestion
- model execution
- CLV automation
- execution adapters

---

## Phase 2 — Sport Foundation

Goal: add the shared four-sport substrate required before serious baseline modeling.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P2-001 | Create `SportConfig` registry | ⬜ TODO | P0 | Seasons, cadence, key numbers, HFA |
| P2-002 | Add sport-aware scheduler behavior | ⬜ TODO | P0 | Only active sports poll and run downstream work |
| P2-003 | Design and migrate sport-specific stat tables | ⬜ TODO | P0 | MLB, NBA, NHL, NFL tables |
| P2-004 | Implement `MLBStatsETLJob` | ⬜ TODO | P0 | Baseball Savant and supporting sources |
| P2-005 | Implement `NBAStatsETLJob` | ⬜ TODO | P0 | NBA Stats API and player impact sources |
| P2-006 | Implement `NHLStatsETLJob` | ⬜ TODO | P1 | NHL analytics and goalie data |
| P2-007 | Implement `NFLStatsETLJob` | ⬜ TODO | P1 | nflverse and supporting sources |
| P2-008 | Implement injury and lineup ingestion | ⬜ TODO | P0 | Rotowire, Daily Faceoff, confirmations |
| P2-009 | Implement weather ingestion for outdoor sports | ⬜ TODO | P1 | MLB and NFL first |
| P2-010 | Add operator-facing sport filters to read views | ⬜ TODO | P2 | Keep views usable as breadth grows |

Phase 2 exit criteria:

- all four sports have declared config
- sport-specific ETL foundations exist
- lineup/injury/weather inputs are stored for downstream use

---

## Phase 3 — Baseline Models and Backtesting

Goal: build sport-specific baseline models and validate them offline before any execution work.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P3-001 | Build MLB pitcher matchup model | ⬜ TODO | P0 | Moneyline, total, and F5 orientation |
| P3-002 | Build NBA lineup-adjusted net rating model | ⬜ TODO | P0 | Spread and total first |
| P3-003 | Build NHL xG plus goalie model | ⬜ TODO | P1 | PDO regression support |
| P3-004 | Build NFL EPA/DVOA situational model | ⬜ TODO | P1 | Key-number awareness required |
| P3-005 | Implement sport-specific feature builders | ⬜ TODO | P0 | Shared interface, specialized inputs |
| P3-006 | Implement model persistence in `model_predictions` | ⬜ TODO | P0 | Version and feature vector storage |
| P3-007 | Build backtesting CLI | ⬜ TODO | P0 | Replay against stored odds |
| P3-008 | Add walk-forward validation | ⬜ TODO | P0 | Prevent look-ahead bias |
| P3-009 | Add CLV and calibration reporting | ⬜ TODO | P0 | Sport-aware reporting cadence |
| P3-010 | Add sport-specific Kelly defaults | ⬜ TODO | P1 | Variance-aware bankroll policy |

Phase 3 exit criteria:

- each prioritized sport has a baseline model
- backtests run on historical data without leakage
- calibration and CLV reporting are available for review

---

## Phase 4 — Decision Engine

Goal: turn model output into risk-checked bet tickets.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P4-001 | Implement EV threshold filter | ⬜ TODO | P0 | Shared rule, sport-aware tuning |
| P4-002 | Implement line shopping | ⬜ TODO | P0 | Best available odds across books |
| P4-003 | Implement Kelly sizer | ⬜ TODO | P0 | Fractional and capped |
| P4-004 | Implement bankroll availability checks | ⬜ TODO | P0 | Ledger-backed |
| P4-005 | Implement correlation guard | ⬜ TODO | P0 | Same-game exposure control |
| P4-006 | Implement circuit breakers | ⬜ TODO | P0 | Daily, weekly, drawdown |
| P4-007 | Build decision-engine integration tests | ⬜ TODO | P1 | Prediction to ticket flow |

---

## Phase 5 — Execution and Paper Validation

Goal: add exactly-once execution semantics and prove the pipeline in paper mode.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P5-001 | Define `BookAdapter` interface | ⬜ TODO | P0 | Shared across books |
| P5-002 | Implement paper adapter | ⬜ TODO | P0 | First execution target |
| P5-003 | Implement placement idempotency and locking | ⬜ TODO | P0 | Financial safety first |
| P5-004 | Implement placement audit trail | ⬜ TODO | P0 | Full request/response metadata |
| P5-005 | Implement settlement and CLV capture | ⬜ TODO | P1 | End-to-end paper accounting |
| P5-006 | Run sustained paper-mode validation | ⬜ TODO | P0 | Confirm no duplicate placement path |

---

## Phase 6 — Live Validation and Expansion

Goal: validate edge with constrained capital and iterate safely.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P6-001 | Enable first live adapter after paper validation | ⬜ TODO | P0 | Roll out conservatively |
| P6-002 | Monitor CLV and calibration by sport | ⬜ TODO | P0 | Do not scale without signal |
| P6-003 | Add sharper odds sources where justified | ⬜ TODO | P1 | Pinnacle, OddsJam, OpticOdds evaluation |
| P6-004 | Introduce ML sidecar where baseline models plateau | ⬜ TODO | P1 | Only after measurement is solid |
| P6-005 | Expand sport-specific prop models | ⬜ TODO | P2 | After game-market process is stable |
