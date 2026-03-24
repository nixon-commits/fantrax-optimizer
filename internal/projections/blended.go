package projections

import (
	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

const (
	steamerWeight       = 0.60
	recentWeight        = 0.40
	minGPForHitterBlend = 4 // require at least 4 games before blending recent stats
)

// PtsPerGameSource can provide a pre-computed points-per-game value.
type PtsPerGameSource interface {
	GetPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool)
}

// BlendedSource wraps a projection source and blends its per-game value
// with recent Fantrax scoring data.
type BlendedSource struct {
	inner    Source
	recent   map[string]fantrax.RecentStat
	scoring  fantrax.ScoringWeights
	nameToID map[string]string // NormalizeName(name) → player ID
}

func NewBlendedSource(
	inner Source,
	recent map[string]fantrax.RecentStat,
	scoring fantrax.ScoringWeights,
	nameToID map[string]string,
) *BlendedSource {
	return &BlendedSource{inner: inner, recent: recent, scoring: scoring, nameToID: nameToID}
}

// GetProjection delegates to the inner source.
func (b *BlendedSource) GetProjection(name, mlbTeam string) (*Projection, bool) {
	return b.inner.GetProjection(name, mlbTeam)
}

// GetPtsPerGame returns blended FP/G: 60% Steamer + 40% recent.
// Falls back to 100% Steamer if no recent data. Returns false if no Steamer projection.
func (b *BlendedSource) GetPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool) {
	proj, ok := b.inner.GetProjection(name, mlbTeam)
	if !ok || proj.G <= 0 {
		return 0, false
	}

	steamerPts := ExpectedPtsFromProj(proj, scoring)

	playerID, idOK := b.nameToID[NormalizeName(name)]
	if !idOK {
		return steamerPts, true
	}

	recent, statOK := b.recent[playerID]
	if !statOK || recent.GamesPlayed < minGPForHitterBlend {
		return steamerPts, true
	}

	recentPtsPerGame := recent.TotalFP / float64(recent.GamesPlayed)
	return steamerWeight*steamerPts + recentWeight*recentPtsPerGame, true
}

// ExpectedPtsFromProj computes per-game fantasy points from a projection.
// This is the canonical implementation; optimizer.expectedPts delegates here.
func ExpectedPtsFromProj(proj *Projection, scoring fantrax.ScoringWeights) float64 {
	if proj.G <= 0 {
		return 0
	}
	singles := proj.Singles
	if singles == 0 && proj.H > 0 {
		singles = proj.H - proj.Doubles - proj.Triples - proj.HR
	}
	xbh := proj.Doubles + proj.Triples + proj.HR
	tb := singles + 2*proj.Doubles + 3*proj.Triples + 4*proj.HR

	statMap := map[string]float64{
		"1B": singles, "2B": proj.Doubles, "3B": proj.Triples,
		"HR": proj.HR, "RBI": proj.RBI, "R": proj.R,
		"BB": proj.BB, "SB": proj.SB, "CS": proj.CS,
		"HBP": proj.HBP, "SO": proj.SO, "GIDP": proj.GIDP,
		"XBH": xbh, "TB": tb,
	}

	var total float64
	for stat, seasonVal := range statMap {
		if pts, ok := scoring[stat]; ok {
			total += (seasonVal / proj.G) * pts
		}
	}
	return total
}
