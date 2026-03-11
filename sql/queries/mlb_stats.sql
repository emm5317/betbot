-- name: UpsertMLBTeamStats :exec
INSERT INTO mlb_team_stats (
    source,
    external_id,
    season,
    season_type,
    stat_date,
    team_name,
    games_played,
    wins,
    losses,
    runs_scored,
    runs_allowed,
    batting_ops,
    team_era
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
SET
    team_name = EXCLUDED.team_name,
    games_played = EXCLUDED.games_played,
    wins = EXCLUDED.wins,
    losses = EXCLUDED.losses,
    runs_scored = EXCLUDED.runs_scored,
    runs_allowed = EXCLUDED.runs_allowed,
    batting_ops = EXCLUDED.batting_ops,
    team_era = EXCLUDED.team_era,
    updated_at = NOW()
WHERE
    mlb_team_stats.team_name IS DISTINCT FROM EXCLUDED.team_name
    OR mlb_team_stats.games_played IS DISTINCT FROM EXCLUDED.games_played
    OR mlb_team_stats.wins IS DISTINCT FROM EXCLUDED.wins
    OR mlb_team_stats.losses IS DISTINCT FROM EXCLUDED.losses
    OR mlb_team_stats.runs_scored IS DISTINCT FROM EXCLUDED.runs_scored
    OR mlb_team_stats.runs_allowed IS DISTINCT FROM EXCLUDED.runs_allowed
    OR mlb_team_stats.batting_ops IS DISTINCT FROM EXCLUDED.batting_ops
    OR mlb_team_stats.team_era IS DISTINCT FROM EXCLUDED.team_era;

-- name: UpsertMLBPitcherStats :exec
INSERT INTO mlb_pitcher_stats (
    source,
    external_id,
    season,
    season_type,
    stat_date,
    player_name,
    team_external_id,
    team_name,
    games_started,
    innings_pitched,
    era,
    fip,
    whip,
    strikeout_rate,
    walk_rate
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
ON CONFLICT (source, season, season_type, stat_date, external_id) DO UPDATE
SET
    player_name = EXCLUDED.player_name,
    team_external_id = EXCLUDED.team_external_id,
    team_name = EXCLUDED.team_name,
    games_started = EXCLUDED.games_started,
    innings_pitched = EXCLUDED.innings_pitched,
    era = EXCLUDED.era,
    fip = EXCLUDED.fip,
    whip = EXCLUDED.whip,
    strikeout_rate = EXCLUDED.strikeout_rate,
    walk_rate = EXCLUDED.walk_rate,
    updated_at = NOW()
WHERE
    mlb_pitcher_stats.player_name IS DISTINCT FROM EXCLUDED.player_name
    OR mlb_pitcher_stats.team_external_id IS DISTINCT FROM EXCLUDED.team_external_id
    OR mlb_pitcher_stats.team_name IS DISTINCT FROM EXCLUDED.team_name
    OR mlb_pitcher_stats.games_started IS DISTINCT FROM EXCLUDED.games_started
    OR mlb_pitcher_stats.innings_pitched IS DISTINCT FROM EXCLUDED.innings_pitched
    OR mlb_pitcher_stats.era IS DISTINCT FROM EXCLUDED.era
    OR mlb_pitcher_stats.fip IS DISTINCT FROM EXCLUDED.fip
    OR mlb_pitcher_stats.whip IS DISTINCT FROM EXCLUDED.whip
    OR mlb_pitcher_stats.strikeout_rate IS DISTINCT FROM EXCLUDED.strikeout_rate
    OR mlb_pitcher_stats.walk_rate IS DISTINCT FROM EXCLUDED.walk_rate;
