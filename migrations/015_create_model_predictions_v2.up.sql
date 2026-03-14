CREATE TABLE model_predictions (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    sport TEXT NOT NULL,
    book_key TEXT NOT NULL,
    market_key TEXT NOT NULL,
    model_family TEXT NOT NULL,
    model_version TEXT NOT NULL,
    manifest_version TEXT NOT NULL,
    feature_vector DOUBLE PRECISION[] NOT NULL,
    predicted_probability DOUBLE PRECISION NOT NULL CHECK (predicted_probability >= 0 AND predicted_probability <= 1),
    market_probability DOUBLE PRECISION NOT NULL CHECK (market_probability >= 0 AND market_probability <= 1),
    closing_probability DOUBLE PRECISION CHECK (closing_probability >= 0 AND closing_probability <= 1),
    event_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (
        game_id,
        source,
        book_key,
        market_key,
        model_family,
        model_version,
        event_time
    )
);

CREATE INDEX model_predictions_sport_event_idx
    ON model_predictions (sport, event_time DESC);

CREATE INDEX model_predictions_game_idx
    ON model_predictions (game_id, event_time DESC);
