# Blended Projections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Blend FanGraphs Steamer projections (60%) with recent Fantrax scoring data (40%) to account for hot streaks and current form.

**Architecture:** New `BlendedSource` wraps existing projection sources and augments them with a pre-computed FP/G from the last 10 Fantrax scoring periods. The optimizer uses a type assertion to check for the blended value before falling back to `expectedPts`. Recent period data is fetched in parallel via `errgroup`.

**Tech Stack:** Go, `golang.org/x/sync/errgroup`, `github.com/pmurley/go-fantrax` (auth_client)

**Spec:** `docs/superpowers/specs/2026-03-22-blended-projections-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/fantrax/recent_stats.go` | Create | Parallel fetch of last N period rosters, aggregate FP/GP per player |
| `internal/fantrax/recent_stats_test.go` | Create | Unit tests for aggregation logic |
| `internal/projections/blended.go` | Create | `BlendedSource` + `PtsPerGameSource` interface, blending logic |
| `internal/projections/blended_test.go` | Create | Unit tests for blend formula and edge cases |
| `internal/optimizer/lineup.go` | Modify | `scoreRoster` checks `PtsPerGameSource` type assertion first |
| `internal/optimizer/lineup_test.go` | Modify | Add test for blended source integration |
| `cmd/main.go` | Modify | Wire up period discovery, recent stats fetch, and `BlendedSource` |

---

### Task 0: Verify per-period stats are not cumulative

Before any code, we need to confirm that calling `GetTeamRosterInfo` with a specific period number returns stats for that single period, not season-to-date. This is a design-breaking assumption.

**Files:**
- None (manual verification)

- [ ] **Step 1: Write a small test program to fetch two different periods and compare**

Create a temporary file `cmd/verify_periods/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/pmurley/go-fantrax/auth_client"
)

func main() {
	_ = godotenv.Load()
	leagueID := os.Getenv("FANTRAX_LEAGUE_ID")
	teamID := os.Getenv("FANTRAX_TEAM_ID")

	client, err := auth_client.NewClient(leagueID, false)
	if err != nil {
		log.Fatal(err)
	}

	period, err := client.GetCurrentPeriod()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Current period: %d\n", period)

	// Fetch two consecutive historical periods
	for _, p := range []int{period - 1, period - 2} {
		roster, err := client.GetTeamRosterInfo(fmt.Sprintf("%d", p), teamID)
		if err != nil {
			log.Printf("Period %d error: %v", p, err)
			continue
		}
		fmt.Printf("\n=== Period %d ===\n", p)
		for _, player := range roster.ActiveRoster {
			if player.Stats != nil && player.Stats.Batting != nil {
				fpg := "nil"
				gp := "nil"
				if player.Stats.Batting.FantasyPointsPerGame != nil {
					fpg = fmt.Sprintf("%.2f", *player.Stats.Batting.FantasyPointsPerGame)
				}
				if player.Stats.Batting.GamesPlayed != nil {
					gp = fmt.Sprintf("%d", *player.Stats.Batting.GamesPlayed)
				}
				fmt.Printf("  %-25s FP/G: %s  GP: %s\n", player.Name, fpg, gp)
			}
		}
	}
}
```

- [ ] **Step 2: Run it and inspect the output**

Run: `go run ./cmd/verify_periods/`

Check: If period N-1 and N-2 show different FP/G values and GP=0 or GP=1, it's per-period. If GP is cumulative (e.g., 50, 51), the design needs to change.

- [ ] **Step 3: Record findings and delete the temp file**

```bash
rm -rf cmd/verify_periods/
```

If cumulative: STOP — the aggregation formula must be redesigned (diff consecutive periods).
If per-period: continue to Task 1.

---

### Task 1: `RecentStat` type and aggregation function

Build the core aggregation logic with no I/O — pure function that takes roster data and produces stats.

**Files:**
- Create: `internal/fantrax/recent_stats.go`
- Create: `internal/fantrax/recent_stats_test.go`

- [ ] **Step 1: Write the failing test for aggregation**

`internal/fantrax/recent_stats_test.go`:

