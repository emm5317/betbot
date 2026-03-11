# 008 Phase 1 Vertical Slice First

## Status

Accepted

## Decision

Phase 1 will stay narrowly focused on a production-shaped ingestion slice:

- PostgreSQL 17
- `pgxpool`
- `sqlc`
- River-backed odds polling
- append-only odds storage
- minimal Fiber operational visibility

Sport-specific ETL breadth, lineup/weather ingestion, and baseline models are intentionally deferred to later phases.

## Why

- the current repo is still scaffold-level
- measurement infrastructure must exist before model quality can be evaluated
- broad early ETL work would slow delivery of the first trustworthy odds archive
- the sport-specialization strategy still benefits from a shared ingestion foundation

## Consequences

- Phase 1 docs and tracker must not over-promise sport-specific implementation breadth
- later phases explicitly own `SportConfig`, ETL expansion, and baseline models
- schema and config should remain compatible with later specialization, but should not be blocked by it
