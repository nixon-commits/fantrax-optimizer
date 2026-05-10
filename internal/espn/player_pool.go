package espn

import (
	"encoding/json"
	"fmt"
)

// playerPoolResponse is the league response shape when `view=kona_player_info`
// is requested. Each entry in `players` wraps the player record under a
// `player` envelope (same as the roster endpoint).
type playerPoolResponse struct {
	Players []struct {
		Player playerNode `json:"player"`
	} `json:"players"`
}

// freeAgentFilter is the X-Fantasy-Filter JSON header that asks ESPN to
// return only free agents (status FREEAGENT or WAIVERS) for the given
// scoring period, sorted by percent-owned descending, capped at limit.
// Shape matches cwendt94/espn-api's working query — ESPN rejects the
// request with "Filter: Value missing" if filterRanksForScoringPeriodIds
// is absent.
//
// limit defaults to 1500 which comfortably covers every relevant FA in a
// 12-team league (the long tail past 1500 is unrostered minor leaguers).
func freeAgentFilter(limit, scoringPeriodID int) string {
	if limit <= 0 {
		limit = 1500
	}
	body := map[string]any{
		"players": map[string]any{
			"filterStatus": map[string]any{
				"value": []string{"FREEAGENT", "WAIVERS"},
			},
			"filterRanksForScoringPeriodIds": map[string]any{
				"value": []int{scoringPeriodID},
			},
			"limit": limit,
			"sortPercOwned": map[string]any{
				"sortAsc":      false,
				"sortPriority": 1,
			},
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// GetFreeAgents returns every MLB-level free agent in the league. The
// returned slice is filtered to OnTeamID==0 as a defense in depth; the
// X-Fantasy-Filter has already narrowed the response to FREEAGENT/WAIVERS
// status players from this league.
//
// limit caps the number of FAs returned; 0 uses the package default (1500).
// scoringPeriodID is required by ESPN's filter; pass 0 to default to 1
// (opening day) — for a FA query the period only affects the player ranks
// included in the response, not the FA list itself.
func (c *Client) GetFreeAgents(limit int) ([]Player, error) {
	// scoringPeriodId is required by the filter even though it doesn't
	// influence the FA membership; period 1 (opening day) is a stable choice.
	filter := freeAgentFilter(limit, 1)
	var raw playerPoolResponse
	if err := c.get(c.freeAgentsURL(), filter, &raw); err != nil {
		return nil, fmt.Errorf("get free agents: %w", err)
	}
	out := make([]Player, 0, len(raw.Players))
	for _, row := range raw.Players {
		// onTeamID 0 marks a true free agent; ESPN occasionally returns
		// players on rosters when the filter is loose. Defensive double-check.
		if row.Player.OnTeamID != 0 {
			continue
		}
		out = append(out, playerFromNode(row.Player, 0))
	}
	return out, nil
}

// teamRosterURL is unused but kept for parity; left out to avoid dead-code
// lint warnings. Use leagueURL with mRoster instead.
var _ = itoa
