# Minor League Prospect Alerting & Evaluation System

## Overview

A new `internal/prospects` package that monitors minor league players (**both hitters and pitchers**) across three dimensions — **call-up news & transactions**, **MiLB performance breakouts**, and **Fantrax roster eligibility changes** — and surfaces a daily prospect report in the GitHub Actions job summary.

Data comes from **MLB Stats API** (transactions, rosters, game logs) and **FanGraphs** (prospect rankings/lists, behind an interface for source swappability). Alerts appear as a new `=== Prospect Report ===` section in the daily GHA run, between roster alerts and the hitter ranking.

> **Audit note:** This plan covers both hitter and pitcher prospects. SP prospect call-ups are among the highest-value alerts in fantasy baseball and must not be omitted.

---

## Architecture

```
                  ┌──────────────────────┐
                  │   cmd/main.go        │
                  │  (adds --prospects)  │
                  └────────┬─────────────┘
                           │
              ┌────────────▼────────────────┐
              │   internal/prospects         │
              │                              │
              │  ┌────────────────────────┐  │
              │  │  monitor.go            │  │  ← orchestrates all three sources
              │  │  Monitor.Run() →Report │  │
              │  └────┬───────┬───────┬───┘  │
              │       │       │       │      │
              │  ┌────▼──┐ ┌─▼────┐ ┌▼────┐ │
              │  │ txns  │ │ perf │ │ rank │ │
              │  │.go    │ │.go   │ │.go   │ │
              │  └───────┘ └──────┘ └──────┘ │
              └──────────────────────────────┘
                    │           │          │
          ┌─────────┘     ┌────┘     ┌────┘
          ▼               ▼          ▼
   MLB Stats API    MLB Stats API   FanGraphs
   /transactions    /people/stats   prospect lists
```

### New package: `internal/prospects`

| File | Responsibility |
|------|---------------|
| `monitor.go` | `Monitor` struct + `Run()` method — orchestrates sources, deduplicates, formats report |
| `transactions.go` | MLB Stats API transaction watcher — call-ups, option/DFA, 40-man adds |
| `performance.go` | MiLB stat breakout detection — recent game logs vs. season averages |
| `rankings.go` | FanGraphs prospect list fetcher — top-100 and team lists |
| `upgrades.go` | Prospect swap engine — compares rostered prospects vs. available FAs by rank |
| `report.go` | `Report` type, formatting, and GHA job summary output |
| `cache.go` | File-based JSON cache (`.prospects-cache/`) for rankings + last-seen transactions |
| `testdata/` | Mock JSON responses for unit tests |

---

## Data Sources & APIs

### 1. MLB Stats API — Transactions (free, no auth)

**Endpoint:** `https://statsapi.mlb.com/api/v1/transactions?startDate=YYYY-MM-DD&endDate=YYYY-MM-DD`

Relevant transaction type codes:
- `"CU"` — Called up from minors
- `"OPT"` — Optioned to minors
- `"DFA"` — Designated for assignment
- `"ASG"` — Assigned (40-man roster add)
- `"RET"` — Recalled from rehab / returned

**What we extract:** Player name, MLB team, transaction type, effective date. Cross-reference against Fantrax roster to flag "this player is on your Minors roster and just got called up" or "this free agent prospect just got called up — consider a pickup."

### 2. MLB Stats API — MiLB Player Stats & Game Logs (free, no auth)

**Endpoint:** `https://statsapi.mlb.com/api/v1/people/{playerId}/stats?stats=gameLog&group=hitting&season=2026&sportId=11,12,13,14`
**Pitchers:** Same endpoint with `group=pitching` for K/9, ERA, WHIP tracking.

Sport IDs: `11` = Triple-A, `12` = Double-A, `13` = High-A, `14` = Single-A.

**What we extract:** Last 14 days of game logs for prospects on your Minors roster. For hitters: compute rolling batting line (AVG, OPS, HR, SB) and compare to season line. For pitchers: compute rolling K/9, ERA, WHIP and compare to season line. Flag significant breakouts with **level-adjusted thresholds** and a **minimum 8-game sample**.

**Breakout thresholds (hitters, by level):**
- AAA: 14-day OPS > season OPS + 0.150
- AA: 14-day OPS > season OPS + 0.200
- A-ball: 14-day OPS > season OPS + 0.250

