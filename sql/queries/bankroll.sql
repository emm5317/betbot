-- name: InsertBankrollEntry :one
INSERT INTO bankroll_ledger (
    entry_type,
    amount_cents,
    currency,
    reference_type,
    reference_id,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id, entry_type, amount_cents, currency, reference_type, reference_id, metadata, created_at;

-- name: GetBankrollBalanceCents :one
SELECT COALESCE(SUM(amount_cents), 0)::BIGINT AS balance_cents
FROM bankroll_ledger;

-- name: GetBankrollCircuitMetrics :one
WITH anchor AS (
    SELECT
        NOW() AS now_ts,
        DATE_TRUNC('day', NOW()) AS day_start_ts,
        DATE_TRUNC('week', NOW()) AS week_start_ts
),
ledger_totals AS (
    SELECT
        COALESCE(SUM(amount_cents), 0)::BIGINT AS current_balance_cents,
        COALESCE(SUM(amount_cents) FILTER (WHERE created_at < (SELECT day_start_ts FROM anchor)), 0)::BIGINT AS day_start_balance_cents,
        COALESCE(SUM(amount_cents) FILTER (WHERE created_at < (SELECT week_start_ts FROM anchor)), 0)::BIGINT AS week_start_balance_cents
    FROM bankroll_ledger
),
running_balance AS (
    SELECT
        SUM(amount_cents) OVER (ORDER BY created_at ASC, id ASC) AS balance_after_cents
    FROM bankroll_ledger
),
peak_balance AS (
    SELECT
        COALESCE(MAX(balance_after_cents), 0)::BIGINT AS peak_balance_cents
    FROM running_balance
)
SELECT
    l.current_balance_cents,
    l.day_start_balance_cents,
    l.week_start_balance_cents,
    p.peak_balance_cents
FROM ledger_totals l
CROSS JOIN peak_balance p;
