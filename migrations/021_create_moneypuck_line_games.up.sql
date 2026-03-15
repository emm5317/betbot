CREATE TABLE IF NOT EXISTS moneypuck_line_games (
    id               BIGSERIAL PRIMARY KEY,
    line_id          TEXT NOT NULL,
    name             TEXT NOT NULL,
    game_id          TEXT NOT NULL,
    season           INTEGER NOT NULL,
    team             TEXT NOT NULL,
    opponent         TEXT NOT NULL,
    home_or_away     TEXT NOT NULL,
    game_date        DATE NOT NULL,
    position         TEXT NOT NULL,
    situation        TEXT NOT NULL,
    icetime          DOUBLE PRECISION,
    ice_time_rank    DOUBLE PRECISION,
    xgoals_percentage DOUBLE PRECISION,
    corsi_percentage  DOUBLE PRECISION,
    fenwick_percentage DOUBLE PRECISION,
    xgoals_for       DOUBLE PRECISION,
    xgoals_against   DOUBLE PRECISION,
    goals_for        DOUBLE PRECISION,
    goals_against    DOUBLE PRECISION,
    UNIQUE (game_id, line_id, situation)
);

CREATE INDEX idx_moneypuck_line_games_season_team_date ON moneypuck_line_games (season, team, game_date);