```go
package fantrax

import (
	"testing"

	"github.com/pmurley/go-fantrax/models"
)

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }

func TestAggregateRecentStats(t *testing.T) {
	// Two periods of data for the same player
	periods := [][]models.RosterPlayer{
		{
			{
				PlayerID: "abc",
				Name:     "Test Player",
				Stats: &models.PlayerStats{
					Batting: &models.BattingStats{
						FantasyPointsPerGame: ptrFloat(8.5),
						GamesPlayed:          ptrInt(1),
					},
				},
			},
		},
		{
			{
				PlayerID: "abc",
				Name:     "Test Player",
				Stats: &models.PlayerStats{
					Batting: &models.BattingStats{
						FantasyPointsPerGame: ptrFloat(3.0),
						GamesPlayed:          ptrInt(1),
					},
				},
			},
		},
	}

	result := aggregateRecentStats(periods)

	stat, ok := result["abc"]
	if !ok {
		t.Fatal("expected player abc in results")
	}
	if stat.GamesPlayed != 2 {
		t.Errorf("expected 2 games, got %d", stat.GamesPlayed)
	}
	// Total FP = 8.5 + 3.0 = 11.5
	if stat.TotalFP != 11.5 {
		t.Errorf("expected 11.5 total FP, got %.2f", stat.TotalFP)
	}
}

func TestAggregateRecentStats_NilStats(t *testing.T) {
	periods := [][]models.RosterPlayer{
		{
			{
				PlayerID: "xyz",
				Name:     "No Game Player",
				Stats: &models.PlayerStats{
					Batting: &models.BattingStats{
						FantasyPointsPerGame: nil,
						GamesPlayed:          ptrInt(0),
					},
				},
			},
		},
	}

	result := aggregateRecentStats(periods)

	stat, ok := result["xyz"]
	if !ok {
		t.Fatal("expected player xyz in results")
	}
	if stat.GamesPlayed != 0 {
		t.Errorf("expected 0 games, got %d", stat.GamesPlayed)
	}
	if stat.TotalFP != 0 {
		t.Errorf("expected 0 total FP, got %.2f", stat.TotalFP)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/fantrax/ -run TestAggregateRecentStats -v`
Expected: FAIL — `aggregateRecentStats` undefined

- [ ] **Step 3: Write minimal implementation**

`internal/fantrax/recent_stats.go`:

```go
package fantrax

import (
	"github.com/pmurley/go-fantrax/models"
)

// RecentStat holds aggregated recent performance for a player.
type RecentStat struct {
	TotalFP     float64
	GamesPlayed int
}

// aggregateRecentStats combines roster data from multiple periods into
// per-player totals. Each period is a slice of RosterPlayers.
func aggregateRecentStats(periods [][]models.RosterPlayer) map[string]RecentStat {
	stats := make(map[string]RecentStat)
	for _, roster := range periods {
		for _, rp := range roster {
			if rp.Stats == nil || rp.Stats.Batting == nil {
				continue
			}
			s := stats[rp.PlayerID]
			if rp.Stats.Batting.GamesPlayed != nil {
				s.GamesPlayed += *rp.Stats.Batting.GamesPlayed
			}
			if rp.Stats.Batting.FantasyPointsPerGame != nil && rp.Stats.Batting.GamesPlayed != nil && *rp.Stats.Batting.GamesPlayed > 0 {
				s.TotalFP += *rp.Stats.Batting.FantasyPointsPerGame * float64(*rp.Stats.Batting.GamesPlayed)
			}
			stats[rp.PlayerID] = s
		}
	}
	return stats
}
```

Note: For a single-day period, `GamesPlayed` is 0 or 1, and `FantasyPointsPerGame` when GP=1 equals total FP. We multiply FP/G × GP to get total FP for that period, which handles the general case.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/fantrax/ -run TestAggregateRecentStats -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/fantrax/recent_stats.go internal/fantrax/recent_stats_test.go
git commit -m "feat: add RecentStat type and aggregation function for period stats"
```

---

### Task 2: `GetRecentStats` method with parallel fetching

Add the method on `*Client` that calls the auth client for each period in parallel.

**Files:**
- Modify: `internal/fantrax/recent_stats.go`

- [ ] **Step 1: Add `GetRecentStats` and `GetCurrentPeriod` methods**

Append to `internal/fantrax/recent_stats.go`:

```go
import (
	"fmt"
	"log"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"
)

// GetCurrentPeriod returns the current Fantrax scoring period number.
func (c *Client) GetCurrentPeriod() (int, error) {
	return c.auth.GetCurrentPeriod()
}

