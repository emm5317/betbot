CREATE TABLE recommendation_calibration_alert_runs (
    id BIGSERIAL PRIMARY KEY,
    sport TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    run_group_hash TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('point_in_time', 'rolling')),
    step_index INTEGER NOT NULL CHECK (step_index >= 0),
    step_count INTEGER NOT NULL CHECK (step_count >= 1),
    window_days INTEGER CHECK (window_days IS NULL OR window_days >= 1),
    current_from DATE,
    current_to DATE,
    baseline_from DATE,
    baseline_to DATE,
    bucket_count INTEGER NOT NULL CHECK (bucket_count >= 1),
    row_limit INTEGER NOT NULL CHECK (row_limit >= 1),
    min_settled_overall INTEGER NOT NULL CHECK (min_settled_overall >= 1),
    min_settled_per_bucket INTEGER NOT NULL CHECK (min_settled_per_bucket >= 1),
    warn_ece_delta DOUBLE PRECISION NOT NULL,
    critical_ece_delta DOUBLE PRECISION NOT NULL,
    warn_brier_delta DOUBLE PRECISION NOT NULL,
    critical_brier_delta DOUBLE PRECISION NOT NULL,
    alert_level TEXT NOT NULL,
    reasons JSONB NOT NULL DEFAULT '[]'::JSONB,
    current_overall_ece DOUBLE PRECISION NOT NULL,
    baseline_overall_ece DOUBLE PRECISION NOT NULL,
    ece_delta DOUBLE PRECISION NOT NULL,
    current_overall_brier DOUBLE PRECISION NOT NULL,
    baseline_overall_brier DOUBLE PRECISION NOT NULL,
    brier_delta DOUBLE PRECISION NOT NULL,
    current_settled_rows INTEGER NOT NULL CHECK (current_settled_rows >= 0),
    baseline_settled_rows INTEGER NOT NULL CHECK (baseline_settled_rows >= 0),
    insufficient_overall_windows INTEGER NOT NULL CHECK (insufficient_overall_windows >= 0),
    current_insufficient_buckets INTEGER NOT NULL CHECK (current_insufficient_buckets >= 0),
    baseline_insufficient_buckets INTEGER NOT NULL CHECK (baseline_insufficient_buckets >= 0),
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX recommendation_calibration_alert_runs_sport_created_idx
    ON recommendation_calibration_alert_runs (sport, created_at DESC, id DESC);

CREATE INDEX recommendation_calibration_alert_runs_created_idx
    ON recommendation_calibration_alert_runs (created_at DESC, id DESC);

CREATE INDEX recommendation_calibration_alert_runs_group_idx
    ON recommendation_calibration_alert_runs (run_group_hash, step_index ASC, id ASC);
