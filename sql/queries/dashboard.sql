-- name: InsertPollRun :one
INSERT INTO poll_runs (
    source,
    started_at
) VALUES (
    $1,
    $2
)
RETURNING id, source, started_at, finished_at, status, games_seen, snapshots_seen, inserts_count, dedup_skips, error_text;

-- name: CompletePollRun :exec
UPDATE poll_runs
SET
    finished_at = $2,
    status = $3,
    games_seen = $4,
    snapshots_seen = $5,
    inserts_count = $6,
    dedup_skips = $7,
    error_text = $8
WHERE id = $1;

-- name: GetLatestPollRun :one
SELECT id, source, started_at, finished_at, status, games_seen, snapshots_seen, inserts_count, dedup_skips, error_text
FROM poll_runs
ORDER BY started_at DESC
LIMIT 1;

-- name: GetDashboardSummary :one
SELECT
    (SELECT COUNT(*)::BIGINT FROM games WHERE commence_time >= NOW() - INTERVAL '7 days') AS games_count,
    (SELECT COUNT(*)::BIGINT FROM odds_history) AS snapshots_count,
    (SELECT MAX(captured_at)::timestamptz FROM odds_history) AS last_snapshot_at;
-- name: GetOddsArchiveSummary :one
SELECT
    COUNT(*)::BIGINT AS snapshots_count,
    MAX(oh.captured_at)::timestamptz AS last_snapshot_at
FROM odds_history AS oh
JOIN games AS g ON g.id = oh.game_id
WHERE
    sqlc.narg(sport)::text IS NULL
    OR g.sport = sqlc.narg(sport)::text;