// GetRecentStats fetches roster data for the last numPeriods scoring periods
// (counting back from currentPeriod-1) and aggregates per-player stats.
// Periods are fetched in parallel. Partial failures are logged and skipped.
func (c *Client) GetRecentStats(currentPeriod, numPeriods int) (map[string]RecentStat, error) {
	// Determine which periods to fetch.
	var periodNums []int
	for i := 1; i <= numPeriods; i++ {
		p := currentPeriod - i
		if p <= 0 {
			break
		}
		periodNums = append(periodNums, p)
	}

	if len(periodNums) == 0 {
		return make(map[string]RecentStat), nil
	}

	type periodResult struct {
		players []models.RosterPlayer
	}

	results := make([]periodResult, len(periodNums))
	var mu sync.Mutex
	var warnings []string

	g := new(errgroup.Group)
	for idx, p := range periodNums {
		idx, p := idx, p
		g.Go(func() error {
			roster, err := c.auth.GetTeamRosterInfo(strconv.Itoa(p), c.teamID)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("period %d: %v", p, err))
				mu.Unlock()
				return nil // don't fail the whole group
			}
			var allPlayers []models.RosterPlayer
			allPlayers = append(allPlayers, roster.ActiveRoster...)
			allPlayers = append(allPlayers, roster.ReserveRoster...)
			results[idx] = periodResult{players: allPlayers}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	for _, w := range warnings {
		log.Printf("WARNING: recent stats: %s", w)
	}

	var periods [][]models.RosterPlayer
	for _, r := range results {
		if r.players != nil {
			periods = append(periods, r.players)
		}
	}

	return aggregateRecentStats(periods), nil
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/fantrax/recent_stats.go go.mod go.sum
git commit -m "feat: add GetRecentStats with parallel period fetching"
```

---

### Task 3: `PtsPerGameSource` interface and `BlendedSource`

**Files:**
- Create: `internal/projections/blended.go`
- Create: `internal/projections/blended_test.go`

- [ ] **Step 1: Write the failing test for BlendedSource**

`internal/projections/blended_test.go`:

```go
package projections

import (
	"testing"

	"github.com/nixon-commits/fantrax-optimizer/internal/fantrax"
)

type stubSource struct {
	proj map[string]*Projection
}

func (s *stubSource) GetProjection(name, mlbTeam string) (*Projection, bool) {
	p, ok := s.proj[NormalizeName(name)]
	return p, ok
}

func TestBlendedSource_WithRecentStats(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{
		"test player": {G: 100, H: 100, HR: 20, RBI: 60, R: 50, BB: 40},
	}}
	scoring := fantrax.ScoringWeights{"HR": 4.0, "RBI": 1.0}

	// Steamer: (20/100)*4 + (60/100)*1 = 0.8 + 0.6 = 1.4 pts/g
	// Recent: 10 FP / 5 GP = 2.0 FP/G
	// Blended: 0.6*1.4 + 0.4*2.0 = 0.84 + 0.8 = 1.64

	recent := map[string]fantrax.RecentStat{
		"player1": {TotalFP: 10.0, GamesPlayed: 5},
	}
	nameToID := map[string]string{
		"test player": "player1",
	}

	src := NewBlendedSource(inner, recent, scoring, nameToID)

	pts, ok := src.GetPtsPerGame("Test Player", "NYY", scoring)
	if !ok {
		t.Fatal("expected GetPtsPerGame to return true")
	}
	// Allow small float tolerance
	if pts < 1.63 || pts > 1.65 {
		t.Errorf("expected ~1.64, got %.4f", pts)
	}
}

func TestBlendedSource_NoRecentStats_FallsBackToSteamer(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{
		"test player": {G: 100, HR: 20},
	}}
	scoring := fantrax.ScoringWeights{"HR": 4.0}

	// Steamer: (20/100)*4 = 0.8
	// No recent data → 100% steamer = 0.8

	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, scoring, map[string]string{
		"test player": "player1",
	})

	pts, ok := src.GetPtsPerGame("Test Player", "NYY", scoring)
	if !ok {
		t.Fatal("expected GetPtsPerGame to return true")
	}
	if pts < 0.79 || pts > 0.81 {
		t.Errorf("expected ~0.8, got %.4f", pts)
	}
}

