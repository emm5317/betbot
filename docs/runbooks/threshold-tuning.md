# Threshold Tuning Guide

## Overview

betbot uses configurable thresholds across the decision engine, execution guardrails, and calibration monitoring. This document inventories every tunable value, describes observation criteria for when to adjust, and defines the tuning procedure.

**Cardinal rule: change one threshold at a time, observe for 7 days, rollback if CLV degrades.**

---

## Threshold Inventory

### Decision Engine — Env Var Configurable

| Threshold | Env Var | Default | Valid Range | Source File | Purpose |
|-----------|---------|---------|-------------|-------------|---------|
| EV threshold | `BETBOT_EV_THRESHOLD` | `0.02` | (0, 1] | `config.go` | Minimum expected value edge to generate a recommendation |
| Kelly fraction | `BETBOT_KELLY_FRACTION` | `0` (sport defaults) | [0, 1] | `config.go` | Global fractional Kelly override; 0 = use sport defaults |
| Max bet fraction | `BETBOT_MAX_BET_FRACTION` | `0` (sport defaults) | [0, 1] | `config.go` | Global per-bet cap as fraction of bankroll; 0 = use sport defaults |

### Correlation Guard — Env Var Configurable

| Threshold | Env Var | Default | Valid Range | Source File | Purpose |
|-----------|---------|---------|-------------|-------------|---------|
| Max picks per game | `BETBOT_CORRELATION_MAX_PICKS_PER_GAME` | `1` | [1, 25] | `config.go`, `correlation.go` | Max retained recommendations per `sport\|game_id` |
| Max stake fraction per game | `BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME` | `0.03` | (0, 1] | `config.go`, `correlation.go` | Max summed stake fraction per game |
| Max picks per sport/day | `BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY` | `0` (disabled) | [0, 500] | `config.go`, `correlation.go` | Optional daily cap per sport; 0 disables |

### Circuit Breakers — Env Var Configurable

| Threshold | Env Var | Default | Valid Range | Source File | Purpose |
|-----------|---------|---------|-------------|-------------|---------|
| Daily loss stop | `BETBOT_DAILY_LOSS_STOP` | `0.05` | [0, 1] | `config.go`, `circuit.go` | Halt recommendations when daily bankroll loss reaches this fraction |
| Weekly loss stop | `BETBOT_WEEKLY_LOSS_STOP` | `0.10` | [0, 1] | `config.go`, `circuit.go` | Halt when weekly bankroll loss reaches this fraction |
| Drawdown breaker | `BETBOT_DRAWDOWN_BREAKER` | `0.15` | [0, 1] | `config.go`, `circuit.go` | Halt when drawdown from peak reaches this fraction |

### Calibration Drift — Compile-Time Defaults

| Threshold | Default | Source File | Purpose |
|-----------|---------|-------------|---------|
| Warn ECE delta | `0.02` | `calibration_alerts.go` | ECE drift level that triggers `warn` alert |
| Critical ECE delta | `0.05` | `calibration_alerts.go` | ECE drift level that triggers `critical` alert |
| Warn Brier delta | `0.01` | `calibration_alerts.go` | Brier score drift for `warn` |
| Critical Brier delta | `0.02` | `calibration_alerts.go` | Brier score drift for `critical` |
| Min settled overall | `100` | `calibration_alerts.go` | Minimum settled bets before drift evaluation is meaningful |
| Min settled per bucket | `20` | `calibration_alerts.go` | Minimum settled per calibration bucket |
| Rolling window days | `14` | `calibration_alerts_rolling.go` | Days per rolling drift comparison window |
| Rolling steps | `5` | `calibration_alerts_rolling.go` | Number of overlapping windows for trend detection |

### Execution — Env Var Configurable

| Threshold | Env Var | Default | Source File | Purpose |
|-----------|---------|---------|-------------|---------|
| Paper mode | `BETBOT_PAPER_MODE` | `true` | `config.go` | Master switch: `true` = paper adapter only |
| Execution adapter | `BETBOT_EXECUTION_ADAPTER` | `paper` | `config.go` | Adapter selection; must match paper mode state |
| Auto-placement enabled | `BETBOT_AUTO_PLACEMENT_ENABLED` | same as `BETBOT_PAPER_MODE` | `config.go` | Enables auto-placement worker; defaults to off in live mode |

