package fantrax

import (
	"fmt"
	"strconv"

	"github.com/pmurley/go-fantrax/models"
)

// extractPitcherStats extracts per-player pitching stats from a single roster snapshot.
// The Fantrax API returns cumulative YTD stats regardless of period requested.
// Nil Stats / Pitching / FantasyPointsPerGame are skipped safely.
func extractPitcherStats(roster []models.RosterPlayer) map[string]RecentStat {
	result := make(map[string]RecentStat)

	for _, rp := range roster {
		if rp.Stats == nil || rp.Stats.Pitching == nil {
			continue
		}
		p := rp.Stats.Pitching
		if p.GamesPlayed == nil {
			continue
		}

		gp := *p.GamesPlayed
		stat := RecentStat{GamesPlayed: gp}

		if p.FantasyPointsPerGame != nil && gp > 0 {
			stat.FPtsPerGame = *p.FantasyPointsPerGame
		}

		result[rp.PlayerID] = stat
	}

	return result
}

// GetRecentPitcherStats fetches the most recent completed period's roster and
// returns the cumulative season-to-date pitching stats for each player.
func (c *Client) GetRecentPitcherStats(currentPeriod, _ int) (map[string]RecentStat, error) {
	period := currentPeriod - 1
	if period < 1 {
		return nil, fmt.Errorf("no completed periods (current=%d)", currentPeriod)
	}

	roster, err := c.auth.GetTeamRosterInfo(strconv.Itoa(period), c.teamID)
	if err != nil {
		return nil, fmt.Errorf("fetch roster for period %d: %w", period, err)
	}

	players := append(roster.ActiveRoster, roster.ReserveRoster...)
	return extractPitcherStats(players), nil
}
