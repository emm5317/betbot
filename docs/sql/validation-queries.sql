-- =============================================================================
-- betbot Paper-Mode Validation Queries
-- =============================================================================
-- Copy-pasteable queries for pipeline health verification.
-- Run against betbot-postgres (port 25432 local dev).
-- Organized by concern; each query is self-contained.
-- =============================================================================


-- ---------------------------------------------------------------------------
-- 1. Bet status distribution (24h)
-- Purpose: Daily heartbeat — confirms bets are flowing through the pipeline.
-- ---------------------------------------------------------------------------
SELECT
    status,
    COUNT(*)                          AS bet_count,
    COALESCE(SUM(stake_cents), 0)     AS total_stake_cents
FROM bets
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY status
ORDER BY status;


-- ---------------------------------------------------------------------------
-- 2. Stuck bets (placed, game completed >4h ago)
-- Purpose: Settlement lag detection — placed bets whose game already finished.
-- ---------------------------------------------------------------------------
SELECT
    b.id            AS bet_id,
    b.sport,
    b.market_key,
    b.recommended_side,
    b.book_key,
    b.stake_cents,
    b.placed_at,
    g.commence_time,
    gr.status       AS game_status,
    gr.home_score,
    gr.away_score,
    gr.captured_at  AS result_captured_at,
    NOW() - gr.captured_at AS time_since_result
FROM bets b
JOIN games g ON g.id = b.game_id
JOIN LATERAL (
    SELECT status, home_score, away_score, captured_at
    FROM game_results
    WHERE game_id = b.game_id
      AND LOWER(status) = 'final'
    ORDER BY captured_at DESC, id DESC
    LIMIT 1
) gr ON TRUE
WHERE b.status = 'placed'
  AND gr.captured_at < NOW() - INTERVAL '4 hours'
ORDER BY gr.captured_at ASC;


-- ---------------------------------------------------------------------------
-- 3. Ledger integrity (SUM check + orphan detection)
-- Purpose: Financial consistency — balance matches and no orphaned entries.
-- ---------------------------------------------------------------------------

-- 3a. Computed balance
SELECT
    COALESCE(SUM(amount_cents), 0) AS computed_balance_cents
FROM bankroll_ledger;

-- 3b. Orphaned reservations: bet_stake_reserved entries without a matching
--     bet_stake_released or bet_settlement_* entry for the same reference_id.
SELECT
    bl.id,
    bl.entry_type,
    bl.amount_cents,
    bl.reference_id,
    bl.created_at
FROM bankroll_ledger bl
WHERE bl.entry_type = 'bet_stake_reserved'
  AND NOT EXISTS (
      SELECT 1
      FROM bankroll_ledger bl2
      WHERE bl2.reference_id = bl.reference_id
        AND bl2.entry_type IN (
            'bet_stake_released',
            'bet_settlement_win',
            'bet_settlement_loss',
            'bet_settlement_push'
        )
  )
  -- Exclude very recent reservations (may be mid-flight)
  AND bl.created_at < NOW() - INTERVAL '1 hour'
ORDER BY bl.created_at ASC;

-- 3c. Ledger entry type breakdown
SELECT
    entry_type,
    COUNT(*)                       AS entry_count,
    COALESCE(SUM(amount_cents), 0) AS total_cents
FROM bankroll_ledger
GROUP BY entry_type
ORDER BY entry_type;


-- ---------------------------------------------------------------------------
-- 4. Unplaced recommendation backlog
-- Purpose: Auto-placement health — snapshots that qualify but have no bet.
-- ---------------------------------------------------------------------------
SELECT
    rs.id           AS snapshot_id,
    rs.sport,
    rs.game_id,
    rs.market_key,
    rs.recommended_side,
    rs.best_book,
    rs.suggested_stake_cents,
    rs.edge,
    rs.rank_score,
    rs.created_at   AS snapshot_created_at
FROM recommendation_snapshots rs
WHERE rs.suggested_stake_cents > 0
  AND NOT EXISTS (
      SELECT 1
      FROM bets b
      WHERE b.snapshot_id = rs.id
  )
