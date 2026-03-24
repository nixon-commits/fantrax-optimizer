# Matchup Adjustments: Platoon Splits + Opposing Pitcher Quality

**Date:** 2026-03-24
**Scope:** Two hitter projection adjustments + one park factor bugfix

## Problem

The optimizer treats all games equally regardless of the opposing starting pitcher's handedness or quality. Platoon splits are among the most stable predictive signals in baseball (~5-8% swing), and opposing pitcher quality can shift hitter output by 10-15%. These are free edges we're leaving on the table.

Additionally, the XBH park factor in `computeParkAdjustment` uses a simple average of H2B/H3B/HR factors instead of weighting by the player's projected distribution.

## Changes

### 1. XBH Park Factor Fix

**File:** `internal/projections/park_adjusted.go`, line 124

**Before:**
```go
"XBH": (pf.H2B + pf.H3B + pf.HR) / 3.0,
```

**After:**
```go
// XBH factor weighted by player's projected distribution (same approach as TB).
if xbh > 0 {
    statFactor["XBH"] = (proj.Doubles*pf.H2B + proj.Triples*pf.H3B + proj.HR*pf.HR) / xbh
}
```

Rationale: A typical player hits far more doubles than triples. The simple average weights triples at 33% when they should be ~5%. In extreme parks like Coors (H3B=2.02), this overinflates XBH adjustments.

### 2. Handedness + FIP Data from FanGraphs

**Files:** `internal/projections/fangraphs.go`, `internal/projections/pitcher_fangraphs.go`

Add fields to existing FanGraphs fetches (same API call, no extra network request):

**Hitter structs:**
- `fgRow` gets `Bats string \`json:"Bats"\`` (private parse struct)
- `Projection` gets `Bats string` (public API type — consumed by `MatchupAdjustedSource`)
- Populated during `NewFanGraphsSource` parsing

**Pitcher structs:**
- `fgPitchRow` gets `Throws string \`json:"Throws"\`` and `FIP float64 \`json:"FIP"\``
- `PitcherProjection` gets `Throws string` and `FIP float64`
- Populated during `NewFanGraphsPitcherSource` parsing

**Data extraction:** Since `FanGraphsSource` and `FanGraphsPitcherSource` store projections in private maps, add exported accessor methods:

```go
// FanGraphsSource
func (s *FanGraphsSource) HitterBats() map[string]string
// Returns NormalizeName(name) → "R"/"L"/"S" for all hitters

// FanGraphsPitcherSource
func (s *FanGraphsPitcherSource) PitcherInfo() (handedness map[string]string, fip map[string]float64, leagueAvgFIP float64)
// Returns NormalizeName(name) → "R"/"L" and NormalizeName(name) → FIP
// leagueAvgFIP is IP-weighted mean across all pitchers with IP > 0
```

IP-weighting for `leagueAvgFIP` prevents fringe pitchers with extreme FIPs and few projected innings from skewing the average.

### 3. MatchupAdjustedSource

**New file:** `internal/projections/matchup_adjusted.go`

A `Source` + `PtsPerGameSource` wrapper that applies two independent multipliers to the inner source's pts/game output.

#### Construction

```go
type OpposingPitcher struct {
    Name   string
    Team   string
    Throws string  // "R" or "L"
    FIP    float64 // from Steamer projection
}

type MatchupAdjustedSource struct {
    inner            Source
    innerPPS         PtsPerGameSource      // type-asserted from inner, may be nil
    opposingPitchers map[string]OpposingPitcher // batting team abbr → opposing SP
    hitterBats       map[string]string          // NormalizeName(name) → "R"/"L"/"S"
    leagueAvgFIP     float64
}

func NewMatchupAdjustedSource(
    inner Source,
    opposingPitchers map[string]OpposingPitcher,
    hitterBats map[string]string,
    leagueAvgFIP float64,
) *MatchupAdjustedSource
```

The constructor type-asserts `inner` to `PtsPerGameSource` (same pattern as `ParkAdjustedSource` lines 38-39), storing it as `innerPPS`. This preserves blended pts/game values from the inner chain rather than recomputing from raw projections.

#### Adjustment Logic

**Platoon multiplier:**

| Matchup | Multiplier | Rationale |
|---------|-----------|-----------|
| Favorable (LHH vs RHP, RHH vs LHP) | 1.00 | Steamer baseline is already favorable-skewed |
| Unfavorable (LHH vs LHP, RHH vs RHP) | 0.93 | Penalty-only avoids double-boosting |
| Switch hitter ("S" or "B") | 1.00 | Switch hitters bat from the opposite side by definition, so they always have the favorable matchup |
| Unknown handedness | 1.00 | Degrade to neutral |

Note: FanGraphs may return `"B"` for switch hitters instead of `"S"`. The `HitterBats()` accessor normalizes `"B"` to `"S"`. The platoon logic treats both as neutral (1.00).

**Pitcher quality multiplier** (higher opposing FIP = weaker pitcher = better for hitters):

```
qualityMult = clamp(opposingFIP / leagueAvgFIP, 0.85, 1.15)
```

- Ace (FIP 2.80, avg 4.00): `2.80/4.00 = 0.70` → capped at 0.85
- Bad pitcher (FIP 5.50): `5.50/4.00 = 1.375` → capped at 1.15
- No data or `leagueAvgFIP == 0`: 1.00 (division guard at both construction and computation time)

**Combined cap:** `clamp(platoonMult * qualityMult, 0.80, 1.15)`

