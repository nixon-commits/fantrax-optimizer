package espn

import (
	"fmt"
	"sort"
	"time"
)

// SeasonInfo bundles per-league season metadata that recap needs:
// the season's first/latest scoring periods, the matchup-period → scoring-
// period mapping, and the active slot configuration. Single mSettings call
// covers all of it.
type SeasonInfo struct {
	SeasonID             int
	FirstScoringPeriod   int           // typically 1 = opening day
	LatestScoringPeriod  int           // most recent period with data (today or recent)
	CurrentMatchupPeriod int           // matchup currently in progress
	MatchupPeriodCount   int           // total matchups in the regular season
	MatchupPeriods       map[int][]int // matchupPeriodId → [scoringPeriodId, ...]
	SlotCounts           map[int]int   // lineupSlotId → number of slots
	SeasonStart          time.Time     // calendar date of scoring period 1 (UTC midnight)
}

// settingsResponseFull is the wider mSettings shape used by recap. The
// minimal one in league.go (used by waivers) only needs scoring; recap
// also needs status + scheduleSettings.
type settingsResponseFull struct {
	SeasonID int `json:"seasonId"`
	Status   struct {
		FirstScoringPeriod   int `json:"firstScoringPeriod"`
		LatestScoringPeriod  int `json:"latestScoringPeriod"`
		CurrentMatchupPeriod int `json:"currentMatchupPeriod"`
	} `json:"status"`
	Settings struct {
		ScheduleSettings struct {
			MatchupPeriodCount int              `json:"matchupPeriodCount"`
			MatchupPeriods     map[string][]int `json:"matchupPeriods"`
		} `json:"scheduleSettings"`
		RosterSettings struct {
			LineupSlotCounts map[string]int `json:"lineupSlotCounts"`
		} `json:"rosterSettings"`
	} `json:"settings"`
}

// GetSeasonInfo fetches mSettings (for slot config, season anchor) and
// mScoreboard+mMatchupScore (for the real matchup → scoring period mapping).
// The settings.scheduleSettings.matchupPeriods field is unreliable in MLB
// leagues — it lists each matchup period as containing a single scoring
// period even when the matchup actually spans 7+ daily periods. The truth
// lives in each matchup's pointsByScoringPeriod keys, which we derive here.
//
// SeasonStart is computed by walking back from today to scoring period 1.
// Past periods are immutable so this anchor is stable per-call.
func (c *Client) GetSeasonInfo(today time.Time) (*SeasonInfo, error) {
	var raw settingsResponseFull
	if err := c.get(c.leagueURL([]string{"mSettings"}), "", &raw); err != nil {
		return nil, fmt.Errorf("get season info: %w", err)
	}

	slots := make(map[int]int, len(raw.Settings.RosterSettings.LineupSlotCounts))
	for k, v := range raw.Settings.RosterSettings.LineupSlotCounts {
		var n int
		fmt.Sscanf(k, "%d", &n)
		slots[n] = v
	}

	out := &SeasonInfo{
		SeasonID:             raw.SeasonID,
		FirstScoringPeriod:   raw.Status.FirstScoringPeriod,
		LatestScoringPeriod:  raw.Status.LatestScoringPeriod,
		CurrentMatchupPeriod: raw.Status.CurrentMatchupPeriod,
		MatchupPeriodCount:   raw.Settings.ScheduleSettings.MatchupPeriodCount,
		SlotCounts:           slots,
	}

	if out.LatestScoringPeriod > 0 {
		todayMidnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		out.SeasonStart = todayMidnight.AddDate(0, 0, -(out.LatestScoringPeriod - 1))
	}

	// Build MatchupPeriods from the actual schedule. mMatchupScore on the
	// latest period populates pointsByScoringPeriod for every past matchup,
	// so a single call covers the whole season.
	mp, err := c.fetchMatchupPeriodMap(out.LatestScoringPeriod)
	if err != nil {
		return nil, fmt.Errorf("derive matchup period map: %w", err)
	}
	out.MatchupPeriods = mp
	return out, nil
}

// fetchMatchupPeriodMap calls mMatchupScore and walks each matchup's
// pointsByScoringPeriod to build the matchupPeriodId → []scoringPeriodId map.
// Each matchup period appears once even though the schedule contains one
// entry per (home, away) pair within it.
func (c *Client) fetchMatchupPeriodMap(latestScoringPeriod int) (map[int][]int, error) {
	matchups, err := c.GetMatchupScores(latestScoringPeriod)
	if err != nil {
		return nil, err
	}
	out := map[int]map[int]bool{}
	for _, m := range matchups {
		if _, ok := out[m.MatchupPeriodID]; !ok {
			out[m.MatchupPeriodID] = map[int]bool{}
		}
		// Both sides should agree on the period set; iterate both to be safe.
		for _, side := range []Side{m.Home, m.Away} {
			for k := range side.PointsByScoringPeriod {
				var p int
				if _, err := fmt.Sscanf(k, "%d", &p); err == nil && p > 0 {
					out[m.MatchupPeriodID][p] = true
				}
			}
		}
	}
	mp := make(map[int][]int, len(out))
	for id, periods := range out {
		ids := make([]int, 0, len(periods))
		for p := range periods {
			ids = append(ids, p)
		}
		sort.Ints(ids)
		mp[id] = ids
	}
	return mp, nil
}

// PeriodForDate returns the 1-indexed scoring period for the given calendar
// date relative to the season start. Returns 0 when date is before opening day.
func (s *SeasonInfo) PeriodForDate(date time.Time) int {
	if s.SeasonStart.IsZero() || date.Before(s.SeasonStart) {
		return 0
	}
	days := int(date.Sub(s.SeasonStart).Hours() / 24)
	return days + 1
}

// DateForPeriod returns the calendar date of the given 1-indexed scoring period.
func (s *SeasonInfo) DateForPeriod(period int) time.Time {
	if period <= 0 || s.SeasonStart.IsZero() {
		return time.Time{}
	}
	return s.SeasonStart.AddDate(0, 0, period-1)
}

// MatchupPeriodForScoringPeriod returns the 1-indexed matchup period that
// contains the given scoring period, or 0 if not found.
func (s *SeasonInfo) MatchupPeriodForScoringPeriod(period int) int {
	for mp, periods := range s.MatchupPeriods {
		for _, p := range periods {
			if p == period {
				return mp
			}
		}
	}
	return 0
}

// MatchupPeriodBounds returns the first and last calendar dates of the
// given matchup period (the bounds of its scoring periods).
func (s *SeasonInfo) MatchupPeriodBounds(matchupPeriod int) (time.Time, time.Time) {
	periods, ok := s.MatchupPeriods[matchupPeriod]
	if !ok || len(periods) == 0 {
		return time.Time{}, time.Time{}
	}
	return s.DateForPeriod(periods[0]), s.DateForPeriod(periods[len(periods)-1])
}
