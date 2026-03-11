-- name: GetLatestSnapshotHash :one
SELECT snapshot_hash
FROM odds_history
WHERE game_id = $1
  AND source = $2
  AND book_key = $3
  AND market_key = $4
  AND outcome_name = $5
  AND outcome_side = $6
  AND point IS NOT DISTINCT FROM $7
ORDER BY captured_at DESC
LIMIT 1;

-- name: InsertOddsSnapshot :one
INSERT INTO odds_history (
    game_id,
    source,
    book_key,
    book_name,
    market_key,
    market_name,
    outcome_name,
    outcome_side,
    price_american,
    point,
    implied_probability,
    snapshot_hash,
    raw_json,
    captured_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: ListLatestOdds :many
WITH latest AS (
    SELECT DISTINCT ON (
        oh.game_id,
        oh.book_key,
        oh.market_key,
        oh.outcome_name,
        oh.outcome_side,
        oh.point
    )
        oh.game_id,
        oh.book_key,
        oh.book_name,
        oh.market_key,
        oh.market_name,
        oh.outcome_name,
        oh.outcome_side,
        oh.price_american,
        oh.point,
        oh.implied_probability,
        oh.captured_at
    FROM odds_history AS oh
    ORDER BY
        oh.game_id,
        oh.book_key,
        oh.market_key,
        oh.outcome_name,
        oh.outcome_side,
        oh.point,
        oh.captured_at DESC
)
SELECT
    latest.game_id,
    g.sport,
    g.home_team,
    g.away_team,
    g.commence_time,
    latest.book_key,
    latest.book_name,
    latest.market_key,
    latest.market_name,
    latest.outcome_name,
    latest.outcome_side,
    latest.price_american,
    latest.point,
    latest.implied_probability,
    latest.captured_at
FROM latest
JOIN games AS g ON g.id = latest.game_id
ORDER BY
    g.commence_time ASC,
    latest.book_key ASC,
    latest.market_key ASC,
    latest.outcome_name ASC
LIMIT $1;

-- name: CountOddsHistoryRows :one
SELECT COUNT(*)::BIGINT
FROM odds_history;
