package fantrax

import (
	"log"
	"strconv"

	"github.com/pmurley/go-fantrax/models"
	"golang.org/x/sync/errgroup"
)

// RecentStat holds aggregated fantasy-point and games-played totals for a player
// across one or more recent scoring periods.
type RecentStat struct {
	TotalFP     float64
	GamesPlayed int
}

// aggregateRecentStats combines per-player stats across multiple periods.
// Each element of periods is a flat slice of RosterPlayer entries for that period.
// Nil Stats / Batting / FantasyPointsPerGame are skipped safely.
func aggregateRecentStats(periods [][]models.RosterPlayer) map[string]RecentStat {
	result := make(map[string]RecentStat)

	for _, period := range periods {
		for _, rp := range period {
			if rp.Stats == nil || rp.Stats.Batting == nil {
				continue
			}
			b := rp.Stats.Batting
			if b.GamesPlayed == nil {
				continue
			}

			stat := result[rp.PlayerID]

			gp := *b.GamesPlayed
			stat.GamesPlayed += gp

			if b.FantasyPointsPerGame != nil && gp > 0 {
				stat.TotalFP += *b.FantasyPointsPerGame * float64(gp)
			}

			result[rp.PlayerID] = stat
		}
	}

	return result
}

// GetCurrentPeriod returns the current Fantrax scoring period number.
func (c *Client) GetCurrentPeriod() (int, error) {
	return c.auth.GetCurrentPeriod()
}

// GetRecentStats fetches roster data for the last numPeriods scoring periods
// and aggregates per-player stats. Periods are fetched in parallel via errgroup.
func (c *Client) GetRecentStats(currentPeriod, numPeriods int) (map[string]RecentStat, error) {
	// Collect valid period numbers (count backwards, skip <= 0).
	var periodNums []int
	for p := currentPeriod - 1; p >= currentPeriod-numPeriods && p > 0; p-- {
		periodNums = append(periodNums, p)
	}

	results := make([][]models.RosterPlayer, len(periodNums))

	var g errgroup.Group
	for i, p := range periodNums {
		i, p := i, p // capture loop vars
		g.Go(func() error {
			roster, err := c.auth.GetTeamRosterInfo(strconv.Itoa(p), c.teamID)
			if err != nil {
				log.Printf("warning: failed to fetch roster for period %d: %v", p, err)
				return nil // non-fatal; leave results[i] as nil
			}
			results[i] = append(roster.ActiveRoster, roster.ReserveRoster...)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Filter out nil slices from failed periods.
	var periods [][]models.RosterPlayer
	for _, r := range results {
		if r != nil {
			periods = append(periods, r)
		}
	}

	return aggregateRecentStats(periods), nil
}