Combined range: `[0.93 * 0.85, 1.00 * 1.15] = [0.79, 1.15]` → clamped to `[0.80, 1.15]`.

#### GetPtsPerGame Flow

1. Get base pts from `innerPPS.GetPtsPerGame` (falls back to `ExpectedPtsFromProj` if inner doesn't implement `PtsPerGameSource`)
2. Look up hitter's bat side via `hitterBats[NormalizeName(name)]`
3. Look up opposing pitcher via `opposingPitchers[mlbTeam]`
4. Compute platoon multiplier from bat side vs pitcher throws
5. Compute pitcher quality multiplier from `opposingFIP / leagueAvgFIP`
6. Return `basePts * clamp(platoonMult * qualityMult, 0.80, 1.15)`

#### GetProjection

Delegates to `inner.GetProjection` (unadjusted), same pattern as `ParkAdjustedSource`.

### 4. Wiring in `cmd/optimize.go`

**Build phase** (shared across dates, after fetching projections):

```go
// After NewFanGraphsSource:
var hitterBats map[string]string
if fgSrc != nil {
    hitterBats = fgSrc.HitterBats()
}

// After NewFanGraphsPitcherSource:
var pitcherHandedness map[string]string
var pitcherFIP map[string]float64
var leagueAvgFIP float64
if fgPitSrc != nil {
    pitcherHandedness, pitcherFIP, leagueAvgFIP = fgPitSrc.PitcherInfo()
}
```

**Per-date phase** (inside parallel `g.Go` loop):

Build `opposingPitchers` by cross-referencing `ProbableStarters(date)` with pitcher data. The inversion from pitcher→team to battingTeam→pitcher uses `GameVenues` data (already fetched per-date) which maps every team to the home team of their game, giving us game pairings:

```go
// ProbableStarters returns pitcherName → pitcherTeam.
// GameVenues returns team → homeTeam (two teams map to same homeTeam = same game).
// Invert: for each game, the away team faces the home pitcher and vice versa.
opposingPitchers := make(map[string]projections.OpposingPitcher)
for pitcherName, pitcherTeam := range probableStarters {
    // Find the team facing this pitcher by scanning venues for the other team
    // in the same game.
    for team, homeTeam := range venues {
        if team == pitcherTeam {
            continue // skip the pitcher's own team
        }
        // Check if they're in the same game: both map to the same homeTeam,
        // OR one of them IS the homeTeam and the other maps to it.
        pitcherHome := venues[pitcherTeam]
        if homeTeam == pitcherHome {
            // team is facing this pitcher
            opp := projections.OpposingPitcher{
                Name: pitcherName, Team: pitcherTeam,
            }
            if h, ok := pitcherHandedness[pitcherName]; ok {
                opp.Throws = h
            }
            if f, ok := pitcherFIP[pitcherName]; ok {
                opp.FIP = f
            }
            opposingPitchers[team] = opp
            break
        }
    }
}
```

Then wrap the source:
```go
dateHitterSrc = hitterProjSrc
if venues != nil && parkFactors != nil {
    dateHitterSrc = NewParkAdjustedSource(...)
}
if len(opposingPitchers) > 0 && leagueAvgFIP > 0 {
    dateHitterSrc = NewMatchupAdjustedSource(dateHitterSrc, opposingPitchers, hitterBats, leagueAvgFIP)
}
```

These maps (`hitterBats`, `pitcherHandedness`, `pitcherFIP`, `leagueAvgFIP`) are built once and only read in the parallel loop — no concurrency issues.

When no probable starters are available (future dates), `opposingPitchers` is empty and no wrapping occurs.

### 5. Source Chain

```
FanGraphs Steamer → BlendedSource (60/40 recent) → ParkAdjustedSource → MatchupAdjustedSource → Optimizer
```

Each layer implements `Source` + `PtsPerGameSource`, delegating `GetProjection` to inner and adjusting only `GetPtsPerGame`.

## Testing

**New file:** `internal/projections/matchup_adjusted_test.go`

| Test | Validates |
|------|-----------|
| Favorable platoon (RHH vs LHP) | No change from base pts |
| Unfavorable platoon (LHH vs LHP) | ~7% reduction |
| Switch hitter vs LHP | Always 1.00 regardless of pitcher hand |
| Ace suppression (low FIP) | Reduced pts, capped at 0.85 |
| Bad pitcher boost (high FIP) | Boosted pts, capped at 1.15 |
| Combined cap | Unfavorable platoon + ace floors at 0.80 |
| No opposing pitcher data | Base pts unchanged |
| Unknown hitter handedness | Base pts unchanged |
| Team not playing today | Base pts unchanged (no entry in opposingPitchers) |
| Full chain composability | BlendedSource → ParkAdjusted → MatchupAdjusted composes correctly |
| Zero leagueAvgFIP guard | No division by zero; returns base pts |

**Updated test:** `TestParkAdjustedSource_CoorsBoost` — verify XBH factor is now player-weighted by asserting a specific expected adjusted value (computed from the player's 20 2B / 5 3B / 15 HR distribution with Coors factors), not just a directional check.

All tests use stub sources, no network calls — consistent with existing test patterns.

## Fallback Behavior

- No probable starters available → no matchup adjustment (same as current)
- Pitcher not in FanGraphs data → no quality adjustment for that game (FIP=0 → multiplier=1.0)
- Hitter not in FanGraphs data → no platoon adjustment for that hitter
- Park factors unavailable → no park adjustment (existing behavior preserved)
- `leagueAvgFIP` is zero → no matchup source created

Each layer degrades independently to neutral (1.0 multiplier).
