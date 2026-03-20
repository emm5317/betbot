# Circuit Breaker Triggered

## What Happened

The circuit breaker guard in `internal/decision/circuit.go` dropped one or more recommendations because a loss threshold was breached. When circuit breakers fire, `GET /recommendations` returns recommendations with `circuit_check_pass: false` and a reason code indicating which threshold tripped.

Possible reason codes:
- `dropped_daily_loss_stop` — daily loss fraction >= `BETBOT_DAILY_LOSS_STOP` (default: 5%)
- `dropped_weekly_loss_stop` — weekly loss fraction >= `BETBOT_WEEKLY_LOSS_STOP` (default: 10%)
- `dropped_drawdown_breaker` — drawdown from peak >= `BETBOT_DRAWDOWN_BREAKER` (default: 15%)

## Diagnosis

### 1. Identify which threshold triggered

Check the recommendation response or logs for the `circuit_check_reason` field. Multiple thresholds can fire simultaneously — the first triggered code is the primary reason.

### 2. Verify current metrics from the ledger

Run query #6 from `docs/sql/validation-queries.sql` (Circuit breaker metrics) to see:
- Current balance vs day-start, week-start, and peak balances
- Computed daily, weekly, and drawdown loss fractions
- Whether fractions genuinely exceed thresholds

### 3. Check for ledger anomalies

If the fractions look unexpectedly high:
- Run query #3 (Ledger integrity) to check for orphaned reservations or missing settlement entries
- Look for duplicate `bet_stake_reserved` entries without matching releases
- Check for settlement ledger entries with wrong amounts

### 4. Review recent bet activity

```sql
SELECT id, sport, market_key, stake_cents, settlement_result, payout_cents, settled_at
FROM bets
WHERE status = 'settled'
  AND settled_at >= DATE_TRUNC('day', NOW())
ORDER BY settled_at DESC;
```

## Resolution

### If the trigger is legitimate (real losses)

The circuit breaker is working as designed. The system will resume placing bets once the loss window rolls over:
- **Daily loss stop**: resets at midnight UTC (start of next `DATE_TRUNC('day', NOW())`)
- **Weekly loss stop**: resets at Monday midnight UTC (start of next `DATE_TRUNC('week', NOW())`)
- **Drawdown breaker**: only recovers when balance rises (winning bets or manual deposit)

No action needed — wait for the window to roll over.

### If the trigger is due to a ledger error

1. Identify the bad entries using query #3
2. **Do not DELETE or UPDATE** `bankroll_ledger` (append-only invariant)
3. Insert correcting entries with `entry_type = 'adjustment'` and clear metadata explaining the fix
4. Verify the circuit breaker metrics return to expected values

### When to adjust thresholds

Adjust thresholds only if:
- The current values cause excessive false-positive halts relative to expected variance
- You have 50+ settled bets and can compute typical drawdown/loss patterns
- You follow the tuning procedure in `docs/runbooks/threshold-tuning.md`

See `docs/runbooks/threshold-tuning.md` for the full tuning process.

## Source Files

- `internal/decision/circuit.go` — circuit breaker evaluation logic
- `internal/config/config.go` — env var defaults (`BETBOT_DAILY_LOSS_STOP`, `BETBOT_WEEKLY_LOSS_STOP`, `BETBOT_DRAWDOWN_BREAKER`)
- `sql/queries/bankroll.sql` — `GetBankrollCircuitMetrics` query
