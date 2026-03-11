# Four-Sport Specialization Deep Dive

This document is the supporting strategy reference for betbot's four-sport focus: `MLB`, `NBA`, `NHL`, and `NFL`.

It is not the canonical phase-order document. Use [betbot-plan.md](betbot-plan.md) for roadmap order and [TRACKER.md](TRACKER.md) for implementation sequencing.

---

## 1. Purpose

The value of specializing is not just strategic branding. It changes:

- which data sources are worth integrating
- which features belong in the model
- which markets deserve primary attention
- which scheduling assumptions are valid
- how Kelly and calibration should be interpreted

betbot should therefore be described as a four-sport system with reusable infrastructure, not a sport-agnostic engine with incidental MLB/NBA/NHL/NFL support.

---

## 2. Shared Product Logic

Across all four sports:

- the market price is the starting prior
- CLV is the primary post-bet validation signal
- append-only odds history is the replay and audit backbone
- sport-specific data and model layers are justified only after ingestion is stable

Cross-sport signals worth standardizing:

- regression-to-the-mean detection
- rest and travel effects
- injury or lineup availability deltas
- market movement and best-line capture

---

## 3. Sport Registry Expectations

The `SportConfig` registry should capture, at minimum:

- season boundaries
- games-per-season expectations
- home-advantage baseline
- key numbers or market anchors
- live and pregame polling cadence
- model family defaults
- Kelly range guidance

This lets scheduling, modeling, and UI behavior stay explicit instead of hiding sport rules in scattered package code.

---

## 4. MLB

### Why MLB matters

- largest game sample of the four sports
- deepest free public analytics stack
- starting pitcher quality is unusually predictive
- F5 markets isolate starting pitching from bullpen noise

### Highest-value data

- Baseball Savant / Statcast
- FanGraphs
- Rotowire lineup and starter confirmations
- weather and park-factor sources

### Highest-value features

- starting pitcher quality
- lineup quality by handedness
- bullpen fatigue
- park factors
- wind and temperature

### Baseline model direction

- team run expectation model
- moneyline and total derived from projected runs
- F5 variant as a lower-noise companion market

---

## 5. NBA

### Why NBA matters

- player availability swings spreads materially
- pace and schedule density affect totals and variance
- rest and travel effects are measurable and frequent

### Highest-value data

- NBA Stats API
- Dunks & Threes / EPM
- DARKO / DPM
- PBPStats
- Rotowire injury and lineup data

### Highest-value features

- lineup-adjusted net rating
- rest differential
- travel distance
- schedule density
- projected pace

### Baseline model direction

- lineup-adjusted spread model
- pace-driven total model
- later player prop models once minutes and usage handling are solid

---

## 6. NHL

### Why NHL matters

- goalie confirmations create discrete timing windows
- xG and actual results diverge meaningfully over short horizons
- PDO regression is a persistent explanatory tool

### Highest-value data

- NHL API
- MoneyPuck
- Natural Stat Trick
- Daily Faceoff

### Highest-value features

- team xG share
- goalie quality
- recent workload
- travel and rest
- PDO regression pressure

### Baseline model direction

- xG plus goalie-adjusted moneyline model
- total model informed by goalie and shot-quality context

---

## 7. NFL

### Why NFL matters

- highest market liquidity and sharpest public attention
- key-number discipline matters more than in the other sports
- weather and rest effects are material
- quarterback changes can move markets dramatically

### Highest-value data

- nflverse / nflfastR
- Pro Football Reference
- Rotowire
- weather APIs

### Highest-value features

- EPA/play and related efficiency metrics
- pressure and pass efficiency
- weather impact
- short-week and bye effects
- key-number proximity

### Baseline model direction

- spread/total model using efficiency plus situational adjustments
- explicit key-number awareness in downstream pricing and validation

---

## 8. Cross-Sport Priorities

### Shared data sources

| Concern | Primary source |
|--------|-----------------|
| odds aggregation | The Odds API first |
| sharper line reference | Pinnacle and later premium aggregators |
| injuries and confirmations | Rotowire, plus Daily Faceoff for NHL |
| weather | OpenWeatherMap or equivalent |

### Shared analytical patterns

| Pattern | MLB | NBA | NHL | NFL |
|--------|-----|-----|-----|-----|
| regression metrics | BABIP | team 3PT% | PDO | fumble recovery / turnover luck |
| rest/travel effects | moderate | high | moderate | situational but high leverage |
| lineup/availability impact | starter and lineup cards | highest of all four | goalie and lines | QB and skill-player health |
| CLV focus | moneyline and F5 | spread and total | moneyline | spread near key numbers |

### Kelly posture

These should be defaults, not absolutes:

- `MLB`: relatively higher fractional Kelly range
- `NBA`: moderate
- `NHL`: conservative
- `NFL`: most conservative

The system should treat these as sport policy, not one global constant forever.

---

## 9. Schema Additions After the Ingestion Slice

Phase 2 now starts with a deliberately small stat-table set:

- MLB team and pitcher tables
- NBA team table
- NHL team and goalie tables
- NFL team and quarterback tables

This keeps the first ETL pass anchored on the entities that matter most for each sport without prematurely locking the repo into exhaustive player modeling.

The sequencing rule remains:

- Phase 1 must not be blocked on these tables
- Phase 2 should add the smallest schema that supports ETL and later model features
- Later phases can add richer batter, skater, lineup, park, and play-level tables once real ingestion needs justify them

---

## 10. Roadmap Impact

This deep dive changes roadmap priorities in two ways:

1. It narrows the near-term product surface to four sports instead of leaving expansion open-ended.
2. It pushes sport-specific ETL and modeling breadth behind the Phase 1 ingestion slice rather than in front of it.

That means:

- Phase 1 remains an odds-ingestion vertical slice
- Phase 2 becomes the sport-foundation phase
- Phase 3 becomes the first serious modeling and backtesting phase

This sequencing is intentional. A broad sports architecture without a trustworthy odds archive is the wrong order of operations.

