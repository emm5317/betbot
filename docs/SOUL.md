# Soul.md — betbot

The house always wins unless you treat this like a trading system.

---

## What betbot Is

betbot is a four-sport quantitative betting system focused on `MLB`, `NBA`, `NHL`, and `NFL`. It exists to identify mispriced probability, measure whether that edge is real, and only then automate sizing and execution.

It is not trying to be a generic sports platform. The product is intentionally specialized where the market depth, public data quality, and year-round operating cadence justify the effort.

---

## Core Thesis

1. **The market is the prior.** The current line already reflects most public information. Start there.
2. **Edge is residual mispricing.** Update the market with sport-specific information the books or the public underweight.
3. **CLV is the honest metric.** If the system is not beating the close, it does not have proven edge.
4. **Data quality is model quality.** Broken ingestion invalidates every downstream conclusion.
5. **Variance is not a bug.** The job is to survive it, not pretend it disappears.

---

## Personality

- Clinical, not emotional
- Probability-first, not outcome-first
- Patient, not action-addicted
- Skeptical of its own models
- More interested in edge persistence than bet count

The operator mindset is closer to a small trading desk than a sportsbook content brand.

---

## Product Boundaries

betbot is not:

- a picks app
- a parlay toy
- a social betting product
- a generic sports-data platform
- a "hot hand" recommendation engine

betbot is:

- a measurement system for odds and closing-line behavior
- a modeling and validation pipeline
- a bankroll and risk engine
- an execution system only after the data and model layers are credible

---

## Four-Sport Posture

Why these leagues:

- `MLB`: unmatched public analytics depth and starter-driven markets
- `NBA`: player availability and schedule stress are large, measurable inputs
- `NHL`: goalie timing and xG divergence create distinct inefficiency windows
- `NFL`: the most efficient major market, but still exploitable through situational rigor

This focus should show up in the product language. betbot is not "sport agnostic." It is reusable in architecture and specialized in strategy.

---

## Design Language

- Data density over decoration
- Monospace for numbers and odds
- Color used semantically, not decoratively
- Interfaces that feel like tooling, not entertainment
- Dark mode default remains appropriate for the product posture

### Typography

| Use | Font | Fallback |
|-----|------|----------|
| Headings | Inter | system-ui |
| Body | Inter | system-ui |
| Data / Numbers / Code | JetBrains Mono | monospace |

### Color Palette

| Token | Hex | Use |
|-------|-----|-----|
| `--tb-bg` | `#0F1419` | Background |
| `--tb-surface` | `#1A2332` | Cards, panels |
| `--tb-border` | `#2A3A4A` | Dividers, borders |
| `--tb-text` | `#E1E8ED` | Primary text |
| `--tb-muted` | `#8899A6` | Secondary text |
| `--tb-positive` | `#17BF63` | Positive EV / CLV |
| `--tb-negative` | `#E0245E` | Negative EV / CLV |
| `--tb-caution` | `#FFAD1F` | Warning states |
| `--tb-accent` | `#1DA1F2` | Interactive elements |

---

## Operator Mindset

The operator:

1. Maintains the data pipeline before touching model complexity.
2. Treats calibration drift as a production incident.
3. Respects circuit breakers and audit trails.
4. Thinks in samples, not anecdotes.
5. Scales only when CLV and process quality justify it.

The correct emotional tone of the system is restraint.

---

## Decision Hierarchy

1. Financial safety
2. Data integrity
3. Measurement quality
4. Operational simplicity
5. Model sophistication

If a new idea conflicts with this order, the idea loses.

---

## Roadmap Mood

**Phase 1:** ingestion vertical slice, operational visibility, no false precision

**Phase 2:** sport foundation, schedules, ETL, and situational inputs

**Phase 3:** baseline models and backtesting

**Phase 4:** decision engine and bankroll control

**Phase 5:** execution and paper validation

**Phase 6:** constrained live deployment and iterative expansion

The north star is not "maximum automation." It is durable edge with controlled risk.
