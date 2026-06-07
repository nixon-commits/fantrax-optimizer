# Windowed Recency + Strategy-Replay Harness — Design

**Date:** 2026-06-06
**Status:** Approved (brainstorm), implementation in progress
**Branch:** `feature/windowed-recency-strategy-replay`

## Problem

The blended projection's "recent" signal is **season-to-date (YTD)**, not a rolling
window — despite the `RecentStat` naming and the CLAUDE.md claim of "last 10 scoring
periods." `GetRecentStats` reads the latest completed period's roster snapshot, and the
Fantrax API returns cumulative YTD `FantasyPointsPerGame` regardless of the period
requested. The second `GetRecentStats(currentPeriod, _ int)` parameter is an ignored
fossil of a removed lookback window.

Consequence (observed in the hitter pipeline): a player who is genuinely hot over the
last two weeks but started the season cold shows a **negative** blend adjustment, because
the hot stretch is averaged against the cold start. The model therefore **cannot surface
recently-hot players** — exactly the capability the user wants for start/sit decisions.

## Goal

Determine, with evidence, whether a **rolling-window recency** signal improves actual
lineup output — specifically whether it correctly surfaces recently-hot hitters such that
starting them reduces points left on the bench.

**Decision metric:** lineup **Gap** (realized points vs hindsight-optimal), hitter slots
only. Projection **MAE / Bias** are supporting diagnostics, not the deciding number.

## Scope (v1)

- **Hitters only.** Pitchers add role-aware blend complexity; port only if a window wins.
- **Backtest-only.** Production `optimize` keeps running YTD untouched. No production flag.
- **Reuse** `DailyFantasyPoints` (the already-corrected per-day FP series) and the existing
  `hitterBlendWeights` stabilization curve and constants.

Non-goals (explicitly deferred): production mode selector, pitcher support, blend-constant
tuning, config-driven scenarios / golden fixtures / pluggable metrics.

## Design

### 1. One engine, four weightings

Every recency mode is a **weighting over the same per-day FP series**. `DailyFantasyPoints`
already produces a correct per-day, per-player FP series and already handles the hard cases
a naive snapshot subtraction would reintroduce (waiver-pickup phantom points zeroed,
two-way crossings backfilled). All modes consume *that* series.

| Mode    | Weighting over per-day FP (through d−1)        |
|---------|------------------------------------------------|
| `ytd`   | equal weight, season start → d−1 (control)     |
| `w14`   | equal weight, last 14 days                     |
| `w30`   | equal weight, last 30 days                     |
| `decay` | `exp(−ln2 · age/halflife)`, ~21d half-life, ~45d horizon |

**Pure core (no I/O, unit-tested):**

```go
// DayFP is one player-day of fantasy production.
type DayFP struct {
    Date   time.Time
    FP     float64
    Played bool
}

// WeightedRecent collapses a per-day series into a RecentStat using a weight
// function over each day's age (in days from the as-of date).
//   FPtsPerGame = Σ(w·dayFP) / Σ(w·dayGP)
//   GamesPlayed = count of games in the window (drives the stabilization weight)
func WeightedRecent(series []DayFP, weight func(ageDays int) float64) fantrax.RecentStat
```

YTD/14d/30d/decay are four small `weight` functions. Window membership is encoded by the
weight returning 0 outside the window (e.g. `w30` returns 1 for ageDays ≤ 30, else 0).
Early-season clamp: when fewer than N days exist, the window is simply whatever is
available (no special case needed — the series is short).

### 2. The blend stays identical — only the source changes

All modes feed the **existing** `BlendedSource` with the **existing** `hitterBlendWeights`
curve, driven by the window's `GamesPlayed`. This is the experimental control: a Gap change
is attributable to the recency window, not to altered blend mechanics. A short window
naturally gets down-weighted (small sample → curve trusts the projection); that is correct
behavior, not a bug, and is consistent across all modes.

### 3. Reusable strategy-replay harness (`internal/backtest`)

A single reusable seam — the recency modes are its first consumers.

```go
type StrategyVariant struct {
    Name  string
    Build func(asOf time.Time) (projections.Source, error) // hitter source as-of d−1
}

type VariantResult struct {
    Name        string
    RealizedPts float64 // total actual FPts of the lineups this variant set
    MeanGap     float64 // mean daily (realized − hindsight-optimal), hitter slots
    MAE         float64 // per-player projection error vs actual
    Bias        float64 // signed mean error
}

func RunStrategyComparison(variants []StrategyVariant, dates []time.Time) ([]VariantResult, error)
```

**Replay loop** — for each day *d*, for each variant:
1. Build the variant's hitter `Source` **as-of d−1** (leakage guard).
2. Run `OptimizeLineup` (hitters) → the lineup that variant would have set.
3. Score that lineup with **actual** FPts for day *d*; accumulate realized points.

Hindsight-optimal is computed once per day (existing `RunLineupAnalysis` machinery) for the
Gap. Output: a stdout table `mode | realized | Gap | MAE | Bias`.

**Recency modes** are constructed as four `StrategyVariant`s, each wrapping `BlendedSource`
over a recency map produced by `WeightedRecent`. The recency map for day *d* is built from a
`DailyFantasyPoints` series **truncated to days ≤ d−1** — the leakage guard lives at this
harness boundary.

### 4. CLI

A backtest sub-analysis, e.g. `backtest --recency-experiment --dates <range>` (hitters
only). Stdout table only; no GHA wiring (offline experiment). Add a `run-all` Makefile line
per the project convention for new top-level command surfaces, if a new subcommand rather
than a flag is chosen.

## Leakage guard (critical)

The series handed to `WeightedRecent` for day *d* must contain **only** days ≤ *d−1*. If
day *d*'s own outcome leaks into its own recency input, every mode looks magically accurate
and a mode that fails live would appear to win. Enforced at the harness boundary and covered
by a dedicated unit test (builder for *d* never reads day ≥ *d*).

## Testing

- **Unit (pure, no network):** `WeightedRecent` weightings (ytd/w14/w30/decay) on a
  synthetic `[]DayFP`; early-season short-series behavior; decay weight math.
- **Unit:** leakage guard — truncation for day *d* excludes day ≥ *d*.
- **Regression:** existing `hitterBlendWeights` / blend tests stay green (curve unchanged).
- **Harness:** `RunStrategyComparison` over a small synthetic set with a stub source/optimizer
  path produces deterministic per-variant aggregates.

## Out of scope / future rounds

- Pitchers (role-aware blend) once a hitter window wins.
- Promote a winning mode to a production `BLEND_RECENCY` selector.
- Tune `hitterBlendWeights` constants for windowed samples (separate experiment — change one
  thing at a time).
- Generalize the harness to non-recency experiments (already supported by the seam; just pass
  different `StrategyVariant`s).