func TestBlendedSource_NoSteamer_ReturnsFalse(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{}}
	scoring := fantrax.ScoringWeights{"HR": 4.0}

	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, scoring, map[string]string{})

	_, ok := src.GetPtsPerGame("Unknown Player", "NYY", scoring)
	if ok {
		t.Error("expected false for unknown player")
	}
}

func TestBlendedSource_GetProjection_Delegates(t *testing.T) {
	proj := &Projection{G: 100, HR: 20}
	inner := &stubSource{proj: map[string]*Projection{"test player": proj}}

	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, nil, nil)

	p, ok := src.GetProjection("Test Player", "NYY")
	if !ok {
		t.Fatal("expected projection found")
	}
	if p.HR != 20 {
		t.Errorf("expected HR=20, got %.0f", p.HR)
	}
}
```

- [ ] **Step 2: Export `NormalizeName`**

Before writing `BlendedSource`, rename `normalizeName` to `NormalizeName` in `internal/projections/fangraphs.go` so it can be used by `cmd/main.go` later. Update all callers:
- `fangraphs.go`: `projKey`, `GetProjection`
- `rolling.go`: `GetProjection`

Run: `go build ./...` to verify.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/projections/ -run TestBlendedSource -v`
Expected: FAIL — `NewBlendedSource` undefined

- [ ] **Step 4: Write the implementation**

`internal/projections/blended.go`:

```go
package projections

import (
	"github.com/nixon-commits/fantrax-optimizer/internal/fantrax"
)

const (
	steamerWeight = 0.60
	recentWeight  = 0.40
)

// PtsPerGameSource can provide a pre-computed points-per-game value,
// bypassing the stat-level projection → expectedPts calculation.
type PtsPerGameSource interface {
	GetPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool)
}

// BlendedSource wraps a projection source and blends its per-game value
// with recent Fantrax scoring data.
type BlendedSource struct {
	inner      Source
	recent     map[string]fantrax.RecentStat // keyed by Fantrax player ID
	scoring    fantrax.ScoringWeights
	nameToID   map[string]string // normalizeName(name) → player ID
}

// NewBlendedSource creates a source that blends Steamer projections with recent stats.
func NewBlendedSource(
	inner Source,
	recent map[string]fantrax.RecentStat,
	scoring fantrax.ScoringWeights,
	nameToID map[string]string,
) *BlendedSource {
	return &BlendedSource{
		inner:    inner,
		recent:   recent,
		scoring:  scoring,
		nameToID: nameToID,
	}
}

// GetProjection delegates to the inner source.
func (b *BlendedSource) GetProjection(name, mlbTeam string) (*Projection, bool) {
	return b.inner.GetProjection(name, mlbTeam)
}

// GetPtsPerGame returns the blended FP/G: 60% Steamer + 40% recent.
// Falls back to 100% Steamer if no recent data exists.
// Returns false if the player has no Steamer projection.
func (b *BlendedSource) GetPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool) {
	proj, ok := b.inner.GetProjection(name, mlbTeam)
	if !ok || proj.G <= 0 {
		return 0, false
	}

	steamerPts := expectedPtsFromProj(proj, scoring)

	// Look up recent stats by player ID.
	playerID, idOK := b.nameToID[NormalizeName(name)]
	if !idOK {
		return steamerPts, true
	}

	recent, statOK := b.recent[playerID]
	if !statOK || recent.GamesPlayed == 0 {
		return steamerPts, true
	}

	recentPtsPerGame := recent.TotalFP / float64(recent.GamesPlayed)
	blended := steamerWeight*steamerPts + recentWeight*recentPtsPerGame
	return blended, true
}

// expectedPtsFromProj computes per-game fantasy points from a projection.
// This duplicates the logic from optimizer.expectedPts to avoid a circular import.
func expectedPtsFromProj(proj *Projection, scoring fantrax.ScoringWeights) float64 {
	if proj.G <= 0 {
		return 0
	}
	singles := proj.Singles
	if singles == 0 && proj.H > 0 {
		singles = proj.H - proj.Doubles - proj.Triples - proj.HR
	}
	xbh := proj.Doubles + proj.Triples + proj.HR
	tb := singles + 2*proj.Doubles + 3*proj.Triples + 4*proj.HR

	statMap := map[string]float64{
		"1B": singles, "2B": proj.Doubles, "3B": proj.Triples,
		"HR": proj.HR, "RBI": proj.RBI, "R": proj.R,
		"BB": proj.BB, "SB": proj.SB, "CS": proj.CS,
		"HBP": proj.HBP, "SO": proj.SO, "GIDP": proj.GIDP,
		"XBH": xbh, "TB": tb,
	}

	var total float64
	for stat, seasonVal := range statMap {
		if pts, ok := scoring[stat]; ok {
			total += (seasonVal / proj.G) * pts
		}
	}
	return total
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/projections/ -run TestBlendedSource -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/projections/blended.go internal/projections/blended_test.go internal/projections/fangraphs.go internal/projections/rolling.go
git commit -m "feat: add BlendedSource with PtsPerGameSource interface, export NormalizeName"
```

