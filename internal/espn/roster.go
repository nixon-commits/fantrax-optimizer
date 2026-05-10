package espn

import "fmt"

// rosterResponse is the trimmed mRoster+mTeam schema. Each team carries a
// roster with entries keyed by playerId; we flatten to []Player.
type rosterResponse struct {
	Teams []struct {
		ID     int `json:"id"`
		Roster struct {
			Entries []rosterEntry `json:"entries"`
		} `json:"roster"`
	} `json:"teams"`
}

type rosterEntry struct {
	LineupSlotID    int `json:"lineupSlotId"`
	PlayerID        int `json:"playerId"`
	PlayerPoolEntry struct {
		Player playerNode `json:"player"`
	} `json:"playerPoolEntry"`
}

// playerNode is the shared subset of fields ESPN includes on player records
// across mRoster and the players endpoint. Both responses use the same
// underlying schema.
type playerNode struct {
	ID                int     `json:"id"`
	FullName          string  `json:"fullName"`
	ProTeamID         int     `json:"proTeamId"`
	DefaultPositionID int     `json:"defaultPositionId"`
	EligibleSlots     []int   `json:"eligibleSlots"`
	OnTeamID          int     `json:"onTeamId"`
	InjuryStatus      string  `json:"injuryStatus"`
	OwnershipPct      float64 `json:"ownership.percentOwned"` // ignore — wrong tag depth; populated below
	Ownership         struct {
		PercentOwned float64 `json:"percentOwned"`
	} `json:"ownership"`
}

// GetTeamRoster returns every player on the configured team ID.
// teamID overrides the client's default; pass 0 to use c.teamID.
func (c *Client) GetTeamRoster(teamID int) ([]Player, error) {
	if teamID == 0 {
		teamID = c.teamID
	}
	if teamID == 0 {
		return nil, fmt.Errorf("espn: team ID required (set ESPN_TEAM_ID or pass explicitly)")
	}

	// mRoster + mTeam both required: mRoster gives us roster entries,
	// mTeam gives us the team list to filter by ID.
	var raw rosterResponse
	if err := c.get(c.leagueURL([]string{"mRoster", "mTeam"}), "", &raw); err != nil {
		return nil, fmt.Errorf("get team roster: %w", err)
	}

	var out []Player
	for _, team := range raw.Teams {
		if team.ID != teamID {
			continue
		}
		for _, e := range team.Roster.Entries {
			out = append(out, playerFromNode(e.PlayerPoolEntry.Player, team.ID))
		}
	}
	return out, nil
}

// playerFromNode converts the raw ESPN schema into the package's Player
// type. eligibleSlots → canonical position names via SlotName; defaultPositionId
// is used as a fallback when no eligibility info is present.
func playerFromNode(n playerNode, onTeamID int) Player {
	p := Player{
		ID:           n.ID,
		Name:         n.FullName,
		MLBTeam:      MLBTeam[n.ProTeamID],
		OnTeamID:     onTeamID,
		InjuryStatus: n.InjuryStatus,
		OwnershipPct: n.Ownership.PercentOwned,
	}
	if p.MLBTeam == "" {
		p.MLBTeam = "FA"
	}
	// EligibleSlots is the canonical source — a player who's "1B/3B-eligible"
	// will have both Slot1B and Slot3B in the list. Fall back to defaultPosition
	// when ESPN doesn't return eligibilities (rare; happens for newly-rostered
	// players whose pool entry is sparse).
	//
	// Normalization rules:
	//  - Outfielders: ESPN returns LF/CF/RF as distinct slots in addition to
	//    the generic OF slot. We collapse all four to a single "OF" so the
	//    rest of the project sees the canonical short name it expects.
	//  - Combo and non-position slots (UTIL, P, IF, 2B/SS, 1B/3B, BENCH, IL)
	//    are skipped — they don't represent base position eligibility.
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		p.Positions = append(p.Positions, name)
	}
	for _, slot := range n.EligibleSlots {
		switch slot {
		case SlotLF, SlotCF, SlotRF, SlotOF:
			add("OF")
			continue
		case SlotUTIL, SlotP, SlotIF, Slot2BSS, Slot1B3B, SlotBench, SlotIL:
			continue
		}
		if name, ok := SlotName[slot]; ok {
			add(name)
		}
	}
	if len(p.Positions) == 0 {
		if name, ok := PositionName[n.DefaultPositionID]; ok {
			p.Positions = []string{name}
		}
	}
	return p
}
