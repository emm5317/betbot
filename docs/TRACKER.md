# TRACKER.md — betbot Progress Tracker

Status: `⬜ TODO` · `🔵 IN PROGRESS` · `✅ DONE` · `🔴 BLOCKED` · `⏸️ DEFERRED`

**Last updated:** 2026-03-18
**Current phase:** Phase 4 — Decision Engine

---

## Current Repo State

- Project scaffold and package layout exist
- Local Docker Compose exists for `betbot` and `betbot-postgres`
- `cmd/server` now runs a Fiber operational surface backed by pgxpool reads
- `cmd/worker`, core migrations, odds polling, and operational views are now wired for the completed Phase 1 slice
- `internal/store` is now generated from `sqlc` query sources and used by the app surface
- `internal/domain` now exposes a concrete `SportConfig` registry for MLB/NBA/NHL/NFL
- Worker scheduling now filters odds polling to sports active in the current season
- Local/dev runtime now supports explicit odds polling disable mode plus placeholder-key auto-guard so compose health stays green without a live Odds API key
- Operator read views now support explicit sport filters (all sports + canonical MLB/NBA/NHL/NFL keys) for `/`, `/odds`, and `/pipeline/health`
- Minimal sport-stat schema is now live: team tables for MLB/NBA/NHL/NFL plus MLB pitcher, NHL goalie, and NFL QB foundations
- MLB stats ETL now has a live MLB Stats API provider, explicit River enqueue path, and sqlc-backed team and pitcher upserts
- NBA stats ETL now has a live stats.nba.com provider path, explicit River enqueue helper, and idempotent nba_team_stats upserts
- Phase 1 integration coverage now includes insert/dedup behavior and Postgres 17 boot smoke
- NHL stats ETL now has a first-pass live team path via the official NHL web API; goalie writes remain deferred pending a date-consistent goalie source
- NFL stats ETL now has a live merged provider path using nflverse team stats plus official NFL standings, explicit River enqueue wiring, and idempotent nfl_team_stats upserts; nfl_qb_stats remain deferred in this phase
- First-pass injury ingestion now persists Rotowire NFL injury availability records via an explicit worker and sqlc-backed upserts; lineup confirmations remain deferred in this phase
- First-pass weather ingestion now persists explicit game-linked MLB and NFL weather snapshots via Open-Meteo plus in-repo venue metadata; roof handling is now policy-driven across outdoor, fixed indoor, and retractable-unknown branches, with explicit date-bounded venue exceptions for Athletics naming variants
- Deterministic local weather smoke harness now exists for MLB/NFL roof-policy coverage, including River enqueue/completion checks and idempotent rerun assertions
- Documentation is now aligned to the four-sport direction: `MLB`, `NBA`, `NHL`, `NFL`
- `model_predictions` persistence is now live with manifest-backed stable feature indexing (`feature_vector` ordering guaranteed by `internal/modeling/features` manifests)
- `cmd/backtest` now runs an end-to-end deterministic replay from stored odds, persists model outputs, and emits `pipeline_report.json` plus `outcomes.csv` artifacts
- Walk-forward validation and CLV/calibration reporting are now emitted together in one pipeline output
- NHL and NFL Phase 3 baseline model packages are now implemented with bounded-output unit coverage
- Sport-specific Kelly defaults are now explicit policy values for MLB/NBA/NHL/NFL and are wired into replay stake recommendations plus decision sizing defaults
- Recommendation-only decision surface is now live via `GET /recommendations`, with ranked best-bet suggestions built from EV thresholding, line shopping, Kelly sizing, and ledger-backed bankroll checks, plus append-only recommendation snapshots for audit replay
- Recommendation monitoring is now live via `GET /recommendations/performance`, with append-only `recommendation_outcomes` persistence, CLV delta capture against close, explicit unavailable/pending/settled statusing, and operator summary metrics (count, avg edge, avg CLV, bankroll pass rate, settled count)
- Recommendation calibration monitoring is now live via `GET /recommendations/calibration`, with deterministic rank-percentile buckets, settled/excluded accounting, per-bucket observed vs expected win rates, Brier scores, mean CLV, and overall ECE (done 2026-03-14)
- Recommendation calibration drift alerting is now live via `GET /recommendations/calibration/alerts`, with sport-scoped baseline/current window comparison, minimum-sample guardrails, deterministic alert reason ordering, and per-bucket calibration-gap/Brier deltas (done 2026-03-14)
- Recommendation calibration drift history and rolling trend visibility are now live via `GET /recommendations/calibration/alerts/history` plus rolling mode on `GET /recommendations/calibration/alerts`, with append-only alert-run persistence, deterministic trend ordering, and auditable per-step metadata (done 2026-03-14)
- Recommendation stake sizing now computes deterministic odds-aware Kelly fractions with hard cap + ledger-backed bankroll gating in `GET /recommendations`, including raw/applied/capped fractions, pre/post bankroll stake values, and deterministic sizing reasons (done 2026-03-14)
- Recommendation correlation guard now enforces deterministic same-game exposure caps in `GET /recommendations` with audit fields (`correlation_check_pass`, `correlation_check_reason`, `correlation_group_key`) and persisted snapshot metadata blocks (done 2026-03-14)
- Recommendation circuit breaker gating now enforces deterministic daily/weekly/drawdown loss stops in `GET /recommendations` from ledger-derived balances, with per-row audit fields (`circuit_check_pass`, `circuit_check_reason`) and persisted snapshot metadata blocks (done 2026-03-15)

