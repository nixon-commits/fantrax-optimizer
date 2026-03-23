package fantrax

import (
	"log"
	"strconv"

	"github.com/pmurley/go-fantrax/models"
	"golang.org/x/sync/errgroup"
)

// aggregateRecentPitcherStats combines per-player pitching stats across multiple periods.
// Nil Stats / Pitching / FantasyPointsPerGame are skipped safely.
func aggregateRecentPitcherStats(periods [][]models.RosterPlayer) map[string]RecentStat {
	result := make(map[string]RecentStat)

	for _, period := range periods {
		for _, rp := range period {
			if rp.Stats == nil || rp.Stats.Pitching == nil {
				continue
			}
			p := rp.Stats.Pitching
			if p.GamesPlayed == nil {
				continue
			}

			stat := result[rp.PlayerID]

			gp := *p.GamesPlayed
			stat.GamesPlayed += gp

			if p.FantasyPointsPerGame != nil && gp > 0 {
				stat.TotalFP += *p.FantasyPointsPerGame * float64(gp)
			}

			result[rp.PlayerID] = stat
		}
	}

	return result
}

// GetRecentPitcherStats fetches roster data for the last numPeriods scoring periods
// and aggregates per-player pitching stats. Periods are fetched in parallel via errgroup.
func (c *Client) GetRecentPitcherStats(currentPeriod, numPeriods int) (map[string]RecentStat, error) {
	var periodNums []int
	for p := currentPeriod - 1; p >= currentPeriod-numPeriods && p > 0; p-- {
		periodNums = append(periodNums, p)
	}

	results := make([][]models.RosterPlayer, len(periodNums))

	var g errgroup.Group
	for i, p := range periodNums {
		i, p := i, p
		g.Go(func() error {
			roster, err := c.auth.GetTeamRosterInfo(strconv.Itoa(p), c.teamID)
			if err != nil {
				log.Printf("warning: failed to fetch roster for period %d: %v", p, err)
				return nil
			}
			results[i] = append(roster.ActiveRoster, roster.ReserveRoster...)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var periods [][]models.RosterPlayer
	for _, r := range results {
		if r != nil {
			periods = append(periods, r)
		}
	}

	return aggregateRecentPitcherStats(periods), nil
}
