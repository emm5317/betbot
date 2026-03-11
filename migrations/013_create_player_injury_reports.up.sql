CREATE TABLE player_injury_reports (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    sport TEXT NOT NULL,
    report_date DATE NOT NULL,
    external_id TEXT NOT NULL,
    player_name TEXT NOT NULL,
    team_external_id TEXT NOT NULL,
    position TEXT NOT NULL,
    injury TEXT NOT NULL,
    status TEXT NOT NULL,
    estimated_return TEXT,
    player_url TEXT NOT NULL,
    raw_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, sport, report_date, external_id)
);

CREATE INDEX player_injury_reports_report_date_idx
    ON player_injury_reports (sport, report_date DESC);

CREATE INDEX player_injury_reports_team_lookup_idx
    ON player_injury_reports (team_external_id, report_date DESC);