**Breakout thresholds (pitchers, by level):**
- AAA: 14-day ERA < season ERA - 1.00, or K/9 > season K/9 + 2.0
- AA: 14-day ERA < season ERA - 1.50, or K/9 > season K/9 + 2.5
- A-ball: 14-day ERA < season ERA - 2.00, or K/9 > season K/9 + 3.0

**Cold streak thresholds:** Only surface for top-50 ranked prospects. Hitters: -0.200 OPS delta. Pitchers: +1.50 ERA delta.

> **Audit note:** Minimum 8 games in the rolling window required before any alert fires. This prevents false positives from small samples (a single 4-hit game can skew 10-game windows).

### 3. Prospect Rankings — MLB Pipeline (primary) + FanGraphs (fallback)

Rankings are fetched behind a `RankingSource` interface so sources can be swapped.

**Primary: MLB Pipeline Top Prospects (free, no auth)**

**Endpoint:** `https://statsapi.mlb.com/api/v1/prospects?season=2026&topN=100`

MLB's Stats API publishes the official MLB Pipeline Top 100 prospect list as JSON. Free, stable, no authentication required — same API family as the schedule and transaction endpoints we already use.

**Fallback: FanGraphs Prospect Rankings (unstable, may require FG+ auth)**

**Endpoint:** `https://www.fangraphs.com/api/prospects/board/prospect-list?type=prospects&pos=all`

> **Warning:** The FanGraphs prospect board endpoint is **not a stable public API** and may require FanGraphs+ authentication. Used as a fallback only if MLB Pipeline is unavailable. Handle 403/401 responses explicitly.

**What we extract:** Rank, player name, team, ETA, position. Cross-reference against your Fantrax Minors roster to annotate which of your prospects are ranked and how highly. Cross-reference against available free agents to find upgrade opportunities.

---

## Alert Types

```go
type AlertKind string

const (
    CalledUp        AlertKind = "called-up"        // MLB transaction: CU/recall
    Optioned        AlertKind = "optioned"          // MLB transaction: OPT/DFA
    PerformanceHot  AlertKind = "performance-hot"   // 14-day breakout in MiLB
    PerformanceCold AlertKind = "performance-cold"  // 14-day slump (informational)
    RankingRise     AlertKind = "ranking-rise"       // Moved up in prospect rankings
    FreeAgentBuzz   AlertKind = "free-agent-buzz"    // Ranked prospect called up, not on your team
    UpgradeAvail    AlertKind = "upgrade-available"  // A higher-ranked FA prospect could replace a rostered one
)
```

### Priority levels

| Priority | Meaning | Examples |
|----------|---------|---------|
| **High** | Actionable now — roster move needed | Your prospect called up; ranked FA called up; upgrade available |
| **Medium** | Monitoring — may need action soon | Hot performance streak; ranking rise |
| **Low** | Informational | Cold streak; optioned player |

---

## Key Types

```go
// ProspectAlert represents a single alert about a prospect.
type ProspectAlert struct {
    Kind       AlertKind
    Priority   string    // "high", "medium", "low"
    PlayerName string
    MLBTeam    string
    Detail     string    // human-readable description
    Stats      string    // optional stat line (e.g., "Last 14d: .342/.401/.658")
    OnMyTeam   bool      // true if player is on your Fantrax roster
    FGRank     int       // FanGraphs prospect rank (0 if unranked)
}

// Report is the full prospect report for a given day.
type Report struct {
    Date       time.Time
    Alerts     []ProspectAlert
    Rankings   []RankedProspect  // your rostered prospects, sorted by FG rank
    Generated  time.Time
}

// RankedProspect is a prospect with ranking info.
type RankedProspect struct {
    Name       string
    MLBTeam    string
    FGRank     int
    FV         int     // FanGraphs future value (e.g., 55, 60)
    ETA        string  // "2026", "2027"
    Level      string  // "AAA", "AA", etc.
    RecentLine string  // last 14d stat line
}
```

---

## Integration with Existing Code

### cmd/main.go changes

