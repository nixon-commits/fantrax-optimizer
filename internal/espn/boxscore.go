package espn

import (
	"fmt"
	"sort"
)

// boxScoreResponse is the trimmed mBoxscore shape — for one scoring period,
// every matchup contains a roster snapshot for both teams with per-player
// applied points for that exact day (via rosterForCurrentScoringPeriod).
type boxScoreResponse struct {
	Schedule []BoxScoreMatchup `json:"schedule"`
}

// BoxScoreMatchup is one head-to-head pairing with daily roster snapshots
// for the scoring period the request was made for.
type BoxScoreMatchup struct {
	ID              int          `json:"id"`
	MatchupPeriodID int          `json:"matchupPeriodId"`
	Home            BoxScoreSide `json:"home"`
	Away            BoxScoreSide `json:"away"`
}

// BoxScoreSide carries one team's roster for the scoring period plus the
// matchup-wide aggregate. RosterForCurrentScoringPeriod is the per-day
// roster snapshot — populated only for matchups whose matchup period
// contains the requested scoringPeriodId.
type BoxScoreSide struct {
	TeamID                        int          `json:"teamId"`
	TotalPoints                   float64      `json:"totalPoints"`
	RosterForCurrentScoringPeriod *RosterBlock `json:"rosterForCurrentScoringPeriod"`
	RosterForMatchupPeriod        *RosterBlock `json:"rosterForMatchupPeriod"`
}

// RosterBlock is the entries + aggregate the box score endpoint returns.
type RosterBlock struct {
	AppliedStatTotal float64       `json:"appliedStatTotal"`
	Entries          []RosterEntry `json:"entries"`
}

// RosterEntry is one player's slot + applied points for the requested period.
type RosterEntry struct {
	LineupSlotID    int             `json:"lineupSlotId"`
	PlayerID        int             `json:"playerId"`
	PlayerPoolEntry PlayerPoolEntry `json:"playerPoolEntry"`
}

// PlayerPoolEntry wraps the player record plus the applied stat total for
// the request's scoring period. AppliedStatTotal is the canonical "FPts
// for this day" field — already filtered by ESPN to that single period.
type PlayerPoolEntry struct {
	AppliedStatTotal float64    `json:"appliedStatTotal"`
	Player           playerNode `json:"player"`
}

// GetBoxScoreForPeriod fetches every team's per-day roster snapshot for a
// single scoring period. One ~500KB call returns all matchups; downstream
// callers filter by team. Caching by period is highly effective since past
// periods are immutable.
func (c *Client) GetBoxScoreForPeriod(scoringPeriod int) ([]BoxScoreMatchup, error) {
	if scoringPeriod <= 0 {
		return nil, fmt.Errorf("espn: scoringPeriod must be positive (got %d)", scoringPeriod)
	}
	views := []string{"mBoxscore"}
	u := c.leagueURL(views)
	u += fmt.Sprintf("&scoringPeriodId=%d", scoringPeriod)

	var raw boxScoreResponse
	if err := c.get(u, "", &raw); err != nil {
		return nil, fmt.Errorf("get box score for period %d: %w", scoringPeriod, err)
	}
	// Sort for determinism.
	sort.Slice(raw.Schedule, func(i, j int) bool {
		if raw.Schedule[i].MatchupPeriodID != raw.Schedule[j].MatchupPeriodID {
			return raw.Schedule[i].MatchupPeriodID < raw.Schedule[j].MatchupPeriodID
		}
		return raw.Schedule[i].Home.TeamID < raw.Schedule[j].Home.TeamID
	})
	return raw.Schedule, nil
}

// FindTeamRoster returns the rosterForCurrentScoringPeriod for the given
// teamID across all matchups, or nil if not found. Used to extract a
// single team's per-day roster from the league-wide box score response.
func FindTeamRoster(matchups []BoxScoreMatchup, teamID int) *RosterBlock {
	for _, m := range matchups {
		if m.Home.TeamID == teamID && m.Home.RosterForCurrentScoringPeriod != nil {
			return m.Home.RosterForCurrentScoringPeriod
		}
		if m.Away.TeamID == teamID && m.Away.RosterForCurrentScoringPeriod != nil {
			return m.Away.RosterForCurrentScoringPeriod
		}
	}
	return nil
}
