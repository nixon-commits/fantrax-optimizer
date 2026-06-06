# rosterbot

Domain glossary for the fantasy-baseball automation. Terms are added lazily as design decisions resolve them — this file is not exhaustive.

## Language

### Scoring

**Scoring Weights**:
The league's map of stat short-name → point value (e.g. `HR → 4`, `SO → -1`). The single source of how production converts to fantasy points. Lives in `internal/scoring` as `Weights`; `fantrax.ScoringWeights` is an alias.
_Avoid_: scoring settings, point values, rules.

**Stat Line**:
A neutral set of raw counting stats for one scope (a season projection or a single game), independent of where it came from. `HitterLine` / `PitcherLine` in `internal/scoring`. Adapters build a Stat Line from a `Projection` or an MLB game log; the scorer derives `1B`/`XBH`/`TB` from it and applies the Scoring Weights.
_Avoid_: stat map, box score, stat dict.

**Expected Points**:
The per-game fantasy-point value of a Stat Line: `ApplyHitter(line, w) / G`. The optimizer ranks players by Expected Points.
_Avoid_: projected points, FPG (use only in field names), value.

**Single-Game FPts**:
The fantasy points a player actually scored in one game — a Stat Line scored without per-game division. Used by the backtest/recap backfill, not the optimizer.
_Avoid_: daily points, game score.