```go
// New flag
prospects := flag.Bool("prospects", false, "run minor league prospect report")

// After roster alerts, before lineup optimization:
if *prospects {
    prospectMonitor := prospects.NewMonitor(ft, cfg)
    report, err := prospectMonitor.Run(today)
    if err != nil {
        log.Printf("WARNING: prospect report failed (%v)", err)
    } else {
        report.Print()           // console output
        report.WriteGHASummary() // $GITHUB_STEP_SUMMARY
    }
}
```

### Monitor.Run() flow

```
1. Fetch your Minors roster from Fantrax     (ft.GetFullHitterRoster)
2. In parallel:
   a. Fetch MLB transactions (last 3 days)    (transactions.go)
   b. Fetch MiLB game logs for your prospects  (performance.go)
   c. Load FanGraphs rankings (from cache or fresh fetch)  (rankings.go)
3. Cross-reference:
   - Match transactions against your roster → CalledUp / Optioned alerts
   - Match transactions against FG top-100 NOT on your roster → FreeAgentBuzz
   - Compute 14-day rolling stats vs season → PerformanceHot / PerformanceCold
   - Compare current FG rank to cached previous → RankingRise
4. Deduplicate, sort by priority, build Report
```

### Fantrax integration points

The existing `fantrax.Player` struct already has `InMinors bool` and `Status string`. We need:

- **Player ID mapping**: The `fantrax.Player.Name` needs to be fuzzy-matched against MLB Stats API player names. We'll reuse `projections.NormalizeName()` and add MLB player ID lookup via the Stats API search endpoint.
- **New method on fantrax.Client**: `GetMinorsRoster() ([]Player, error)` — filters both hitter and pitcher rosters to only Minors-status players.

> **Audit warning: Player name matching is the single biggest failure risk.** Cross-referencing players across Fantrax, MLB Stats API, and FanGraphs via name + team is fragile. Hispanic multi-surnames, Jr./II suffixes, transliteration differences, and MiLB affiliate vs. parent org mismatches will cause silent failures. Mitigations:
> - Make `player-ids.json` cache manually editable for hand-correcting persistent mismatches
> - Add a match confidence score and log unmatched players loudly (never silently skip)
> - Use Fantrax player ID as a secondary lookup key if available

### MLB Stats API player ID resolution

New function in `prospects/`:
```go
// ResolveMLBPlayerID searches the MLB Stats API for a player by name and team.
// Returns the MLB player ID needed for game log queries.
func ResolveMLBPlayerID(name, team string) (int, error)
```

**Endpoint:** `https://statsapi.mlb.com/api/v1/people/search?names={name}&sportIds=11,12,13,14`

Resolved IDs are cached in `.prospects-cache/player-ids.json` to avoid repeated lookups.

---

## Prospect Upgrade Recommendations

A core goal of the system: don't just monitor your current prospects — actively recommend **swapping a weaker rostered prospect for a stronger available one**. This lives in a new file `upgrades.go` and produces `UpgradeAvail` alerts.

### How it works

```
1. Fetch your Minors roster (players with Status="Minors")
2. For each rostered prospect, look up their FanGraphs rank (0 = unranked)
3. Fetch the full FanGraphs prospect list (top 100+)
4. Query Fantrax free agent pool for prospects NOT on any team
5. For each available prospect ranked higher than a rostered one by ≥ threshold:
   → Emit an UpgradeAvail alert: "Drop X (#85) → Pick up Y (#32)"
```

### Upgrade logic

```go
// upgrades.go

// UpgradeCandidate represents a recommended swap.
type UpgradeCandidate struct {
    Drop       RankedProspect  // your worst-ranked rostered prospect
    Add        RankedProspect  // the better available free agent
    RankGap    int             // how much higher the FA is ranked (positive = better)
}

// FindUpgrades compares your Minors roster against available free agents.
// Returns swap recommendations sorted by rank gap (largest improvement first).
//
// Rules:
//   - Only recommends swaps where the FA is ranked ≥ threshold spots higher
//   - Unranked rostered prospects are always eligible for replacement
//     (treated as rank 999 for comparison purposes)
//   - Respects Minors slot capacity — won't recommend adds if slots are full
//     unless paired with a corresponding drop
//   - Deduplicates: each rostered player appears in at most one recommendation
//     (paired with the best available upgrade)
func FindUpgrades(
    rosteredProspects []RankedProspect,
    availableFAs      []RankedProspect,
    rankThreshold     int,
    minorsCapacity    int,
) []UpgradeCandidate
```

