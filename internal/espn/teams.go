package espn

import "fmt"

// teamsResponse is the trimmed mTeam shape. ESPN returns much more on each
// team (records, points scored, draft data); recap only needs id/name/logo.
type teamsResponse struct {
	Teams []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Abbrev string `json:"abbrev"`
		Logo   string `json:"logo"`
	} `json:"teams"`
}

// GetLeagueTeams returns the league's teams as parallel maps:
//
//	teamID (string) → display name
//	teamID (string) → logo URL
//
// Team IDs are stringified so the rest of the project (which uses string
// teamIDs from Fantrax) doesn't have to dual-track types.
func (c *Client) GetLeagueTeams() (map[string]string, map[string]string, error) {
	var raw teamsResponse
	if err := c.get(c.leagueURL([]string{"mTeam"}), "", &raw); err != nil {
		return nil, nil, fmt.Errorf("get teams: %w", err)
	}
	names := make(map[string]string, len(raw.Teams))
	logos := make(map[string]string, len(raw.Teams))
	for _, t := range raw.Teams {
		id := fmt.Sprintf("%d", t.ID)
		names[id] = t.Name
		logos[id] = t.Logo
	}
	return names, logos, nil
}
