CREATE TABLE bankroll_ledger (
    id BIGSERIAL PRIMARY KEY,
    entry_type TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    reference_type TEXT NOT NULL DEFAULT '',
    reference_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX bankroll_ledger_created_at_idx
    ON bankroll_ledger (created_at DESC);
