package projections

import (
	"github.com/nixon-commits/fantrax-optimizer/internal/fantrax"
	"github.com/pmurley/go-fantrax/auth_client"
)

const (
	spSteamerWeight = 0.85
	spRecentWeight  = 0.15
	rpSteamerWeight = 0.70
	rpRecentWeight  = 0.30
	minGPForBlend   = 4 // minimum games played before blending recent data
)

// PitcherPtsPerGameSource can provide a pre-computed pitcher points-per-game value.
type PitcherPtsPerGameSource interface {
	GetPitcherPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool)
}

// PitcherBlendedSource wraps a pitcher projection source and blends its per-game
// value with recent Fantrax pitching data. Uses role-aware weights:
// SPs get 85/15 Steamer/recent; RPs get 70/30.
type PitcherBlendedSource struct {
	inner      PitcherSource
	recent     map[string]fantrax.RecentStat
	scoring    fantrax.ScoringWeights
	nameToID   map[string]string   // NormalizeName(name) → player ID
	playerPos  map[string][]string // player ID → position IDs
}

func NewPitcherBlendedSource(
	inner PitcherSource,
	recent map[string]fantrax.RecentStat,
	scoring fantrax.ScoringWeights,
	nameToID map[string]string,
	playerPos map[string][]string,
) *PitcherBlendedSource {
	return &PitcherBlendedSource{
		inner: inner, recent: recent, scoring: scoring,
		nameToID: nameToID, playerPos: playerPos,
	}
}

// GetPitcherProjection delegates to the inner source.
func (b *PitcherBlendedSource) GetPitcherProjection(name, mlbTeam string) (*PitcherProjection, bool) {
	return b.inner.GetPitcherProjection(name, mlbTeam)
}

// GetPitcherPtsPerGame returns blended FP/G with role-aware weights.
// Falls back to 100% Steamer if no recent data or insufficient games.
func (b *PitcherBlendedSource) GetPitcherPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool) {
	proj, ok := b.inner.GetPitcherProjection(name, mlbTeam)
	if !ok || proj.G <= 0 {
		return 0, false
	}

	steamerPts := PitcherExpectedPtsFromProj(proj, scoring)

	playerID, idOK := b.nameToID[NormalizeName(name)]
	if !idOK {
		return steamerPts, true
	}

	recent, statOK := b.recent[playerID]
	if !statOK || recent.GamesPlayed < minGPForBlend {
		return steamerPts, true
	}

	recentPtsPerGame := recent.TotalFP / float64(recent.GamesPlayed)

	// Determine role from position eligibility.
	sw, rw := rpSteamerWeight, rpRecentWeight
	if isSPEligible(b.playerPos[playerID]) {
		sw, rw = spSteamerWeight, spRecentWeight
	}

	return sw*steamerPts + rw*recentPtsPerGame, true
}

// isSPEligible returns true if the player has SP position eligibility.
func isSPEligible(positions []string) bool {
	for _, pos := range positions {
		if pos == auth_client.PosSP { // "015"
			return true
		}
	}
	return false
}

// PitcherExpectedPtsFromProj computes per-game fantasy points from a pitcher projection.
func PitcherExpectedPtsFromProj(proj *PitcherProjection, scoring fantrax.ScoringWeights) float64 {
	if proj.G <= 0 {
		return 0
	}

	statMap := map[string]float64{
		"K":   proj.K,
		"BB":  proj.BBA,
		"H":   proj.HA,
		"ER":  proj.ER,
		"HR":  proj.HRA,
		"W":   proj.W,
		"L":   proj.L,
		"QS":  proj.QS,
		"SV":  proj.SV,
		"HLD": proj.HLD,
		"BS":  proj.BS,
		"IP":  proj.IP,
		"HBP": proj.HBP,
		"WP":  proj.WP,
		"BK":  proj.BK,
		"CG":  proj.CG,
		"SHO": proj.SHO,
		"PKO": proj.PKO,
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
