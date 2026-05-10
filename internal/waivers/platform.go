package waivers

// FreeAgent is the platform-neutral free-agent representation consumed by
// Run. Both *fantrax.Client (via the cmd-layer adapter) and *espn.Client
// (via its adapter) project their native types into this shape.
//
// Positions are canonical short names: "C", "1B", "2B", "3B", "SS", "OF",
// "DH", "SP", "RP". The position eligibility filter and signal-tagging
// dispatch (hitter vs pitcher path) key off these names exactly.
type FreeAgent struct {
	Name      string
	MLBTeam   string   // 3-letter MLBAM abbrev, e.g. "NYY"
	Positions []string // canonical short names
	Display   string   // optional pretty label for the report (e.g. "1B,3B");
	// when empty, the report falls back to joining Positions.
}

// RosteredPlayer is one player on the user's team — used to compute the
// drop-target floor (worstRosteredHitter / worstRosteredPitcher).
type RosteredPlayer struct {
	Name      string
	MLBTeam   string
	InMinors  bool
	IsInjured bool
}

// Platform is the read-only surface waivers needs from a fantasy provider.
// Adapters live in cmd/ so internal/fantrax stays untouched and internal/espn
// stays free of waivers-specific shapes.
//
// All methods are read-only. The free-agent list is already filtered to MLB
// FAs (no minor-league-only players); the position filter (--positions) is
// applied by Run itself on the canonical Positions field.
type Platform interface {
	// GetFreeAgents returns every MLB free agent in the league.
	GetFreeAgents() ([]FreeAgent, error)
	// GetHitterScoringWeights returns the league's hitter scoring config
	// (stat short-name → points), suitable for ExpectedPtsFromProj.
	GetHitterScoringWeights() (map[string]float64, error)
	// GetPitcherScoringWeights returns the league's pitcher scoring config.
	GetPitcherScoringWeights() (map[string]float64, error)
	// GetHitterRoster returns the hitters on the user's team (active+reserve).
	GetHitterRoster() ([]RosteredPlayer, error)
	// GetPitcherRoster returns the pitchers on the user's team (active+reserve).
	GetPitcherRoster() ([]RosteredPlayer, error)
}
