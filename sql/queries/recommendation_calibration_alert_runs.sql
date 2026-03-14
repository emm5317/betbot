-- name: InsertRecommendationCalibrationAlertRun :execrows
INSERT INTO recommendation_calibration_alert_runs (
    sport,
    request_hash,
    run_group_hash,
    mode,
    step_index,
    step_count,
    window_days,
    current_from,
    current_to,
    baseline_from,
    baseline_to,
    bucket_count,
    row_limit,
    min_settled_overall,
    min_settled_per_bucket,
    warn_ece_delta,
    critical_ece_delta,
    warn_brier_delta,
    critical_brier_delta,
    alert_level,
    reasons,
    current_overall_ece,
    baseline_overall_ece,
    ece_delta,
    current_overall_brier,
    baseline_overall_brier,
    brier_delta,
    current_settled_rows,
    baseline_settled_rows,
    insufficient_overall_windows,
    current_insufficient_buckets,
    baseline_insufficient_buckets,
    payload
) VALUES (
    sqlc.arg(sport),
    sqlc.arg(request_hash),
    sqlc.arg(run_group_hash),
    sqlc.arg(mode),
    sqlc.arg(step_index),
    sqlc.arg(step_count),
    sqlc.narg(window_days),
    sqlc.narg(current_from),
    sqlc.narg(current_to),
    sqlc.narg(baseline_from),
    sqlc.narg(baseline_to),
    sqlc.arg(bucket_count),
    sqlc.arg(row_limit),
    sqlc.arg(min_settled_overall),
    sqlc.arg(min_settled_per_bucket),
    sqlc.arg(warn_ece_delta),
    sqlc.arg(critical_ece_delta),
    sqlc.arg(warn_brier_delta),
    sqlc.arg(critical_brier_delta),
    sqlc.arg(alert_level),
    sqlc.arg(reasons)::jsonb,
    sqlc.arg(current_overall_ece),
    sqlc.arg(baseline_overall_ece),
    sqlc.arg(ece_delta),
    sqlc.arg(current_overall_brier),
    sqlc.arg(baseline_overall_brier),
    sqlc.arg(brier_delta),
    sqlc.arg(current_settled_rows),
    sqlc.arg(baseline_settled_rows),
    sqlc.arg(insufficient_overall_windows),
    sqlc.arg(current_insufficient_buckets),
    sqlc.arg(baseline_insufficient_buckets),
    sqlc.narg(payload)::jsonb
);

-- name: ListRecommendationCalibrationAlertRuns :many
SELECT
    id,
    sport,
    request_hash,
    run_group_hash,
    mode,
    step_index,
    step_count,
    window_days,
    current_from,
    current_to,
    baseline_from,
    baseline_to,
    bucket_count,
    row_limit,
    min_settled_overall,
    min_settled_per_bucket,
    warn_ece_delta,
    critical_ece_delta,
    warn_brier_delta,
    critical_brier_delta,
    alert_level,
    reasons,
    current_overall_ece,
    baseline_overall_ece,
    ece_delta,
    current_overall_brier,
    baseline_overall_brier,
    brier_delta,
    current_settled_rows,
    baseline_settled_rows,
    insufficient_overall_windows,
    current_insufficient_buckets,
    baseline_insufficient_buckets,
    payload,
    created_at
FROM recommendation_calibration_alert_runs
WHERE
    (
        sqlc.narg(sport)::text IS NULL
        OR sport = sqlc.narg(sport)::text
    )
    AND (
        sqlc.narg(date_from)::date IS NULL
        OR created_at::date >= sqlc.narg(date_from)::date
    )
    AND (
        sqlc.narg(date_to)::date IS NULL
        OR created_at::date <= sqlc.narg(date_to)::date
    )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);