ORDER BY rs.rank_score DESC, rs.id ASC
LIMIT 50;


-- ---------------------------------------------------------------------------
-- 5. Settlement lag (minutes from game result capture to settled_at)
-- Purpose: Settlement SLA — how long after game completion until bet settles.
-- ---------------------------------------------------------------------------
SELECT
    b.id            AS bet_id,
    b.sport,
    b.market_key,
    b.settled_at,
    gr.captured_at  AS result_captured_at,
    EXTRACT(EPOCH FROM (b.settled_at - gr.captured_at)) / 60.0 AS settlement_lag_minutes
FROM bets b
JOIN LATERAL (
    SELECT captured_at
    FROM game_results
    WHERE game_id = b.game_id
      AND LOWER(status) = 'final'
    ORDER BY captured_at DESC, id DESC
    LIMIT 1
) gr ON TRUE
WHERE b.status = 'settled'
  AND b.settled_at IS NOT NULL
ORDER BY b.settled_at DESC
LIMIT 50;


-- ---------------------------------------------------------------------------
-- 6. Circuit breaker metrics (current state)
-- Purpose: Guardrail verification — current loss fractions vs thresholds.
-- ---------------------------------------------------------------------------
WITH anchor AS (
    SELECT
        NOW()                        AS now_ts,
        DATE_TRUNC('day', NOW())     AS day_start_ts,
        DATE_TRUNC('week', NOW())    AS week_start_ts
),
ledger_totals AS (
    SELECT
        COALESCE(SUM(amount_cents), 0)::BIGINT AS current_balance_cents,
        COALESCE(SUM(amount_cents) FILTER (WHERE created_at < (SELECT day_start_ts FROM anchor)), 0)::BIGINT AS day_start_balance_cents,
        COALESCE(SUM(amount_cents) FILTER (WHERE created_at < (SELECT week_start_ts FROM anchor)), 0)::BIGINT AS week_start_balance_cents
    FROM bankroll_ledger
),
running_balance AS (
    SELECT
        SUM(amount_cents) OVER (ORDER BY created_at ASC, id ASC) AS balance_after_cents
    FROM bankroll_ledger
),
peak_balance AS (
    SELECT
        COALESCE(MAX(balance_after_cents), 0)::BIGINT AS peak_balance_cents
    FROM running_balance
)
SELECT
    l.current_balance_cents,
    l.day_start_balance_cents,
    l.week_start_balance_cents,
    p.peak_balance_cents,
    -- Computed loss fractions (matches decision/circuit.go logic)
    CASE WHEN l.day_start_balance_cents > 0 AND l.current_balance_cents < l.day_start_balance_cents
         THEN ROUND((l.day_start_balance_cents - l.current_balance_cents)::NUMERIC / l.day_start_balance_cents, 4)
         ELSE 0 END AS daily_loss_fraction,
    CASE WHEN l.week_start_balance_cents > 0 AND l.current_balance_cents < l.week_start_balance_cents
         THEN ROUND((l.week_start_balance_cents - l.current_balance_cents)::NUMERIC / l.week_start_balance_cents, 4)
         ELSE 0 END AS weekly_loss_fraction,
    CASE WHEN p.peak_balance_cents > 0 AND l.current_balance_cents < p.peak_balance_cents
         THEN ROUND((p.peak_balance_cents - l.current_balance_cents)::NUMERIC / p.peak_balance_cents, 4)
         ELSE 0 END AS drawdown_fraction,
    -- Default thresholds for reference
    0.05 AS daily_loss_stop_default,
    0.10 AS weekly_loss_stop_default,
    0.15 AS drawdown_breaker_default
FROM ledger_totals l
CROSS JOIN peak_balance p;


-- ---------------------------------------------------------------------------
-- 7. Correlation guard violations (multi-bet games)
-- Purpose: Exposure check — games with more than 1 placed/settled bet.
-- ---------------------------------------------------------------------------
SELECT
    b.game_id,
    g.home_team,
    g.away_team,
    b.sport,
    COUNT(*)                      AS bet_count,
    SUM(b.stake_cents)            AS total_stake_cents,
    ARRAY_AGG(b.id ORDER BY b.id) AS bet_ids
