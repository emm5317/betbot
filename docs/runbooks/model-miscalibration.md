# Model Miscalibration

## What Happened

A calibration drift alert has fired via `GET /recommendations/calibration/alerts`. The system detected that the model's predicted probabilities have drifted from their historical baseline beyond configured thresholds.

Alert levels:
- `insufficient_sample` — not enough settled bets to evaluate (< 100 overall or < 20 per bucket)
- `ok` — drift within thresholds
- `warn` — ECE delta >= 0.02 or Brier delta >= 0.01
- `critical` — ECE delta >= 0.05 or Brier delta >= 0.02

## Diagnosis

### 1. Check alert history trend

```bash
curl http://127.0.0.1:18080/recommendations/calibration/alerts/history?sport=<sport>&limit=10
```

Or run query #11 from `docs/sql/validation-queries.sql` to see recent alert runs and whether the drift is increasing, stable, or recovering.

### 2. Check rolling drift windows

```bash
curl "http://127.0.0.1:18080/recommendations/calibration/alerts?sport=<sport>&mode=rolling"
```

Rolling mode shows 5 overlapping 14-day windows. Look for:
- **Sudden jump in one step** — likely a single bad batch of predictions or unusual game outcomes
- **Steady upward trend across steps** — systematic drift, model may need retraining
- **Oscillation** — normal variance, especially with small sample sizes

### 3. Examine per-bucket calibration

```bash
curl "http://127.0.0.1:18080/recommendations/calibration?sport=<sport>"
```

Identify which probability buckets are miscalibrated:
- High-confidence buckets (top/bottom) miscalibrated → model is overconfident
- Mid-range buckets miscalibrated → model has poor discrimination
- All buckets shifted in one direction → systematic bias

### 4. Cross-reference with recent bet outcomes

Run query #8 from `docs/sql/validation-queries.sql` (Settlement accuracy spot-check) filtered to the sport in question. Look for:
- Streaks of unexpected outcomes
- Unusual score lines that could skew calibration
- Bets on games that were postponed/rescheduled

## Response by Severity

### `insufficient_sample`

No action required. The system needs more settled bets before calibration is meaningful. Continue monitoring — this status is expected in the first weeks of paper trading.

### `warn`

1. **Monitor for 7 days** — warn-level drift often self-corrects with more data
2. Check if the drift is concentrated in specific buckets or sport-wide
3. Review CLV performance via `GET /recommendations/performance` — if CLV is still positive, the model may be fine despite calibration drift
4. Log the alert and date in your review notes
5. No changes to auto-placement needed

### `critical`

1. **Consider disabling auto-placement** for the affected sport:
   - Set `BETBOT_AUTO_PLACEMENT_ENABLED=false` or
   - Restart the worker without the sport's prediction job
2. Review the full calibration report for the sport
3. Check if a specific model version or feature input changed recently
4. If the drift is real and sustained (3+ consecutive critical alerts):
   - Run a backtest against recent historical data to validate
   - Compare backtest calibration against live calibration
   - The model may need retraining or feature engineering updates

## Resolution

### Calibration recovers naturally

If the alert drops back to `ok` or `warn` within 7 days:
- Re-enable auto-placement if it was disabled
- Document the episode and likely cause (e.g., unusual game week, small sample)

### Model needs retraining

If critical drift persists:
1. Run `go run cmd/backtest/main.go --sport <sport> --season <current>` with the latest data
2. Compare backtest `pipeline_report.json` calibration metrics against the live calibration endpoint
3. If backtest also shows drift, the model's assumptions have shifted — retrain or adjust features
4. If backtest looks fine but live does not, investigate data pipeline issues (stale features, missing odds updates)
5. **Never deploy a retrained model without backtesting** (critical invariant #9)

### Thresholds need adjustment

If alerts fire too frequently on acceptable drift levels, adjust thresholds via `docs/runbooks/threshold-tuning.md`. The relevant thresholds are:

| Threshold | Default | Env var (not currently exposed) | Source |
|-----------|---------|--------------------------------|--------|
| Warn ECE delta | 0.02 | — | `calibration_alerts.go:DefaultWarnECEDelta` |
| Critical ECE delta | 0.05 | — | `calibration_alerts.go:DefaultCriticalECEDelta` |
| Warn Brier delta | 0.01 | — | `calibration_alerts.go:DefaultWarnBrierDelta` |
| Critical Brier delta | 0.02 | — | `calibration_alerts.go:DefaultCriticalBrierDelta` |
| Min settled overall | 100 | — | `calibration_alerts.go:DefaultMinSettledOverall` |
| Min settled per bucket | 20 | — | `calibration_alerts.go:DefaultMinSettledPerBucket` |

These are currently compile-time defaults. Changing them requires a code update and redeploy.

## Source Files

- `internal/decision/calibration_alerts.go` — drift thresholds and evaluation logic
- `internal/decision/calibration_alerts_rolling.go` — rolling window drift (14-day windows, 5 steps)
- `internal/decision/calibration.go` — base calibration computation
- `internal/server/calibration.go` — HTTP endpoints
