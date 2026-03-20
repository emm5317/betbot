# Paper-Mode Validation Runbook

## Purpose

This runbook defines the sustained validation cadence for the betbot paper-trading pipeline. It must be followed daily until P6-001 exit criteria are met. All diagnostic SQL is in `docs/sql/validation-queries.sql`.

---

## Daily Checks (~5 min)

### 1. Health endpoints

```bash
curl http://127.0.0.1:18080/health
# Expect: 200 OK

curl http://127.0.0.1:18080/pipeline/health
# Expect: recent poll within BETBOT_RECENT_POLL_WINDOW (default 20 min)
```

### 2. Bet status distribution

Run query #1 from `validation-queries.sql`. Expected:
- Zero `pending` bets (should transition to `placed` or `failed` immediately)
- Zero `failed` bets (impossible in paper mode — treat as P0 bug, see `placement-failure.md`)
- `placed` and `settled` counts growing daily

### 3. No stuck bets

Run query #2. Expected: **zero rows**. Any result means a placed bet's game has completed >4 hours ago without settlement. If rows appear:
- Check auto-settlement worker is running (query #10, look for `auto_settlement` jobs)
- Check if game scores are available (The Odds API scores endpoint)
- See settlement lag analysis (query #5)

### 4. Ledger integrity

Run query #3a (computed balance). Record the value.

Run query #3b (orphaned reservations). Expected: **zero rows**. Orphans indicate:
- A crashed placement that wrote Tx1 but never reached Tx2
- See `placement-failure.md` for repair procedure

Run query #3c (entry type breakdown). Verify:
- Every `bet_stake_reserved` has a matching `bet_stake_released` or `bet_settlement_*` entry (net to zero per bet lifecycle)

### 5. Unplaced recommendation backlog

Run query #4. Expected: small or empty. A large backlog indicates:
- Auto-placement worker is not running
- Recommendation snapshots fail `buildAutoPlacementInput` validation
- Check River job health (query #10) for `auto_placement` job status

### 6. River job health

Run query #10. Expected:
- All recent jobs in `completed` state
- Zero `discarded` jobs (indicates permanent failure after retries)
- `auto_placement` and `auto_settlement` jobs running at expected intervals (15 min / 30 min)
- `odds_poll` jobs running at configured interval (default 5 min)

---

## Weekly Checks (~15 min)

### 7. CLV tracking

```bash
curl "http://127.0.0.1:18080/recommendations/performance?sport=icehockey_nhl"
```

Review:
- `avg_clv_delta` — positive is good (we're beating the closing line)
- `settled_count` — growing week over week
- `avg_edge` — model's claimed edge at time of recommendation

### 8. Calibration drift

```bash
curl "http://127.0.0.1:18080/recommendations/calibration/alerts?mode=rolling&sport=icehockey_nhl"
```

Review:
- Alert `level` across rolling steps — look for trends
- If `critical`: see `model-miscalibration.md`
- If `insufficient_sample`: expected early in paper trading, continue monitoring

### 9. Bet distribution audit

```sql
SELECT sport, market_key, recommended_side, COUNT(*) AS bet_count,
       SUM(stake_cents) AS total_stake_cents
FROM bets
WHERE status IN ('placed', 'settled')
  AND created_at >= NOW() - INTERVAL '7 days'
GROUP BY sport, market_key, recommended_side
ORDER BY bet_count DESC;
```

Flag extreme concentration:
- >80% of bets on one side (e.g., all home) → model bias
- >90% of stake on one sport → missing sport coverage
- Single book receiving all bets → line shopping may not be working

### 10. Settlement accuracy spot-check

Run query #8. Pick 3-5 random settled bets and manually verify:
- `settlement_result` matches actual game score (home/away)
- `payout_cents` is correct given `american_odds` and `stake_cents`
- Win payout formula: `stake + stake * (odds/100)` for positive odds, `stake + stake * (100/|odds|)` for negative

### 11. Correlation guard verification

Run query #7. Expected: **zero rows** (no game has more than 1 bet, given default `BETBOT_CORRELATION_MAX_PICKS_PER_GAME=1`).

If rows appear, verify:
- The correlation policy allows it (env var override)
- Total stake per game is within `BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME`

### 12. Circuit breaker dry-run

Run query #6 and verify:
- Loss fractions are computed correctly from ledger balances
- Fractions match what `GET /recommendations` reports in its circuit breaker status
- If any fraction approaches its threshold (within 2%), note it for closer monitoring

---

## On-Alert Response

| Alert | Runbook |
|-------|---------|
| Failed bets appear | [placement-failure.md](placement-failure.md) |
| Circuit breaker fires | [circuit-breaker-triggered.md](circuit-breaker-triggered.md) |
| Calibration drift `warn` or `critical` | [model-miscalibration.md](model-miscalibration.md) |
| Odds polling stops | [data-pipeline-outage.md](data-pipeline-outage.md) |
| Threshold adjustment needed | [threshold-tuning.md](threshold-tuning.md) |

---

## Exit Criteria for P6-001 Gate

All of the following must be satisfied before enabling a live adapter:

| # | Criterion | How to verify |
|---|-----------|---------------|
| 1 | 14 consecutive days of clean daily checks | Review log of daily check results |
| 2 | 50+ bets placed and settled through full pipeline | `SELECT COUNT(*) FROM bets WHERE status = 'settled'` |
| 3 | Ledger integrity verified 3+ times with zero discrepancies | Query #3, recorded dates |
| 4 | CLV measurement operational | `GET /recommendations/performance` returns populated data |
| 5 | At least one circuit breaker scenario manually tested | Temporarily lower a threshold, verify recommendations are dropped, restore |
| 6 | Settlement lag consistently <60 min after game completion | Query #5, all rows show `settlement_lag_minutes < 60` |
| 7 | Zero stuck bets across the validation period | Query #2 has returned zero rows on every daily check |

### Circuit breaker manual test procedure

1. Note current `BETBOT_DAILY_LOSS_STOP` value
2. Set it to `0.001` (extremely tight) and restart the server
3. Confirm `GET /recommendations` returns dropped recommendations with `circuit_check_reason: dropped_daily_loss_stop`
4. Restore the original threshold and restart
5. Confirm recommendations flow normally again

---

## Validation Log Template

Use this template to record each daily check:

```
Date: YYYY-MM-DD
Health: [OK / ISSUE]
Bet status (24h): pending=X placed=X settled=X failed=X
Stuck bets: X
Ledger balance: X cents
Orphaned reservations: X
Unplaced backlog: X
River jobs discarded: X
Notes:
```