---

### Task 4: Wire `PtsPerGameSource` into the optimizer

**Files:**
- Modify: `internal/optimizer/lineup.go` (lines 178-199, `scoreRoster`)
- Modify: `internal/optimizer/lineup_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/optimizer/lineup_test.go`:

```go
// stubBlendedSource implements both Source and PtsPerGameSource.
type stubBlendedSource struct {
	projData map[string]*projections.Projection
	ptsData  map[string]float64 // name → pts/g
}

func (s *stubBlendedSource) GetProjection(name, _ string) (*projections.Projection, bool) {
	p, ok := s.projData[name]
	return p, ok
}

func (s *stubBlendedSource) GetPtsPerGame(name, _ string, _ fantrax.ScoringWeights) (float64, bool) {
	pts, ok := s.ptsData[name]
	return pts, ok
}

func TestOptimizeLineup_UsesBlendedPtsPerGame(t *testing.T) {
	// BlendedSource provides pre-computed pts that differ from expectedPts.
	src := &stubBlendedSource{
		projData: map[string]*projections.Projection{
			"Player A": {G: 100, HR: 10}, // expectedPts with HR=4 would be 0.4
			"Player B": {G: 100, HR: 20}, // expectedPts with HR=4 would be 0.8
		},
		ptsData: map[string]float64{
			"Player A": 5.0, // blended overrides to 5.0
			"Player B": 3.0, // blended overrides to 3.0
		},
	}

	scoring := fantrax.ScoringWeights{"HR": 4.0}

	roster := []fantrax.Player{
		{ID: "a", Name: "Player A", MLBTeam: "NYY", Positions: []string{"012", "014"}, Status: "Reserve"},
		{ID: "b", Name: "Player B", MLBTeam: "NYY", Positions: []string{"012", "014"}, Status: "Reserve"},
	}

	slots := []fantrax.Slot{{PosID: "012", PosName: "OF"}}
	playingToday := map[string]bool{"NYY": true}

	result := OptimizeLineup(roster, playingToday, src, scoring, slots)

	// Player A should be chosen (5.0 > 3.0) even though Player B has more HR in Steamer
	if len(result.Scored) < 2 {
		t.Fatalf("expected 2 scored players, got %d", len(result.Scored))
	}
	// First scored player (highest pts) should be Player A
	if result.Scored[0].Player.Name != "Player A" {
		t.Errorf("expected Player A ranked first, got %s", result.Scored[0].Player.Name)
	}
	if result.Scored[0].ExpectedPts != 5.0 {
		t.Errorf("expected 5.0 pts, got %.2f", result.Scored[0].ExpectedPts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/optimizer/ -run TestOptimizeLineup_UsesBlendedPtsPerGame -v`
Expected: FAIL — Player A will have ~0.4 pts instead of 5.0 (blended not wired yet)

- [ ] **Step 3: Modify `scoreRoster` to check `PtsPerGameSource`**

In `internal/optimizer/lineup.go`, update the `scoreRoster` function:

