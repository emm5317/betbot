CREATE TABLE recommendation_snapshots (
    id BIGSERIAL PRIMARY KEY,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sport TEXT NOT NULL,
    game_id BIGINT NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    event_time TIMESTAMPTZ NOT NULL,
    event_date DATE NOT NULL,
    market_key TEXT NOT NULL,
    recommended_side TEXT NOT NULL CHECK (recommended_side IN ('home', 'away')),
    best_book TEXT NOT NULL,
    best_american_odds INTEGER NOT NULL CHECK (best_american_odds <= -100 OR best_american_odds >= 100),
    model_probability DOUBLE PRECISION NOT NULL CHECK (model_probability >= 0 AND model_probability <= 1),
    market_probability DOUBLE PRECISION NOT NULL CHECK (market_probability >= 0 AND market_probability <= 1),
    edge DOUBLE PRECISION NOT NULL CHECK (edge >= 0 AND edge <= 1),
    suggested_stake_fraction DOUBLE PRECISION NOT NULL CHECK (suggested_stake_fraction >= 0 AND suggested_stake_fraction <= 1),
    suggested_stake_cents BIGINT NOT NULL CHECK (suggested_stake_cents >= 0),
    bankroll_check_pass BOOLEAN NOT NULL,
    bankroll_check_reason TEXT NOT NULL,
    rank_score DOUBLE PRECISION NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX recommendation_snapshots_event_idx
    ON recommendation_snapshots (event_date, sport, rank_score DESC, generated_at DESC);

CREATE INDEX recommendation_snapshots_generated_idx
    ON recommendation_snapshots (generated_at DESC);
