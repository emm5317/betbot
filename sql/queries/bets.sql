-- name: InsertBet :one
INSERT INTO bets (
    idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, adapter_name, metadata
) VALUES (
    @idempotency_key, @snapshot_id, @game_id, @sport, @market_key,
    @recommended_side, @book_key, @american_odds, @stake_cents,
    @model_probability, @market_probability, @edge, 'pending', @adapter_name, @metadata
)
RETURNING id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, metadata, created_at, updated_at;

-- name: GetBetByIdempotencyKey :one
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, metadata, created_at, updated_at
FROM bets
WHERE idempotency_key = @idempotency_key;

-- name: UpdateBetPlaced :exec
UPDATE bets
SET status = 'placed', external_bet_id = @external_bet_id, placed_at = NOW(), updated_at = NOW()
WHERE id = @id AND status = 'pending';

-- name: UpdateBetFailed :exec
UPDATE bets
SET status = 'failed', error_message = @error_message, updated_at = NOW()
WHERE id = @id AND status = 'pending';

-- name: UpdateBetSettled :exec
UPDATE bets
SET status = 'settled', settlement_result = @settlement_result, payout_cents = @payout_cents,
    clv_delta = @clv_delta, closing_probability = @closing_probability, settled_at = NOW(), updated_at = NOW()
WHERE id = @id AND status = 'placed';

-- name: ListOpenBets :many
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, metadata, created_at, updated_at
FROM bets
WHERE status = 'placed'
ORDER BY created_at ASC;

-- name: ListBetsByStatus :many
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, metadata, created_at, updated_at
FROM bets
WHERE status = @status::bet_status
ORDER BY created_at DESC
LIMIT @row_limit;
