-- name: InsertRecommendationSnapshot :one
INSERT INTO recommendation_snapshots (
    generated_at,
    sport,
    game_id,
    event_time,
    event_date,
    market_key,
    recommended_side,
    best_book,
    best_american_odds,
    model_probability,
    market_probability,
    edge,
    suggested_stake_fraction,
    suggested_stake_cents,
    bankroll_check_pass,
    bankroll_check_reason,
    rank_score,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
RETURNING id, generated_at, sport, game_id, event_time, event_date, market_key, recommended_side, best_book, best_american_odds, model_probability, market_probability, edge, suggested_stake_fraction, suggested_stake_cents, bankroll_check_pass, bankroll_check_reason, rank_score, metadata, created_at;

-- name: ListRecommendationSnapshots :many
SELECT
    id,
    generated_at,
    sport,
    game_id,
    event_time,
    event_date,
    market_key,
    recommended_side,
    best_book,
    best_american_odds,
    model_probability,
    market_probability,
    edge,
    suggested_stake_fraction,
    suggested_stake_cents,
    bankroll_check_pass,
    bankroll_check_reason,
    rank_score,
    metadata,
    created_at
FROM recommendation_snapshots
WHERE
    (
        sqlc.narg(sport)::text IS NULL
        OR sport = sqlc.narg(sport)::text
    )
    AND (
        sqlc.narg(event_date)::date IS NULL
        OR event_date = sqlc.narg(event_date)::date
    )
ORDER BY rank_score DESC, generated_at DESC, id ASC
LIMIT sqlc.arg(row_limit);

-- name: GetRecommendationSnapshotByID :one
SELECT id, generated_at, sport, game_id, event_time, event_date, market_key,
    recommended_side, best_book, best_american_odds, model_probability,
    market_probability, edge, suggested_stake_fraction, suggested_stake_cents,
    bankroll_check_pass, bankroll_check_reason, rank_score, metadata, created_at
FROM recommendation_snapshots
WHERE id = @id;
