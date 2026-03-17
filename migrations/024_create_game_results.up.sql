CREATE TABLE game_results (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    status TEXT NOT NULL,
    home_score INTEGER,
    away_score INTEGER,
    result_hash TEXT NOT NULL,
    raw_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (btrim(status) <> ''),
    UNIQUE (game_id, source, result_hash)
);

CREATE INDEX game_results_game_lookup_idx
    ON game_results (game_id, captured_at DESC, id DESC);

CREATE INDEX game_results_source_external_lookup_idx
    ON game_results (source, external_id, captured_at DESC);

CREATE INDEX game_results_final_lookup_idx
    ON game_results (game_id, captured_at DESC, id DESC)
    WHERE lower(status) = 'final';
