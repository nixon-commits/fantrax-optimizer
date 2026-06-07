# Windowed Recency + Strategy-Replay Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a backtest-only strategy-replay harness that compares the current YTD "recency" signal against rolling-window variants (14d / 30d / half-life decay) by the lineup Gap metric, hitters only.

**Architecture:** A pure `WeightedRecent` core collapses a per-day FP series (from `DailyFantasyPoints`) into a `fantrax.RecentStat` via a weight function — YTD/14d/30d/decay are four weight functions. A reusable `StrategyVariant` seam + `RunStrategyComparison` loop in `internal/backtest` replays the hitter optimizer per day under each variant's projections, scores the resulting lineup against actual FPts, and reports realized points / Gap / MAE / Bias. Production `optimize` is untouched.

**Tech Stack:** Go 1.x, existing `internal/projections` (BlendedSource), `internal/optimizer` (OptimizeLineup), `internal/fantrax` (DailyFantasyPoints, RecentStat), Cobra CLI.

---

## File Structure

- **Create** `internal/projections/recency.go` — `DayFP`, `WeightFunc`, the four weight constructors, and pure `WeightedRecent`. One responsibility: turn a day-series into a RecentStat.
- **Create** `internal/projections/recency_test.go` — unit tests for the pure core (weightings, leakage guard, early-season).
- **Create** `internal/backtest/strategy_replay.go` — `StrategyVariant`, `VariantResult`, series builder, `RunStrategyComparison`, realized-points + MAE/Bias helpers.
- **Create** `internal/backtest/strategy_replay_test.go` — harness tests with a stub source.
- **Modify** `cmd/backtest.go` — add `--recency-experiment` flag; wire FanGraphs base source + four variants + table output.
- **Modify** `Makefile` — add a `run-all` smoke line for the new flag.
- **Modify** `CLAUDE.md` + `README.md` — correct the "last 10 scoring periods" drift; document the harness.

---

## Task 1: Pure recency core

**Files:**
- Create: `internal/projections/recency.go`
- Test: `internal/projections/recency_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/projections/recency_test.go
package projections

import (
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// series: played 10 FP on each of the last 3 days, plus an old cold 0 FP day.
func sampleSeries() []DayFP {
	return []DayFP{
		{Date: day("2026-04-01"), FP: 0, Played: true},   // old cold game
		{Date: day("2026-05-28"), FP: 10, Played: true},
		{Date: day("2026-05-29"), FP: 10, Played: true},
		{Date: day("2026-05-30"), FP: 10, Played: true},
		{Date: day("2026-05-31"), FP: 99, Played: true},   // eval day — must be excluded
	}
}

func TestWeightedRecent_YTD_AveragesAllPriorGames(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), YTDWeight)
	// (0+10+10+10)/4 = 7.5 over 4 games; the eval-day 99 is excluded.
	if math.Abs(got.FPtsPerGame-7.5) > 1e-9 {
		t.Fatalf("FPtsPerGame = %v, want 7.5", got.FPtsPerGame)
	}
	if got.GamesPlayed != 4 {
		t.Fatalf("GamesPlayed = %d, want 4", got.GamesPlayed)
	}
}

func TestWeightedRecent_Window_DropsOldGames(t *testing.T) {
	// last-7-day window: only the three May 28–30 games qualify (April 1 is too old).
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), WindowWeight(7))
	if math.Abs(got.FPtsPerGame-10.0) > 1e-9 {
		t.Fatalf("FPtsPerGame = %v, want 10.0", got.FPtsPerGame)
	}
	if got.GamesPlayed != 3 {
		t.Fatalf("GamesPlayed = %d, want 3", got.GamesPlayed)
	}
}

func TestWeightedRecent_LeakageGuard_ExcludesEvalDayAndLater(t *testing.T) {
	// A future game must never count, even with YTD weighting.
	s := append(sampleSeries(), DayFP{Date: day("2026-06-05"), FP: 100, Played: true})
	got := WeightedRecent(s, day("2026-05-31"), YTDWeight)
	if got.GamesPlayed != 4 {
		t.Fatalf("GamesPlayed = %d, want 4 (eval-day and future excluded)", got.GamesPlayed)
	}
}

func TestWeightedRecent_Decay_WeightsRecentMore(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), DecayWeight(7))
	// Decay pulls the average above the flat 7.5 (recent 10s outweigh the old 0).
	if !(got.FPtsPerGame > 7.5) {
		t.Fatalf("decay FPtsPerGame = %v, want > 7.5", got.FPtsPerGame)
	}
}

func TestWeightedRecent_NoPriorGames_ReturnsZeroValue(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-04-01"), YTDWeight)
	if got.GamesPlayed != 0 || got.FPtsPerGame != 0 {
		t.Fatalf("got %+v, want zero value", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/projections/ -run TestWeightedRecent -v`