- Live prediction bridge now connects offline models to the recommendation surface: `internal/prediction/nhl.go` runs the XGGoalieModel on upcoming games every 15 minutes via a River periodic job, persists to `model_predictions` with `source="live"`, and `GET /recommendations` joins those predictions to upcoming odds via `ListLatestOddsForUpcoming`. Manual trigger available at `POST /predictions/run`. Pattern documented in CLAUDE.md for adding new sports.
- Execution layer foundations are now in place: `BookAdapter` interface, paper adapter, `PlacementOrchestrator` with idempotency + locking, settlement with CLV capture, audit trail, `bets` table migration, and `POST /execution/place` + `GET /execution/bets` server endpoints.
- Recommendation auto-placement is now live: a River periodic worker (`auto_placement`) runs every 15 minutes (and on startup), reads unplaced recommendation snapshots in deterministic rank order, and places via the shared `PlacementOrchestrator` path using deterministic snapshot-scoped idempotency keys (done 2026-03-15).
- Manual bet tracking now includes automatic settlement: a River periodic worker (`auto_settlement`) runs every 30 minutes, fetches completed game scores from The Odds API, settles matching open `h2h` bets deterministically, and writes settlement ledger entries via shared execution settlement/audit helpers (done 2026-03-15).
- Free-source MLB historical import path is now scaffolded for 2025-first replay: local XLSX odds import (`scripts/import_historical_odds.py`), MLB-StatsAPI outcomes import (`scripts/import_game_results.py`), normalized scraper CSV wrapper (`scripts/import_scraped_odds.py`), and pybaseball team/pitcher stat importer (`scripts/import_mlb_features_pybaseball.py`) (done 2026-03-16).
- Odds-mode backtest replay now joins latest final `game_results` when available and emits explicit `outcome_calibration` metrics plus per-row actual-score fields in `outcomes.csv` artifacts (done 2026-03-16).
- Execution runtime guardrails now require explicit adapter selection (`BETBOT_EXECUTION_ADAPTER`) and disable auto-placement by default when `BETBOT_PAPER_MODE=false`, so live rollout starts fail-closed and manual-first (done 2026-03-18).

The current implementation target is Phase 5 sustained paper-mode validation — recommendation→placement→settlement is now automated in paper mode, and remaining work is sustained validation and operational hardening.

---

## Phase 1 — Data Foundation Vertical Slice