### Threshold configuration

> **Audit recommendation: Use FV-tier-based thresholds instead of a flat rank gap.** Prospect rankings are not linear in talent — the gap between #5 and #25 is enormous while #75 vs #95 is negligible. Prefer FanGraphs Future Value (FV) directly, as a gap of one FV tier (e.g., 45→55) is always meaningful regardless of ranking position.

**Tiered threshold approach (recommended over flat threshold):**

| Prospect rank range | Minimum rank gap to trigger alert |
|---------------------|----------------------------------|
| Top 10 | 5 (any top-10 FA available is worth grabbing) |
| 11-50 | 15 |
| 51-100 | 25 |
| Below 100 / unranked | Any ranked FA triggers alert |

**Alternative: FV-based threshold** — trigger when available FA's FV exceeds rostered prospect's FV by ≥ 1 tier (5 points). This is more stable than rank ordinals which shift with list updates.

| Variable | Default | Description |
|----------|---------|-------------|
| `PROSPECT_UPGRADE_RANK_THRESHOLD` | `20` | Fallback flat threshold if FV data unavailable |
| `PROSPECT_UPGRADE_USE_FV` | `true` | Prefer FV-tier comparison over rank ordinals |

**Why a threshold?** Without it, the system would nag about every marginal 1-2 spot difference. The tiered approach ensures alerts are calibrated to the actual talent gap at each level of the ranking.

### Special cases

**Unranked rostered prospects:** If you're holding a prospect who isn't on any FanGraphs list (rank = 0, treated as 999), *any* ranked free agent above the threshold triggers an alert. This catches stale roster holds — players you drafted on upside who have since fallen off prospect lists.

**Multiple upgrades for the same slot:** If two free agents are both better than your worst prospect, the system recommends the single best swap (highest-ranked FA). The report also notes "2 other upgrades available" as a breadcrumb so you know there are options.

**ETA awareness:** Upgrades are annotated with the FanGraphs ETA field. A swap from an ETA-2028 prospect to an ETA-2026 prospect carries extra weight — the report marks these with a "near-term" tag since the new prospect could contribute to your MLB roster sooner.

### Free agent detection

> **Audit warning: GATING RISK.** The `go-fantrax` library does not appear to expose a free agent search/filter endpoint. Before designing the upgrade engine around this method, verify that `go-fantrax/auth_client` supports player search. If not, options: (a) contribute the endpoint to `go-fantrax`, or (b) use the authenticated client's raw HTTP capabilities to hit the Fantrax player search API directly (same pattern as `ApplyLineup`). **Scope this investigation before committing to the upgrade feature.**

New method needed on the Fantrax client:

```go
// GetAvailableProspects returns minor-league players not owned by any team
// in the league. Uses the Fantrax player search/filter API.
func (c *Client) GetAvailableProspects() ([]Player, error)
```

This queries the Fantrax player pool filtered to unowned + minor league status. The result is cross-referenced against FanGraphs rankings to build the `availableFAs` list.

### GHA output section

The upgrade recommendations appear as a new table in the prospect report:

```markdown
### 🔄 Prospect Upgrade Recommendations
| Drop | Rank | ← Swap → | Add | Rank | Gap | ETA |
|------|------|----------|-----|------|-----|-----|
| Tyler Black | #92 | → | Ethan Salas | #18 | +74 | 2026 (near-term) |
| (unranked) Kevin Alcántara | — | → | Marcelo Mayer | #45 | — | 2026 |

_Threshold: upgrades shown when rank gap ≥ 20. Adjust via `PROSPECT_UPGRADE_RANK_THRESHOLD`._
```

---

## GHA Job Summary Output

The report writes to `$GITHUB_STEP_SUMMARY` using GitHub-flavored markdown:

