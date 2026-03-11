-- name: UpsertNBATeamStats :exec
INSERT INTO nba_team_stats (
    source,
    external_id,
    season,
    season_type,
    stat_date,
    team_name,
    games_played,
    wins,
    losses,
    offensive_rating,
    defensive_rating,
    net_rating,
    pace
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
SET
    team_name = EXCLUDED.team_name,
    games_played = EXCLUDED.games_played,
    wins = EXCLUDED.wins,
    losses = EXCLUDED.losses,
    offensive_rating = EXCLUDED.offensive_rating,
    defensive_rating = EXCLUDED.defensive_rating,
    net_rating = EXCLUDED.net_rating,
    pace = EXCLUDED.pace,
    updated_at = NOW()
WHERE
    nba_team_stats.team_name IS DISTINCT FROM EXCLUDED.team_name
    OR nba_team_stats.games_played IS DISTINCT FROM EXCLUDED.games_played
    OR nba_team_stats.wins IS DISTINCT FROM EXCLUDED.wins
    OR nba_team_stats.losses IS DISTINCT FROM EXCLUDED.losses
    OR nba_team_stats.offensive_rating IS DISTINCT FROM EXCLUDED.offensive_rating
    OR nba_team_stats.defensive_rating IS DISTINCT FROM EXCLUDED.defensive_rating
    OR nba_team_stats.net_rating IS DISTINCT FROM EXCLUDED.net_rating
    OR nba_team_stats.pace IS DISTINCT FROM EXCLUDED.pace;
