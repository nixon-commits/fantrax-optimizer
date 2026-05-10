package espn

// Player is the platform-neutral representation of an ESPN player as the
// waivers command consumes it. We don't share this type with internal/fantrax
// — the waivers Platform interface defines its own neutral types and the
// adapter in cmd/ converts.
type Player struct {
	ID           int
	Name         string
	MLBTeam      string   // 3-letter MLB abbreviation, e.g. "NYY"; "FA" if proTeamId is 0
	Positions    []string // canonical short names: ["OF"], ["SP"], ["1B","DH"]
	OnTeamID     int      // 0 if free agent
	InjuryStatus string   // ESPN's injury status string, e.g. "ACTIVE", "IL10", "DAY_TO_DAY"
	OwnershipPct float64  // % of leagues that have rostered the player
}

// FreeAgent flag derived from OnTeamID == 0.
func (p Player) IsFreeAgent() bool { return p.OnTeamID == 0 }

// IsInjured reports whether ESPN's injury status indicates the player is
// unavailable. Consumed by the waivers drop-target floor calculation.
func (p Player) IsInjured() bool {
	switch p.InjuryStatus {
	case "", "ACTIVE", "NORMAL":
		return false
	}
	return true
}

// Settings is the league-wide scoring + slot configuration parsed from the
// mSettings view. Only the fields waivers needs are populated.
type Settings struct {
	HitterWeights  map[string]float64 // stat short-name → points (hitting only)
	PitcherWeights map[string]float64 // stat short-name → points (pitching only)
	SeasonID       int
}
