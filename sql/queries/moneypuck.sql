-- name: UpsertMoneypuckTeamGame :exec
INSERT INTO moneypuck_team_games (
    season, game_id, team, opponent, home_or_away, game_date, situation, is_playoff,
    xgoals_percentage, xgoals_for, xgoals_against,
    score_venue_adjusted_xgoals_for, score_venue_adjusted_xgoals_against,
    corsi_percentage, fenwick_percentage,
    shots_on_goal_for, shots_on_goal_against,
    shot_attempts_for, shot_attempts_against,
    high_danger_shots_for, high_danger_shots_against,
    high_danger_xgoals_for, high_danger_xgoals_against,
    goals_for, goals_against
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
)
ON CONFLICT (game_id, team, situation) DO UPDATE SET
    season = EXCLUDED.season,
    opponent = EXCLUDED.opponent,
    home_or_away = EXCLUDED.home_or_away,
    game_date = EXCLUDED.game_date,
    is_playoff = EXCLUDED.is_playoff,
    xgoals_percentage = EXCLUDED.xgoals_percentage,
    xgoals_for = EXCLUDED.xgoals_for,
    xgoals_against = EXCLUDED.xgoals_against,
    score_venue_adjusted_xgoals_for = EXCLUDED.score_venue_adjusted_xgoals_for,
    score_venue_adjusted_xgoals_against = EXCLUDED.score_venue_adjusted_xgoals_against,
    corsi_percentage = EXCLUDED.corsi_percentage,
    fenwick_percentage = EXCLUDED.fenwick_percentage,
    shots_on_goal_for = EXCLUDED.shots_on_goal_for,
    shots_on_goal_against = EXCLUDED.shots_on_goal_against,
    shot_attempts_for = EXCLUDED.shot_attempts_for,
    shot_attempts_against = EXCLUDED.shot_attempts_against,
    high_danger_shots_for = EXCLUDED.high_danger_shots_for,
    high_danger_shots_against = EXCLUDED.high_danger_shots_against,
    high_danger_xgoals_for = EXCLUDED.high_danger_xgoals_for,
    high_danger_xgoals_against = EXCLUDED.high_danger_xgoals_against,
    goals_for = EXCLUDED.goals_for,
    goals_against = EXCLUDED.goals_against;

-- name: UpsertMoneypuckGoalieGame :exec
INSERT INTO moneypuck_goalie_games (
    season, game_id, player_id, name, team, opponent, home_or_away, game_date, situation,
    icetime, xgoals, goals, gsax, shots_against, high_danger_xgoals, high_danger_goals
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (game_id, player_id, situation) DO UPDATE SET
    season = EXCLUDED.season,
    name = EXCLUDED.name,
    team = EXCLUDED.team,
    opponent = EXCLUDED.opponent,
    home_or_away = EXCLUDED.home_or_away,
    game_date = EXCLUDED.game_date,
    icetime = EXCLUDED.icetime,
    xgoals = EXCLUDED.xgoals,
    goals = EXCLUDED.goals,
    gsax = EXCLUDED.gsax,
    shots_against = EXCLUDED.shots_against,
    high_danger_xgoals = EXCLUDED.high_danger_xgoals,
    high_danger_goals = EXCLUDED.high_danger_goals;

-- name: GetTeamRolling5on5Stats :many
-- Returns the last N 5on5 games for a team strictly before a given date, ordered newest first.
-- The caller computes rolling averages from these rows.
SELECT
    game_id, game_date, opponent, home_or_away,
    xgoals_percentage, xgoals_for, xgoals_against,
    score_venue_adjusted_xgoals_for, score_venue_adjusted_xgoals_against,
    corsi_percentage, fenwick_percentage,
    shots_on_goal_for, shots_on_goal_against,
    shot_attempts_for, shot_attempts_against,
    high_danger_shots_for, high_danger_shots_against,
    high_danger_xgoals_for, high_danger_xgoals_against,
    goals_for, goals_against
FROM moneypuck_team_games
WHERE team = $1
  AND situation = '5on5'
  AND game_date < $2
  AND season = $3
  AND is_playoff = FALSE
ORDER BY game_date DESC
LIMIT $4;

-- name: GetGameResult :many
-- Returns goals for/against from the "all" situation for both teams in a game.
-- Two rows returned: one per team, with home_or_away indicating side.
SELECT
    team, opponent, home_or_away, goals_for, goals_against
FROM moneypuck_team_games
WHERE game_id = $1
  AND situation = 'all'
ORDER BY home_or_away ASC;

-- name: GetStartingGoalie :one
-- Returns the goalie with the most 5on5 icetime for a team in a given game.
SELECT
    player_id, name, team, icetime, xgoals, goals, gsax,
    high_danger_xgoals, high_danger_goals
FROM moneypuck_goalie_games
WHERE game_id = $1
  AND team = $2
  AND situation = '5on5'
ORDER BY icetime DESC
LIMIT 1;

-- name: GetGoalieSeasonGSAx :one
-- Returns cumulative 5on5 GSAx for a goalie in a season before a given date.
SELECT
    COALESCE(SUM(gsax), 0)::DOUBLE PRECISION AS cumulative_gsax,
    COALESCE(SUM(icetime), 0)::DOUBLE PRECISION AS cumulative_icetime,
    COUNT(*)::BIGINT AS games_played
FROM moneypuck_goalie_games
WHERE player_id = $1
  AND season = $2
  AND situation = '5on5'
  AND game_date < $3;

-- name: ListSeasonGameDates :many
-- Returns distinct game dates for a season (for odds backfill targeting).
SELECT DISTINCT game_date
FROM moneypuck_team_games
WHERE season = $1
  AND situation = 'all'
  AND is_playoff = FALSE
ORDER BY game_date ASC;

-- name: ListSeasonTeamGames :many
-- Returns all games for a team in a season (all situation), ordered by date.
SELECT
    game_id, game_date, opponent, home_or_away, goals_for, goals_against
FROM moneypuck_team_games
WHERE team = $1
  AND season = $2
  AND situation = 'all'
  AND is_playoff = FALSE
ORDER BY game_date ASC;

-- name: CountMoneypuckTeamGames :one
SELECT COUNT(*)::BIGINT FROM moneypuck_team_games;

-- name: CountMoneypuckGoalieGames :one
SELECT COUNT(*)::BIGINT FROM moneypuck_goalie_games;
