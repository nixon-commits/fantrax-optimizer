# Prospect Alerting & Evaluation System — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a minor league prospect monitoring system that surfaces daily alerts about call-ups, MiLB performance breakouts, and prospect upgrade opportunities in the GHA job summary.

**Architecture:** New `internal/prospects` package with 5 source files. Flat exported functions (matching `roster.CheckRoster` convention), not a Monitor struct. `RankingSource` interface with `ChainedRankingSource` for MLB Pipeline (primary) + FanGraphs (fallback). All HTTP endpoints use `var` URLs for test swapping. Two new methods on `fantrax.Client` (`GetMinorsRoster`, `GetAvailableProspects`).

**Tech Stack:** Go, MLB Stats API (free, no auth), FanGraphs (fallback), go-fantrax `auth_client.GetPlayerPool`, `errgroup` for parallelism.

**Spec:** `docs/PROSPECT_SYSTEM_PLAN.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/prospects/prospects.go` | All exported types, interfaces, constants (~80 lines) |
| `internal/prospects/transactions.go` | MLB Stats API transaction fetch + alert classification (~120 lines) |
| `internal/prospects/performance.go` | MiLB game logs, player ID resolver, breakout detection (~260 lines) |
| `internal/prospects/rankings.go` | MLB Pipeline + FG ranking sources, cache, upgrade engine (~280 lines) |
| `internal/prospects/run.go` | Orchestrator, stdout/GHA formatting (~200 lines) |
| `internal/prospects/transactions_test.go` | Transaction fetch + classification tests |
| `internal/prospects/performance_test.go` | Breakout detection + level threshold tests |
| `internal/prospects/rankings_test.go` | Ranking source + upgrade engine tests |
| `internal/prospects/run_test.go` | Orchestrator degraded-mode test |
| `internal/fantrax/client.go` | Add `GetMinorsRoster()` + `GetAvailableProspects()` (+30 lines) |
| `internal/config/config.go` | Add prospect config fields (+8 lines) |
| `cmd/main.go` | Add `--prospects` flag + wiring (+15 lines) |
| `.github/workflows/lineup.yml` | Add `--prospects` to run step |
| `.gitignore` | Add `.prospects-cache/` |

---

## Conventions to Follow

**Test pattern** (from `internal/schedule/mlb_test.go`):
```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(fixture)
}))
defer srv.Close()
origURL := someURL
someURL = srv.URL + "/path"
defer func() { someURL = origURL }()
```

**Name normalization**: Always use `projections.NormalizeName()` and `projections.NormalizeTeam()` for cross-system matching.

**Non-fatal errors**: Source failures log warnings, don't fatal. Report prints whatever data it has.

**Client methods**: Wrap go-fantrax models into local `fantrax.Player` types via `toPlayer()`.

---

## Task 1: Types and Interfaces

**Files:**
- Create: `internal/prospects/prospects.go`

- [ ] **Step 1: Create the types file with all shared types and interfaces**

```go
package prospects

import "time"

// AlertKind classifies the prospect alert.
type AlertKind string

const (
	CalledUp        AlertKind = "called-up"
	Optioned        AlertKind = "optioned"
	PerformanceHot  AlertKind = "performance-hot"
	PerformanceCold AlertKind = "performance-cold"
	FreeAgentBuzz   AlertKind = "free-agent-buzz"
	UpgradeAvail    AlertKind = "upgrade-available"
)

// ProspectAlert represents a single alert about a prospect.
type ProspectAlert struct {
	Kind       AlertKind
	Priority   string // "high", "medium", "low"
	PlayerName string
	MLBTeam    string
	Position   string // "SS", "SP", etc.
	Detail     string // human-readable description
	Stats      string // optional stat line
	OnMyTeam   bool
	Rank       int  // MLB Pipeline rank, 0 = unranked
	IsPitcher  bool
}

// RankedProspect is a prospect with ranking info.
type RankedProspect struct {
	Name      string
	MLBTeam   string
	MLBID     int    // MLB Stats API player ID
	Position  string // "SS", "SP", etc.
	Rank      int    // 1-100, 0 = unranked
	FV        int    // future value grade (55, 60, etc.), 0 if unavailable
	ETA       string // "2026", "2027"
	Level     string // "AAA", "AA", "A+", "A"
	IsPitcher bool
}

// UpgradeCandidate represents a recommended prospect swap.
type UpgradeCandidate struct {
	Drop     RankedProspect
	Add      RankedProspect
	RankGap  int    // positive = Add is higher ranked
	NearTerm bool   // true if Add's ETA is current or next season
}

// Report is the full prospect report for a given day.
type Report struct {
	Date     time.Time
	Alerts   []ProspectAlert
	Rankings []RankedProspect   // your rostered prospects, sorted by rank
	Upgrades []UpgradeCandidate
}

// RankingSource provides prospect ranking data.
// Implementations: MLBPipelineSource, FanGraphsRankingSource.
type RankingSource interface {
	GetTopProspects(season int) ([]RankedProspect, error)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jnixon/fantrax && go build ./internal/prospects/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/prospects/prospects.go
git commit -m "feat(prospects): add types, interfaces, and constants"
```