```markdown
## 🔍 Prospect Report — 2026-04-15

### 🚨 Action Required
| Player | Team | Alert | Detail |
|--------|------|-------|--------|
| Spencer Torkelson | DET | Called Up | Recalled from AAA Toledo — move from Minors slot |

### 📈 Hot Prospects (Your Roster)
| Player | Team | Level | Last 14d | Season | FG Rank |
|--------|------|-------|----------|--------|---------|
| Jackson Chourio | MIL | AAA | .342/.401/.658 | .278/.330/.445 | #12 |

### 👀 Free Agent Watch
| Player | Team | Alert | FG Rank | Detail |
|--------|------|-------|---------|--------|
| Jasson Dominguez | NYY | Called Up | #8 | Top prospect called up — available in your league? |

### 🔄 Prospect Upgrade Recommendations
| Drop | Rank | → Swap → | Add | Rank | Gap | ETA |
|------|------|----------|-----|------|-----|-----|
| Tyler Black | #92 | → | Ethan Salas | #18 | +74 | 2026 (near-term) |
| Kevin Alcántara | — | → | Marcelo Mayer | #45 | +954 | 2026 |

_Threshold: upgrades shown when rank gap ≥ 20. Adjust via `PROSPECT_UPGRADE_RANK_THRESHOLD`._

### 📊 Your Prospect Rankings
| Rank | Player | Team | FV | ETA | Level |
|------|--------|------|----|-----|-------|
| #12 | Jackson Chourio | MIL | 60 | 2026 | AAA |
| #45 | Colton Cowser | BAL | 50 | 2026 | AAA |
```

---

## Caching Strategy

**Directory:** `.prospects-cache/` (gitignored, same pattern as `.fantrax-cache/`)

