package fantrax

import (
	"sort"
	"time"

	"github.com/pmurley/go-fantrax/auth_client"
)

// MatchupWeekBounds returns the inclusive [start, end] calendar dates of the
// matchup week that contains date for the given fantasy teamID.
// It groups consecutive same-opponent matchup entries (which are weekly, not daily)
// and uses date ranges to determine which week the target date falls in.
// Returns zero times if no matchup week contains the date.
func MatchupWeekBounds(
	matchups []auth_client.Matchup,
	teamID string,
	seasonStart time.Time,
	date time.Time,
) (weekStart, weekEnd time.Time) {
	type entry struct {
		period   int
		opponent string
		date     time.Time
	}

	var mine []entry
	for _, m := range matchups {
		var opp string
		if m.AwayTeam.TeamID == teamID {
			opp = m.HomeTeam.TeamID
		} else if m.HomeTeam.TeamID == teamID {
			opp = m.AwayTeam.TeamID
		} else {
			continue
		}
		t, err := parseMatchupDate(m.Date)
		if err != nil {
			continue
		}
		mine = append(mine, entry{m.ScoringPeriod, opp, t})
	}

	sort.Slice(mine, func(i, j int) bool { return mine[i].date.Before(mine[j].date) })

	dateYMD := date.Format("2006-01-02")

	// Walk sorted entries and group consecutive same-opponent runs.
	i := 0
	for i < len(mine) {
		j := i + 1
		for j < len(mine) && mine[j].opponent == mine[i].opponent {
			j++
		}
		// Run is [i, j). The run starts on mine[i].date and ends the day
		// before the next run starts (or on the last entry's date if it's the
		// final run — we add 6 days as a reasonable week length).
		runStart := mine[i].date
		var runEnd time.Time
		if j < len(mine) {
			runEnd = mine[j].date.AddDate(0, 0, -1)
		} else {
			runEnd = mine[j-1].date.AddDate(0, 0, 6)
		}

		startYMD := runStart.Format("2006-01-02")
		endYMD := runEnd.Format("2006-01-02")
		if dateYMD >= startYMD && dateYMD <= endYMD {
			return runStart, runEnd
		}
		i = j
	}
	return time.Time{}, time.Time{}
}

// GetMatchupWeekBounds is a convenience method that fetches all matchups and
// returns the week boundaries for the given date.
func (c *Client) GetMatchupWeekBounds(date time.Time, seasonStart time.Time) (weekStart, weekEnd time.Time, err error) {
	result, err := c.auth.GetAllMatchups()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	ws, we := MatchupWeekBounds(result.Matchups, c.teamID, seasonStart, date)
	return ws, we, nil
}
