CREATE TABLE moneypuck_goalie_games (
    id                  BIGSERIAL PRIMARY KEY,
    season              INT NOT NULL,
    game_id             TEXT NOT NULL,
    player_id           TEXT NOT NULL,
    name                TEXT NOT NULL,
    team                TEXT NOT NULL,
    opponent            TEXT NOT NULL,
    home_or_away        TEXT NOT NULL CHECK (home_or_away IN ('HOME', 'AWAY')),
    game_date           DATE NOT NULL,
    situation           TEXT NOT NULL CHECK (situation IN ('5on5', 'all')),

    -- Core goalie stats
    icetime             DOUBLE PRECISION,
    xgoals              DOUBLE PRECISION,
    goals               DOUBLE PRECISION,
    gsax                DOUBLE PRECISION,

    -- Shot detail
    shots_against       DOUBLE PRECISION,
    high_danger_xgoals  DOUBLE PRECISION,
    high_danger_goals   DOUBLE PRECISION,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (game_id, player_id, situation)
);

CREATE INDEX idx_mpgg_season_team_date ON moneypuck_goalie_games (season, team, game_date);
CREATE INDEX idx_mpgg_team_sit_date_ice ON moneypuck_goalie_games (team, situation, game_date, icetime DESC);
CREATE INDEX idx_mpgg_player_season ON moneypuck_goalie_games (player_id, season, game_date);
