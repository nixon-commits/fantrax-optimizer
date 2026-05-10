package espn

import "fmt"

// matchupSchedule is the trimmed mScoreboard shape. Each schedule entry is
// one matchup (home vs away) tied to a matchupPeriodId.
type matchupSchedule struct {
	Schedule []ScheduleMatchup `json:"schedule"`
}

// ScheduleMatchup is one head-to-head pairing for a matchup period.
type ScheduleMatchup struct {
	ID              int  `json:"id"`
	MatchupPeriodID int  `json:"matchupPeriodId"`
	Home            Side `json:"home"`
	Away            Side `json:"away"`
}

// Side is one team's representation inside a matchup.
type Side struct {
	TeamID      int     `json:"teamId"`
	TotalPoints float64 `json:"totalPoints"`
	// PointsByScoringPeriod maps scoringPeriodId (string-keyed in JSON) to
	// the team's point total for that day. Populated by mMatchupScore;
	// absent or empty for future matchups.
	PointsByScoringPeriod map[string]float64 `json:"pointsByScoringPeriod"`
}

// GetMatchups returns every scheduled matchup for the season with matchupPeriodId
// populated. NOTE: the `mScoreboard` view returns the schedule but leaves
// matchupPeriodId as 0 on every entry — useless for grouping. `mMatchupScore`
// populates it correctly, so we use that view here.
func (c *Client) GetMatchups() ([]ScheduleMatchup, error) {
	var raw matchupSchedule
	if err := c.get(c.leagueURL([]string{"mMatchupScore"}), "", &raw); err != nil {
		return nil, fmt.Errorf("get matchups: %w", err)
	}
	return raw.Schedule, nil
}

// GetMatchupScores returns the schedule with each side's pointsByScoringPeriod
// populated for periods within the given matchup. Used by recap to derive
// daily team totals when per-player box scores aren't needed.
func (c *Client) GetMatchupScores(matchupPeriod int) ([]ScheduleMatchup, error) {
	views := []string{"mMatchupScore"}
	u := c.leagueURL(views)
	// scoringPeriodId scopes which periods get pointsByScoringPeriod populated.
	// Pass the last period of the matchup; ESPN backfills the prior days.
	u += fmt.Sprintf("&scoringPeriodId=%d", matchupPeriod)
	var raw matchupSchedule
	if err := c.get(u, "", &raw); err != nil {
		return nil, fmt.Errorf("get matchup scores: %w", err)
	}
	return raw.Schedule, nil
}
