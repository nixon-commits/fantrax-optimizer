# Minor League Prospect Alerting & Evaluation System

## Overview

A new `internal/prospects` package that monitors minor league players across three dimensions — **call-up news & transactions**, **MiLB performance breakouts**, and **Fantrax roster eligibility changes** — and surfaces a daily prospect report in the GitHub Actions job summary.

Data comes from **MLB Stats API** (transactions, rosters, game logs) and **FanGraphs** (prospect rankings/lists). Alerts appear as a new `=== Prospect Report ===` section in the daily GHA run, between roster alerts and the hitter ranking.

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

Sport IDs: `11` = Triple-A, `12` = Double-A, `13` = High-A, `14` = Single-A.

**What we extract:** Last 14 days of game logs for hitters on your Minors roster. Compute rolling batting line (AVG, OPS, HR, SB) and compare to season line. Flag significant breakouts (e.g., 14-day OPS > season OPS + 0.150).

### 3. FanGraphs Prospect Rankings (scrape/API)

**Endpoint:** `https://www.fangraphs.com/api/prospects/board/prospect-list?type=prospects&pos=all`

FanGraphs publishes prospect rankings as JSON. We'll fetch the top-100 list and cache it (rankings don't change daily — refresh weekly).

**What we extract:** Rank, player name, team, FV (future value), ETA, key tools/grades. Cross-reference against your Fantrax Minors roster to annotate which of your prospects are ranked and how highly.

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
prospects := flag.Bool("prospects", true, "run minor league prospect report")

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
- **New method on fantrax.Client**: `GetMinorsRoster() ([]Player, error)` — filters `GetFullHitterRoster()` to only Minors-status players. (Simple convenience wrapper.)

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

| Variable | Default | Description |
|----------|---------|-------------|
| `PROSPECT_UPGRADE_RANK_THRESHOLD` | `20` | Minimum FG rank gap to trigger an upgrade alert. E.g., if set to 20, a #50-ranked FA won't trigger a swap for your #60 prospect, but a #35-ranked FA will. |

**Why a threshold?** Without it, the system would nag about every marginal 1-2 spot difference. A gap of 20+ ranks represents a meaningful talent difference worth the roster churn. You can tune this down to 10 for aggressive management or up to 30+ for a more conservative approach.

### Special cases

**Unranked rostered prospects:** If you're holding a prospect who isn't on any FanGraphs list (rank = 0, treated as 999), *any* ranked free agent above the threshold triggers an alert. This catches stale roster holds — players you drafted on upside who have since fallen off prospect lists.

**Multiple upgrades for the same slot:** If two free agents are both better than your worst prospect, the system recommends the single best swap (highest-ranked FA). The report also notes "2 other upgrades available" as a breadcrumb so you know there are options.

**ETA awareness:** Upgrades are annotated with the FanGraphs ETA field. A swap from an ETA-2028 prospect to an ETA-2026 prospect carries extra weight — the report marks these with a "near-term" tag since the new prospect could contribute to your MLB roster sooner.

### Free agent detection

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
| `player-ids.json` | Permanent (IDs don't change) | `{"normalized_name\|team": mlbID}` |
| `rankings.json` | Weekly | FanGraphs top prospect list + fetch timestamp |
| `last-transactions.json` | Daily | Last processed transaction date to avoid re-alerting |

---

## Configuration

### New env vars (all optional, with sensible defaults)

| Variable | Default | Description |
|----------|---------|-------------|
| `PROSPECT_LOOKBACK_DAYS` | `3` | How many days of MLB transactions to scan |
| `PROSPECT_HOT_OPS_DELTA` | `0.150` | OPS increase over season avg to trigger "hot" alert |
| `PROSPECT_ROLLING_DAYS` | `14` | Window for rolling performance stats |
| `PROSPECT_RANK_CACHE_HOURS` | `168` (7 days) | How long to cache FanGraphs rankings |
| `PROSPECT_UPGRADE_RANK_THRESHOLD` | `20` | Minimum FG rank gap to recommend a prospect swap |

These go into `internal/config/config.go` as optional fields on `Config`, defaulting if not set.

### GHA workflow changes

Add `GITHUB_STEP_SUMMARY` output and ensure the prospect report runs:

```yaml
- name: Optimize lineup + prospect report
  run: go run ./cmd --prospects
  env:
    # ... existing secrets ...
    GITHUB_STEP_SUMMARY: ${{ github.step_summary }}  # already available by default
```

No new secrets required — MLB Stats API and FanGraphs prospects are public.

---

## Testing Strategy

All tests are unit tests with no network dependencies, consistent with the rest of the project.

| Test file | What it covers |
|-----------|---------------|
| `transactions_test.go` | Parse mock transaction JSON; filter by type codes; match against roster |
| `performance_test.go` | Compute rolling stats from mock game logs; detect breakouts and slumps |
| `rankings_test.go` | Parse mock FanGraphs JSON; cross-reference against roster |
| `upgrades_test.go` | Threshold logic, unranked-player handling, ETA tagging, dedup, capacity limits |
| `monitor_test.go` | End-to-end: mock all sources → verify Report contents and dedup |
| `report_test.go` | Verify markdown formatting and GHA summary output |
| `cache_test.go` | Read/write/expiry logic for file-based cache |

Mock data lives in `internal/prospects/testdata/`. HTTP endpoints are overridden via `var` URLs (same pattern as `schedule/mlb.go`).

---

## Implementation Order

### Phase 1: Foundation (transactions + roster cross-reference)
1. Create `internal/prospects/` package scaffold
2. Implement `transactions.go` — MLB Stats API transaction fetcher
3. Implement `cache.go` — file-based JSON cache
4. Implement `monitor.go` — orchestrator with just transaction alerts
5. Implement `report.go` — console + GHA summary formatting
6. Wire into `cmd/main.go` with `--prospects` flag
7. Unit tests for Phase 1

### Phase 2: Performance monitoring
8. Implement MLB player ID resolution + caching
9. Implement `performance.go` — MiLB game log fetcher + breakout detection
10. Add performance alerts to `monitor.go`
11. Unit tests for Phase 2

### Phase 3: External rankings + upgrade recommendations
12. Implement `rankings.go` — FanGraphs prospect list fetcher
13. Add ranking annotations and `FreeAgentBuzz` alerts
14. Add "Your Prospect Rankings" table to report
15. Implement `upgrades.go` — compare rostered prospects vs. available FAs
16. Add `GetAvailableProspects()` to Fantrax client (unowned minor leaguers)
17. Add "Prospect Upgrade Recommendations" table to report
18. Unit tests for Phase 3 (including upgrade threshold logic and edge cases)

### Phase 4: Polish & deploy
19. Add new config fields + env var defaults
20. Update GHA workflow
21. Update CLAUDE.md with new commands and package docs
22. End-to-end testing with `--dry-run --prospects`

---

## Estimated Effort

- **Phase 1:** ~3-4 files, ~400 lines — the core loop
- **Phase 2:** ~2 files, ~300 lines — stat computation
- **Phase 3:** ~3-4 files, ~400 lines — rankings, upgrades engine, FA pool query
- **Phase 4:** ~config + docs, ~100 lines

Total: **~1,200 lines of new code** across ~10-12 new files, plus ~200 lines of test code per phase.