```go
func scoreRoster(
	roster []fantrax.Player,
	playingToday map[string]bool,
	projSrc projections.Source,
	scoring fantrax.ScoringWeights,
) []ScoredPlayer {
	// Check if source provides pre-computed pts/game.
	pps, hasPPS := projSrc.(projections.PtsPerGameSource)

	scored := make([]ScoredPlayer, 0, len(roster))
	for _, p := range roster {
		hasGame := playingToday[p.MLBTeam]
		var pts float64
		found := false

		if hasPPS {
			if blended, ok := pps.GetPtsPerGame(p.Name, p.MLBTeam, scoring); ok {
				pts = blended
				found = true
			}
		}

		if !found {
			proj, ok := projSrc.GetProjection(p.Name, p.MLBTeam)
			if ok && proj.G > 0 {
				pts = expectedPts(proj, scoring)
			}
		}

		scored = append(scored, ScoredPlayer{
			Player:      p,
			ExpectedPts: pts,
			HasGame:     hasGame,
		})
	}
	return scored
}
```

- [ ] **Step 4: Run all optimizer tests**

Run: `go test ./internal/optimizer/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/optimizer/lineup.go internal/optimizer/lineup_test.go
git commit -m "feat: scoreRoster checks PtsPerGameSource before expectedPts"
```

---

### Task 5: Wire everything together in `cmd/main.go`

**Files:**
- Modify: `cmd/main.go` (lines 72-82, projections section)

- [ ] **Step 1: Update the projections section**

Replace the projections section in `cmd/main.go` (lines 72-82) with:

```go
	// --- Projections ---
	var projSrc projections.Source
	fgSrc, err := projections.NewFanGraphsSource()
	if err != nil {
		log.Printf("WARNING: fangraphs unavailable (%v) — using rolling stats only", err)
		projSrc = projections.NewRollingSource()
	} else {
		log.Printf("fangraphs projections loaded")
		rolling := projections.NewRollingSource()
		baseSrc := projections.NewChainedSource(fgSrc, rolling)

		// --- Recent stats for blending ---
		currentPeriod, err := ft.GetCurrentPeriod()
		if err != nil {
			log.Printf("WARNING: could not get current period (%v) — using Steamer only", err)
			projSrc = baseSrc
		} else if currentPeriod <= 1 {
			log.Printf("season not started (period %d) — using Steamer only", currentPeriod)
			projSrc = baseSrc
		} else {
			log.Printf("current period: %d, fetching last 10 periods...", currentPeriod)
			recentStats, err := ft.GetRecentStats(currentPeriod, 10)
			if err != nil {
				log.Printf("WARNING: recent stats unavailable (%v) — using Steamer only", err)
				projSrc = baseSrc
			} else {
				log.Printf("recent stats loaded: %d players with data", len(recentStats))
				// Build name→ID mapping from roster.
				nameToID := make(map[string]string)
				for _, p := range roster {
					nameToID[projections.NormalizeName(p.Name)] = p.ID
				}
				projSrc = projections.NewBlendedSource(baseSrc, recentStats, scoring, nameToID)
			}
		}
	}
```

- [ ] **Step 2: Verify build and tests pass**

Run: `go build ./... && go test ./internal/...`
Expected: Build success, all tests pass

- [ ] **Step 3: Commit**

```bash
git add cmd/main.go
git commit -m "feat: wire blended projections into main with period discovery"
```

---

### Task 6: End-to-end dry run verification

**Files:**
- None (manual testing)

- [ ] **Step 1: Run with dry-run to see blended scores**

Run: `go run ./cmd --dry-run --date 2026-03-27`

Check:
- Log output shows "current period: X, fetching last 10 periods..."
- Log output shows "recent stats loaded: Y players with data"
- Hitter ranking shows blended FP/G values (may differ from pure Steamer)
- Players with recent games should show different scores than pure Steamer

- [ ] **Step 2: Run without date override (today)**

Run: `go run ./cmd --dry-run`

Check: Same behavior, using today's date.

- [ ] **Step 3: Verify graceful degradation before season**

If testing before season starts, verify:
- Log shows "season not started" message
- Falls back to pure Steamer scores

---

### Task 7: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add blended projections documentation**

Add to the architecture section, after the `internal/projections` entry:

```markdown
**Blended scoring** — `BlendedSource` in `projections/blended.go` wraps Steamer with recent Fantrax stats (last 10 scoring periods). Formula: `0.60 * steamerPtsPerGame + 0.40 * recentFP/G`. Falls back to 100% Steamer when no recent data. The `PtsPerGameSource` interface (type assertion, not on `Source`) lets the optimizer use pre-computed blended values.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add blended projections to CLAUDE.md architecture"
```
