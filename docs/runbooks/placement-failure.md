# Placement Failure

## What Happened

One or more bets have `status = 'failed'` in the `bets` table. This means the `PlacementOrchestrator` in `internal/execution/placement.go` completed Tx1 (insert pending bet + reserve stake) but the adapter call failed, so Tx2 marked the bet as failed and released the reserved stake.

## Diagnosis

### 1. Find failed bets

```sql
SELECT id, idempotency_key, sport, market_key, book_key, stake_cents,
       error_message, adapter_name, created_at
FROM bets
WHERE status = 'failed'
ORDER BY created_at DESC
LIMIT 20;
```

### 2. Check the error message

The `error_message` column contains the adapter error. In paper mode, the paper adapter always succeeds, so:

**In paper mode: a failed bet should be impossible.** Treat any paper-mode failure as a P0 bug — the orchestrator, adapter wiring, or database state has a defect.

In live mode, common causes include:
- Network timeout to sportsbook API
- Odds moved / bet rejected by book
- Account limit reached
- API authentication failure

### 3. Verify ledger balance was restored

The two-transaction protocol releases the stake on failure. Confirm:

```sql
-- For a specific failed bet's idempotency key:
SELECT id, entry_type, amount_cents, reference_id, created_at
FROM bankroll_ledger
WHERE reference_id = '<idempotency_key>'
ORDER BY created_at ASC;
```

You should see exactly two entries:
1. `bet_stake_reserved` with negative `amount_cents`
2. `bet_stake_released` with positive `amount_cents` (same magnitude)

If only the reservation exists without a release, there is a ledger leak — see **Ledger repair** below.

### 4. Verify idempotency

The same idempotency key should not be retried after failure. The orchestrator allows re-placement only if the existing bet has `status = 'failed'`:

```sql
SELECT id, status, idempotency_key, created_at
FROM bets
WHERE idempotency_key = '<key>'
ORDER BY created_at ASC;
```

If you see multiple rows for the same key with different IDs, investigate the idempotency check path.

## Resolution

### Paper mode: investigate as a bug

1. Check server/worker logs for the full error stack
2. Look for database connectivity issues around the failure timestamp
3. Check if the `games` row still exists (cascade delete could orphan bets)
4. File a bug — paper adapter failures indicate an infrastructure or logic defect

### Live mode: expected failure path

1. The stake has been released — no financial exposure
2. If the failure is transient (network), the next auto-placement cycle (15 min) may generate a new recommendation snapshot and attempt placement again
3. If the failure is systemic (account limited, auth failure), see `docs/runbooks/account-limited.md`

### Ledger repair

If a reservation exists without a matching release:

1. **Do not DELETE or UPDATE** `bankroll_ledger`
2. Insert a correcting entry:
   ```sql
   INSERT INTO bankroll_ledger (entry_type, amount_cents, currency, reference_type, reference_id, metadata)
   VALUES ('bet_stake_released', <stake_cents>, 'USD', 'bet', '<idempotency_key>',
           '{"reason": "manual_repair", "bet_id": <bet_id>}'::jsonb);
   ```
3. Verify with query #3 from `docs/sql/validation-queries.sql`

## Source Files

- `internal/execution/placement.go` — two-transaction placement protocol
- `internal/execution/audit.go` — settlement ledger writes
- `internal/worker/placement_job.go` — auto-placement worker (15 min interval, 200 row limit)
- `internal/execution/adapters/paper/` — paper adapter (always succeeds)
