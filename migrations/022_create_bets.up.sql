CREATE TYPE bet_status AS ENUM ('pending', 'placed', 'settled', 'failed', 'voided');

CREATE TABLE bets (
    id              BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT        NOT NULL UNIQUE,
    snapshot_id     BIGINT      NOT NULL REFERENCES recommendation_snapshots (id),
    game_id         BIGINT      NOT NULL REFERENCES games (id),
    sport           TEXT        NOT NULL,
    market_key      TEXT        NOT NULL,
    recommended_side TEXT       NOT NULL CHECK (recommended_side IN ('home', 'away')),
    book_key        TEXT        NOT NULL,
    american_odds   INTEGER     NOT NULL CHECK (american_odds <= -100 OR american_odds >= 100),
    stake_cents     BIGINT      NOT NULL CHECK (stake_cents > 0),
    model_probability   DOUBLE PRECISION NOT NULL CHECK (model_probability > 0 AND model_probability < 1),
    market_probability  DOUBLE PRECISION NOT NULL CHECK (market_probability > 0 AND market_probability < 1),
    edge            DOUBLE PRECISION NOT NULL CHECK (edge >= 0),
    status          bet_status  NOT NULL DEFAULT 'pending',
    external_bet_id TEXT,
    adapter_name    TEXT        NOT NULL,
    placed_at       TIMESTAMPTZ,
    settled_at      TIMESTAMPTZ,
    settlement_result TEXT      CHECK (settlement_result IS NULL OR settlement_result IN ('win', 'loss', 'push')),
    payout_cents    BIGINT,
    clv_delta       DOUBLE PRECISION,
    closing_probability DOUBLE PRECISION,
    error_message   TEXT,
    metadata        JSONB       NOT NULL DEFAULT '{}'::JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX bets_game_id_idx ON bets (game_id);
CREATE INDEX bets_status_idx ON bets (status);
CREATE INDEX bets_snapshot_id_idx ON bets (snapshot_id);
CREATE INDEX bets_created_at_idx ON bets (created_at DESC);
