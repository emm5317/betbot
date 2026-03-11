-- name: UpsertNFLTeamStats :exec
INSERT INTO nfl_team_stats (
    source,
    external_id,
    season,
    season_type,
    stat_date,
    team_name,
    games_played,
    wins,
    losses,
    ties,
    points_for,
    points_against,
    offensive_epa_per_play,
    defensive_epa_per_play,
    offensive_success_rate,
    defensive_success_rate
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
SET
    team_name = EXCLUDED.team_name,
    games_played = EXCLUDED.games_played,
    wins = EXCLUDED.wins,
    losses = EXCLUDED.losses,
    ties = EXCLUDED.ties,
    points_for = EXCLUDED.points_for,
    points_against = EXCLUDED.points_against,
    offensive_epa_per_play = EXCLUDED.offensive_epa_per_play,
    defensive_epa_per_play = EXCLUDED.defensive_epa_per_play,
    offensive_success_rate = EXCLUDED.offensive_success_rate,
    defensive_success_rate = EXCLUDED.defensive_success_rate,
    updated_at = NOW()
WHERE
    nfl_team_stats.team_name IS DISTINCT FROM EXCLUDED.team_name
    OR nfl_team_stats.games_played IS DISTINCT FROM EXCLUDED.games_played
    OR nfl_team_stats.wins IS DISTINCT FROM EXCLUDED.wins
    OR nfl_team_stats.losses IS DISTINCT FROM EXCLUDED.losses
    OR nfl_team_stats.ties IS DISTINCT FROM EXCLUDED.ties
    OR nfl_team_stats.points_for IS DISTINCT FROM EXCLUDED.points_for
    OR nfl_team_stats.points_against IS DISTINCT FROM EXCLUDED.points_against
    OR nfl_team_stats.offensive_epa_per_play IS DISTINCT FROM EXCLUDED.offensive_epa_per_play
    OR nfl_team_stats.defensive_epa_per_play IS DISTINCT FROM EXCLUDED.defensive_epa_per_play
    OR nfl_team_stats.offensive_success_rate IS DISTINCT FROM EXCLUDED.offensive_success_rate
    OR nfl_team_stats.defensive_success_rate IS DISTINCT FROM EXCLUDED.defensive_success_rate;
