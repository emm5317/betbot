-- name: InsertGameResultSnapshot :exec
INSERT INTO game_results (
    game_id,
    source,
    external_id,
    status,
    home_score,
    away_score,
    result_hash,
    raw_json,
    captured_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (game_id, source, result_hash) DO NOTHING;

-- name: GetLatestFinalGameResultForGame :one
SELECT id, game_id, source, external_id, status, home_score, away_score, result_hash, raw_json, captured_at, created_at
FROM game_results
WHERE game_id = $1
  AND lower(status) = 'final'
ORDER BY captured_at DESC, id DESC
LIMIT 1;

-- name: CountGameResultRows :one
SELECT COUNT(*)::BIGINT
FROM game_results;
