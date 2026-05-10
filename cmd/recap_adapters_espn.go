package cmd

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nixon-commits/rosterbot/internal/cache"
	"github.com/nixon-commits/rosterbot/internal/espn"
	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

// espnRecapAdapter implements recap.Platform against ESPN's v3 API. The
// translation strategy: every method returns fantrax.* types so the recap
// package, backtest, and optimizer don't need to know ESPN exists.
//
// The trickiest piece is the slot ID translation: ESPN uses numeric slot
// IDs (0=C, 5=OF, 13=P, etc.) while the project's backtest+optimizer
// pipeline special-cases the Fantrax PosName values ("UT", "INF", "P")
// for slot eligibility. To make the existing optimizer "just work", this
// adapter rewrites ESPN slot IDs into Fantrax position ID strings before
// emitting any fantrax.Slot or DayPlayerFP values.
type espnRecapAdapter struct {
	client *espn.Client
	today  time.Time

	seasonOnce sync.Once
	season     *espn.SeasonInfo
	seasonErr  error
}

func newESPNRecapAdapter(c *espn.Client, today time.Time) *espnRecapAdapter {
	return &espnRecapAdapter{client: c, today: today}
}

// loadSeason fetches and caches mSettings exactly once per adapter lifetime.
// Almost every other method needs SeasonInfo (slot config, period bounds,
// season start anchor) so we centralize the fetch.
func (a *espnRecapAdapter) loadSeason() (*espn.SeasonInfo, error) {
	a.seasonOnce.Do(func() {
		a.season, a.seasonErr = a.client.GetSeasonInfo(a.today)
	})
	return a.season, a.seasonErr
}

// ---------------------------------------------------------------------------
// Slot ID translation: ESPN → Fantrax
// ---------------------------------------------------------------------------

// espnSlotToFantrax maps ESPN's lineupSlotId to the Fantrax position ID
// the optimizer/backtest expects. Combo slots collapse to their Fantrax
// equivalents:
//   - ESPN UTIL (12) → Fantrax UT ("014")
//   - ESPN IF   (19) → Fantrax INF ("008")
//   - ESPN P    (13) → Fantrax P  ("017")
//   - ESPN DH   (11) → Fantrax UT ("014") since Fantrax has no separate DH
var espnSlotToFantrax = map[int]string{
	espn.SlotC:    "001", // C
	espn.Slot1B:   "002", // 1B
	espn.Slot2B:   "003", // 2B
	espn.Slot3B:   "004", // 3B
	espn.SlotSS:   "005", // SS
	espn.SlotOF:   "012", // OF
	espn.SlotIF:   "008", // INF
	espn.SlotUTIL: "014", // UT
	espn.SlotDH:   "014", // DH → UT (Fantrax has no DH; UT accepts any hitter)
	espn.SlotP:    "017", // P (any pitcher)
	espn.SlotSP:   "015", // SP
	espn.SlotRP:   "016", // RP
	espn.SlotLF:   "012", // LF → OF
	espn.SlotCF:   "012", // CF → OF
	espn.SlotRF:   "012", // RF → OF
}

// espnSlotToFantraxPosName mirrors espnSlotToFantrax for the PosName field
// the optimizer's special-case eligibility logic keys off ("UT", "INF", "P").
var espnSlotToFantraxPosName = map[int]string{
	espn.SlotC:    "C",
	espn.Slot1B:   "1B",
	espn.Slot2B:   "2B",
	espn.Slot3B:   "3B",
	espn.SlotSS:   "SS",
	espn.SlotOF:   "OF",
	espn.SlotIF:   "INF",
	espn.SlotUTIL: "UT",
	espn.SlotP:    "P",
	espn.SlotSP:   "SP",
	espn.SlotRP:   "RP",
}

// activeSlotOrder is the canonical display order recap uses for hitter slots.
// Mirrors the order in fantrax.GetActiveSlots so the rendered HTML reads the
// same regardless of platform.
var activeSlotOrder = []int{
	espn.SlotC, espn.Slot1B, espn.Slot2B, espn.Slot3B, espn.SlotSS,
	espn.SlotIF, espn.SlotOF, espn.SlotUTIL,
}