Expected: FAIL — `undefined: DayFP`, `WeightedRecent`, `YTDWeight`, `WindowWeight`, `DecayWeight`.

- [ ] **Step 3: Write the implementation**

```go
// internal/projections/recency.go
package projections

import (
	"math"
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

// DayFP is one player-day of fantasy production.
type DayFP struct {
	Date   time.Time
	FP     float64
	Played bool
}

// WeightFunc returns the weight for a day given its age in whole days from the
// as-of (evaluation) date. Age is always >= 1 for eligible days (the eval day
// and anything later is excluded by WeightedRecent's leakage guard).
type WeightFunc func(ageDays int) float64

// YTDWeight weights every prior game equally (the current season-to-date model).
func YTDWeight(_ int) float64 { return 1 }

// WindowWeight weights games in the trailing n days equally, others zero.
func WindowWeight(n int) WeightFunc {
	return func(age int) float64 {
		if age >= 1 && age <= n {
			return 1
		}
		return 0
	}
}

// DecayWeight applies exponential decay with the given half-life in days.
func DecayWeight(halfLifeDays float64) WeightFunc {
	lambda := math.Ln2 / halfLifeDays
	return func(age int) float64 {
		if age < 1 {
			return 0
		}
		return math.Exp(-lambda * float64(age))
	}
}

// WeightedRecent collapses a player's per-day series into a RecentStat as of
// evalDate, using only games strictly before evalDate (leakage guard).
//
//	FPtsPerGame = Σ(w·dayFP) / Σ(w)   over played days
//	GamesPlayed = count of played days with non-zero weight
func WeightedRecent(series []DayFP, evalDate time.Time, weight WeightFunc) fantrax.RecentStat {
	var sumW, sumWFP float64
	var games int
	for _, d := range series {
		if !d.Date.Before(evalDate) { // leakage guard: only days < evalDate
			continue
		}
		if !d.Played {
			continue
		}
		age := int(evalDate.Sub(d.Date).Hours() / 24)
		w := weight(age)
		if w == 0 {
			continue
		}
		sumW += w
		sumWFP += w * d.FP
		games++
	}
	if sumW == 0 {
		return fantrax.RecentStat{}
	}
	return fantrax.RecentStat{FPtsPerGame: sumWFP / sumW, GamesPlayed: games}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/projections/ -run TestWeightedRecent -v`
Expected: PASS (all 5).

- [ ] **Step 5: Commit**

```bash
git add internal/projections/recency.go internal/projections/recency_test.go
git commit -m "feat(projections): pure WeightedRecent core for windowed recency

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Harness types + hitter series builder

**Files:**
- Create: `internal/backtest/strategy_replay.go`
- Test: `internal/backtest/strategy_replay_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/backtest/strategy_replay_test.go
package backtest

