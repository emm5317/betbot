# Soul.md — betbot

> The house always wins — unless you're the house.

---

## What betbot Is

betbot is a quantitative sports betting trading system. It exists to find and exploit mispriced probability across sportsbook markets, using the same discipline applied in quantitative finance: statistical modeling, portfolio-aware position sizing, systematic execution, and honest performance attribution.

It is not a gambling tool. It is not a picks service. It is not a parlay optimizer. It is a **trading system** that happens to operate in sports betting markets.

---

## Core Thesis

Sports betting markets are prediction markets with a rake. Sportsbooks embed a 4–10% margin (vig) in every line. Most bettors lose because they are paying that margin without possessing offsetting edge.

betbot's thesis:

1. **The closing line is truth.** The final odds before game start represent the market's best estimate of true probability. This is the benchmark — not game outcomes.
2. **Edge is mispriced probability.** A team that wins 80% of the time is a bad bet if the market prices them at 95%. A team that wins 30% is a great bet at +400. betbot finds the gap.
3. **CLV is the only honest metric.** Closing Line Value — whether you beat the final market price — is the only performance measure that separates skill from noise over meaningful sample sizes.
4. **Variance is the cost of doing business.** Even a +3% EV strategy has losing months. The system is designed to survive variance, not avoid it.

---

## Identity & Voice

### Name: betbot

The repository name is `betbot`, but the operating posture is still trading-first, not gambling-first. The mental model is a trading desk, not a ticket window.

### Personality

- **Clinical, not emotional.** Decisions are made by math. The system does not have "gut feelings" or "locks of the week."
- **Honest about uncertainty.** Probabilities are probabilities, not certainties. The system expresses confidence in ranges, not absolutes.
- **Patient.** The system waits for edge. No edge, no bet. An idle day is a successful day.
- **Self-critical.** Calibration monitoring, backtest regression, and CLV tracking exist because the system assumes its own models are wrong until proven otherwise.

### Design Language

betbot's interfaces (dashboard, reports, CLI output) follow these principles:

- **Data density over decoration.** Every pixel earns its place. No decorative charts. No gamification.
- **Monospace for numbers.** Financial data and odds are displayed in monospace. Alignment matters.
- **Color is semantic.** Green = positive EV / positive CLV. Red = negative. Amber = caution / near threshold. No arbitrary palette — color encodes meaning.
- **Dark mode default.** Trading terminals are dark. So is this.

### Typography

| Use | Font | Fallback |
|-----|------|----------|
| Headings | Inter | system-ui |
| Body | Inter | system-ui |
| Data / Numbers / Code | JetBrains Mono | monospace |

### Color Palette

| Token | Hex | Use |
|-------|-----|-----|
| `--tb-bg` | `#0F1419` | Background (dark) |
| `--tb-surface` | `#1A2332` | Cards, panels |
| `--tb-border` | `#2A3A4A` | Dividers, borders |
| `--tb-text` | `#E1E8ED` | Primary text |
| `--tb-muted` | `#8899A6` | Secondary text, labels |
| `--tb-positive` | `#17BF63` | Positive EV, wins, +CLV |
| `--tb-negative` | `#E0245E` | Negative EV, losses, −CLV |
| `--tb-caution` | `#FFAD1F` | Warnings, near-threshold |
| `--tb-accent` | `#1DA1F2` | Interactive elements, links |
| `--tb-accent-dim` | `#1A8CD8` | Hover states |

---

## What betbot Is NOT

- **Not a gambling app.** There are no "hot picks," no streaks, no leaderboards. If it feels like a casino, something is wrong.
- **Not a tips service.** betbot does not produce human-readable recommendations. It produces bet tickets with quantified edge, confidence intervals, and position sizes.
- **Not a parlay builder.** Parlays create correlated exposure that the Kelly framework penalizes. The system may place them only when the correlation is explicitly modeled and the EV justifies the structure.
- **Not infallible.** The system will have losing streaks. The architecture exists to survive them — circuit breakers, loss stops, drawdown halts. Losing is expected. Blowing up is not.

---

## Operator Mindset

The person running betbot is an **operator**, not a gambler. The operator's job:

1. **Maintain the data pipeline.** Data quality is model quality. A missed odds snapshot is a missed opportunity — or worse, a stale-data bet.
2. **Monitor calibration.** If the model says 60% and the actual rate drifts to 52%, the model is broken. Disable it. Investigate. Fix. Redeploy after backtesting.
3. **Respect the circuit breakers.** When the system halts, it halted for a reason. Do not override without understanding why.
4. **Think in samples, not bets.** A single bet outcome carries near-zero information. 100 bets start to tell a story. 500 bets reveal whether you have edge. Act accordingly.
5. **Protect account longevity.** The best model in the world is worthless if every book has limited your account. Manage volume, timing, and bet patterns to extend useful life.

---

## Decision Framework

When making product or engineering decisions for betbot, apply this hierarchy:

1. **Financial safety first.** If a feature could cause a double-placed bet, a bankroll error, or a missing audit record, it does not ship until the safety case is airtight.
2. **Data integrity second.** If a schema change could corrupt the odds history or break backtest replay, it requires explicit migration and validation.
3. **Measurement third.** If you can't measure whether a change improves CLV or calibration, the change is speculative. Ship the measurement first.
4. **Model sophistication last.** A simple Elo model with excellent data infrastructure beats an XGBoost ensemble with broken data every time.

---

## Relationship to Clientsite

betbot is a **separate product** from Clientsite. It shares technical DNA — Go, PostgreSQL, River, Fiber, HTMX — but has its own repository, its own deployment, and its own domain logic. The shared stack is a pragmatic choice that reduces cognitive overhead, not a coupling decision.

betbot does not share a database, codebase, or deployment pipeline with Clientsite. They are sibling projects with a common operator and a common technical philosophy.

---

## Long-Term Vision

**Phase 1 (current):** Single-sport value betting with CLV-validated edge. Prove the thesis works with real capital over a statistically significant sample.

**Phase 2:** Multi-sport expansion. Each sport gets its own modeling pipeline but shares common infrastructure (odds ingestion, decision engine, execution layer, bankroll management).

**Phase 3:** Market-making behavior — identifying structural inefficiencies across books and systematically capturing the spread between them.

**Phase 4:** If the thesis is proven and capital efficiency justifies it, evaluate whether betbot's edge framework generalizes to other prediction markets (politics, crypto, weather derivatives).

The North Star is **compounding edge over time** — not maximum excitement, not maximum volume, not maximum complexity. Simplicity that works beats complexity that might work.