var pitcherSlotOrder = []int{espn.SlotSP, espn.SlotRP, espn.SlotP}

// ---------------------------------------------------------------------------
// Platform method implementations
// ---------------------------------------------------------------------------

func (a *espnRecapAdapter) GetSeasonDateRange() (time.Time, time.Time, error) {
	s, err := a.loadSeason()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end := s.DateForPeriod(s.LatestScoringPeriod)
	return s.SeasonStart, end, nil
}

func (a *espnRecapAdapter) GetTeams() (map[string]string, map[string]string, error) {
	return a.client.GetLeagueTeams()
}

func (a *espnRecapAdapter) GetActiveSlots() ([]fantrax.Slot, error) {
	s, err := a.loadSeason()
	if err != nil {
		return nil, err
	}
	return slotsFromCounts(s.SlotCounts, activeSlotOrder), nil
}

func (a *espnRecapAdapter) GetPitcherSlots() ([]fantrax.Slot, error) {
	s, err := a.loadSeason()
	if err != nil {
		return nil, err
	}
	return slotsFromCounts(s.SlotCounts, pitcherSlotOrder), nil
}

// slotsFromCounts expands ESPN's lineupSlotCounts into the per-slot list
// recap/backtest expects (one fantrax.Slot per slot instance, in display
// order). Slots not in the provided order list are skipped.
func slotsFromCounts(counts map[int]int, order []int) []fantrax.Slot {
	var out []fantrax.Slot
	for _, espnSlot := range order {
		n := counts[espnSlot]
		posID := espnSlotToFantrax[espnSlot]
		posName := espnSlotToFantraxPosName[espnSlot]
		for i := 0; i < n; i++ {
			out = append(out, fantrax.Slot{PosID: posID, PosName: posName})
		}
	}
	return out
}

func (a *espnRecapAdapter) GetAllMatchupEntries() ([]fantrax.MatchupEntry, error) {
	s, err := a.loadSeason()
	if err != nil {
		return nil, err
	}
	matchups, err := a.client.GetMatchups()
	if err != nil {
		return nil, err
	}
	// Fantrax's MatchupEntry has one entry per (date, home, away). We expand
	// each ESPN matchup into one entry per scoring period in its matchup
	// period, so recap's pairsForWeek date-window filter works unchanged.
	var out []fantrax.MatchupEntry
	for _, m := range matchups {
		periods := s.MatchupPeriods[m.MatchupPeriodID]
		if len(periods) == 0 {
			continue
		}
		homeID := fmt.Sprintf("%d", m.Home.TeamID)
		awayID := fmt.Sprintf("%d", m.Away.TeamID)
		for _, period := range periods {
			date := s.DateForPeriod(period)
			if date.IsZero() {
				continue
			}
			out = append(out, fantrax.MatchupEntry{
				ScoringPeriod: period,
				Date:          date.Format("Mon Jan 2, 2006"),
				HomeID:        homeID,
				AwayID:        awayID,
			})
		}
	}
	return out, nil
}

func (a *espnRecapAdapter) GetMatchupWeekNumberForDate(date time.Time) (int, error) {
	s, err := a.loadSeason()
	if err != nil {
		return 0, err
	}
	period := s.PeriodForDate(date)
	if period == 0 {
		return 0, nil
	}
	return s.MatchupPeriodForScoringPeriod(period), nil
}

func (a *espnRecapAdapter) GetMatchupWeekByNumber(n int) (time.Time, time.Time, error) {
	s, err := a.loadSeason()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if n < 1 || n > s.MatchupPeriodCount {
		return time.Time{}, time.Time{}, nil
	}
	start, end := s.MatchupPeriodBounds(n)
	return start, end, nil
}

// ---------------------------------------------------------------------------
// DailyFantasyPoints — the heavy lift
// ---------------------------------------------------------------------------

// boxScoreCacheEntry wraps the per-period league-wide box score for caching.
// Entries are keyed by scoring period ID; past periods are immutable so we
// use a long TTL.
type boxScoreCacheEntry struct {
	Matchups []espn.BoxScoreMatchup `json:"matchups"`
}

