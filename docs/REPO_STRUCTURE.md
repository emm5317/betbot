# Repository Structure — betbot

This document describes the checked-in repository layout and the intended responsibility of each major area. It is deliberately higher signal than the previous file-by-file dump so it stays truthful as the repo evolves.

Important: many packages already exist in the tree but are still placeholders. Presence of a path does not imply that subsystem is implemented.

---

## 1. Top-Level Layout

```text
betbot/
├── cmd/              # binary entrypoints
├── internal/         # application code
├── sql/              # sqlc query source
├── migrations/       # append-only schema migrations
├── proto/            # gRPC contracts for the Python sidecar
├── ml/               # Python ML sidecar
├── templates/        # server-rendered HTML templates
├── static/           # static assets served by Fiber
├── testdata/         # fixtures and golden datasets
├── scripts/          # utility scripts
├── deploy/           # Docker, systemd, nginx
└── docs/             # planning and architecture docs
```

---

## 2. Current Truth vs Intended Shape

| Area | Current truth | Intended shape |
|------|---------------|----------------|
| `cmd/server` | minimal bootstrap server | Fiber app with health and operational views |
| `cmd/worker` | placeholder | River-backed worker process |
| `internal/store` | generated/stub structure present | `sqlc`-generated access layer used by all app code |
| `internal/ingestion/*` | mostly placeholders | odds poller plus sport-specific ETL workers |
| `internal/modeling/*` | placeholders | sport-specific feature builders and baseline models |
| `templates/` and `static/` | layout exists | minimal Phase 1 operational views, later richer dashboard |
| `deploy/` | local runtime baseline exists | production-oriented app, worker, and ML deployment paths |

Canonical planning references:

- [Plan](betbot-plan.md)
- [Architecture](ARCHITECTURE.md)
- [Tracker](TRACKER.md)

---

## 3. `cmd/` Entry Points

| Path | Responsibility | Notes |
|------|----------------|-------|
| `cmd/server` | HTTP service entrypoint | current code is bootstrap-level |
| `cmd/worker` | background jobs entrypoint | target runtime is River |
| `cmd/backtest` | offline replay engine | intentionally separate binary |

No business logic should live in `cmd/`; only wiring and startup.

---

## 4. `internal/` Package Responsibilities

### Domain

- `internal/domain`
- pure business types
- should remain infrastructure-light
- includes sport identity and later `SportConfig` policy

### Ingestion

- `internal/ingestion/oddspoller`
- `internal/ingestion/statsetl`
- `internal/ingestion/injuries`
- `internal/ingestion/weather`

Direction:

- Phase 1 implements odds polling first
- later phases add sport-specific ETL and situational inputs

### Modeling

- `internal/modeling`
- `internal/modeling/elo`
- `internal/modeling/features`

Direction:

- shared interfaces
- sport-specific feature builders and model implementations for MLB/NBA/NHL/NFL

### Decision and Execution

- `internal/decision`
- `internal/execution`

These remain later-phase packages. Their presence in the tree is not evidence of complete implementation.

### Store and Runtime Wiring

- `internal/store`
- `internal/server`
- `internal/worker`
- `internal/config`
- `internal/backtest`
- `internal/alerts`

Intent:

- `internal/store` is the `sqlc` output target
- `internal/server` holds Fiber setup, routes, and handlers
- `internal/worker` holds River client and scheduler wiring
- `internal/config` owns `BETBOT_` configuration loading

---

## 5. Database and Query Sources

| Path | Responsibility |
|------|----------------|
| `migrations/` | append-only database schema history |
| `sql/queries/` | hand-written SQL used by `sqlc` |
| `sql/sqlc.yaml` | generation config |

Rules:

- edit queries in `sql/queries/`, not in generated store files
- edit schema via new migrations, not by changing old live-applied migrations
- generated files under `internal/store` should not be hand-edited

---

## 6. ML and RPC Boundary

| Path | Responsibility |
|------|----------------|
| `proto/model/v1` | Go/Python gRPC contract |
| `ml/` | Python sidecar and training code |

The ML sidecar is a later-phase component. betbot should remain operational without it during the initial ingestion and baseline modeling phases.

---

## 7. UI and Assets

| Path | Responsibility |
|------|----------------|
| `templates/layouts` | shared HTML shells |
| `templates/pages` | full pages |
| `templates/partials` | HTMX fragments |
| `static/css` | app styles |
| `static/js` | client-side behavior |
| `static/fonts` | vendored typography |

Phase 1 expectation:

- keep UI scope small
- operational views first
- avoid designing the full dashboard before the data pipeline exists

---

## 8. Fixtures, Scripts, and Deploy

### Fixtures

- `testdata/odds_snapshots`
- `testdata/backtest`
- `testdata/elo`
- `testdata/placement`

### Scripts

- import and export helpers
- partition helpers
- bankroll seeding helpers

### Deploy

Actual checked-in deployment names are:

- `deploy/systemd/betbot-server.service`
- `deploy/systemd/betbot-worker.service`
- `deploy/systemd/betbot-ml.service`
- `deploy/nginx/betbot.conf`

The old `tradebot` naming in documentation is retired.

---

## 9. Documentation Set

| Path | Responsibility |
|------|----------------|
| `docs/betbot-plan.md` | canonical product plan and phase order |
| `docs/ARCHITECTURE.md` | technical architecture reference |
| `docs/TRACKER.md` | executable implementation tracker |
| `docs/SPORT_OPTIMIZATION.md` | four-sport specialization deep dive |
| `docs/SOUL.md` | product voice and operator posture |
| `docs/decisions/` | ADRs for major project choices |

---

## 10. Naming and Scope Conventions

- product name: `betbot`
- near-term sport scope: `MLB`, `NBA`, `NHL`, `NFL`
- generic or soccer-oriented language is out of near-term scope
- package presence does not imply implementation completeness
- docs should distinguish current baseline from target architecture whenever that distinction matters