FROM bets b
JOIN games g ON g.id = b.game_id
WHERE b.status IN ('placed', 'settled')
GROUP BY b.game_id, g.home_team, g.away_team, b.sport
HAVING COUNT(*) > 1
ORDER BY COUNT(*) DESC;


-- ---------------------------------------------------------------------------
-- 8. Settlement accuracy spot-check (bets + game_results)
-- Purpose: Manual verification — compare bet outcomes against actual scores.
-- ---------------------------------------------------------------------------
SELECT
    b.id            AS bet_id,
    b.sport,
    b.market_key,
    b.recommended_side,
    b.american_odds,
    b.stake_cents,
    b.settlement_result,
    b.payout_cents,
    b.clv_delta,
    g.home_team,
    g.away_team,
    gr.home_score,
    gr.away_score,
    gr.status       AS game_status
FROM bets b
JOIN games g ON g.id = b.game_id
LEFT JOIN LATERAL (
    SELECT home_score, away_score, status
    FROM game_results
    WHERE game_id = b.game_id
      AND LOWER(status) = 'final'
    ORDER BY captured_at DESC, id DESC
    LIMIT 1
) gr ON TRUE
WHERE b.status = 'settled'
ORDER BY b.settled_at DESC
LIMIT 20;


-- ---------------------------------------------------------------------------
-- 9. PnL summary by sport
-- Purpose: Performance overview — net profit/loss per sport.
-- ---------------------------------------------------------------------------
SELECT
    sport,
    COUNT(*)                                                           AS total_bets,
    COUNT(*) FILTER (WHERE status = 'settled')                         AS settled_bets,
    COUNT(*) FILTER (WHERE status = 'placed')                          AS open_bets,
    COALESCE(SUM(stake_cents) FILTER (WHERE status IN ('placed','settled')), 0) AS total_staked_cents,
    COALESCE(SUM(payout_cents) FILTER (WHERE status = 'settled'), 0)   AS total_returned_cents,
    COALESCE(SUM(payout_cents) FILTER (WHERE status = 'settled'), 0)
      - COALESCE(SUM(stake_cents) FILTER (WHERE status = 'settled'), 0) AS net_pnl_cents,
    COUNT(*) FILTER (WHERE status = 'settled' AND settlement_result = 'win') AS wins,
    COUNT(*) FILTER (WHERE status = 'settled' AND settlement_result = 'loss') AS losses,
    COUNT(*) FILTER (WHERE status = 'settled' AND settlement_result = 'push') AS pushes,
    ROUND(AVG(clv_delta) FILTER (WHERE status = 'settled' AND clv_delta IS NOT NULL), 4) AS avg_clv_delta
FROM bets
GROUP BY sport
ORDER BY sport;


-- ---------------------------------------------------------------------------
-- 10. River job health (24h)
-- Purpose: Worker health — job state distribution for recent jobs.
-- ---------------------------------------------------------------------------
SELECT
    kind,
    state,
    COUNT(*)                          AS job_count,
    MIN(created_at)                   AS earliest,
    MAX(created_at)                   AS latest
FROM river_job
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY kind, state
ORDER BY kind, state;


-- ---------------------------------------------------------------------------
-- 11. Calibration alert history (recent runs)
-- Purpose: Drift trend — recent calibration alert evaluation runs.
-- ---------------------------------------------------------------------------
SELECT
    id,
    sport,
    mode,
    alert_level,
    reasons,
    step_index,
    step_count,
    ece_delta,
    brier_delta,
    created_at
FROM recommendation_calibration_alert_runs
ORDER BY created_at DESC
LIMIT 20;


-- ---------------------------------------------------------------------------
-- 12. Recent poll runs
-- Purpose: Ingestion heartbeat — confirm odds polling is running.
-- ---------------------------------------------------------------------------
SELECT
    id,
    source,
    started_at,
    finished_at,
    status,
    games_seen,
    snapshots_seen,
    inserts_count,
    dedup_skips,
    error_text
FROM poll_runs
ORDER BY started_at DESC
LIMIT 10;
