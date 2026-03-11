CREATE TABLE odds_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY,
    game_id BIGINT NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    book_key TEXT NOT NULL,
    book_name TEXT NOT NULL,
    market_key TEXT NOT NULL,
    market_name TEXT NOT NULL,
    outcome_name TEXT NOT NULL,
    outcome_side TEXT NOT NULL,
    price_american INTEGER NOT NULL,
    point DOUBLE PRECISION,
    implied_probability DOUBLE PRECISION NOT NULL,
    snapshot_hash TEXT NOT NULL,
    raw_json JSONB NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (captured_at, id)
) PARTITION BY RANGE (captured_at);

CREATE INDEX odds_history_latest_lookup_idx
    ON odds_history (game_id, source, book_key, market_key, outcome_name, outcome_side, captured_at DESC);

CREATE INDEX odds_history_captured_at_idx
    ON odds_history (captured_at DESC);

CREATE OR REPLACE FUNCTION ensure_odds_history_partition(partition_start DATE)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    partition_end DATE := (partition_start + INTERVAL '1 month')::DATE;
    partition_name TEXT := format('odds_history_%s', to_char(partition_start, 'YYYYMM'));
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF odds_history FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        partition_start,
        partition_end
    );
END;
$$;

SELECT ensure_odds_history_partition(date_trunc('month', NOW())::DATE);
SELECT ensure_odds_history_partition((date_trunc('month', NOW()) + INTERVAL '1 month')::DATE);