import (
	"testing"
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestBuildHitterSeries_GroupsByPlayerAndDropsPitchers(t *testing.T) {
	days := []fantrax.DayRoster{
		{Date: d("2026-05-01"), Players: []fantrax.DayPlayerFP{
			{PlayerID: "h1", FPts: 5, HadGame: true, IsPitcher: false},
			{PlayerID: "p1", FPts: 9, HadGame: true, IsPitcher: true},
		}},
		{Date: d("2026-05-02"), Players: []fantrax.DayPlayerFP{
			{PlayerID: "h1", FPts: 7, HadGame: true, IsPitcher: false},
		}},
	}
	got := BuildHitterSeries(days)
	if _, ok := got["p1"]; ok {
		t.Fatalf("pitcher leaked into hitter series")
	}
	if len(got["h1"]) != 2 {
		t.Fatalf("h1 series len = %d, want 2", len(got["h1"]))
	}
	if got["h1"][1].FP != 7 || !got["h1"][1].Played {
		t.Fatalf("h1 day2 = %+v, want FP 7 played", got["h1"][1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backtest/ -run TestBuildHitterSeries -v`
Expected: FAIL — `undefined: BuildHitterSeries`.

- [ ] **Step 3: Write the implementation**

```go
// internal/backtest/strategy_replay.go
package backtest

import (
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
	"github.com/nixon-commits/rosterbot/internal/projections"
)

// StrategyVariant is one named projection strategy the replay harness evaluates.
// Build returns the hitter Source the variant would use to set a lineup on the
// given evaluation date, using only data available before that date.
type StrategyVariant struct {
	Name  string
	Build func(asOf time.Time) (projections.Source, error)
}

// VariantResult is the aggregate scorecard for one variant across the window.
type VariantResult struct {
	Name        string
	RealizedPts float64 // total actual FPts of the lineups this variant set
	MeanGap     float64 // mean daily (realized − hindsight-optimal), hitter slots
	MAE         float64 // mean abs per-player projection error vs actual
	Bias        float64 // signed mean per-player error (projected − actual)
}

// BuildHitterSeries groups per-day hitter FPts into a per-player DayFP series.
func BuildHitterSeries(days []fantrax.DayRoster) map[string][]projections.DayFP {
	series := make(map[string][]projections.DayFP)
	for _, day := range days {
		for _, p := range day.Players {
			if p.IsPitcher {
				continue
			}
			series[p.PlayerID] = append(series[p.PlayerID], projections.DayFP{
				Date:   day.Date,
				FP:     p.FPts,
				Played: p.HadGame,
			})
		}
	}
	return series
}
```

> Task 2 uses only `time`, `fantrax`, `projections`. Task 3 adds `fmt`, `math`,
> and `optimizer` to this import block when it adds the replay loop.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/backtest/ -run TestBuildHitterSeries -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/backtest/strategy_replay.go internal/backtest/strategy_replay_test.go
git commit -m "feat(backtest): strategy-replay types + hitter series builder

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: RunStrategyComparison replay loop

**Files:**
- Modify: `internal/backtest/strategy_replay.go`
- Test: `internal/backtest/strategy_replay_test.go`

- [ ] **Step 1: Write the failing test**

A stub source lets one "variant" prefer player A and another prefer player B; the
harness must score each variant's chosen lineup by ACTUAL FPts.

```go
// append to internal/backtest/strategy_replay_test.go
import (
	// add to existing import block:
	"github.com/nixon-commits/rosterbot/internal/projections"
)

// stubPtsSource returns a fixed pts/game per player name via the PtsPerGameSource path.
type stubPtsSource struct{ pts map[string]float64 }

func (s stubPtsSource) GetProjection(name, _ string) (*projections.Projection, bool) {
	return &projections.Projection{G: 1}, true
}
func (s stubPtsSource) GetPtsPerGame(name, _ string, _ fantrax.ScoringWeights) (float64, bool) {
	v, ok := s.pts[name]
	return v, ok
}

func TestRunStrategyComparison_ScoresChosenLineupByActuals(t *testing.T) {
	// One UT slot, two hitters with a game. Variant "likesA" projects A higher,
	// "likesB" projects B higher. Actuals: A scored 2, B scored 8.
	day := fantrax.DayRoster{
		Date: d("2026-05-10"),
		Players: []fantrax.DayPlayerFP{
			{PlayerID: "a", Name: "A", MLBTeam: "NYY", FPts: 2, HadGame: true, Positions: []string{"014"}},
			{PlayerID: "b", Name: "B", MLBTeam: "NYY", FPts: 8, HadGame: true, Positions: []string{"014"}},
		},
	}
	slots := []fantrax.Slot{{PosID: "014", Count: 1}} // one UT slot

	variants := []StrategyVariant{
		{Name: "likesA", Build: func(time.Time) (projections.Source, error) {
			return stubPtsSource{pts: map[string]float64{"A": 99, "B": 1}}, nil
		}},
		{Name: "likesB", Build: func(time.Time) (projections.Source, error) {
			return stubPtsSource{pts: map[string]float64{"A": 1, "B": 99}}, nil
		}},
	}

	got, err := RunStrategyComparison(variants, []fantrax.DayRoster{day}, slots, fantrax.ScoringWeights{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]VariantResult{}
	for _, r := range got {
		byName[r.Name] = r
	}
	if byName["likesA"].RealizedPts != 2 {
		t.Fatalf("likesA realized = %v, want 2 (it started A)", byName["likesA"].RealizedPts)
	}
	if byName["likesB"].RealizedPts != 8 {
		t.Fatalf("likesB realized = %v, want 8 (it started B)", byName["likesB"].RealizedPts)
	}
	// Hindsight-optimal is 8 (start B), so likesA's Gap is 2-8 = -6.
	if byName["likesA"].MeanGap != -6 {
		t.Fatalf("likesA gap = %v, want -6", byName["likesA"].MeanGap)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backtest/ -run TestRunStrategyComparison -v`
Expected: FAIL — `undefined: RunStrategyComparison`.

- [ ] **Step 3: Write the implementation**

Update the import block of `strategy_replay.go` to add `fmt`, `math`, `optimizer`:

```go
import (
	"fmt"
	"math"
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
	"github.com/nixon-commits/rosterbot/internal/optimizer"
	"github.com/nixon-commits/rosterbot/internal/projections"
)
```

Append to `strategy_replay.go`:

```go
// RunStrategyComparison replays the hitter optimizer for each variant across the
// given days and reports realized points, mean Gap to hindsight-optimal, and
// per-player projection MAE/Bias. Hitters only; pitchers are ignored.
func RunStrategyComparison(
	variants []StrategyVariant,
	days []fantrax.DayRoster,
	hitterSlots []fantrax.Slot,
	scoring fantrax.ScoringWeights,
) ([]VariantResult, error) {
	type acc struct {
		realized, gapSum, absErr, signErr float64
		errN, dayN                        int
	}
	accs := make([]acc, len(variants))

	for _, day := range days {
		hitters, _ := splitPlayers(day.Players)
		actualByID := make(map[string]float64, len(hitters))
		for _, p := range hitters {
			actualByID[p.PlayerID] = p.FPts
		}
		// Hindsight-optimal hitter points for the day (existing helper).
		optimal := hitterOptimalPts(optimizeHitters(hitters, hitterSlots))

		for i, v := range variants {
			src, err := v.Build(day.Date)
			if err != nil {
				return nil, fmt.Errorf("variant %s build %s: %w", v.Name, day.Date.Format("2006-01-02"), err)
			}
			roster := toPlayers(hitters)
			playing := teamsWithGames(hitters)
			res := optimizer.OptimizeLineup(roster, playing, src, scoring, hitterSlots, nil)

			chosen := chosenHitterIDs(res)
			var realized float64
			for id := range chosen {
				realized += actualByID[id]
			}
			accs[i].realized += realized
			accs[i].gapSum += realized - optimal
			accs[i].dayN++

			// Diagnostics: per-player projection error over hitters who had a game.
			for _, p := range hitters {
				if !p.HadGame {
					continue
				}
				proj, ok := src.(projections.PtsPerGameSource)
				if !ok {
					continue
				}
				pred, has := proj.GetPtsPerGame(p.Name, p.MLBTeam, scoring)
				if !has {
					continue
				}
				e := pred - p.FPts
				accs[i].absErr += math.Abs(e)
				accs[i].signErr += e
				accs[i].errN++
			}
		}
	}

	out := make([]VariantResult, len(variants))
	for i, v := range variants {
		a := accs[i]
		out[i] = VariantResult{Name: v.Name, RealizedPts: a.realized}
		if a.dayN > 0 {
			out[i].MeanGap = a.gapSum / float64(a.dayN)
		}
		if a.errN > 0 {
			out[i].MAE = a.absErr / float64(a.errN)
			out[i].Bias = a.signErr / float64(a.errN)
		}
	}
	return out, nil
}

// chosenHitterIDs returns the set of player IDs the optimizer placed in active
// slots (mirrors the in-lineup predicate used by hitterOptimalPts).
func chosenHitterIDs(r optimizer.Result) map[string]bool {
	benched := make(map[string]bool, len(r.ToBench))
	for _, id := range r.ToBench {
		benched[id] = true
	}
	activated := make(map[string]bool, len(r.ToActivate))
	for _, ps := range r.ToActivate {
		activated[ps.PlayerID] = true
	}
	chosen := make(map[string]bool)
	for _, sp := range r.Scored {
		inLineup := (sp.Player.Status == "Active" && !benched[sp.Player.ID]) || activated[sp.Player.ID]
		if inLineup && sp.HasGame {
			chosen[sp.Player.ID] = true
		}
	}
	return chosen
}
```

> Note: `splitPlayers`, `optimizeHitters`, `hitterOptimalPts`, `toPlayers`, and
> `teamsWithGames` already exist in `internal/backtest/backtest.go` (same package).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/backtest/ -run 'TestRunStrategyComparison|TestBuildHitterSeries' -v`
Expected: PASS both.

- [ ] **Step 5: Run the full package + vet**

Run: `go test ./internal/backtest/... && go vet ./internal/backtest/...`
Expected: ok.

- [ ] **Step 6: Commit**

```bash
git add internal/backtest/strategy_replay.go internal/backtest/strategy_replay_test.go
git commit -m "feat(backtest): RunStrategyComparison replay loop with Gap + MAE/Bias

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Wire `--recency-experiment` into the backtest CLI

**Files:**
- Modify: `cmd/backtest.go`

This task builds the four variants from a shared FanGraphs base hitter source and
the per-player series, then prints a comparison table. The base source loading
mirrors `cmd/optimize.go:280-311` (FanGraphs depthcharts-ros wrapped with the
rolling fallback). The base projection is the current snapshot, identical across
all variants — fair for isolating the recency effect (documented in the spec).

- [ ] **Step 1: Add the flag**

In `cmd/backtest.go` `init()`, add:

```go
backtestCmd.Flags().BoolVar(&backtestRecencyExperiment, "recency-experiment", false, "compare YTD vs 14d/30d/decay recency by lineup Gap (hitters only)")
```

And declare the var alongside the other backtest flag vars:

```go
var backtestRecencyExperiment bool
```

- [ ] **Step 2: Branch in `runBacktest`**

Near the top of `runBacktest`, after `days` is fetched (the existing
`ft.DailyFantasyPoints(...)` call), add:

```go
if backtestRecencyExperiment {
	return runRecencyExperiment(ft, cfg, days, hitterSlots, hitterScoring)
}
```

> `hitterSlots` is already resolved in `runBacktest`. Fetch `hitterScoring` just
> before the branch the same way `cmd/optimize.go:235` does:
> `hitterScoring, err := ft.GetScoringWeights()`.

- [ ] **Step 3: Implement `runRecencyExperiment`**

Add to `cmd/backtest.go`:

```go
func runRecencyExperiment(
	ft *fantrax.Client,
	cfg *config.Config,
	days []fantrax.DayRoster,
	hitterSlots []fantrax.Slot,
	hitterScoring fantrax.ScoringWeights,
) error {
	// Base hitter projection source (depthcharts-ros), shared across variants.
	// Mirror cmd/optimize.go:181,289-290 base-source construction.
	projTTL := cacheTTL(24 * time.Hour) // cacheTTL helper already used in cmd/optimize.go:103
	fgSrc, _, err := projections.LoadBattingProjections("depthcharts-ros", cacheDir, projTTL)
	if err != nil {
		return fmt.Errorf("load base projections: %w", err)
	}
	rolling := projections.NewRollingSource()
	baseSrc := projections.NewChainedSource(fgSrc, rolling)

	// Roster nameToID for the blend (hitters only).
	hitterRoster, err := ft.GetHitterRoster()
	if err != nil {
		return fmt.Errorf("hitter roster: %w", err)
	}
	nameToID := make(map[string]string)
	for _, p := range hitterRoster {
		nameToID[projections.NormalizeName(p.Name)] = p.ID
	}

	series := backtest.BuildHitterSeries(days)

	mkVariant := func(name string, w projections.WeightFunc) backtest.StrategyVariant {
		return backtest.StrategyVariant{
			Name: name,
			Build: func(asOf time.Time) (projections.Source, error) {
				recent := make(map[string]fantrax.RecentStat, len(series))
				for id, s := range series {
					recent[id] = projections.WeightedRecent(s, asOf, w)
				}
				return projections.NewBlendedSource(baseSrc, recent, hitterScoring, nameToID, cfg.BlendMinGP), nil
			},
		}
	}

	variants := []backtest.StrategyVariant{
		mkVariant("ytd", projections.YTDWeight),
		mkVariant("w14", projections.WindowWeight(14)),
		mkVariant("w30", projections.WindowWeight(30)),
		mkVariant("decay21", projections.DecayWeight(21)),
	}

	results, err := backtest.RunStrategyComparison(variants, days, hitterSlots, hitterScoring)
	if err != nil {
		return err
	}

	fmt.Printf("\nRecency strategy comparison (hitters, %d days)\n", len(days))
	fmt.Printf("%-10s %12s %10s %8s %8s\n", "mode", "realized", "mean gap", "MAE", "bias")
	for _, r := range results {
		fmt.Printf("%-10s %12.1f %10.2f %8.2f %8.2f\n", r.Name, r.RealizedPts, r.MeanGap, r.MAE, r.Bias)
	}
	return nil
}
```

> Verified names (cmd/optimize.go): `projections.LoadBattingProjections(system, cacheDir, projTTL) (Source, LoadResult, error)` (L181), `projections.NewRollingSource()` (L289), `projections.NewChainedSource(fgSrc, rolling)` (L290), `ft.GetHitterRoster()` (L223), `ft.GetScoringWeights()` (L235), `cfg.BlendMinGP` (L310). `cacheDir` and the `cacheTTL` helper are package-level in `cmd/`.

- [ ] **Step 4: Build and smoke-test (dry, read-only)**

Run: `go build ./... && go vet ./...`
Expected: clean build.

Run (warm cache, if season data exists): `go run . backtest --recency-experiment --dates 2026-05-01:2026-05-14`
Expected: a 4-row table (`ytd`/`w14`/`w30`/`decay21`) with realized/gap/MAE/bias. (If no snapshots exist locally, the command will error on data fetch — that's a data availability issue, not a code failure; note it and move on.)

- [ ] **Step 5: Commit**

```bash
git add cmd/backtest.go internal/backtest/strategy_replay.go internal/backtest/strategy_replay_test.go
git commit -m "feat(cmd): backtest --recency-experiment compares recency strategies

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Docs + Makefile

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `Makefile`

- [ ] **Step 1: Fix the CLAUDE.md recency drift**

In `CLAUDE.md`, the "Blended scoring" section says recency uses "last 10 scoring
periods." Replace with an accurate description and a pointer to the experiment:

> Recent stats are the player's **season-to-date** FP/G (the Fantrax roster API
> returns cumulative YTD regardless of period). A backtest-only strategy-replay
> harness (`backtest --recency-experiment`) compares this YTD signal against
> rolling-window variants (14d/30d/half-life) by lineup Gap; see
> `docs/superpowers/specs/2026-06-06-windowed-recency-strategy-replay-design.md`.

- [ ] **Step 2: Document the command in README.md**

Add under the backtest commands:

```
go run . backtest --recency-experiment --dates 2026-05-01:2026-05-14  # compare recency strategies (hitters)
```

- [ ] **Step 3: Add a run-all smoke line in the Makefile**

In the `run-all` recipe, after the existing `backtest` line, add:

```make
	@echo "== backtest --recency-experiment =="; time go run . backtest --recency-experiment --dates 2026-05-01:2026-05-07 || true
```

- [ ] **Step 4: Verify**

Run: `go vet ./... && go build ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md Makefile
git commit -m "docs: correct recency description + document strategy-replay harness

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Final verification

- [ ] Run full suite: `go test ./...` — expected ok across packages.
- [ ] `go vet ./...` and `go mod tidy` — clean.
- [ ] Confirm production untouched: `git grep -n "recency-experiment" cmd/optimize.go` returns nothing; `optimize` still uses `GetRecentStats` (YTD) unchanged.
