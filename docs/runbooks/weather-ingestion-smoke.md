# Weather Ingestion Smoke Harness

Purpose: deterministic local smoke check for MLB/NFL weather ingestion roof policy branches.

## Prerequisites

- Docker Desktop/Engine is running and accessible to the current shell.
- Local Postgres credentials in the command URL have permission to create/drop test databases.

## One-command run

```powershell
./scripts/weather_smoke.ps1 -EnsurePostgres
```

Optional explicit database URL:

```powershell
./scripts/weather_smoke.ps1 -DatabaseUrl "postgres://betbot:betbot-dev-password@127.0.0.1:5432/betbot?sslmode=disable" -EnsurePostgres
```

## What this smoke run verifies

The harness executes `TestWeatherSyncRiverSmokeHarness` and validates:

- Fixtures seeded for three game-linked branches:
  - Outdoor: `MLB Boston Red Sox` (`roof_type=outdoor`)
  - Fixed indoor: `NFL Atlanta Falcons` (`roof_type=dome`)
  - Retractable unknown: `NFL Dallas Cowboys` (`roof_type=retractable`)
- `weather_sync` is enqueued through River and reaches `completed`.
- Outdoor row has weather metrics (`temperature_f`, `wind_speed_mph`) populated.
- Dome row has weather metrics `NULL` and `raw_json.reason = fixed-roof-indoor`.
- Retractable row has weather metrics `NULL` and `raw_json.reason = retractable-roof-state-unknown`.
- Idempotent rerun behavior: second `weather_sync` completion preserves row count and keeps `updated_at` unchanged when values are unchanged.

## Expected pass output snippets

```text
=== RUN   TestWeatherSyncRiverSmokeHarness
--- PASS: TestWeatherSyncRiverSmokeHarness
PASS
weather smoke harness: PASS
```

## Expected fail signals

Look for assertion text from the smoke test, for example:

- `weather_sync job <id> reached terminal non-success state`
- `outdoor temperature_f is nil, expected provider metrics`
- `indoor reason = <...>, want "fixed-roof-indoor"`
- `updated_at changed on idempotent rerun`
