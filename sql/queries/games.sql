-- name: UpsertGame :one
INSERT INTO games (
    source,
    external_id,
    sport,
    home_team,
    away_team,
    commence_time
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (source, external_id) DO UPDATE
SET
    sport = EXCLUDED.sport,
    home_team = EXCLUDED.home_team,
    away_team = EXCLUDED.away_team,
    commence_time = EXCLUDED.commence_time,
    updated_at = NOW()
RETURNING id, source, external_id, sport, home_team, away_team, commence_time, created_at, updated_at;

-- name: ListUpcomingGames :many
SELECT id, source, external_id, sport, home_team, away_team, commence_time, created_at, updated_at
FROM games
WHERE commence_time >= NOW() - INTERVAL '12 hours'
ORDER BY commence_time ASC
LIMIT $1;