func (a *espnRecapAdapter) DailyFantasyPoints(
	teamID string,
	start, end, seasonStart time.Time,
	cacheDir string,
	cacheTTL time.Duration,
) ([]fantrax.DayRoster, error) {
	s, err := a.loadSeason()
	if err != nil {
		return nil, err
	}

	// Walk each calendar day in [start, end]. Convert to ESPN's scoring
	// period via SeasonInfo (which anchors period 1 = opening day).
	var teamIDInt int
	fmt.Sscanf(teamID, "%d", &teamIDInt)

	c := cache.New[boxScoreCacheEntry](cacheDir, cacheTTL)

	var days []fantrax.DayRoster
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		period := s.PeriodForDate(d)
		if period <= 0 {
			continue
		}
		// Skip future periods — ESPN returns empty rosters for those and
		// we'd rather emit a zero-length DayRoster than a garbage one.
		if period > s.LatestScoringPeriod {
			continue
		}
		key := fmt.Sprintf("espn-boxscore-%d-%d", s.SeasonID, period)
		entry, err := c.Get(key, func() (boxScoreCacheEntry, error) {
			matchups, ferr := a.client.GetBoxScoreForPeriod(period)
			if ferr != nil {
				return boxScoreCacheEntry{}, ferr
			}
			return boxScoreCacheEntry{Matchups: matchups}, nil
		})
		if err != nil {
			return nil, fmt.Errorf("box score period %d: %w", period, err)
		}
		roster := espn.FindTeamRoster(entry.Matchups, teamIDInt)
		if roster == nil {
			// Team didn't have a matchup that day (shouldn't happen during
			// the regular season, but be defensive — emit an empty entry).
			days = append(days, fantrax.DayRoster{Date: d, Period: period})
			continue
		}
		days = append(days, fantrax.DayRoster{
			Date:    d,
			Period:  period,
			Players: convertRosterToDayPlayers(roster),
		})
	}
	return days, nil
}

// convertRosterToDayPlayers turns one ESPN per-day roster into the
// project-canonical DayPlayerFP shape. Per-player FPts come from
// playerPoolEntry.appliedStatTotal — ESPN's "applied points for this
// scoring period" is precisely the daily delta we want.
func convertRosterToDayPlayers(roster *espn.RosterBlock) []fantrax.DayPlayerFP {
	if roster == nil {
		return nil
	}
	out := make([]fantrax.DayPlayerFP, 0, len(roster.Entries))
	for _, e := range roster.Entries {
		p := e.PlayerPoolEntry.Player
		slotPosID := espnSlotToFantrax[e.LineupSlotID]
		// statusID matches Fantrax's encoding: "1"=active, "2"=bench/reserve,
		// "3"=IL, "9"=minors. ESPN active is anything not in BENCH/IL.
		statusID := "1"
		switch e.LineupSlotID {
		case espn.SlotBench:
			statusID = "2"
		case espn.SlotIL:
			statusID = "3"
		}
		positions := convertEligibleSlots(p.EligibleSlots)
		isPitcher := isPitcherEligible(positions)
		fp := e.PlayerPoolEntry.AppliedStatTotal
		hadGame := fp != 0
		out = append(out, fantrax.DayPlayerFP{
			PlayerID:      fmt.Sprintf("%d", p.ID),
			Name:          p.FullName,
			MLBTeam:       MLBTeamOrFA(p.ProTeamID),
			PosShortNames: posDisplay(positions, isPitcher),
			SlotPosID:     slotPosID,
			StatusID:      statusID,
			FPts:          fp,
			Active:        statusID == "1",
			HadGame:       hadGame,
			IsPitcher:     isPitcher,
			Positions:     positions,
		})
	}
	return out
}

// convertEligibleSlots maps ESPN's eligibleSlots IDs to the Fantrax position
// ID strings the optimizer expects. Outfield specifics (LF/CF/RF) collapse to
// the canonical OF; combo slots (UTIL, IF, P) are passed through to their
// Fantrax equivalents so the special-case eligibility checks fire correctly.
func convertEligibleSlots(slots []int) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range slots {
		fantraxID, ok := espnSlotToFantrax[s]
		if !ok {
			continue
		}
		// Skip non-position slots in eligibility output (BENCH, IL).
		switch s {
		case espn.SlotBench, espn.SlotIL:
			continue
		}
		if !seen[fantraxID] {
			seen[fantraxID] = true
			out = append(out, fantraxID)
		}
	}
	sort.Strings(out)
	return out
}

