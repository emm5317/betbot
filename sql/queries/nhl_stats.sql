-- name: UpsertNHLTeamStats :exec
INSERT INTO nhl_team_stats (
    source,
    external_id,
    season,
    season_type,
    stat_date,
    team_name,
    games_played,
    wins,
    losses,
    ot_losses,
    goals_for_per_game,
    goals_against_per_game,
    expected_goals_share,
    save_percentage
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
SET
    team_name = EXCLUDED.team_name,
    games_played = EXCLUDED.games_played,
    wins = EXCLUDED.wins,
    losses = EXCLUDED.losses,
    ot_losses = EXCLUDED.ot_losses,
    goals_for_per_game = EXCLUDED.goals_for_per_game,
    goals_against_per_game = EXCLUDED.goals_against_per_game,
    expected_goals_share = EXCLUDED.expected_goals_share,
    save_percentage = EXCLUDED.save_percentage,
    updated_at = NOW()
WHERE
    nhl_team_stats.team_name IS DISTINCT FROM EXCLUDED.team_name
    OR nhl_team_stats.games_played IS DISTINCT FROM EXCLUDED.games_played
    OR nhl_team_stats.wins IS DISTINCT FROM EXCLUDED.wins
    OR nhl_team_stats.losses IS DISTINCT FROM EXCLUDED.losses
    OR nhl_team_stats.ot_losses IS DISTINCT FROM EXCLUDED.ot_losses
    OR nhl_team_stats.goals_for_per_game IS DISTINCT FROM EXCLUDED.goals_for_per_game
    OR nhl_team_stats.goals_against_per_game IS DISTINCT FROM EXCLUDED.goals_against_per_game
    OR nhl_team_stats.expected_goals_share IS DISTINCT FROM EXCLUDED.expected_goals_share
    OR nhl_team_stats.save_percentage IS DISTINCT FROM EXCLUDED.save_percentage;
