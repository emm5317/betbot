# NHL MoneyPuck Backtesting Pipeline

## Overview

This document covers the NHL backtesting pipeline built on MoneyPuck historical data. The system replaces synthetic/deterministic features with real NHL analytics (xG, goalie GSAx, PDO) for model validation against actual game outcomes and market odds.

---

## Data Sources

### MoneyPuck Team Stats (`moneypuck_team_games`)

Per-game team-level stats from MoneyPuck, filtered to `5on5` (model features) and `all` (game scores) situations.

| Field | Usage |
|---|---|
| `xgoals_percentage` | Primary xG share signal (5on5) |
| `xgoals_for` / `xgoals_against` | Raw expected goals (5on5) |
| `score_venue_adjusted_xgoals_for/against` | Home/score-adjusted xG (5on5) |
| `corsi_percentage` / `fenwick_percentage` | Shot attempt dominance (5on5) |
| `shots_on_goal_for/against` | For PDO computation (5on5) |
| `goals_for` / `goals_against` | Actual scoring, PDO, game results (all) |
| `high_danger_shots_for/against` | Shot quality (5on5) |

**Coverage:** 18 seasons (2008–2025), 91,480 rows, 30–32 teams per season.

### MoneyPuck Goalie Stats (`moneypuck_goalie_games`)

Per-game goalie stats. GSAx (Goals Saved Above Expected) = `xgoals - goals`.

| Field | Usage |
|---|---|
| `icetime` | Identifies starting goalie (most 5on5 ice time) |
| `xgoals` | Expected goals against |
| `goals` | Actual goals against |
| `gsax` | Pre-computed `xgoals - goals` (positive = good) |

**Coverage:** 17 seasons (2008–2025), 91,476 rows.

### Odds Data (`games` + `odds_history`)

| Source | Format | Games | Seasons | Markets |
|---|---|---|---|---|
| `NHL 2023-24 data.csv` | `--type odds` | 957 | 2023–24 (Dec–Jun) | h2h, spreads, totals |
| `nhl-202425-asplayed.csv` | `--type asplayed` | 1,312 | 2024–25 | Scores only (no odds) |
| `nhl-202526-asplayed.csv` | `--type asplayed` | 1,042 | 2025–26 (Oct–Apr) | h2h, spreads, totals |

---

## Import Pipeline

### CLI: `cmd/import/moneypuck/main.go`

```bash
# MoneyPuck team stats (all seasons)
go run cmd/import/moneypuck/main.go --type teams --file "all_teams.csv"

# MoneyPuck goalies
go run cmd/import/moneypuck/main.go --type goalies --file 2008_to_2024.csv
go run cmd/import/moneypuck/main.go --type goalies --file "2025 -2.csv"

# 2023-24 odds (snake_case team names)
go run cmd/import/moneypuck/main.go --type odds --file "NHL 2023-24 data.csv"

# 2024-25 scores only, 2025-26 scores + odds (Odds API full names)
go run cmd/import/moneypuck/main.go --type asplayed --file nhl-202425-asplayed.csv
go run cmd/import/moneypuck/main.go --type asplayed --file nhl-202526-asplayed.csv
```

**Flags:** `--dry-run`, `--season-filter <year>`, `--batch-size <n>`

All imports are idempotent (upsert on unique constraints).

### Team Name Mapping

Three naming formats are normalized to a canonical 3-letter abbreviation:

| Format | Example | Used By |
|---|---|---|
| MoneyPuck abbreviation | `TBL`, `T.B` (legacy) | MoneyPuck CSVs |
| Snake_case | `tampa_bay_lightning` | 2023-24 odds CSV |
| Odds API full name | `Tampa Bay Lightning` | As-played CSVs, `games` table |

Handled by `internal/ingestion/moneypuck/teammap.go`. Includes legacy abbreviation drift (T.B→TBL in 2021), franchise relocations (ATL→WPG, ARI→UTA), and name aliases (Utah Hockey Club → Utah Mammoth in 2025-26).

---

## Backtesting Architecture

### Two Validation Paths

**Path A — Outcome-Based (all 18 seasons)**
- Uses actual game scores from MoneyPuck `all` situation rows
- Measures: calibration (predicted vs actual win rate), Brier score, log-loss
- No market odds required
- Available for 2008–2025

**Path B — CLV-Based (seasons with odds)**
- Uses market odds from `odds_history` as ground truth
- Measures: CLV (closing line value), model edge vs market
- Available for 2023-24 and 2025-26

Both paths use real MoneyPuck features (xG, GSAx, PDO) instead of synthetic hash-derived values.

### Rolling Feature Builder

For each game, team stats are computed from a rolling window of prior games **in the same season**, strictly before the game date (no future leakage).

| Model Input | Computation | Rolling Window |
|---|---|---|
| `NHLContext.HomeXGShare` | Mean `xgoals_percentage` (5on5) | 20 games |
| `NHLContext.HomeGoalieGSAx` | Cumulative season 5on5 GSAx for starting goalie | Season-to-date |
| `NHLContext.HomePDO` | `(GF/SOG_F) + (1 - GA/SOG_A)` (5on5) | 20 games |
| `TeamQuality.HomeOffenseRating` | `goals_for_per_game * 32` (5on5) | 20 games |
| `TeamQuality.HomeDefenseRating` | `goals_against_per_game * 34` (5on5) | 20 games |

**Minimum sample:** 10 games. Games earlier in the season fall back to deterministic features.

**Starting goalie:** Identified by most 5on5 ice time in the game (`GetStartingGoalie` query).

### Outcome Grading

For Path A, actual game results are queried from `moneypuck_team_games` (situation='all'):
- `home_goals > away_goals` → home win (outcome = 1.0)
- `home_goals < away_goals` → away win (outcome = 0.0)
- Calibration: "model predicted 60% home win → did home team win ~60% of the time?"

---

## Key Queries (`sql/queries/moneypuck.sql`)

| Query | Purpose |
|---|---|
| `GetTeamRolling5on5Stats` | Last N 5on5 games before a date for a team |
| `GetGameResult` | Actual goals from `all` situation (both teams) |
| `GetStartingGoalie` | Goalie with most 5on5 icetime in a game |
| `GetGoalieSeasonGSAx` | Cumulative GSAx before a date |
| `ListSeasonGameDates` | All game dates in a season |
| `ListSeasonTeamGames` | All games for a team in a season |

---

## Database Schema

### `moneypuck_team_games`
- **Unique:** `(game_id, team, situation)`
- **Indexes:** `(season, team, game_date)`, `(game_id, situation)`, `(game_date, situation)`

### `moneypuck_goalie_games`
- **Unique:** `(game_id, player_id, situation)`
- **Indexes:** `(season, team, game_date)`, `(team, situation, game_date, icetime DESC)`, `(player_id, season, game_date)`

---

## Current Data Inventory (as imported)

| Table | Rows |
|---|---|
| `moneypuck_team_games` | 91,480 |
| `moneypuck_goalie_games` | 91,476 |
| `games` | 3,311 |
| `odds_history` | 11,994 |

Odds coverage: 1,999 games with h2h + spreads + totals (2023-24 + 2025-26).
Scores coverage: 3,311 games (2023-24 + 2024-25 + 2025-26).
MoneyPuck coverage: ~21,000 games across 18 seasons.
