-- name: UpsertModelPrediction :one
INSERT INTO model_predictions (
    game_id,
    source,
    sport,
    book_key,
    market_key,
    model_family,
    model_version,
    manifest_version,
    feature_vector,
    predicted_probability,
    market_probability,
    closing_probability,
    event_time
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (
    game_id,
    source,
    book_key,
    market_key,
    model_family,
    model_version,
    event_time
) DO UPDATE
SET
    manifest_version = EXCLUDED.manifest_version,
    feature_vector = EXCLUDED.feature_vector,
    predicted_probability = EXCLUDED.predicted_probability,
    market_probability = EXCLUDED.market_probability,
    closing_probability = EXCLUDED.closing_probability,
    updated_at = NOW()
WHERE
    model_predictions.manifest_version IS DISTINCT FROM EXCLUDED.manifest_version
    OR model_predictions.feature_vector IS DISTINCT FROM EXCLUDED.feature_vector
    OR model_predictions.predicted_probability IS DISTINCT FROM EXCLUDED.predicted_probability
    OR model_predictions.market_probability IS DISTINCT FROM EXCLUDED.market_probability
    OR model_predictions.closing_probability IS DISTINCT FROM EXCLUDED.closing_probability
RETURNING id, game_id, source, sport, book_key, market_key, model_family, model_version, manifest_version, feature_vector, predicted_probability, market_probability, closing_probability, event_time, created_at, updated_at;

-- name: ListBacktestReplayRows :many
WITH home_snapshots AS (
    SELECT
        g.id AS game_id,
        g.source,
        g.external_id,
        g.sport,
        g.home_team,
        g.away_team,
        g.commence_time,
        oh.book_key,
        oh.market_key,
        oh.implied_probability AS home_implied_probability,
        oh.captured_at,
        oh.id,
        ROW_NUMBER() OVER (
            PARTITION BY g.id, oh.book_key, oh.market_key
            ORDER BY oh.captured_at ASC, oh.id ASC
        ) AS rn_open,
        ROW_NUMBER() OVER (
            PARTITION BY g.id, oh.book_key, oh.market_key
            ORDER BY oh.captured_at DESC, oh.id DESC
        ) AS rn_close
    FROM odds_history AS oh
    JOIN games AS g ON g.id = oh.game_id
    WHERE
        oh.outcome_side = 'home'
        AND (
            sqlc.narg(sport)::text IS NULL
            OR g.sport = sqlc.narg(sport)::text
        )
        AND (
            sqlc.narg(season)::int IS NULL
            OR EXTRACT(YEAR FROM g.commence_time AT TIME ZONE 'UTC')::int = sqlc.narg(season)::int
        )
        AND (
            sqlc.narg(market_key)::text IS NULL
            OR oh.market_key = sqlc.narg(market_key)::text
        )
),
open_rows AS (
    SELECT *
    FROM home_snapshots
    WHERE rn_open = 1
),
close_rows AS (
    SELECT *
    FROM home_snapshots
    WHERE rn_close = 1
),
latest_final_results AS (
    SELECT DISTINCT ON (gr.game_id)
        gr.game_id,
        gr.home_score,
        gr.away_score
    FROM game_results AS gr
    WHERE lower(gr.status) = 'final'
      AND gr.home_score IS NOT NULL
      AND gr.away_score IS NOT NULL
    ORDER BY
        gr.game_id,
        gr.captured_at DESC,
        gr.id DESC
)
SELECT
    o.game_id,
    o.source,
    o.external_id,
    o.sport,
    o.home_team,
    o.away_team,
    o.commence_time,
    o.book_key,
    o.market_key,
    o.home_implied_probability AS opening_home_implied_probability,
    c.home_implied_probability AS closing_home_implied_probability,
    o.captured_at AS opening_captured_at,
    c.captured_at AS closing_captured_at,
    lr.home_score AS actual_home_score,
    lr.away_score AS actual_away_score,
    (lr.game_id IS NOT NULL)::boolean AS has_actual_result,
    COALESCE((lr.home_score > lr.away_score)::boolean, false)::boolean AS actual_home_win
FROM open_rows AS o
JOIN close_rows AS c
  ON c.game_id = o.game_id
 AND c.book_key = o.book_key
 AND c.market_key = o.market_key
LEFT JOIN latest_final_results AS lr
  ON lr.game_id = o.game_id
WHERE c.captured_at >= o.captured_at
ORDER BY
    o.commence_time ASC,
    o.game_id ASC,
    o.book_key ASC,
    o.market_key ASC
LIMIT sqlc.arg(row_limit);

-- name: ListModelPredictionsForSportSeason :many
SELECT
    id,
    game_id,
    source,
    sport,
    book_key,
    market_key,
    model_family,
    model_version,
    manifest_version,
    feature_vector,
    predicted_probability,
    market_probability,
    closing_probability,
    event_time,
    created_at,
    updated_at
FROM model_predictions
WHERE
    sport = sqlc.arg(sport)
    AND (
        sqlc.narg(season)::int IS NULL
        OR EXTRACT(YEAR FROM event_time AT TIME ZONE 'UTC')::int = sqlc.narg(season)::int
    )
    AND (
        sqlc.narg(model_family)::text IS NULL
        OR model_family = sqlc.narg(model_family)::text
    )
ORDER BY event_time ASC, id ASC;

-- name: CountModelPredictions :one
SELECT COUNT(*)::BIGINT
FROM model_predictions;
