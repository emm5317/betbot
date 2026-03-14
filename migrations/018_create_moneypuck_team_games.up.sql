CREATE TABLE moneypuck_team_games (
    id                              BIGSERIAL PRIMARY KEY,
    season                          INT NOT NULL,
    game_id                         TEXT NOT NULL,
    team                            TEXT NOT NULL,
    opponent                        TEXT NOT NULL,
    home_or_away                    TEXT NOT NULL CHECK (home_or_away IN ('HOME', 'AWAY')),
    game_date                       DATE NOT NULL,
    situation                       TEXT NOT NULL CHECK (situation IN ('5on5', 'all')),
    is_playoff                      BOOLEAN NOT NULL DEFAULT FALSE,

    -- Core expected goals
    xgoals_percentage               DOUBLE PRECISION,
    xgoals_for                      DOUBLE PRECISION,
    xgoals_against                  DOUBLE PRECISION,
    score_venue_adjusted_xgoals_for DOUBLE PRECISION,
    score_venue_adjusted_xgoals_against DOUBLE PRECISION,

    -- Possession metrics
    corsi_percentage                DOUBLE PRECISION,
    fenwick_percentage              DOUBLE PRECISION,

    -- Shot counts
    shots_on_goal_for               DOUBLE PRECISION,
    shots_on_goal_against           DOUBLE PRECISION,
    shot_attempts_for               DOUBLE PRECISION,
    shot_attempts_against           DOUBLE PRECISION,

    -- High danger
    high_danger_shots_for           DOUBLE PRECISION,
    high_danger_shots_against       DOUBLE PRECISION,
    high_danger_xgoals_for          DOUBLE PRECISION,
    high_danger_xgoals_against      DOUBLE PRECISION,

    -- Actual scoring
    goals_for                       DOUBLE PRECISION,
    goals_against                   DOUBLE PRECISION,

    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (game_id, team, situation)
);

CREATE INDEX idx_mptg_season_team_date ON moneypuck_team_games (season, team, game_date);
CREATE INDEX idx_mptg_game_situation ON moneypuck_team_games (game_id, situation);
CREATE INDEX idx_mptg_date_situation ON moneypuck_team_games (game_date, situation);