---

## Task 2: Config Fields

**Files:**
- Modify: `internal/config/config.go:12-21` (Config struct)
- Modify: `internal/config/config.go:27-36` (Load function)

- [ ] **Step 1: Add prospect fields to Config struct**

Add after `MinorsSlots int` (line 20):

```go
	// Prospect report settings (all optional, with defaults).
	ProspectRollingDays    int
	ProspectMinGames       int
	ProspectRankCacheHours int
	ProspectRankThreshold  int
```

- [ ] **Step 2: Load the new fields in Load()**

Add after `MinorsSlots: envInt("FANTRAX_MINORS_SLOTS", 0),` (line 35):

```go
		ProspectRollingDays:    envInt("PROSPECT_ROLLING_DAYS", 14),
		ProspectMinGames:       envInt("PROSPECT_MIN_GAMES", 8),
		ProspectRankCacheHours: envInt("PROSPECT_RANK_CACHE_HOURS", 168),
		ProspectRankThreshold:  envInt("PROSPECT_UPGRADE_RANK_THRESHOLD", 20),
```

- [ ] **Step 3: Verify it compiles and existing tests pass**

Run: `cd /Users/jnixon/fantrax && go build ./... && go test ./internal/...`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add prospect report configuration fields"
```

---

## Task 3: Fantrax Client Methods

**Files:**
- Modify: `internal/fantrax/client.go` (add 2 methods after `GetFullHitterRoster` at line 164)

- [ ] **Step 1: Add GetMinorsRoster method**

Add after `GetFullHitterRoster()` (after line 164):

```go
// GetMinorsRoster returns all players (hitters and pitchers) currently
// in your Minors roster slot. Used by the prospect report.
func (c *Client) GetMinorsRoster() ([]Player, error) {
	roster, err := c.auth.GetCurrentPeriodTeamRosterInfo(c.teamID)
	if err != nil {
		return nil, fmt.Errorf("get minors roster: %w", err)
	}
	var players []Player
	for _, rp := range roster.MinorsRoster {
		players = append(players, toPlayer(rp))
	}
	return players, nil
}
```

- [ ] **Step 2: Add GetAvailableProspects method**

Add after `GetMinorsRoster`:

```go
// GetAvailableProspects returns minor-league-eligible players not owned
// by any team in the league. Uses the Fantrax player pool API.
func (c *Client) GetAvailableProspects() ([]Player, error) {
	pool, err := c.auth.GetPlayerPool(
		auth_client.WithStatusFilter(auth_client.StatusFilterAvailable),
	)
	if err != nil {
		return nil, fmt.Errorf("get available prospects: %w", err)
	}
	var players []Player
	for _, pp := range pool {
		if !pp.MinorsEligible {
			continue
		}
		players = append(players, Player{
			ID:            pp.PlayerID,
			Name:          pp.Name,
			MLBTeam:       pp.MLBTeamShortName,
			Positions:     pp.Positions,
			PosShortNames: pp.PosShortNames,
			InMinors:      true,
		})
	}
	return players, nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jnixon/fantrax && go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/fantrax/client.go
git commit -m "feat(fantrax): add GetMinorsRoster and GetAvailableProspects methods"
```

---

## Task 4: Transaction Fetch + Classification

**Files:**
- Create: `internal/prospects/transactions.go`
- Create: `internal/prospects/transactions_test.go`

- [ ] **Step 1: Write the transaction tests**

```go
package prospects

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchTransactionAlerts_CalledUpOnMyTeam(t *testing.T) {
	fixture := map[string]interface{}{
		"transactions": []map[string]interface{}{
			{
				"person":         map[string]interface{}{"fullName": "Jackson Chourio"},
				"toTeam":         map[string]interface{}{"abbreviation": "MIL"},
				"fromTeam":       map[string]interface{}{"abbreviation": "MIL"},
				"typeCode":       "CU",
				"date":           "2026-03-22T00:00:00",
				"description":    "Called up",
				"transactionType": "Call Up",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	origURL := mlbTransactionsURL
	mlbTransactionsURL = srv.URL + "?startDate=%s&endDate=%s"
	defer func() { mlbTransactionsURL = origURL }()

	myMinors := map[string]bool{"jackson chourio": true}
	rankings := map[string]int{"jackson chourio": 12}

	from := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)

	alerts, err := FetchTransactionAlerts(from, to, myMinors, rankings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Kind != CalledUp {
		t.Errorf("expected CalledUp, got %s", alerts[0].Kind)
	}
	if !alerts[0].OnMyTeam {
		t.Error("expected OnMyTeam=true")
	}
	if alerts[0].Priority != "high" {
		t.Errorf("expected high priority, got %s", alerts[0].Priority)
	}
}

func TestFetchTransactionAlerts_FreeAgentBuzz(t *testing.T) {
	fixture := map[string]interface{}{
		"transactions": []map[string]interface{}{
			{
				"person":         map[string]interface{}{"fullName": "Jasson Dominguez"},
				"toTeam":         map[string]interface{}{"abbreviation": "NYY"},
				"fromTeam":       map[string]interface{}{"abbreviation": "NYY"},
				"typeCode":       "CU",
				"date":           "2026-03-22T00:00:00",
				"description":    "Called up",
				"transactionType": "Call Up",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	origURL := mlbTransactionsURL
	mlbTransactionsURL = srv.URL + "?startDate=%s&endDate=%s"
	defer func() { mlbTransactionsURL = origURL }()

	myMinors := map[string]bool{} // not on my team
	rankings := map[string]int{"jasson dominguez": 8}

	alerts, err := FetchTransactionAlerts(
		time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		myMinors, rankings,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Kind != FreeAgentBuzz {
		t.Errorf("expected FreeAgentBuzz, got %s", alerts[0].Kind)
	}
	if alerts[0].Rank != 8 {
		t.Errorf("expected rank 8, got %d", alerts[0].Rank)
	}
}

func TestFetchTransactionAlerts_OptionedLowPriority(t *testing.T) {
	fixture := map[string]interface{}{
		"transactions": []map[string]interface{}{
			{
				"person":         map[string]interface{}{"fullName": "Spencer Torkelson"},
				"toTeam":         map[string]interface{}{"abbreviation": "DET"},
				"fromTeam":       map[string]interface{}{"abbreviation": "DET"},
				"typeCode":       "OPT",
				"date":           "2026-03-22T00:00:00",
				"description":    "Optioned",
				"transactionType": "Optioned",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	origURL := mlbTransactionsURL
	mlbTransactionsURL = srv.URL + "?startDate=%s&endDate=%s"
	defer func() { mlbTransactionsURL = origURL }()

	myMinors := map[string]bool{"spencer torkelson": true}

	alerts, err := FetchTransactionAlerts(
		time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		myMinors, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Priority != "low" {
		t.Errorf("expected low priority, got %s", alerts[0].Priority)
	}
}

func TestFetchTransactionAlerts_EmptyResponse(t *testing.T) {
	fixture := map[string]interface{}{"transactions": []interface{}{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	origURL := mlbTransactionsURL
	mlbTransactionsURL = srv.URL + "?startDate=%s&endDate=%s"
	defer func() { mlbTransactionsURL = origURL }()

	alerts, err := FetchTransactionAlerts(
		time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run TestFetchTransaction -v`
Expected: FAIL — `FetchTransactionAlerts` not defined

- [ ] **Step 3: Write the implementation**

```go
package prospects

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nixon-commits/fantrax-optimizer/internal/projections"
)

var mlbTransactionsURL = "https://statsapi.mlb.com/api/v1/transactions?startDate=%s&endDate=%s"

// FetchTransactionAlerts fetches MLB transactions and cross-references against
// your Minors roster and ranked prospects to produce alerts.
// myMinors: normalized name → true for players on your Minors roster.
// rankings: normalized name → rank (1-100) for ranked prospects.
func FetchTransactionAlerts(
	from, to time.Time,
	myMinors map[string]bool,
	rankings map[string]int,
) ([]ProspectAlert, error) {
	url := fmt.Sprintf(mlbTransactionsURL, from.Format("2006-01-02"), to.Format("2006-01-02"))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("mlb transactions fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mlb transactions: status %d", resp.StatusCode)
	}

	var payload struct {
		Transactions []struct {
			Person struct {
				FullName string `json:"fullName"`
			} `json:"person"`
			ToTeam struct {
				Abbreviation string `json:"abbreviation"`
			} `json:"toTeam"`
			TypeCode string `json:"typeCode"`
			Date     string `json:"date"`
		} `json:"transactions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("mlb transactions decode: %w", err)
	}

	var alerts []ProspectAlert
	for _, txn := range payload.Transactions {
		name := projections.NormalizeName(txn.Person.FullName)
		team := projections.NormalizeTeam(txn.ToTeam.Abbreviation)
		rank := rankings[name]

		switch txn.TypeCode {
		case "CU", "RET": // Called up / Recalled
			if myMinors[name] {
				alerts = append(alerts, ProspectAlert{
					Kind:       CalledUp,
					Priority:   "high",
					PlayerName: txn.Person.FullName,
					MLBTeam:    team,
					Detail:     "Called up — move from Minors slot",
					OnMyTeam:   true,
					Rank:       rank,
				})
			} else if rank > 0 {
				alerts = append(alerts, ProspectAlert{
					Kind:       FreeAgentBuzz,
					Priority:   "high",
					PlayerName: txn.Person.FullName,
					MLBTeam:    team,
					Detail:     fmt.Sprintf("#%d prospect called up — available in your league?", rank),
					Rank:       rank,
				})
			}

		case "OPT", "DFA": // Optioned / DFA
			if myMinors[name] {
				alerts = append(alerts, ProspectAlert{
					Kind:       Optioned,
					Priority:   "low",
					PlayerName: txn.Person.FullName,
					MLBTeam:    team,
					Detail:     fmt.Sprintf("Optioned/DFA (%s)", txn.TypeCode),
					OnMyTeam:   true,
					Rank:       rank,
				})
			}
		}
	}

	return alerts, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run TestFetchTransaction -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/prospects/transactions.go internal/prospects/transactions_test.go
git commit -m "feat(prospects): MLB transaction alerts with call-up and FA buzz detection"
```

---

## Task 5: Rankings — MLB Pipeline Source + FanGraphs Fallback + Cache

**Files:**
- Create: `internal/prospects/rankings.go`
- Create: `internal/prospects/rankings_test.go`

- [ ] **Step 1: Write ranking source tests**

Key tests to include:
- `TestMLBPipelineSource_ParsesResponse` — mock MLB API, verify parsed `RankedProspect` fields
- `TestFanGraphsRankingSource_Returns403` — verify error on 403
- `TestChainedRankingSource_FallsThrough` — pipeline fails → FG succeeds
- `TestLoadRankings_UsesCacheWhenFresh` — no HTTP calls when cache is recent
- `TestLoadRankings_FetchesWhenStale` — HTTP call when cache expired
- `TestFindUpgrades_TieredThreshold` — verify tiered rank gap logic
- `TestFindUpgrades_UnrankedAlwaysReplaceable` — unranked (rank=0) replaced by any ranked FA
- `TestFindUpgrades_NearTermETA` — ETA tagging for current season
- `TestFindUpgrades_DeduplicatesRostered` — each rostered player at most one recommendation
- `TestUpgradeThreshold_AllBuckets` — verify all tier boundaries

Each test follows the inline fixture + `httptest.NewServer` + `var` URL swap pattern (for HTTP tests) or pure struct construction (for pure function tests).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run "TestMLBPipeline|TestFanGraphs|TestChained|TestLoadRankings|TestFindUpgrades|TestUpgradeThreshold" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write the implementation**

`rankings.go` contains:

1. **`MLBPipelineSource`** implementing `RankingSource`:
   - `var mlbPipelineURL = "https://statsapi.mlb.com/api/v1/prospects?season=%d&topN=100"`
   - Parses inline anonymous struct response
   - Uses `projections.NormalizeName()` and `projections.NormalizeTeam()`

2. **`FanGraphsRankingSource`** implementing `RankingSource`:
   - `var fgProspectURL = "https://www.fangraphs.com/api/prospects/board/prospect-list?type=prospects&pos=all"`
   - Returns descriptive error for 401/403
   - `var ErrSourceUnavailable = errors.New("ranking source unavailable")`

3. **`ChainedRankingSource`** composing multiple `RankingSource`s:
   - Tries each in order, falls through on `ErrSourceUnavailable`
   - Same pattern as `projections.ChainedSource`

4. **Cache helpers** (unexported):
   - `rankingsCacheFile = ".prospects-cache/rankings.json"`
   - `type rankingsCache struct { FetchedAt time.Time; Prospects []RankedProspect }`
   - `loadRankingsCache(maxAge)` / `saveRankingsCache(prospects)`

5. **`LoadRankings(season, cacheHours int) ([]RankedProspect, error)`**:
   - Check cache → if fresh, return cached
   - Else fetch via `ChainedRankingSource(MLBPipeline, FanGraphs)`
   - Save to cache on success

6. **`FindUpgrades(rostered, available []RankedProspect, currentYear string) []UpgradeCandidate`**:
   - Pure function, no I/O
   - `upgradeThreshold(rank int) int` — tiered: top10→5, 11-50→15, 51-100→25, unranked→1
   - FV-based comparison when both have FV > 0 (gap ≥ 5 points = significant)
   - Deduplicates: each rostered player appears at most once (paired with best FA)
   - Sorts by rank gap descending
   - Tags `NearTerm = true` when Add.ETA == currentYear

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run "TestMLBPipeline|TestFanGraphs|TestChained|TestLoadRankings|TestFindUpgrades|TestUpgradeThreshold" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/prospects/rankings.go internal/prospects/rankings_test.go
git commit -m "feat(prospects): MLB Pipeline rankings, FG fallback, cache, and upgrade engine"
```

---

## Task 6: Performance Monitoring — Game Logs + Breakout Detection

**Files:**
- Create: `internal/prospects/performance.go`
- Create: `internal/prospects/performance_test.go`

- [ ] **Step 1: Write performance tests**

Key tests:
- `TestComputeHitterBreakout_HotAAA` — OPS delta > 0.150 at AAA → hot
- `TestComputeHitterBreakout_MinGameFilter` — < 8 games → nil result
- `TestComputeHitterBreakout_LevelThresholds` — AA needs +0.200, A-ball needs +0.250
- `TestComputePitcherBreakout_HotERA` — ERA drop > 1.00 at AAA → hot
- `TestComputePitcherBreakout_HotK9` — K/9 rise > 2.0 at AAA → hot
- `TestComputeHitterBreakout_ColdTop50Only` — cold streak at rank 51 → nil; at rank 12 → alert
- `TestResolveMLBPlayerID_CacheLookup` — cached ID returned without HTTP
- `TestResolveMLBPlayerID_SearchAPI` — mock search, verify ID stored to cache
- `TestFetchPerformanceAlerts_Integration` — mock all endpoints, verify alert generation

All pure function tests (`computeHitter/PitcherBreakout`) use hand-crafted `gameLogEntry` slices — no httptest needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run "TestCompute|TestResolve|TestFetchPerformance" -v`
Expected: FAIL

- [ ] **Step 3: Write the implementation**

`performance.go` contains:

1. **URL vars**:
   ```go
   var mlbPlayerSearchURL = "https://statsapi.mlb.com/api/v1/people/search?names=%s&sportIds=11,12,13,14,1"
   var mlbGameLogURL = "https://statsapi.mlb.com/api/v1/people/%d/stats?stats=gameLog&group=%s&season=%d&sportId=11,12,13,14"
   ```

2. **Player ID cache** (`.prospects-cache/player-ids.json`):
   - `map[string]int` keyed by `"normalized_name|team"` (same format as `projections.projKey`)
   - `loadPlayerIDCache()` / `savePlayerIDCache(cache)`
   - Manually editable JSON file

3. **`resolveMLBPlayerID(name, team string, cache map[string]int) (int, bool)`**:
   - Check cache first
   - Hit search API, score candidates by name + team match
   - Log loudly on miss: `"WARNING: no MLB ID found for %q (%s) — skipping"`
   - Store to cache on success

4. **Game log types**:
   ```go
   type gameLogEntry struct {
       Date  string
       Level string // "AAA", "AA", "A+", "A"
       // Hitter fields
       AB, H, Doubles, Triples, HR, BB, HBP, SF int
       // Pitcher fields
       IP            float64
       ER, SO, BBA int // BBA to avoid name collision with hitter BB
       HA          int // hits allowed
   }
   ```

5. **Level-adjusted thresholds**:
   ```go
   var opsHotThreshold = map[string]float64{"AAA": 0.150, "AA": 0.200, "A+": 0.250, "A": 0.250}
   var opsColdThreshold = -0.200 // uniform
   var eraHotThreshold = map[string]float64{"AAA": -1.00, "AA": -1.50, "A+": -2.00, "A": -2.00}
   var k9HotThreshold = map[string]float64{"AAA": 2.0, "AA": 2.5, "A+": 3.0, "A": 3.0}
   ```

6. **`computeHitterBreakout(logs []gameLogEntry, minGames int, level string) (hot, cold bool, recentLine, seasonLine string)`**
7. **`computePitcherBreakout(logs []gameLogEntry, minGames int, level string) (hot, cold bool, recentLine, seasonLine string)`**

8. **`FetchPerformanceAlerts(prospects []fantrax.Player, rankings map[string]int, season, rollingDays, minGames int) ([]ProspectAlert, error)`**:
   - Load player ID cache
   - `errgroup` with semaphore (buffered chan, cap=5) for parallel per-player fetches
   - For each prospect: resolve MLB ID → fetch game logs → compute breakout → append alerts
   - Cold alerts only for rank > 0 && rank <= 50
   - Save updated player ID cache

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run "TestCompute|TestResolve|TestFetchPerformance" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/prospects/performance.go internal/prospects/performance_test.go
git commit -m "feat(prospects): MiLB breakout detection with level-adjusted thresholds"
```

---

## Task 7: Orchestrator + Report Formatting

**Files:**
- Create: `internal/prospects/run.go`
- Create: `internal/prospects/run_test.go`

- [ ] **Step 1: Write orchestrator tests**

Key tests:
- `TestRunProspectReport_DegradedNoRankings` — all ranking sources fail, report still generates with transaction alerts only
- `TestFormatReport_Markdown` — verify GHA markdown table output contains expected sections

These are integration-level tests. Use interface stubs or mock the `var` URLs.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -run "TestRun|TestFormat" -v`
Expected: FAIL

- [ ] **Step 3: Write the implementation**

`run.go` contains:

1. **Transaction cursor** (inline, ~20 lines):
   ```go
   var txnCursorFile = ".prospects-cache/last-transactions.json"
   func loadTxnCursor() time.Time    // returns zero → use default lookback
   func saveTxnCursor(date time.Time) error
   ```

2. **`RunProspectReport(ft *fantrax.Client, cfg config.Config, today time.Time) error`**:
   - `os.MkdirAll(".prospects-cache", 0755)`
   - `ft.GetMinorsRoster()` → build `myMinors` name map
   - Parallel via `errgroup` (2 goroutines):
     - `LoadRankings(season, cfg.ProspectRankCacheHours)`
     - `ft.GetAvailableProspects()` (non-fatal on error)
   - After rankings loaded (sequential — needs rankingsMap):
     - `FetchTransactionAlerts(cursorDate, today, myMinors, rankingsMap)`
     - `FetchPerformanceAlerts(minorsRoster, rankingsMap, ...)`
   - `FindUpgrades(myRanked, faRanked, currentYear)`
   - Build `Report`, sort alerts by priority
   - Print to stdout
   - Write GHA summary (if `$GITHUB_STEP_SUMMARY` is set)
   - Advance transaction cursor

3. **`printReport(r Report)`** — stdout format:
   ```
   === Prospect Report ===
   [HIGH]  CALLED UP     Jackson Chourio (MIL)   move from Minors slot
   [HIGH]  FA BUZZ       Jasson Dominguez (NYY)  #8 — available in league?
   [MED]   HOT STREAK    Colton Cowser (BAL)     AAA last 14d: .342/.401/.658
   ---
   Upgrade: Drop Tyler Black (#92) → Add Ethan Salas (#18) [+74 spots, ETA 2026]
   ```

4. **`writeGHASummary(r Report, path string)`** — GHA markdown tables matching the format in `docs/PROSPECT_SYSTEM_PLAN.md`. Opens file with `os.O_APPEND|os.O_WRONLY|os.O_CREATE` so multiple sections accumulate.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jnixon/fantrax && go test ./internal/prospects/... -v`
Expected: all PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/jnixon/fantrax && go test ./internal/... -v`
Expected: all PASS (no regressions)

- [ ] **Step 6: Commit**

```bash
git add internal/prospects/run.go internal/prospects/run_test.go
git commit -m "feat(prospects): orchestrator with parallel fetching and GHA summary output"
```

---

## Task 8: Wire into cmd/main.go

**Files:**
- Modify: `cmd/main.go:20-24` (add flag)
- Modify: `cmd/main.go:67-84` (add prospect report call)

- [ ] **Step 1: Add the --prospects flag**

Add after line 23 (`checkRoster` flag):

```go
	runProspects := flag.Bool("prospects", false, "run minor league prospect report")
```

- [ ] **Step 2: Add the prospect report call**

Add after the roster alerts block (after line 84, before `// --- Fetch hitter roster`):

```go
	// --- Prospect report (if requested) ---
	if *runProspects {
		if err := prospects.RunProspectReport(ft, *cfg, today); err != nil {
			log.Printf("WARNING: prospect report failed (%v)", err)
		}
	}
```

- [ ] **Step 3: Add the import**

Add to the import block:

```go
	"github.com/nixon-commits/fantrax-optimizer/internal/prospects"
```

- [ ] **Step 4: Verify it compiles and all tests pass**

Run: `cd /Users/jnixon/fantrax && go build ./... && go test ./internal/... -v`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add cmd/main.go
git commit -m "feat: wire --prospects flag into main entry point"
```

---

## Task 9: GHA Workflow + Gitignore

**Files:**
- Modify: `.github/workflows/lineup.yml:23`
- Modify: `.gitignore`

- [ ] **Step 1: Add --prospects to GHA workflow**

Change line 23 from:
```yaml
        run: go run ./cmd
```
To:
```yaml
        run: go run ./cmd --prospects
```

- [ ] **Step 2: Add .prospects-cache/ to gitignore**

Append to `.gitignore`:
```
.prospects-cache/
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/lineup.yml .gitignore
git commit -m "chore: enable prospect report in GHA workflow and gitignore cache"
```

---

## Task 10: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add prospect commands to Commands section**

Add after the existing commands:
```
go run ./cmd --prospects --dry-run  # run prospect report locally
```

- [ ] **Step 2: Add internal/prospects to Architecture section**

Add after the optimizer description:
```
**`internal/prospects`** — monitors minor league prospects across MLB transactions,
MiLB performance breakouts, and prospect ranking sources (MLB Pipeline primary,
FanGraphs fallback). Produces a daily prospect report in the GHA job summary with
call-up alerts, hot streak detection, free agent watch, and upgrade recommendations.
Separate from roster alerts (which detect slot mismatches); this focuses on external
data to find new players to pick up.
```

- [ ] **Step 3: Add new env vars to Configuration section**

Document the optional prospect env vars: `PROSPECT_ROLLING_DAYS`, `PROSPECT_MIN_GAMES`, `PROSPECT_RANK_CACHE_HOURS`, `PROSPECT_UPGRADE_RANK_THRESHOLD`.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add prospect system to CLAUDE.md"
```

---

## Task 11: End-to-End Verification

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jnixon/fantrax && go test ./internal/... -v`
Expected: all PASS

- [ ] **Step 2: Run build**

Run: `cd /Users/jnixon/fantrax && go build ./...`
Expected: no errors

- [ ] **Step 3: Dry-run with prospects**

Run: `cd /Users/jnixon/fantrax && go run ./cmd --dry-run --prospects`
Expected: prospect report section appears in output (may show warnings if no Fantrax credentials configured — that's OK)

- [ ] **Step 4: Verify idempotency**

Run the same command again. The transaction cursor should prevent re-alerting. Second run should show the same performance alerts but no duplicate transaction alerts.
