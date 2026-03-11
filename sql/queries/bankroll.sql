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
