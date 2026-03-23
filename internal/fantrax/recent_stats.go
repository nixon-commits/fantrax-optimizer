package fantrax

import (
	"github.com/pmurley/go-fantrax/models"
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