Goal: ship a working ingestion slice with PostgreSQL 17, `pgxpool`, `sqlc`, River, The Odds API polling, append-only odds storage, and minimal Fiber operational visibility.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P1-001 | Keep scaffold aligned with canonical docs | ✅ DONE | P0 | Repo structure is in place |
| P1-002 | Upgrade local/runtime baseline to PostgreSQL 17 | ✅ DONE | P0 | Compose, docs, and env examples updated |
| P1-003 | Add `pgxpool` bootstrap and pool config env vars | ✅ DONE | P0 | `AfterConnect` sets UTC |
| P1-004 | Implement `games` migration | ✅ DONE | P0 | Includes sport and external ID |
| P1-005 | Implement `odds_history` migration with partitions | ✅ DONE | P0 | Append-only with `raw_json` and `snapshot_hash` |
| P1-006 | Implement `bankroll_ledger` migration | ✅ DONE | P1 | Foundation only in this phase |
| P1-007 | Configure `sqlc` for PostgreSQL + `pgx/v5` | ✅ DONE | P0 | Generated store layer is live and replaces the handwritten fallback |
| P1-008 | Write `games`, `odds_history`, `bankroll`, and dashboard queries | ✅ DONE | P0 | Minimal Phase 1 query set is live |
| P1-009 | Wire River client and worker registration | ✅ DONE | P0 | Queues: ingestion and maintenance |
| P1-010 | Implement The Odds API client | ✅ DONE | P0 | Timeout, API key config, rate limiting |
| P1-011 | Implement odds normalization | ✅ DONE | P0 | Canonical game and market records |
| P1-012 | Implement snapshot deduplication | ✅ DONE | P0 | Skip unchanged market rows |
| P1-013 | Implement `games` upsert from incoming odds data | ✅ DONE | P0 | Idempotent external ID handling |
| P1-014 | Implement `OddsPollJob` | ✅ DONE | P0 | Poll, normalize, dedup, insert, log |
| P1-015 | Replace bootstrap server with Fiber v3 | ✅ DONE | P1 | Route surface remains intentionally small |
| P1-016 | Implement `/health` readiness semantics | ✅ DONE | P0 | Includes DB and worker dependencies |
| P1-017 | Build minimal `/odds` operational view | ✅ DONE | P1 | Reads from stored latest-odds path |
| P1-018 | Build minimal `/pipeline/health` view | ✅ DONE | P1 | Last successful poll, insert count, and errors |
| P1-019 | Add structured logging for server and worker | ✅ DONE | P1 | Poll metrics, dedup skips, and latencies are logged |
| P1-020 | Add unit tests for normalization and implied probability | ✅ DONE | P0 | Core normalization and implied probability coverage added |
| P1-021 | Add integration tests for insert and dedup behavior | ✅ DONE | P1 | `internal/integration` covers duplicate-skip and changed-snapshot insert flow |
| P1-022 | Add migration/boot smoke test against Postgres 17 | ✅ DONE | P1 | Smoke test provisions a clean Postgres 17 database and boots server + worker |
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
| P2-001 | Create `SportConfig` registry | ✅ DONE | P0 | Registry now captures seasons, cadence, market anchors, HFA, and model posture |
| P2-002 | Add sport-aware scheduler behavior | ✅ DONE | P0 | Worker now enqueues odds poll jobs with active sport keys only |
| P2-003 | Design and migrate sport-specific stat tables | ✅ DONE | P0 | Added minimal team foundations for all four sports plus MLB pitcher, NHL goalie, and NFL QB tables |
| P2-004 | Implement `MLBStatsETLJob` | ✅ DONE | P0 | Real MLB Stats API provider plus explicit enqueue path now back the MLB team and pitcher ETL |
| P2-005 | Implement `NBAStatsETLJob` | ✅ DONE | P0 | NBA ETL now uses a live stats.nba.com provider for `nba_team_stats`, plus worker, enqueue helper, and idempotent integration coverage |
| P2-006 | Implement `NHLStatsETLJob` | ✅ DONE | P1 | First-pass NHL ETL now writes `nhl_team_stats` from the official NHL web API; goalie writes are deferred until a date-consistent source is selected |
| P2-007 | Implement `NFLStatsETLJob` | ✅ DONE | P1 | Live first-pass NFL ETL now writes `nfl_team_stats` from nflverse team stats merged with official NFL standings; `nfl_qb_stats` remain deferred pending a clean success-rate-capable source |
| P2-008 | Implement injury and lineup ingestion | ✅ DONE | P0 | First pass now persists Rotowire NFL injury availability records; broad lineup-confirmation coverage remains deferred until a credible machine-readable source is selected |
| P2-009 | Implement weather ingestion for outdoor sports | ✅ DONE | P1 | Open-Meteo-backed first pass now persists game-linked MLB and NFL weather rows; roofed venues are explicit and broader venue cases remain deferred |
| P2-010b | Tighten retractable-roof and venue exception policy for persisted weather ingestion | ✅ DONE | P1 | Weather ingest now uses explicit roof-policy branches (outdoor fetch, fixed-indoor placeholder, retractable-unknown placeholder) and a shared Athletics/Oakland Athletics date override for the temporary Las Vegas venue window |
| P2-010c | Build reproducible local weather smoke harness (MLB/NFL roof policy) | ✅ DONE | P1 | Added deterministic River-backed smoke test plus scripts/weather_smoke.ps1 runner and runbook checks for outdoor, dome, retractable, job completion, and idempotent rerun behavior |
| P2-010d | Make local compose health-green without live Odds API key | ✅ DONE | P1 | Added explicit odds polling disable switch, placeholder-key guard, worker scheduling skip path, and health/read-view semantics for intentional local polling disablement |
| P2-010 | Add operator-facing sport filters to read views | ✅ DONE | P2 | Read views now accept explicit canonical sport filter keys with 400 responses for invalid filters and scoped odds archive metrics on home/pipeline |

