# BETBOT — Sports Betting Trading System

**Comprehensive Architecture & Implementation Plan**
Go · PostgreSQL · River · HTMX
March 2026 · CONFIDENTIAL

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Foundational Mental Model](#2-foundational-mental-model)
3. [Core Betting Strategies](#3-core-betting-strategies)
4. [Data Sources & Integration](#4-data-sources--integration)
5. [Technical Architecture](#5-technical-architecture)
6. [Go Implementation Details](#6-go-implementation-details)
7. [Risk Management](#7-risk-management)
8. [Backtesting Engine](#8-backtesting-engine)
9. [Monitoring & Dashboards](#9-monitoring--dashboards)
10. [Legal & Compliance](#10-legal--compliance)
11. [Implementation Roadmap](#11-implementation-roadmap)
12. [Appendix](#12-appendix)

---

## 1. Executive Summary

betbot is a quantitative sports betting trading system designed to identify, size, and execute positive expected value (EV) wagers across multiple sportsbooks. The system treats sports betting markets as prediction markets with an embedded vig, applying the same discipline used in quantitative finance: probability modeling, portfolio-aware position sizing, real-time execution, and rigorous performance attribution.

The core thesis is not to predict game outcomes — it is to find **mispriced probability**. A team can be a poor bet even at an 80% win rate if the market prices them at 95%. betbot systematically exploits the gap between model-estimated probability and market-implied probability.

The system is built on Go with PostgreSQL, leveraging Fiber v3 for HTTP services, River for job orchestration, and a clean domain-driven architecture. The modeling layer may incorporate Python microservices for ML workloads via gRPC, while core infrastructure remains in Go for performance and operational simplicity.

### 1.3 Current Repo Baseline

The repository already includes the project scaffold, a local Docker Compose stack, a standalone `betbot` app container, a `betbot-postgres` Postgres container, and a minimal health-checked HTTP bootstrap service. The target architecture in this document remains the end-state, but the current implementation baseline is intentionally smaller:

- Local health endpoint is available at `http://127.0.0.1:18080/health`
- App container listens on `8080` internally and is published on host port `18080`
- Current bootstrap server is minimal `net/http`; Fiber dashboard work remains planned
- PostgreSQL is available locally through Docker Compose before managed Postgres is introduced

### 1.1 Design Principles

- **Measurement before modeling.** Build data infrastructure and CLV tracking first. You cannot improve what you cannot measure.
- **Market as prior.** The closing line is the market's best probability estimate. Model the residuals — situations where the market is systematically wrong — rather than building from scratch.
- **Exactly-once execution.** Bet placement is a financial transaction. Idempotency, distributed locks, and audit logging are non-negotiable.
- **Portfolio-aware sizing.** Kelly Criterion applied with fractional scaling and correlation-adjusted exposure limits. Multiple bets on the same game are effectively one bet.
- **Backtesting primacy.** A replay engine against historical odds data must validate any model before live capital deployment.

### 1.2 Key Metrics

| Metric | Definition | Target |
|--------|-----------|--------|
| CLV (Closing Line Value) | Did you beat the closing line? The only honest performance metric. | > 0 consistently |
| Expected Value (EV) | EV = (P_win × Payout) − (P_lose × Stake) | > vig margin (~4–10%) |
| Calibration | Model says 58% → actual win rate ≈ 58% | Brier score < 0.20 |
| Sharpe Equivalent | Risk-adjusted return on bankroll over time | > 1.0 annualized |
| Account Longevity | Months before soft-limited on recreational books | > 6 months per book |

---

## 2. Foundational Mental Model

Sports betting markets are prediction markets with a rake. The sportsbook embeds a 4–10% margin in every line. Your edge must exceed that margin to be profitable. The math mirrors quantitative trading.

### 2.1 Expected Value Framework

The fundamental equation:

```
EV = (P_win × Payout) − (P_lose × Stake)
```

You are not trying to predict winners. You are trying to find mispriced probability. A team can be a bad bet even if they win 80% of the time, if the line prices them at 95%. Conversely, a 30% winner at +400 odds is a strong positive EV play.

### 2.2 Implied Probability Conversion

| Odds Type | Formula | Example |
|-----------|---------|---------|
| Positive (+110) | `p = 100 / (odds + 100)` | +110 → 100/210 = 47.6% |
| Negative (−150) | `p = \|odds\| / (\|odds\| + 100)` | −150 → 150/250 = 60.0% |
| Decimal (2.10) | `p = 1 / decimal_odds` | 2.10 → 1/2.10 = 47.6% |
| Overround (vig) | `Sum of implied probs − 100%` | 104.5% → 4.5% vig |

The vig means the sum of implied probabilities across all outcomes exceeds 100%. To derive no-vig (fair) probabilities, normalize by dividing each implied probability by the overround total.

### 2.3 The Closing Line as Ground Truth

The closing line — the final odds offered before game start — represents the market's best estimate of true probability, per the efficient market hypothesis applied to sports. Sharp bettors do not evaluate performance by win/loss record. They evaluate by **Closing Line Value (CLV)**: did you consistently beat the closing line? If your bets beat closing over a large sample, your model has edge regardless of short-term variance.

---

## 3. Core Betting Strategies

### 3.1 Value Betting (Sharp Betting)

The foundational strategy. Build your own probability model; if your estimated probability exceeds the implied probability embedded in the market, you have positive expected value.

- Your model estimates Team A wins at p = 0.58
- Market implies p = 0.50 at +100 odds
- Edge = 0.58 − 0.50 = 0.08 (8 percentage points)
- Scale the bet using Kelly Criterion

The quality of the probability model is the entire business. Everything else is infrastructure to exploit that model's output efficiently.

### 3.2 Arbitrage (Surebetting)

Exploit price discrepancies across multiple sportsbooks by placing opposing bets that guarantee profit regardless of outcome.

```
Book A: Team A at +110 (47.6%)  |  Book B: Team B at +115 (46.5%)
Combined implied = 94.1% → 5.9% arbitrage margin
```

Arbitrage is nearly risk-free but margins are thin (1–3%), books close accounts quickly, and it requires multi-book infrastructure with speed — lines move fast. The software pattern is a polling loop that detects spread divergence and triggers simultaneous placement.

### 3.3 Kelly Criterion (Optimal Bet Sizing)

Given your estimated win probability `p`, loss probability `q = 1 − p`, and net fractional odds `b` (decimal odds minus 1), the optimal fraction of bankroll to wager is:

```
f* = (b × p − q) / b
```

In practice, use **fractional Kelly** at 25–50% of the full Kelly output. Full Kelly is theoretically optimal for maximizing the log of wealth, but it produces catastrophic drawdowns during correlated bad runs. Fractional Kelly sacrifices marginal growth rate for dramatically lower variance.

### 3.4 Line Shopping / Best-of-Market

Before any model logic: always bet at the best available line. Half-point differences on NFL spreads have significant EV implications, especially when crossing key numbers (3, 7, 10). This is table-stakes infrastructure — every serious operator maintains real-time visibility across all available books.

### 3.5 Closing Line Value (CLV) Tracking

CLV is the primary performance attribution metric:

```
CLV = (Closing_implied − Bet_implied) / Bet_implied
```

Positive CLV over a sustained sample (500+ bets) is the strongest possible evidence of genuine edge, independent of win/loss noise.

### 3.6 Steam Following / Reverse Line Movement

- **Steam moves:** Sharp money hits a book, the line moves fast. Follow the move before other books update. Requires sub-second polling and execution infrastructure.
- **Reverse Line Movement (RLM):** Public bets 70% on Team A, but the line moves toward Team B. This signals sharp money on B. Fade the public.

These are derivative strategies — piggybacking on sharp action rather than modeling independently. They work but have decaying alpha as more participants adopt them.

### 3.7 Model-Based / ML Approaches

| Approach | Description | Tradeoffs |
|----------|-------------|-----------|
| Regression | Team quality metrics (DVOA, EPA/play, xG) → spread prediction | Interpretable; limited feature interaction |
| Elo / Glicko | Iterative rating with margin-adjusted updates | Simple; effective head-to-head; poor at context |
| Ensemble ML (XGBoost / LightGBM) | Feature matrix → probability → compare to market | Powerful; requires disciplined validation |
| Bayesian Updating | Market prior (Vegas line), update with injury/weather/lineup | Most defensible: models residuals |

**Recommended approach:** Use the market as your prior and model residuals — situations where the market is systematically wrong due to information asymmetry, slow reaction to news, or structural bias.

### 3.8 Proposition / Player Props

Often the least efficient market. Books price props with less rigor than game lines due to lower volume and limited sharp action. Projections-based models combined with cross-book line comparison can yield consistent edge, especially early in the week before sharp money lands.

---

## 4. Data Sources & Integration

### 4.1 Odds & Lines

| Source | Type | Notes |
|--------|------|-------|
| The Odds API | REST API | Best multi-book aggregator; free tier; spreads, ML, totals, props |
| OddsJam | SaaS + API | Includes sharp book data (Pinnacle, Circa, BetOnline) |
| Pinnacle | Direct | Gold standard sharp book; lines are consensus truth |
| SBR Odds | Historical DB | Historical odds going back years; essential for backtesting |
| DK / FD / BetMGM | Scraping / 3rd party | No public APIs; requires aggregators |

### 4.2 Sports Statistics

| Source | Sport | Notes |
|--------|-------|-------|
| nflverse / nflfastR | NFL | Play-by-play EPA, CPOE, DVOA proxies; free on GitHub |
| Stathead / PFR | NFL/NBA/MLB | Historical stats; scraping-friendly |
| Sportradar | All major | Enterprise-grade; backbone of most books' data |
| Stats Perform (Opta) | Soccer / global | Best soccer xG data |
| Baseball Savant | MLB | Statcast data (exit velocity, spin rate); free API |
| NBA Stats API | NBA | Unofficial but stable; tracking, shot quality, lineups |

### 4.3 Sharp & Market Intelligence

| Source | Notes |
|--------|-------|
| Pinnacle closing lines | Archive yourself; most valuable free signal |
| Action Network | Public betting %, sharp money indicators; has API |
| Pregame.com | Consensus % and line history |
| Don Best | Pro-tier real-time line movement; expensive |

### 4.4 Injury / News / Situational

| Source | Notes |
|--------|-------|
| Rotowire API | Injury reports, lineup confirmations |
| X / Twitter (beat reporters) | Real-time; NLP extraction required |
| Weather APIs (OpenWeatherMap) | Critical for outdoor sports — NFL totals especially |

---

## 5. Technical Architecture

### 5.1 System Overview

```
┌─────────────────────────────────────────────────────────┐
│                  DATA INGESTION LAYER                    │
│  Odds API poller │ Stats ETL │ Injury scraper │ CLV log  │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                  POSTGRES DATA STORE                     │
│  games │ odds_history │ model_predictions │ bets │ CLV   │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                    MODELING LAYER                        │
│  Feature engineering │ Probability model │ EV calc       │
│  (Go/gonum for Elo/regression | Python sidecar for ML)  │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                   DECISION ENGINE                        │
│  EV threshold │ Kelly sizer │ Correlation guard          │
│  Bankroll manager │ Risk limits │ Circuit breakers        │
└──────────────────────────┬──────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────┐
│                   EXECUTION LAYER                        │
│  Book adapters (DK, FD, BetMGM, Pinnacle)               │
│  Idempotency │ Placement confirmation │ Audit log        │
└─────────────────────────────────────────────────────────┘
```

| Layer | Responsibility | Key Components |
|-------|---------------|----------------|
| Data Ingestion | Poll, fetch, normalize, store raw data | Odds poller, Stats ETL, Injury scraper, CLV logger |
| Data Store | Persist all state; source of truth | PostgreSQL 17: games, odds_history, predictions, bets, clv |
| Modeling | Feature engineering, probability estimation, EV | Go (gonum) for Elo/regression; Python sidecar for ML via gRPC |
| Decision Engine | Filter, size, deduplicate, risk-check | EV threshold, Kelly sizer, bankroll mgr, correlation guard |
| Execution | Place bets, confirm, retry, audit | Book adapters, idempotency keys, audit log |

### 5.2 Data Ingestion Layer

**Odds Polling Worker:** A Go worker process using River as the job queue. Polls The Odds API at configurable intervals (60s live, 300s pre-game). Stores both raw JSON and normalized rows for full reconstructability.

- Rate limiting: token bucket respecting API quotas
- Deduplication: hash-based detection of duplicate snapshots
- Schema: raw JSON alongside parsed columns for future reprocessing

**Stats ETL Pipeline:** Scheduled River jobs pulling sport stats from upstream sources and loading to normalized Postgres tables. Each sport gets its own ETL worker. Runs daily; triggered on-demand for in-season updates.

**Injury & Situational Scraper:** Real-time monitoring of Rotowire API and structured feeds for injury updates, lineup confirmations, and weather. Updates `situational_factors` table keyed by `game_id`.

### 5.3 PostgreSQL Data Store

#### Core Schema

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `games` | Master game registry | id, sport, home_team, away_team, start_time, status, result |
| `odds_history` | Full odds timeline per game/book | game_id, book, market_type, odds, implied_prob, captured_at, raw_json |
| `model_predictions` | Model outputs per game | game_id, model_version, predicted_prob, features_json, created_at |
| `bets` | Bet ledger (state machine) | id, game_id, book, market, odds, stake, status, placed_at, settled_at |
| `clv_log` | CLV attribution per bet | bet_id, bet_implied, closing_implied, clv_pct |
| `bankroll_ledger` | Explicit capital tracking | id, event_type, amount, balance_after, bet_id, timestamp |

#### Schema Design Principles

- Store raw JSON alongside normalized rows — you will need to reconstruct full odds history for backtesting.
- Bankroll as an **explicit ledger**, not an inferred balance. Track pending/placed/settled with explicit state transitions.
- `odds_history` is **append-only**. Never update; always insert. Complete time series for replay.
- PostgreSQL partitioning on `odds_history` by month to manage table growth.

### 5.4 Modeling Layer

**Go-Native Models (gonum):** Elo ratings, logistic regression, basic feature-weighted scoring. Keeps operational footprint minimal — no cross-language overhead.

**Python ML Sidecar:** Ensemble models (XGBoost, LightGBM), Bayesian updating, and calibration via gRPC. Platt scaling or isotonic regression ensures calibrated outputs.

**Feature Matrix:** Team quality (EPA, DVOA, net rating, Elo delta), situational factors (rest days, travel, altitude, surface), injury impact (weighted by WAR/usage rate), market signals (line movement, public betting %), weather, historical matchup data.

### 5.5 Decision Engine

- **EV Threshold Filter:** Only pass bets where estimated EV exceeds minimum (default: 2% edge).
- **Kelly Sizer:** Fractional Kelly (default 25%) with hard cap at 3% of bankroll per bet.
- **Correlation Guard:** Detect overlapping `game_id` exposure; reduce effective Kelly when correlated bets are pending.
- **Bankroll Manager:** State machine — Available → Pending → Placed → Settled (Win/Loss/Push). Daily/weekly loss stops and drawdown circuit breakers.

### 5.6 Execution Layer

- **Book Adapters:** Common interface (`PlaceBet`, `CheckStatus`, `GetBalance`) per sportsbook. Initial targets: DK, FD, BetMGM, Pinnacle.
- **Idempotency:** Generate idempotency key per game/market/book/timestamp. Distributed locks (Postgres advisory or Redis). Read-back verification before marking placed.
- **Clock Sync:** NTP on all workers. Database-side `NOW()` as canonical clock, not app-layer time.
- **Audit Log:** Every placement attempt (success or failure) with timestamps and response payloads.

---

## 6. Go Implementation Details

### 6.1 Project Structure

```
tradebot/
  cmd/
    server/       → main HTTP service (Fiber v3)
    worker/       → River worker process
    backtest/     → CLI replay engine
  internal/
    domain/       → core types: Game, Odds, Bet, Bankroll
    ingestion/    → odds poller, stats ETL, injury scraper
    modeling/     → EV calc, Elo, feature engineering
    decision/     → Kelly sizer, correlation guard, risk mgr
    execution/    → book adapters, placement, audit
    store/        → sqlc-generated Postgres queries
  proto/          → gRPC definitions for Python sidecar
  migrations/     → SQL migration files
  config/         → environment-specific config
```

### 6.2 Key Dependencies

| Package | Purpose | Notes |
|---------|---------|-------|
| `gofiber/fiber/v3` | HTTP framework | Dashboard UI + API |
| `riverqueue/river` | Job queue | Odds polling, settlement, CLV capture |
| `sqlc-dev/sqlc` | Type-safe SQL | Generate Go from SQL; zero ORM overhead |
| `gonum/gonum` | Numerical computing | Elo, regression, probability in pure Go |
| `jackc/pgx/v5` | PostgreSQL driver | Connection pooling, COPY for bulk inserts |
| `grpc/grpc-go` | RPC to Python sidecar | ML inference with protobuf schemas |
| `rs/zerolog` | Structured logging | JSON logs with microsecond timestamps |

### 6.3 River Job Definitions

| Job Type | Schedule | Description |
|----------|----------|-------------|
| `OddsPollJob` | 60s (live) / 300s (pre-game) | Fetch odds, store in `odds_history` |
| `StatsETLJob` | Daily 04:00 UTC | Pull sport stats, load to Postgres |
| `InjuryScanJob` | Every 15 min | Check Rotowire + feeds for updates |
| `ModelRunJob` | On odds change / hourly | Run probability model, emit predictions |
| `EVScreenJob` | After ModelRunJob | Screen predictions for +EV bets |
| `PlacementJob` | On +EV detection | Execute via book adapter |
| `SettlementJob` | Post-game | Reconcile outcomes, update bankroll |
| `CLVCaptureJob` | At game start | Archive closing odds, compute CLV |

### 6.4 Configuration

All tunable parameters are config-driven:

- Kelly fraction (default: 0.25)
- EV threshold minimum (default: 0.02)
- Max single-bet exposure (default: 0.03 of bankroll)
- Daily loss stop (default: 0.05 of bankroll)
- Weekly loss stop (default: 0.10 of bankroll)
- Drawdown circuit breaker (configurable % from peak)
- Polling intervals per sport and game state
- Book-specific: max stake, supported markets, auth credentials

---

## 7. Risk Management

Risk management is the part most people skip. It is the difference between a system that compounds wealth and one that blows up.

### 7.1 Correlation Risk

Parlays and same-game props create correlated exposure. Kelly must account for portfolio correlation — multiple bets on the same game are effectively one bet. The correlation guard detects overlapping `game_id` exposure and reduces effective Kelly accordingly.

### 7.2 Model Calibration

Never bet on a single model output. Bootstrap predictions or use Platt scaling for calibrated probabilities. A model that says 58% should actually win 58% of the time — verify on holdout data. Track a calibration curve (predicted vs. actual by decile) as a persistent metric.

### 7.3 Account Longevity

Sharp books (Pinnacle, Bookmaker) tolerate winners. Recreational books (DK, FD) will limit profitable accounts. Strategies:

- Vary bet sizing — avoid round numbers that signal algorithmic behavior
- Occasional recreational bets at minimal stake
- Time placement to avoid being first on a market
- Rotate across accounts and books
- Primary volume on sharp-tolerant books; recreational for best-line cherry-picking

### 7.4 Variance Budgeting

Even a +3% EV strategy will have losing months. Budget for 50–100 bets before drawing conclusions. Bankroll sized to survive a 3–5σ downswing without forced delevering.

### 7.5 Hard Limits & Circuit Breakers

| Control | Threshold | Action |
|---------|-----------|--------|
| Max single bet | 3% of bankroll | Hard cap regardless of Kelly |
| Daily loss stop | 5% of bankroll | Halt new bets for day |
| Weekly loss stop | 10% of bankroll | Halt until manual review |
| Drawdown breaker | 15% from peak | Full system halt; manual restart |
| Correlated exposure | 5% aggregate on same game | Reject additional bets |

---

## 8. Backtesting Engine

The backtesting harness is arguably more important than live execution. No model should touch real capital until replayed against historical data.

### 8.1 Replay Architecture

Standalone CLI (`cmd/backtest`) walks through historical `odds_history` and simulates: model prediction → EV screening → Kelly sizing → virtual placement → settlement. Outputs:

- Cumulative PnL curve
- CLV distribution histogram
- Calibration plot (predicted vs. actual by decile)
- Sharpe-equivalent ratio
- Maximum drawdown and duration
- Win rate, average odds, average edge per bet

### 8.2 Avoiding Backtest Pitfalls

- **Look-ahead bias:** Only use data available at the hypothetical bet time. Never include closing line or result in features.
- **Survivorship bias:** Include postponed, cancelled, and missing-data games.
- **Overfitting:** Walk-forward cross-validation. Train on data up to week N, predict N+1, roll forward.
- **Execution realism:** Model actual available odds at the time, not best-of-market. Account for line movement.
- **Cost modeling:** Include the vig in all simulated bets.

---

## 9. Monitoring & Dashboards

### 9.1 Operational Dashboard (HTMX/Alpine.js)

Web dashboard served by Fiber, built with HTMX and Alpine.js:

- **Live odds board:** Best-of-market odds across all tracked games/books
- **Pending bets:** All open bets with real-time PnL estimation
- **Bankroll chart:** Cumulative PnL with drawdown overlay
- **Model diagnostics:** Latest run output, feature weights, calibration curve
- **CLV tracker:** Rolling average with confidence interval

### 9.2 Alerting

River-driven alerting jobs:

- Circuit breaker triggered
- Bet placement failure
- Data pipeline failure (odds API down, ETL error)
- Arbitrage opportunity above threshold
- Account health warning

---

## 10. Legal & Compliance

### 10.1 Jurisdictional Considerations

Sports betting regulation is jurisdiction-dependent and changes frequently. Automated or API-based betting may be restricted even where manual betting is legal. Verify current state law, operator terms, and licensing requirements before any live deployment.

### 10.2 Sportsbook Terms of Service

Most licensed US books prohibit automated tools in ToS. Violation risks account closure. Offshore books (Pinnacle, Bookmaker.eu) are more tolerant of winners and automated systems.

### 10.3 Tax Treatment

Profits are taxable as ordinary income in the US. The `bankroll_ledger` table doubles as tax documentation. Consult a tax professional.

---

## 11. Implementation Roadmap

Follows the principle: **measurement before modeling**.

### Phase 1: Data Foundation (Weeks 1–4)

1. PostgreSQL schema: `games`, `odds_history`, `bankroll_ledger`
2. Odds API polling worker with River
3. Raw JSON + normalized storage pipeline
4. CLV capture job (closing odds archival)
5. Basic HTMX dashboard: odds board + pipeline health

### Phase 2: Modeling & Backtesting (Weeks 5–8)

1. Elo-based probability model in Go (gonum)
2. Feature engineering pipeline
3. Backtesting CLI with walk-forward validation
4. Calibration verification
5. EV screening against historical data

### Phase 3: Decision Engine (Weeks 9–11)

1. Kelly sizer with fractional scaling
2. Correlation guard
3. Bankroll state machine
4. Hard limits and circuit breakers
5. Dashboard: bankroll chart, pending bets, CLV tracker

### Phase 4: Execution (Weeks 12–14)

1. Book adapter interface + first adapter
2. Idempotency keys, distributed locks, confirmation
3. Audit log
4. Paper trading mode

### Phase 5: Live Deployment (Weeks 15–16)

1. Deploy with minimal capital
2. Monitor CLV, calibration, PnL daily
3. Gradual bankroll increase after 100+ bet validation
4. Additional book adapters

### Phase 6: Advanced Models (Ongoing)

1. Python ML sidecar (XGBoost/LightGBM via gRPC)
2. Bayesian updating with market prior
3. Player prop models
4. Steam detection and RLM signals
5. Multi-sport expansion

---

## 12. Appendix

### A. Key Formulas

| Formula | Expression | Notes |
|---------|-----------|-------|
| Expected Value | `EV = (P_win × Payout) − (P_lose × Stake)` | Core profitability measure |
| Kelly Criterion | `f* = (b×p − q) / b` | p = win prob, q = 1−p, b = decimal odds − 1 |
| Implied Prob (+odds) | `p = 100 / (odds + 100)` | American positive odds |
| Implied Prob (−odds) | `p = \|odds\| / (\|odds\| + 100)` | American negative odds |
| CLV | `(closing_implied − bet_implied) / bet_implied` | Performance attribution |
| No-Vig Probability | `p_i / Σ(all p_i)` | Remove vig by normalizing |
| Fractional Kelly | `f_actual = k × f*` where k ∈ [0.25, 0.50] | Reduce variance |
| Sharpe (betting) | `mean(returns) / std(returns)` | Risk-adjusted performance |

### B. Glossary

| Term | Definition |
|------|-----------|
| CLV | Closing Line Value — whether your bet beats the final market price |
| Vig / Juice / Rake | The sportsbook's built-in margin |
| Sharp | A sophisticated, winning bettor whose action moves lines |
| Square / Recreational | A casual bettor; books profit from this segment |
| Steam Move | Rapid line movement caused by sharp money |
| RLM | Reverse Line Movement — line moves opposite to public betting % |
| Handle | Total dollar amount wagered on a market |
| Closing Line | The final odds offered before game start |
| Overround | Sum of implied probabilities minus 100% |
| EV | Expected Value — average profit/loss per unit wagered |
