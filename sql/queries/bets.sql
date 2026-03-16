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
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at;

-- name: GetBetByIdempotencyKey :one
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at
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
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at
FROM bets
WHERE status = 'placed'
ORDER BY created_at ASC;

-- name: ListBetsByStatus :many
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at
FROM bets
WHERE status = @status::bet_status
ORDER BY created_at DESC
LIMIT @row_limit;

-- name: InsertManualBet :one
INSERT INTO bets (
    idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, adapter_name, user_notes, metadata
) VALUES (
    @idempotency_key, @snapshot_id, @game_id, @sport, @market_key,
    @recommended_side, @book_key, @american_odds, @stake_cents,
    @model_probability, @market_probability, @edge, 'placed', 'manual', @user_notes, '{}'::JSONB
)
RETURNING id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at;

-- name: GetBetByID :one
SELECT id, idempotency_key, snapshot_id, game_id, sport, market_key,
    recommended_side, book_key, american_odds, stake_cents,
    model_probability, market_probability, edge, status, external_bet_id,
    adapter_name, placed_at, settled_at, settlement_result, payout_cents,
    clv_delta, closing_probability, error_message, user_notes, metadata, created_at, updated_at
FROM bets
WHERE id = @id;

-- name: ListBetsWithFilters :many
SELECT b.id, b.idempotency_key, b.snapshot_id, b.game_id, b.sport, b.market_key,
    b.recommended_side, b.book_key, b.american_odds, b.stake_cents,
    b.model_probability, b.market_probability, b.edge, b.status, b.external_bet_id,
    b.adapter_name, b.placed_at, b.settled_at, b.settlement_result, b.payout_cents,
    b.clv_delta, b.closing_probability, b.error_message, b.user_notes, b.metadata,
    b.created_at, b.updated_at,
    g.home_team, g.away_team, g.commence_time
FROM bets b
JOIN games g ON g.id = b.game_id
WHERE (@sport::TEXT = '' OR b.sport = @sport)
  AND (@status_filter::TEXT = '' OR b.status = @status_filter::bet_status)
ORDER BY b.created_at DESC
LIMIT @row_limit;

-- name: GetBetPnLSummary :one
SELECT
    COALESCE(COUNT(*), 0)::BIGINT AS total_bets,
    COALESCE(COUNT(*) FILTER (WHERE status = 'placed'), 0)::BIGINT AS open_bets,
    COALESCE(COUNT(*) FILTER (WHERE status = 'settled'), 0)::BIGINT AS settled_bets,
    COALESCE(COUNT(*) FILTER (WHERE status = 'voided'), 0)::BIGINT AS voided_bets,
    COALESCE(SUM(stake_cents) FILTER (WHERE status IN ('placed', 'settled')), 0)::BIGINT AS total_staked_cents,
    COALESCE(SUM(payout_cents) FILTER (WHERE status = 'settled'), 0)::BIGINT AS total_returned_cents,
    COALESCE(SUM(payout_cents) FILTER (WHERE status = 'settled'), 0)::BIGINT
      - COALESCE(SUM(stake_cents) FILTER (WHERE status = 'settled'), 0)::BIGINT AS net_pnl_cents
FROM bets
WHERE (@sport::TEXT = '' OR sport = @sport);

-- name: VoidBet :exec
UPDATE bets
SET status = 'voided', updated_at = NOW()
WHERE id = @id AND status = 'placed';
