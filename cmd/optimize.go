package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nixon-commits/fantrax-optimizer/internal/fantrax"
	"github.com/nixon-commits/fantrax-optimizer/internal/optimizer"
	"github.com/nixon-commits/fantrax-optimizer/internal/projections"
	"github.com/nixon-commits/fantrax-optimizer/internal/roster"
	"github.com/nixon-commits/fantrax-optimizer/internal/schedule"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	datesStr    string
	checkRoster bool
)

var optimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Optimize daily lineup for hitters and pitchers",
	RunE:  runOptimize,
}

func init() {
	optimizeCmd.Flags().StringVar(&datesStr, "dates", "", "date(s) for schedule lookup: YYYY-MM-DD, YYYY-MM-DD:YYYY-MM-DD, or 'all' (default: today)")
	optimizeCmd.Flags().BoolVar(&checkRoster, "check-roster", true, "check for roster slot mismatches (IL/minors)")
	rootCmd.AddCommand(optimizeCmd)
}

func runOptimize(cmd *cobra.Command, args []string) error {
	today := time.Now().Truncate(24 * time.Hour)

	// Parse dates early for non-"all" cases; "all" needs the Fantrax client.
	var dates []time.Time
	needsSeasonLookup := datesStr == "all"
	if !needsSeasonLookup {
		var err error
		dates, err = parseDates(datesStr, today)
		if err != nil {
			return fmt.Errorf("invalid --dates: %w", err)
		}
	}

	cfg, ft, err := initApp(dates)
	if err != nil {
		return err
	}

	// Resolve "all" now that the client is available.
	if needsSeasonLookup {
		start, end, err := ft.GetSeasonDateRange()
		if err != nil {
			return fmt.Errorf("get season date range: %w", err)
		}
		if start.Before(today) {
			start = today
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			cfg.Dates = append(cfg.Dates, d)
		}
		log.Printf("season range: %s to %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	log.Printf("dates=%s dry-run=%v", formatDates(cfg.Dates), cfg.DryRun)

	// --- Roster alerts (if requested) ---
	if checkRoster {
		fullRoster, counts, err := ft.GetFullHitterRoster()
		if err != nil {
			return fmt.Errorf("get full roster: %w", err)
		}
		counts.ILCapacity = cfg.ILSlots
		counts.MinorsCapacity = cfg.MinorsSlots
		alerts := roster.CheckRoster(fullRoster, counts)
		if len(alerts) > 0 {
			fmt.Println("\n=== Roster Alerts ===")
			for _, a := range alerts {
				label := alertLabel(a.Type)
				fmt.Printf("  ⚠ %-25s (%s)  %s → %s\n", a.Player.Name, a.Player.MLBTeam, label, a.Suggestion)
			}
			fmt.Println()
		}
	}

	// --- Fetch hitter roster, slots, scoring (shared across dates) ---
	hitterRoster, err := ft.GetHitterRoster()
	if err != nil {
		return fmt.Errorf("get hitter roster: %w", err)
	}
	log.Printf("hitter roster: %d hitters (%d active)", len(hitterRoster), countActive(hitterRoster))

	hitterSlots, err := ft.GetActiveSlots()
	if err != nil {
		return fmt.Errorf("get hitter slots: %w", err)
	}
	log.Printf("hitter active slots: %d", len(hitterSlots))

	hitterScoring, err := ft.GetScoringWeights()
	if err != nil {
		return fmt.Errorf("get hitter scoring: %w", err)
	}
	log.Printf("hitter scoring weights: %d categories", len(hitterScoring))

	// --- Fetch pitcher roster, slots, scoring (shared across dates) ---
	pitcherRoster, err := ft.GetPitcherRoster()
	if err != nil {
		return fmt.Errorf("get pitcher roster: %w", err)
	}
	log.Printf("pitcher roster: %d pitchers (%d active)", len(pitcherRoster), countActive(pitcherRoster))

	pitcherSlots, err := ft.GetPitcherSlots()
	if err != nil {
		return fmt.Errorf("get pitcher slots: %w", err)
	}
	log.Printf("pitcher active slots: %d", len(pitcherSlots))

	pitcherScoring, err := ft.GetPitcherScoringWeights()
	if err != nil {
		return fmt.Errorf("get pitcher scoring: %w", err)
	}
	log.Printf("pitcher scoring weights: %d categories", len(pitcherScoring))

	// --- Current period (shared by hitter + pitcher blending) ---
	currentPeriod, periodErr := ft.GetCurrentPeriod()
	if periodErr != nil {
		log.Printf("WARNING: could not get current period (%v) — using Steamer only", periodErr)
	} else {
		log.Printf("current period: %d", currentPeriod)
	}

	// --- Hitter projections (shared across dates) ---
	var hitterProjSrc projections.Source
	fgSrc, err := projections.NewFanGraphsSource()
	if err != nil {
		log.Printf("WARNING: fangraphs batting unavailable (%v) — using rolling stats only", err)
		hitterProjSrc = projections.NewRollingSource()
	} else {
		log.Printf("fangraphs batting projections loaded")
		rolling := projections.NewRollingSource()
		baseSrc := projections.NewChainedSource(fgSrc, rolling)

		if periodErr != nil || currentPeriod <= 1 {
			if currentPeriod <= 1 {
				log.Printf("season not started (period %d) — using Steamer only", currentPeriod)
			}
			hitterProjSrc = baseSrc
		} else {
			log.Printf("fetching last 10 hitter periods...")
			recentStats, err := ft.GetRecentStats(currentPeriod, 10)
			if err != nil {
				log.Printf("WARNING: recent hitter stats unavailable (%v) — using Steamer only", err)
				hitterProjSrc = baseSrc
			} else {
				log.Printf("recent hitter stats loaded: %d players with data", len(recentStats))
				nameToID := make(map[string]string)
				for _, p := range hitterRoster {
					nameToID[projections.NormalizeName(p.Name)] = p.ID
				}
				hitterProjSrc = projections.NewBlendedSource(baseSrc, recentStats, hitterScoring, nameToID)
			}
		}
	}

	// --- Pitcher projections (shared across dates) ---
	var pitcherProjSrc projections.PitcherSource
	fgPitSrc, err := projections.NewFanGraphsPitcherSource()
	if err != nil {
		log.Printf("WARNING: fangraphs pitching unavailable (%v) — using rolling stats only", err)
		pitcherProjSrc = projections.NewPitcherRollingSource()
	} else {
		log.Printf("fangraphs pitching projections loaded")
		pitRolling := projections.NewPitcherRollingSource()
		pitBaseSrc := projections.NewPitcherChainedSource(fgPitSrc, pitRolling)

		if periodErr != nil || currentPeriod <= 1 {
			pitcherProjSrc = pitBaseSrc
		} else {
			recentPitStats, err := ft.GetRecentPitcherStats(currentPeriod, 10)
			if err != nil {
				log.Printf("WARNING: recent pitcher stats unavailable (%v) — using Steamer only", err)
				pitcherProjSrc = pitBaseSrc
			} else {
				log.Printf("recent pitcher stats loaded: %d players with data", len(recentPitStats))
				pitNameToID := make(map[string]string)
				pitPlayerPos := make(map[string][]string)
				for _, p := range pitcherRoster {
					pitNameToID[projections.NormalizeName(p.Name)] = p.ID
					pitPlayerPos[p.ID] = p.Positions
				}
				pitcherProjSrc = projections.NewPitcherBlendedSource(pitBaseSrc, recentPitStats, pitcherScoring, pitNameToID, pitPlayerPos)
			}
		}
	}

	multiDate := len(cfg.Dates) > 1
	schedClient := schedule.NewClient()

	// Get season start date for period calculation.
	seasonStart, _, err := ft.GetSeasonDateRange()
	if err != nil {
		log.Printf("WARNING: could not get season start (%v) — only today's lineup can be set", err)
	}

	// Build name/slot lookup maps for display.
	playerName := make(map[string]string)
	for _, p := range hitterRoster {
		playerName[p.ID] = p.Name
	}
	for _, p := range pitcherRoster {
		playerName[p.ID] = p.Name
	}
	slotName := make(map[string]string)
	for _, s := range hitterSlots {
		slotName[s.PosID] = s.PosName
	}
	for _, s := range pitcherSlots {
		slotName[s.PosID] = s.PosName
	}

	// --- Parallel fetch + optimize for all dates ---
	type dateResult struct {
		date          time.Time
		period        int
		isToday       bool
		hitterResult  optimizer.Result
		pitcherResult optimizer.PitcherResult
		warnings      []string
	}

	results := make([]dateResult, len(cfg.Dates))

	var g errgroup.Group
	for i, date := range cfg.Dates {
		i, date := i, date
		g.Go(func() error {
			isToday := date.Equal(today)
			period := fantrax.PeriodForDate(seasonStart, date)

			var warnings []string

			// Fetch period-specific rosters.
			dateHitterRoster := hitterRoster
			datePitcherRoster := pitcherRoster
			if !isToday && period > 0 {
				if r, err := ft.GetHitterRosterForPeriod(period); err == nil {
					dateHitterRoster = r
				} else {
					warnings = append(warnings, fmt.Sprintf("could not fetch hitter roster for period %d (%v) — using current", period, err))
				}
				if r, err := ft.GetPitcherRosterForPeriod(period); err == nil {
					datePitcherRoster = r
				} else {
					warnings = append(warnings, fmt.Sprintf("could not fetch pitcher roster for period %d (%v) — using current", period, err))
				}
			}

			// MLB schedule + probable pitchers.
			playingToday, err := schedClient.TeamsPlayingOn(date)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("mlb schedule unavailable for %s (%v) — assuming all teams play", date.Format("2006-01-02"), err))
				allPlayers := append(dateHitterRoster, datePitcherRoster...)
				playingToday = allTeamsPlaying(allPlayers)
			}

			probableStarters, err := schedClient.ProbableStarters(date)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("probable pitchers unavailable for %s (%v) — SPs default to start", date.Format("2006-01-02"), err))
				probableStarters = map[string]string{} // empty = default to start
			}

			// Optimize hitters.
			hitterResult := optimizer.OptimizeLineup(dateHitterRoster, playingToday, hitterProjSrc, hitterScoring, hitterSlots)

			// Optimize pitchers.
			pitcherResult := optimizer.OptimizePitcherLineup(datePitcherRoster, playingToday, probableStarters, pitcherProjSrc, pitcherScoring, pitcherSlots)

			results[i] = dateResult{
				date:          date,
				period:        period,
				isToday:       isToday,
				hitterResult:  hitterResult,
				pitcherResult: pitcherResult,
				warnings:      warnings,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel optimize: %w", err)
	}

	// --- Sequential print + apply ---
	for _, dr := range results {
		for _, w := range dr.warnings {
			log.Printf("WARNING: %s", w)
		}

		if multiDate {
			header := dr.date.Format("2006-01-02")
			if dr.isToday {
				header += " (today)"
			}
			fmt.Printf("\n=== %s ===\n", header)
		}

		// --- Print hitter ranking ---
		fmt.Println("\n=== Hitter Ranking ===")
		fmt.Printf("%-25s %-6s %-8s %s\n", "Player", "Team", "Pts/G", "Game?")
		fmt.Println(strings.Repeat("-", 55))
		for _, sp := range dr.hitterResult.Scored {
			game := "no"
			if sp.HasGame {
				game = "YES"
			}
			fmt.Printf("%-25s %-6s %-8.2f %s\n", sp.Player.Name, sp.Player.MLBTeam, sp.ExpectedPts, game)
		}

		// --- Print pitcher ranking ---
		fmt.Println("\n=== Pitcher Ranking ===")
		fmt.Printf("%-25s %-6s %-8s %-6s %-6s %s\n", "Player", "Team", "Pts/G", "Role", "Prob", "Game?")
		fmt.Println(strings.Repeat("-", 72))
		for _, sp := range dr.pitcherResult.Scored {
			game := "no"
			if sp.HasGame {
				game = "YES"
			}
			role := sp.Player.PosShortNames
			if role == "" {
				role = "P"
			}
			prob := ""
			if sp.IsStarter {
				prob = "YES"
			}
			fmt.Printf("%-25s %-6s %-8.2f %-6s %-6s %s\n", sp.Player.Name, sp.Player.MLBTeam, sp.ExpectedPts, role, prob, game)
		}

		// --- Combine changes ---
		allActivate := append(dr.hitterResult.ToActivate, dr.pitcherResult.ToActivate...)
		allBench := append(dr.hitterResult.ToBench, dr.pitcherResult.ToBench...)

		// --- Print planned moves ---
		fmt.Println("\n=== Planned Lineup Changes ===")
		if len(allActivate) == 0 && len(allBench) == 0 {
			fmt.Println("No changes needed.")
			continue
		}

		for _, ps := range allActivate {
			fmt.Printf("  ACTIVATE  %-25s → %s\n", playerName[ps.PlayerID], slotName[ps.PosID])
		}
		for _, id := range allBench {
			fmt.Printf("  BENCH     %s\n", playerName[id])
		}

		if cfg.DryRun {
			fmt.Println("\n[DRY RUN] No changes applied.")
			continue
		}

		// --- Resolve period for this date ---
		dateKey := dr.date.Format("2006-01-02")
		if dr.period == 0 && !dr.isToday {
			fmt.Printf("\n[SKIP] No scoring period found for %s — changes not applied.\n", dateKey)
			continue
		}

		// --- Apply combined lineup (sequential — Fantrax API is not concurrent-safe) ---
		fmt.Printf("\nApplying lineup for %s (period %d)...\n", dateKey, dr.period)
		if err := ft.ApplyLineup(dr.period, allActivate, allBench); err != nil {
			return fmt.Errorf("apply lineup: %w", err)
		}
		fmt.Println("Lineup applied successfully.")
	}

	return nil
}
