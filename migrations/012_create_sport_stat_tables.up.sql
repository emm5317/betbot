CREATE TABLE mlb_team_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    runs_scored INTEGER NOT NULL DEFAULT 0,
    runs_allowed INTEGER NOT NULL DEFAULT 0,
    batting_ops DOUBLE PRECISION,
    team_era DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX mlb_team_stats_stat_date_idx
    ON mlb_team_stats (stat_date DESC);

CREATE TABLE mlb_pitcher_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    player_name TEXT NOT NULL,
    team_external_id TEXT NOT NULL,
    team_name TEXT NOT NULL,
    games_started INTEGER NOT NULL DEFAULT 0,
    innings_pitched DOUBLE PRECISION,
    era DOUBLE PRECISION,
    fip DOUBLE PRECISION,
    whip DOUBLE PRECISION,
    strikeout_rate DOUBLE PRECISION,
    walk_rate DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX mlb_pitcher_stats_stat_date_idx
    ON mlb_pitcher_stats (stat_date DESC);

CREATE INDEX mlb_pitcher_stats_team_lookup_idx
    ON mlb_pitcher_stats (team_external_id, stat_date DESC);

CREATE TABLE nba_team_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    offensive_rating DOUBLE PRECISION,
    defensive_rating DOUBLE PRECISION,
    net_rating DOUBLE PRECISION,
    pace DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX nba_team_stats_stat_date_idx
    ON nba_team_stats (stat_date DESC);

CREATE TABLE nhl_team_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    ot_losses INTEGER NOT NULL DEFAULT 0,
    goals_for_per_game DOUBLE PRECISION,
    goals_against_per_game DOUBLE PRECISION,
    expected_goals_share DOUBLE PRECISION,
    save_percentage DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX nhl_team_stats_stat_date_idx
    ON nhl_team_stats (stat_date DESC);

CREATE TABLE nhl_goalie_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    player_name TEXT NOT NULL,
    team_external_id TEXT NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    starts INTEGER NOT NULL DEFAULT 0,
    minutes_played INTEGER,
    save_percentage DOUBLE PRECISION,
    goals_against_average DOUBLE PRECISION,
    goals_saved_above_expected DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX nhl_goalie_stats_stat_date_idx
    ON nhl_goalie_stats (stat_date DESC);

CREATE INDEX nhl_goalie_stats_team_lookup_idx
    ON nhl_goalie_stats (team_external_id, stat_date DESC);

CREATE TABLE nfl_team_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    ties INTEGER NOT NULL DEFAULT 0,
    points_for INTEGER NOT NULL DEFAULT 0,
    points_against INTEGER NOT NULL DEFAULT 0,
    offensive_epa_per_play DOUBLE PRECISION,
    defensive_epa_per_play DOUBLE PRECISION,
    offensive_success_rate DOUBLE PRECISION,
    defensive_success_rate DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX nfl_team_stats_stat_date_idx
    ON nfl_team_stats (stat_date DESC);

CREATE TABLE nfl_qb_stats (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    season INTEGER NOT NULL,
    season_type TEXT NOT NULL DEFAULT 'regular',
    stat_date DATE NOT NULL,
    player_name TEXT NOT NULL,
    team_external_id TEXT NOT NULL,
    team_name TEXT NOT NULL,
    games_played INTEGER NOT NULL DEFAULT 0,
    pass_attempts INTEGER NOT NULL DEFAULT 0,
    completion_percentage DOUBLE PRECISION,
    yards_per_attempt DOUBLE PRECISION,
    epa_per_play DOUBLE PRECISION,
    success_rate DOUBLE PRECISION,
    cpoe DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, season, season_type, stat_date, external_id)
);

CREATE INDEX nfl_qb_stats_stat_date_idx
    ON nfl_qb_stats (stat_date DESC);

CREATE INDEX nfl_qb_stats_team_lookup_idx
    ON nfl_qb_stats (team_external_id, stat_date DESC);