// isPitcherEligible mirrors fantrax's pitcher-position check by Fantrax IDs.
// Player has at least one of P/SP/RP if any of "015"/"016"/"017" appear.
func isPitcherEligible(positions []string) bool {
	for _, p := range positions {
		if p == "015" || p == "016" || p == "017" {
			return true
		}
	}
	return false
}

// posDisplay returns a comma-separated display label for a player's
// positions using project-canonical short names (matches fantrax output).
func posDisplay(positions []string, isPitcher bool) string {
	fantraxIDToName := map[string]string{
		"001": "C", "002": "1B", "003": "2B", "004": "3B", "005": "SS",
		"008": "INF", "012": "OF", "014": "UT",
		"015": "SP", "016": "RP", "017": "P",
	}
	var parts []string
	seen := map[string]bool{}
	for _, p := range positions {
		name, ok := fantraxIDToName[p]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, name)
	}
	return joinComma(parts)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// MLBTeamOrFA returns the MLBAM abbrev for the given ESPN proTeamId, or
// "FA" when 0/unknown. Re-exported equivalent of espn.MLBTeam[id] with the
// FA fallback already applied.
func MLBTeamOrFA(proTeamID int) string {
	if name, ok := espn.MLBTeam[proTeamID]; ok && name != "" {
		return name
	}
	return "FA"
}

// ---------------------------------------------------------------------------
// GetTeamPitcherStarts — derived from the same per-period box scores
// ---------------------------------------------------------------------------

func (a *espnRecapAdapter) GetTeamPitcherStarts(
	teamID string,
	start, end, seasonStart time.Time,
) ([]fantrax.DatedPitcherStart, error) {
	s, err := a.loadSeason()
	if err != nil {
		return nil, err
	}
	var teamIDInt int
	fmt.Sscanf(teamID, "%d", &teamIDInt)

	// Reuse the long-TTL box score cache via the same FileCache so we avoid
	// duplicate fetches when DailyFantasyPoints already touched these periods.
	c := cache.New[boxScoreCacheEntry](".cache", 30*24*time.Hour)

	var out []fantrax.DatedPitcherStart
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		period := s.PeriodForDate(d)
		if period <= 0 || period > s.LatestScoringPeriod {
			continue
		}
		key := fmt.Sprintf("espn-boxscore-%d-%d", s.SeasonID, period)
		entry, err := c.Get(key, func() (boxScoreCacheEntry, error) {
			matchups, ferr := a.client.GetBoxScoreForPeriod(period)
			if ferr != nil {
				return boxScoreCacheEntry{}, ferr
			}
			return boxScoreCacheEntry{Matchups: matchups}, nil
		})
		if err != nil {
			return nil, fmt.Errorf("box score period %d: %w", period, err)
		}
		roster := espn.FindTeamRoster(entry.Matchups, teamIDInt)
		if roster == nil {
			continue
		}
		for _, e := range roster.Entries {
			// Active pitcher slots only (P=13, SP=14). Skip BE/IL/RP for the
			// pitcher-starts series since recap uses this for SP highlights.
			if e.LineupSlotID != espn.SlotP && e.LineupSlotID != espn.SlotSP {
				continue
			}
			fp := e.PlayerPoolEntry.AppliedStatTotal
			if fp == 0 {
				continue // no-game day
			}
			// v1 limitation: we don't filter out relief appearances. ESPN's
			// stats array includes a GS count (statId 33) per scoring period
			// but exposing it through the typed boxscore decoder requires a
			// schema expansion. For now any nonzero-FPts SP/P-slot day counts
			// as a "start" — this slightly inflates Best/Worst single-start
			// awards in leagues where pitchers swing between SP and relief.
			p := e.PlayerPoolEntry.Player
			out = append(out, fantrax.DatedPitcherStart{
				PitcherName: p.FullName,
				Date:        d,
				FPts:        fp,
				MLBTeam:     MLBTeamOrFA(p.ProTeamID),
			})
		}
	}
	return out, nil
}