Phase 2 exit criteria:

- all four sports have declared config
- sport-specific ETL foundations exist
- lineup/injury/weather inputs are stored for downstream use

---

## Phase 3 — Baseline Models and Backtesting

Goal: build sport-specific baseline models and validate them offline before any execution work.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P3-001 | Build MLB pitcher matchup model | ✅ DONE | P0 | Added validated baseline predictor in `internal/modeling/mlb` with starter + team context outputs for full-game moneyline/total and first-five side/total orientation (done 2026-03-13) |
| P3-002 | Build NBA lineup-adjusted net rating model | ✅ DONE | P0 | Added validated baseline predictor in `internal/modeling/nba` with lineup-availability net-rating adjustments plus margin/total, win, and spread-cover outputs (done 2026-03-13) |
| P3-003 | Build NHL xG plus goalie model | ✅ DONE | P1 | Added `internal/modeling/nhl` xG+goalie baseline with explicit PDO-regression pressure, bounded probability outputs, and unit validation coverage (done 2026-03-14) |
| P3-004 | Build NFL EPA/DVOA situational model | ✅ DONE | P1 | Added `internal/modeling/nfl` EPA/DVOA situational baseline with key-number-aware spread-cover adjustment, wind handling, and unit validation coverage (done 2026-03-14) |
| P3-005 | Implement sport-specific feature builders | ✅ DONE | P0 | Added deterministic shared builder contract + registry in `internal/modeling/features` with validated MLB/NBA/NHL/NFL feature builders covering market priors, team quality, situational, injury/weather, and sport-specific contexts (done 2026-03-13) |
| P3-006 | Implement model persistence in `model_predictions` | ✅ DONE | P0 | Added migration `015_create_model_predictions_v2`, sqlc prediction queries, and `internal/modeling.PersistPrediction` manifest-backed vector encoding/persistence wiring (done 2026-03-14) |
| P3-007 | Build backtesting CLI | ✅ DONE | P0 | Implemented `cmd/backtest` and `internal/backtest` deterministic replay pipeline consuming stored odds snapshots, dispatching baseline models, persisting predictions, and writing JSON/CSV artifacts (done 2026-03-14) |
| P3-008 | Add walk-forward validation | ✅ DONE | P0 | Backtest pipeline now emits deterministic no-look-ahead walk-forward folds over replay outcomes with per-fold CLV/calibration metrics (done 2026-03-14) |
| P3-009 | Add CLV and calibration reporting | ✅ DONE | P0 | Backtest pipeline now emits unified CLV + calibration reports and per-sport calibration artifacts from one run output (`pipeline_report.json` + `outcomes.csv`) (done 2026-03-14) |
| P3-010 | Add sport-specific Kelly defaults | ✅ DONE | P1 | Added explicit MLB/NBA/NHL/NFL Kelly + per-bet cap defaults, wired into backtest virtual bankroll and shared decision sizing with deterministic bounds coverage (done 2026-03-14) |

Phase 3 exit criteria:

- each prioritized sport has a baseline model
- backtests run on historical data without leakage
- calibration and CLV reporting are available for review in a single replay artifact output

---

## Phase 4 — Decision Engine

