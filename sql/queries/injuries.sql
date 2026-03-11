-- name: UpsertPlayerInjuryReport :exec
INSERT INTO player_injury_reports (
    source,
    sport,
    report_date,
    external_id,
    player_name,
    team_external_id,
    position,
    injury,
    status,
    estimated_return,
    player_url,
    raw_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (source, sport, report_date, external_id) DO UPDATE
SET
    player_name = EXCLUDED.player_name,
    team_external_id = EXCLUDED.team_external_id,
    position = EXCLUDED.position,
    injury = EXCLUDED.injury,
    status = EXCLUDED.status,
    estimated_return = EXCLUDED.estimated_return,
    player_url = EXCLUDED.player_url,
    raw_json = EXCLUDED.raw_json,
    updated_at = NOW()
WHERE
    player_injury_reports.player_name IS DISTINCT FROM EXCLUDED.player_name
    OR player_injury_reports.team_external_id IS DISTINCT FROM EXCLUDED.team_external_id
    OR player_injury_reports.position IS DISTINCT FROM EXCLUDED.position
    OR player_injury_reports.injury IS DISTINCT FROM EXCLUDED.injury
    OR player_injury_reports.status IS DISTINCT FROM EXCLUDED.status
    OR player_injury_reports.estimated_return IS DISTINCT FROM EXCLUDED.estimated_return
    OR player_injury_reports.player_url IS DISTINCT FROM EXCLUDED.player_url
    OR player_injury_reports.raw_json IS DISTINCT FROM EXCLUDED.raw_json;