### Worker Intervals — Compile-Time Defaults

| Parameter | Default | Source File | Purpose |
|-----------|---------|-------------|---------|
| Auto-placement interval | `15 min` | `placement_job.go` | How often the placement worker runs |
| Auto-placement row limit | `200` | `placement_job.go` | Max snapshots processed per placement run |
| Auto-settlement interval | `30 min` | `settlement_job.go` | How often the settlement worker runs |

### Ingestion — Env Var Configurable

| Threshold | Env Var | Default | Source File | Purpose |
|-----------|---------|---------|-------------|---------|
| Odds poll interval | `BETBOT_ODDS_API_POLL_INTERVAL` | `5m` | `config.go` | How often to poll The Odds API |
| Odds API rate limit | `BETBOT_ODDS_API_RATE_LIMIT_INTERVAL` | `750ms` | `config.go` | Minimum interval between API calls |
| Recent poll window | `BETBOT_RECENT_POLL_WINDOW` | `20m` | `config.go` | Window for "recent poll" health checks |
| Odds polling enabled | `BETBOT_ODDS_POLLING_ENABLED` | `true` | `config.go` | Master switch for odds polling |

---

## Observation Criteria

### When to tighten (make more conservative)

| Symptom | Threshold to adjust | Direction |
|---------|---------------------|-----------|
| Excessive bet volume per day | `BETBOT_EV_THRESHOLD` | Increase (e.g., 0.02 → 0.03) |
| Too many bets on same game | `BETBOT_CORRELATION_MAX_PICKS_PER_GAME` | Decrease to 1 |
| Large single-game exposure | `BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME` | Decrease (e.g., 0.03 → 0.02) |
| Drawdown too deep before halt | `BETBOT_DRAWDOWN_BREAKER` | Decrease (e.g., 0.15 → 0.10) |
| Calibration alerts fire too late | Warn/critical ECE/Brier deltas | Decrease (code change) |

### When to loosen (make more permissive)

| Symptom | Threshold to adjust | Direction |
|---------|---------------------|-----------|
| Very few recommendations despite edge | `BETBOT_EV_THRESHOLD` | Decrease (e.g., 0.02 → 0.015) |
| Circuit breaker firing too often on normal variance | `BETBOT_DAILY_LOSS_STOP` | Increase (e.g., 0.05 → 0.07) |
| False-positive calibration alerts | Warn ECE/Brier deltas | Increase (code change) |
| Calibration always `insufficient_sample` | Min settled overall/per bucket | Decrease (code change) |
| Single-sport daily cap blocking valid bets | `BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY` | Increase or set to 0 to disable |

---

## Tuning Procedure

### 1. Document the baseline

Before any change, record:
- Current threshold value
- Recent 7-day metrics (bet count, CLV, PnL, circuit breaker triggers)
- Reason for the change
- Expected outcome

### 2. Change one threshold

- For env-var thresholds: update `.env` and restart the affected service
- For compile-time thresholds: update the source constant, rebuild, and deploy
- **Never change more than one threshold at a time** — makes root-cause analysis impossible

### 3. Observe for 7 days

Monitor daily using the paper validation checklist (`docs/runbooks/paper-validation.md`):
- Did the symptom improve?
- Did CLV degrade?
- Did any guardrail fire unexpectedly?
- Did bet volume or exposure change significantly?

### 4. Evaluate

- **Improved, no regression**: keep the change, document the decision in commit message
- **No clear effect**: revert unless there is a strong theoretical reason to keep
- **CLV degraded**: revert immediately — the threshold existed for a reason
- **Mixed results**: extend observation to 14 days before deciding

### 5. Record

After each tuning round, update this runbook's inventory table if defaults changed, and note the date and rationale in the commit message.