| File | Refresh Interval | Contents |
|------|-----------------|----------|
| `player-ids.json` | Permanent (IDs don't change); manually editable for corrections | `{"normalized_name\|team": mlbID}` |
| `rankings.json` | Weekly | FanGraphs top prospect list + fetch timestamp |
| `last-transactions.json` | Cursor-based (see below) | Last processed transaction date to avoid re-alerting |

> **Audit improvement:** Instead of a fixed lookback window, `last-transactions.json` stores the last-processed transaction date. Each run scans from that date forward, making the system resilient to missed GHA runs. On first run (no cache), defaults to 7-day lookback.

---

## Configuration

### New env vars (all optional, with sensible defaults)

| Variable | Default | Description |
|----------|---------|-------------|
| `PROSPECT_LOOKBACK_DAYS` | `7` | Initial lookback on first run (subsequent runs use cursor) |
| `PROSPECT_HOT_OPS_DELTA` | `0.150` | OPS increase over season avg to trigger "hot" alert (AAA; scales by level) |
| `PROSPECT_ROLLING_DAYS` | `14` | Window for rolling performance stats |
| `PROSPECT_MIN_GAMES` | `8` | Minimum games in rolling window before alerts fire |
| `PROSPECT_RANK_CACHE_HOURS` | `168` (7 days) | How long to cache FanGraphs rankings |
| `PROSPECT_UPGRADE_RANK_THRESHOLD` | `20` | Fallback flat rank gap (used when FV data unavailable) |
| `PROSPECT_UPGRADE_USE_FV` | `true` | Prefer FV-tier comparison over rank ordinals |

These go into `internal/config/config.go` as optional fields on `Config`, defaulting if not set.

### GHA workflow changes

Add `GITHUB_STEP_SUMMARY` output and ensure the prospect report runs:

```yaml
- name: Optimize lineup + prospect report
  run: go run ./cmd --prospects
  env:
    # ... existing secrets ...
    # Note: $GITHUB_STEP_SUMMARY is already available — do NOT pass it explicitly
```

No new secrets required — MLB Stats API and FanGraphs prospects are public.

> **Audit suggestion:** Consider a second, lighter-weight afternoon run (e.g., 8pm UTC / 4pm ET) with a `--transactions-only` flag that skips performance and ranking checks. This catches same-day call-ups before other fantasy managers can react.

---

## Testing Strategy

All tests are unit tests with no network dependencies, consistent with the rest of the project.

| Test file | What it covers |
|-----------|---------------|
| `transactions_test.go` | Parse mock transaction JSON; filter by type codes; match against roster |
| `performance_test.go` | Compute rolling stats from mock game logs; detect breakouts and slumps; verify min-game filter; test level-adjusted thresholds |
| `rankings_test.go` | Parse mock FanGraphs JSON; cross-reference against roster |
| `upgrades_test.go` | Tiered threshold logic, FV-based comparison, unranked-player handling, ETA tagging, dedup, capacity limits |
| `monitor_test.go` | End-to-end: mock all sources → verify Report contents and dedup |
| `report_test.go` | Verify markdown formatting and GHA summary output |
| `cache_test.go` | Read/write/expiry logic for file-based cache |

Mock data lives in `internal/prospects/testdata/`. HTTP endpoints are overridden via `var` URLs (same pattern as `schedule/mlb.go`).

---

## Implementation Order

### Phase 0: Feasibility check (GATING)
0. **Verify `GetAvailableProspects` API feasibility** — investigate whether `go-fantrax/auth_client` supports player search/filter. If not, scope the effort to add it. This gates Phase 3.

### Phase 1: Foundation (transactions + roster cross-reference)
1. Create `internal/prospects/` package scaffold with `RankingSource` interface
2. Implement `transactions.go` — MLB Stats API transaction fetcher (hitters AND pitchers)
3. Implement `cache.go` — file-based JSON cache with cursor-based transaction tracking
4. Implement `monitor.go` — orchestrator with just transaction alerts
5. Implement `report.go` — console + GHA summary formatting
6. Wire into `cmd/main.go` with `--prospects` flag (default: false)
7. Unit tests for Phase 1

### Phase 2: Performance monitoring
8. Implement MLB player ID resolution + caching (manually editable cache)
9. Implement `performance.go` — MiLB game log fetcher + breakout detection for **both hitters and pitchers**
   - Level-adjusted thresholds (AAA/AA/A-ball)
   - Minimum 8-game sample filter
   - Pitcher metrics: K/9, ERA, WHIP
10. Add performance alerts to `monitor.go`; parallelize per-player fetches with `errgroup` + semaphore (3-5 concurrency)
11. Unit tests for Phase 2

### Phase 3: External rankings + upgrade recommendations
12. Implement `rankings.go` behind `RankingSource` interface — FanGraphs prospect list fetcher with 403/401 error handling
13. Add ranking annotations and `FreeAgentBuzz` alerts
14. Add "Your Prospect Rankings" table to report
15. Implement `upgrades.go` — FV-tier-based comparison with tiered rank thresholds as fallback
16. Add `GetAvailableProspects()` to Fantrax client (unowned minor leaguers) — **depends on Phase 0 feasibility**
17. Add "Prospect Upgrade Recommendations" table to report
18. Unit tests for Phase 3 (including tiered threshold logic, FV comparison, and edge cases)

### Phase 4: Polish & deploy
19. Add new config fields + env var defaults
20. Update GHA workflow (add `--prospects` flag; consider afternoon `--transactions-only` run)
21. Unify prospect call-up alerts with existing roster alert system to avoid duplicate notifications
22. Update CLAUDE.md with new commands and package docs
23. End-to-end testing with `--dry-run --prospects`

---

## Estimated Effort

- **Phase 0:** ~1 day investigation — API feasibility
- **Phase 1:** ~3-4 files, ~500 lines — the core loop (hitters + pitchers)
- **Phase 2:** ~2 files, ~400 lines — stat computation with level-adjusted thresholds
- **Phase 3:** ~3-4 files, ~500 lines — rankings interface, FV-based upgrades, FA pool query
- **Phase 4:** ~config + docs + alert unification, ~150 lines

Total: **~1,550 lines of new code** across ~12-14 new files, plus ~250 lines of test code per phase.

---

## Strategic Audit Notes

This plan was audited by the fantasy-baseball-strategist agent on 2026-03-23. Key changes incorporated:

1. **Pitcher prospect coverage added** — SP call-ups are highest-value fantasy alerts
2. **Level-adjusted breakout thresholds** with minimum 8-game sample requirement
3. **FV-tier-based upgrade thresholds** instead of flat rank gap
4. **RankingSource interface** for FanGraphs source swappability
5. **Cursor-based transaction tracking** instead of fixed lookback window
6. **Player name matching warnings** with manual cache editing support
7. **GetAvailableProspects API feasibility** flagged as Phase 0 gating work
8. **`--prospects` flag defaults to false** for better DX
9. **Afternoon transaction-only run** suggested for competitive edge

### Future considerations (not in scope for v1)
- **Service time manipulation detection** — flag top prospects dominating AAA but not called up
- **Injury replacement call-up prediction** — cross-reference MLB IL transactions with org prospect depth
- **Automatic roster moves** — optimizer acts on prospect call-up alerts (move from Minors to Active)
