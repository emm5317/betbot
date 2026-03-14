-- name: InsertRecommendationOutcomeIfChanged :execrows
INSERT INTO recommendation_outcomes (
    snapshot_id,
    evaluation_status,
    close_american_odds,
    close_probability,
    realized_result,
    clv_delta,
    settled_at,
    notes,
    metadata
)
SELECT
    sqlc.arg(snapshot_id),
    sqlc.arg(evaluation_status),
    sqlc.narg(close_american_odds),
    sqlc.narg(close_probability),
    sqlc.narg(realized_result),
    sqlc.narg(clv_delta),
    sqlc.narg(settled_at),
    sqlc.arg(notes),
    sqlc.arg(metadata)::jsonb
WHERE NOT EXISTS (
    SELECT 1
    FROM recommendation_outcomes AS ro
    WHERE ro.snapshot_id = sqlc.arg(snapshot_id)
      AND ro.evaluation_status = sqlc.arg(evaluation_status)
      AND ro.close_american_odds IS NOT DISTINCT FROM sqlc.narg(close_american_odds)
      AND ro.close_probability IS NOT DISTINCT FROM sqlc.narg(close_probability)
      AND ro.realized_result IS NOT DISTINCT FROM sqlc.narg(realized_result)
      AND ro.clv_delta IS NOT DISTINCT FROM sqlc.narg(clv_delta)
      AND ro.settled_at IS NOT DISTINCT FROM sqlc.narg(settled_at)
      AND ro.notes = sqlc.arg(notes)
      AND ro.metadata = sqlc.arg(metadata)::jsonb
);

-- name: ListRecommendationPerformanceSnapshots :many
SELECT
    rs.id AS snapshot_id,
    rs.generated_at,
    rs.sport,
    rs.game_id,
    g.home_team,
    g.away_team,
    rs.event_time,
    rs.event_date,
    rs.market_key,
    rs.recommended_side,
    rs.best_book,
    rs.best_american_odds,
    rs.model_probability,
    rs.market_probability,
    rs.edge,
    rs.suggested_stake_fraction,
    rs.suggested_stake_cents,
    rs.bankroll_check_pass,
    rs.bankroll_check_reason,
    rs.rank_score,
    rs.metadata AS snapshot_metadata,
    COALESCE(close_line.id, 0)::BIGINT AS close_line_id,
    COALESCE(close_line.price_american, 0)::INTEGER AS close_american_odds,
    COALESCE(close_line.implied_probability, 0)::DOUBLE PRECISION AS close_probability,
    COALESCE(close_line.captured_at, rs.event_time) AS close_captured_at,
    COALESCE(close_line.raw_json, '{}'::jsonb) AS close_raw_json,
    COALESCE(latest_outcome.id, 0)::BIGINT AS persisted_outcome_id,
    COALESCE(latest_outcome.evaluation_status, '') AS persisted_status,
    latest_outcome.close_american_odds AS persisted_close_american_odds,
    latest_outcome.close_probability AS persisted_close_probability,
    latest_outcome.realized_result AS persisted_realized_result,
    latest_outcome.clv_delta AS persisted_clv_delta,
    latest_outcome.settled_at AS persisted_settled_at,
    COALESCE(latest_outcome.notes, '') AS persisted_notes,
    COALESCE(latest_outcome.metadata, '{}'::jsonb) AS persisted_metadata,
    COALESCE(latest_outcome.created_at, rs.generated_at) AS persisted_created_at
FROM recommendation_snapshots AS rs
JOIN games AS g
    ON g.id = rs.game_id
LEFT JOIN LATERAL (
    SELECT
        oh.price_american,
        oh.implied_probability,
        oh.captured_at,
        oh.raw_json,
        oh.id
    FROM odds_history AS oh
    WHERE oh.game_id = rs.game_id
      AND oh.market_key = rs.market_key
      AND oh.outcome_side = rs.recommended_side
      AND (oh.book_key = rs.best_book OR oh.book_name = rs.best_book)
      AND oh.captured_at <= rs.event_time
    ORDER BY oh.captured_at DESC, oh.id DESC
    LIMIT 1
) AS close_line ON TRUE
LEFT JOIN LATERAL (
    SELECT
        ro.evaluation_status,
        ro.close_american_odds,
        ro.close_probability,
        ro.realized_result,
        ro.clv_delta,
        ro.settled_at,
        ro.notes,
        ro.metadata,
        ro.created_at,
        ro.id
    FROM recommendation_outcomes AS ro
    WHERE ro.snapshot_id = rs.id
    ORDER BY ro.created_at DESC, ro.id DESC
    LIMIT 1
) AS latest_outcome ON TRUE
WHERE
    (
        sqlc.narg(sport)::text IS NULL
        OR rs.sport = sqlc.narg(sport)::text
    )
    AND (
        sqlc.narg(date_from)::date IS NULL
        OR rs.event_date >= sqlc.narg(date_from)::date
    )
    AND (
        sqlc.narg(date_to)::date IS NULL
        OR rs.event_date <= sqlc.narg(date_to)::date
    )
ORDER BY rs.generated_at DESC, rs.rank_score DESC, rs.id ASC
LIMIT sqlc.arg(row_limit);
