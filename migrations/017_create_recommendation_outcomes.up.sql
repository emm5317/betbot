CREATE TABLE recommendation_outcomes (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES recommendation_snapshots (id) ON DELETE CASCADE,
    evaluation_status TEXT NOT NULL CHECK (evaluation_status IN ('close_unavailable', 'pending_outcome', 'settled')),
    close_american_odds INTEGER CHECK (close_american_odds IS NULL OR close_american_odds <= -100 OR close_american_odds >= 100),
    close_probability DOUBLE PRECISION CHECK (close_probability IS NULL OR (close_probability >= 0 AND close_probability <= 1)),
    realized_result TEXT CHECK (realized_result IS NULL OR realized_result IN ('win', 'loss', 'push', 'unknown')),
    clv_delta DOUBLE PRECISION,
    settled_at TIMESTAMPTZ,
    notes TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX recommendation_outcomes_snapshot_idx
    ON recommendation_outcomes (snapshot_id, created_at DESC, id DESC);

CREATE INDEX recommendation_outcomes_status_idx
    ON recommendation_outcomes (evaluation_status, created_at DESC, id DESC);
