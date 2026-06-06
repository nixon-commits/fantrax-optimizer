package projections

import (
	"math"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
	scoringpkg "github.com/nixon-commits/rosterbot/internal/scoring"
)

const (
	hitterStabilizationPA = 250.0 // 50/50 at ~66 GP (roughly mid-June)
	hitterPAPerGame       = 3.8   // approximate PA per game played
	hitterBaseFloor       = 0.30  // base projection never drops below 30%
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
	minGP    int
}

func NewBlendedSource(
	inner Source,
	recent map[string]fantrax.RecentStat,
	scoring fantrax.ScoringWeights,
	nameToID map[string]string,
	minGP int,
) *BlendedSource {
	return &BlendedSource{inner: inner, recent: recent, scoring: scoring, nameToID: nameToID, minGP: minGP}
}

// GetProjection delegates to the inner source.
func (b *BlendedSource) GetProjection(name, mlbTeam string) (*Projection, bool) {
	return b.inner.GetProjection(name, mlbTeam)
}

// GetPtsPerGame returns blended FP/G using PA-based dynamic weights.
// Falls back to 100% base projection if no recent data.
func (b *BlendedSource) GetPtsPerGame(name, mlbTeam string, scoring fantrax.ScoringWeights) (float64, bool) {
	proj, hasProj := b.inner.GetProjection(name, mlbTeam)

	playerID, idOK := b.nameToID[NormalizeName(name)]
	var recent fantrax.RecentStat
	var hasRecent bool
	if idOK {
		recent, hasRecent = b.recent[playerID]
		hasRecent = hasRecent && recent.GamesPlayed >= b.minGP
	}

	if !hasProj || proj.G <= 0 {
		// No base projection — use recent stats only if available.
		if hasRecent {
			return recent.FPtsPerGame, true
		}
		return 0, false
	}

	basePts := ExpectedPtsFromProj(proj, scoring)
	if !hasRecent {
		return basePts, true
	}

	sw, rw := hitterBlendWeights(recent.GamesPlayed)
	return sw*basePts + rw*recent.FPtsPerGame, true
}

// hitterBlendWeights computes dynamic base/recent weights based on games played.
func hitterBlendWeights(gamesPlayed int) (base, season float64) {
	approxPA := float64(gamesPlayed) * hitterPAPerGame
	seasonWeight := approxPA / (approxPA + hitterStabilizationPA)
	base = math.Max(1-seasonWeight, hitterBaseFloor)
	season = 1 - base
	return
}

// HitterBlendWeightsForDisplay returns the base/season weight percentages for display.
func HitterBlendWeightsForDisplay(gamesPlayed int) (basePct, seasonPct float64) {
	return hitterBlendWeights(gamesPlayed)
}

// HitterBreakdown holds the individual components of a blended hitter score.
type HitterBreakdown struct {
	BasePts     float64
	RecentFPG   float64
	BaseWt      float64
	RecentWt    float64
	BlendedPts  float64
	GamesPlayed int
	HasRecent   bool // true if recent data was used in blending
}

// GetHitterBreakdown returns the blending components for a player.
// Returns nil if the player has no base projection.
func (b *BlendedSource) GetHitterBreakdown(name, mlbTeam string, scoring fantrax.ScoringWeights) *HitterBreakdown {
	proj, ok := b.inner.GetProjection(name, mlbTeam)
	if !ok || proj.G <= 0 {
		return nil
	}

	basePts := ExpectedPtsFromProj(proj, scoring)
	bd := &HitterBreakdown{
		BasePts:    basePts,
		BaseWt:     1.0,
		BlendedPts: basePts,
	}

	playerID, idOK := b.nameToID[NormalizeName(name)]
	if !idOK {
		return bd
	}

	recent, statOK := b.recent[playerID]
	if !statOK || recent.GamesPlayed < b.minGP {
		return bd
	}

	bd.HasRecent = true
	bd.GamesPlayed = recent.GamesPlayed
	bd.RecentFPG = recent.FPtsPerGame
	bd.BaseWt, bd.RecentWt = hitterBlendWeights(recent.GamesPlayed)
	bd.BlendedPts = bd.BaseWt*basePts + bd.RecentWt*bd.RecentFPG
	return bd
}

// ExpectedPtsFromProj computes per-game fantasy points from a projection by
// adapting it to a scoring.HitterLine and dividing the season total by games.
// 1B/XBH/TB are derived inside scoring.ApplyHitter (1B from H, not the
// FanGraphs-supplied Singles — the two are equal by construction).
func ExpectedPtsFromProj(proj *Projection, scoring fantrax.ScoringWeights) float64 {
	if proj.G <= 0 {
		return 0
	}
	line := scoringpkg.HitterLine{
		H: proj.H, Doubles: proj.Doubles, Triples: proj.Triples, HR: proj.HR,
		RBI: proj.RBI, R: proj.R, BB: proj.BB, SB: proj.SB, CS: proj.CS,
		HBP: proj.HBP, SO: proj.SO, GIDP: proj.GIDP,
	}
	return scoringpkg.ApplyHitter(line, scoring) / proj.G
}