Goal: turn model output into risk-checked bet tickets.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P4-001 | Implement EV threshold filter | ✅ DONE | P0 | Shared rule with sport-aware defaults, override support, and decision-engine filter wiring (done 2026-03-14) |
| P4-002 | Implement line shopping | ✅ DONE | P0 | Best available odds across books implemented in `internal/decision/lineshopper.go` with deterministic tie-break and validation coverage (done 2026-03-14) |
| P4-003 | Implement Kelly sizer | ✅ DONE | P0 | Added deterministic odds-aware Kelly sizing with sport defaults + optional overrides, exposing raw/applied/capped fractions and deterministic reason codes in recommendation responses (done 2026-03-14) |
| P4-004 | Implement bankroll availability checks | ✅ DONE | P0 | Added ledger-backed bankroll gate on recommendation sizing, deterministic insufficient-funds handling via available-balance cap, and persisted sizing audit metadata on snapshots (done 2026-03-14) |
| P4-005 | Implement correlation guard | ✅ DONE | P0 | Deterministic same-game exposure control via max picks + max summed stake fraction, optional sport/day cap, response audit fields, and snapshot metadata persistence (done 2026-03-14) |
| P4-006 | Implement circuit breakers | ✅ DONE | P0 | Deterministic recommendation gating on daily loss stop, weekly loss stop, and peak drawdown from ledger-derived balances, with response/snapshot audit fields (done 2026-03-15) |
| P4-007 | Build decision-engine integration tests | ✅ DONE | P1 | Added deterministic decision/server integration coverage for EV->line->sizing->correlation->circuit flow, tie-order invariants, and retained-only snapshot metadata assertions (done 2026-03-15) |
| P4-008 | Build recommendation-only best-bets pull surface | ✅ DONE | P0 | Added `GET /recommendations` with sport/date/limit filters, ranked decision assembly, and append-only `recommendation_snapshots` persistence (done 2026-03-14) |
| P4-009 | Add recommendation performance + CLV monitoring surface | ✅ DONE | P0 | Added append-only `recommendation_outcomes`, pure CLV/outcome computation, and `GET /recommendations/performance` with filter validation, deterministic rows, and aggregate summary metrics (done 2026-03-14) |
| P4-010 | Add recommendation calibration monitoring by sport + rank bucket | ✅ DONE | P0 | Added `GET /recommendations/calibration` with filter echo, deterministic rank-percentile bucketing (1..20), settled/excluded handling, per-bucket observed/expected rates, calibration gap, Brier, mean CLV, plus overall observed/expected, Brier, and ECE summary metrics (done 2026-03-14) |
| P4-011 | Add recommendation calibration drift alerts with sample guardrails | ✅ DONE | P0 | Added `GET /recommendations/calibration/alerts` with current vs baseline windows, configurable thresholds, minimum settled overall/per-bucket guardrails, deterministic reason ordering, and per-bucket delta metrics (done 2026-03-14) |
| P4-012 | Add calibration drift history and rolling trend windows | ✅ DONE | P0 | Added append-only `recommendation_calibration_alert_runs` persistence, `GET /recommendations/calibration/alerts/history`, and `mode=rolling` support on `GET /recommendations/calibration/alerts` with deterministic per-step trend rows and persisted run metadata (done 2026-03-14) |
| P4-013 | Build live prediction bridge (NHL first) | ✅ DONE | P0 | Added `internal/prediction` package with `NHLPredictionService`, River periodic job (15-min), `POST /predictions/run` manual trigger, `ListUpcomingGamesForSport` + `GetLatestMarketProbabilityForGame` + `ListLatestOddsForUpcoming` sqlc queries, and server wiring so `GET /recommendations` returns live NHL candidates. Validated end-to-end with real Odds API data. Pattern is documented for adding new sports. (done 2026-03-15) |

---

## Phase 5 — Execution and Paper Validation

Goal: add exactly-once execution semantics and prove the pipeline in paper mode.

| ID | Task | Status | Priority | Notes |
|----|------|--------|----------|-------|
| P5-001 | Define `BookAdapter` interface | ✅ DONE | P0 | `internal/execution/adapter.go` with `PlaceBet` + `CheckBetStatus` contract shared across books (done 2026-03-15) |
| P5-002 | Implement paper adapter | ✅ DONE | P0 | `internal/execution/adapters/paper/` with deterministic accept behavior and unit tests (done 2026-03-15) |
| P5-003 | Implement placement idempotency and locking | ✅ DONE | P0 | `internal/execution/idempotency.go` + `placement.go` with `PlacementOrchestrator`, idempotency key generation, distributed locking, and read-back verification (done 2026-03-15) |
| P5-004 | Implement placement audit trail | ✅ DONE | P0 | `internal/execution/audit.go` with full request/response metadata persistence, wired to `POST /execution/place` and `GET /execution/bets` endpoints (done 2026-03-15) |
| P5-005 | Implement settlement and CLV capture | ✅ DONE | P1 | `internal/execution/settlement.go` + `clvcapture.go` with closing-line delta computation and unit tests (done 2026-03-15) |
| P5-006 | Run sustained paper-mode validation | ⬜ TODO | P0 | Auto-placement and auto-settlement River jobs are now live; remaining work is sustained runbook validation, monitoring review cadence, and threshold tuning under paper traffic |

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











