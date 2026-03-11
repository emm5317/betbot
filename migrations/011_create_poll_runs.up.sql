CREATE TABLE poll_runs (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'running',
    games_seen INTEGER NOT NULL DEFAULT 0,
    snapshots_seen INTEGER NOT NULL DEFAULT 0,
    inserts_count INTEGER NOT NULL DEFAULT 0,
    dedup_skips INTEGER NOT NULL DEFAULT 0,
    error_text TEXT NOT NULL DEFAULT ''
);

CREATE INDEX poll_runs_started_at_idx
    ON poll_runs (started_at DESC);
