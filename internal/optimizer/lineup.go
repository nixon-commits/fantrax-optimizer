package optimizer

import (
	"sort"

	"github.com/nixon-commits/fantrax-optimizer/internal/fantrax"
	"github.com/nixon-commits/fantrax-optimizer/internal/projections"
)

// ScoredPlayer pairs a player with their expected fantasy points per game.
type ScoredPlayer struct {
	Player      fantrax.Player
	ExpectedPts float64
	HasGame     bool
}

// Result describes the lineup changes the optimizer wants to make.
type Result struct {
	ToActivate []fantrax.PlayerSlot
	ToBench    []string // player IDs to move to reserve
	Scored     []ScoredPlayer
}

// OptimizeLineup computes the optimal daily hitter lineup.
func OptimizeLineup(
	roster []fantrax.Player,
	playingToday map[string]bool,
	projSrc projections.Source,
	scoring fantrax.ScoringWeights,
	slots []fantrax.Slot,
) Result {
	scored := scoreRoster(roster, playingToday, projSrc, scoring)

	// Players with games first, then by expected pts descending.
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].HasGame != scored[j].HasGame {
			return scored[i].HasGame
		}
		return scored[i].ExpectedPts > scored[j].ExpectedPts
	})

	assigned := make(map[string]bool)
	var toActivate []fantrax.PlayerSlot

	for _, slot := range slots {
		for _, sp := range scored {
			if assigned[sp.Player.ID] {
				continue
			}
			if !fantrax.EligibleForSlot(sp.Player.Positions, slot) {
				continue
			}
			toActivate = append(toActivate, fantrax.PlayerSlot{
				PlayerID: sp.Player.ID,
				PosID:    slot.PosID,
			})
			assigned[sp.Player.ID] = true
			break
		}
	}

	// Anyone not assigned who was previously active gets benched.
	var toBench []string
	for _, p := range roster {
		if p.Status == "Active" && !assigned[p.ID] {
			toBench = append(toBench, p.ID)
		}
	}

	return Result{
		ToActivate: toActivate,
		ToBench:    toBench,
		Scored:     scored,
	}
}

func scoreRoster(
	roster []fantrax.Player,
	playingToday map[string]bool,
	projSrc projections.Source,
	scoring fantrax.ScoringWeights,
) []ScoredPlayer {
	scored := make([]ScoredPlayer, 0, len(roster))
	for _, p := range roster {
		hasGame := playingToday[p.MLBTeam]
		proj, ok := projSrc.GetProjection(p.Name, p.MLBTeam)
		var pts float64
		if ok && proj.G > 0 {
			pts = expectedPts(proj, scoring)
		}
		scored = append(scored, ScoredPlayer{
			Player:      p,
			ExpectedPts: pts,
			HasGame:     hasGame,
		})
	}
	return scored
}

// expectedPts converts a season projection to expected fantasy points per game.
// Handles derived stats: 1B (if not projected directly), XBH, TB.
func expectedPts(proj *projections.Projection, scoring fantrax.ScoringWeights) float64 {
	if proj.G <= 0 {
		return 0
	}

	// Derive stats that may not be directly in the projection.
	singles := proj.Singles
	if singles == 0 && proj.H > 0 {
		singles = proj.H - proj.Doubles - proj.Triples - proj.HR
	}
	xbh := proj.Doubles + proj.Triples + proj.HR
	tb := singles + 2*proj.Doubles + 3*proj.Triples + 4*proj.HR

	statMap := map[string]float64{
		"1B":   singles,
		"2B":   proj.Doubles,
		"3B":   proj.Triples,
		"HR":   proj.HR,
		"RBI":  proj.RBI,
		"R":    proj.R,
		"BB":   proj.BB,
		"SB":   proj.SB,
		"CS":   proj.CS,
		"HBP":  proj.HBP,
		"SO":   proj.SO,
		"GIDP": proj.GIDP,
		"XBH":  xbh,
		"TB":   tb,
	}

	var total float64
	for stat, seasonVal := range statMap {
		if pts, ok := scoring[stat]; ok {
			perGame := seasonVal / proj.G
			total += perGame * pts
		}
	}
	return total
}
