package fantrax

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/pmurley/go-fantrax/auth_client"
	"github.com/pmurley/go-fantrax/models"
)

// ScoringPeriod represents a scoring period with its date range.
type ScoringPeriod struct {
	Number    int
	Caption   string
	StartDate time.Time
	EndDate   time.Time
}

var periodNumRe = regexp.MustCompile(`Scoring Period (\d+)`)
var dateRangeRe = regexp.MustCompile(`\(.*?(\w+ \w+ \d+, \d{4})\s*-\s*(\w+ \w+ \d+, \d{4})\)`)

// standingsURL is the Fantrax API endpoint for standings. Var for test overriding.
var standingsURL = "https://www.fantrax.com/fxpa/req"

// GetScoringPeriodsAndTeams fetches all scoring periods and the team ID→name map
// from a single getStandings call with view=SCHEDULE.
func (c *Client) GetScoringPeriodsAndTeams() ([]ScoringPeriod, map[string]string, error) {
	fullRequest := map[string]interface{}{
		"msgs": []auth_client.FantraxMessage{
			{
				Method: "getStandings",
				Data: map[string]string{
					"leagueId": c.leagueID,
					"view":     "SCHEDULE",
				},
			},
		},
		"uiv":    3,
		"refUrl": fmt.Sprintf("https://www.fantrax.com/fantasy/league/%s/standings", c.leagueID),
		"dt":     0,
		"at":     0,
		"av":     "0.0",
		"tz":     "UTC",
		"v":      "179.0.1",
	}

	jsonStr, err := json.Marshal(fullRequest)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal standings request: %w", err)
	}

	req, err := http.NewRequest("POST", standingsURL+"?leagueId="+c.leagueID, bytes.NewBuffer(jsonStr))
	if err != nil {
		return nil, nil, fmt.Errorf("create standings request: %w", err)
	}

	resp, err := c.auth.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send standings request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("standings API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read standings response: %w", err)
	}

	var standingsResp auth_client.StandingsResponse
	if err := json.Unmarshal(body, &standingsResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal standings response: %w", err)
	}

	if len(standingsResp.Responses) == 0 {
		return nil, nil, fmt.Errorf("no response data in standings")
	}

	data := standingsResp.Responses[0].Data

	teams := make(map[string]string, len(data.FantasyTeamInfo))
	for id, info := range data.FantasyTeamInfo {
		teams[id] = info.Name
	}

	var periods []ScoringPeriod
	for _, table := range data.TableList {
		m := periodNumRe.FindStringSubmatch(table.Caption)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])

		dm := dateRangeRe.FindStringSubmatch(table.SubCaption)
		if dm == nil {
			continue
		}

		start, err := time.Parse("Mon Jan 2, 2006", dm[1])
		if err != nil {
			continue
		}
		end, err := time.Parse("Mon Jan 2, 2006", dm[2])
		if err != nil {
			continue
		}

		periods = append(periods, ScoringPeriod{
			Number:    num,
			Caption:   table.Caption,
			StartDate: start,
			EndDate:   end,
		})
	}

	return periods, teams, nil
}

// gsRosterRequest adds scoringCategoryType and statsType to the standard roster request.
type gsRosterRequest struct {
	LeagueID            string `json:"leagueId"`
	Reload              string `json:"reload"`
	Period              string `json:"period"`
	TeamID              string `json:"teamId"`
	View                string `json:"view"`
	ScoringCategoryType string `json:"scoringCategoryType"`
	StatsType           string `json:"statsType"`
}

// GetTeamGS returns the total Games Started for active-slot pitchers on a team
// across all daily periods within the given matchup scoring period.
// seasonStart is the first day of the season (period 1), used to convert dates
// to daily period numbers.
func (c *Client) GetTeamGS(teamID string, sp ScoringPeriod, seasonStart, today time.Time) (int, error) {
	// Determine the last date to check: either the period end or today, whichever is earlier.
	endDate := sp.EndDate
	todayTrunc := today.Truncate(24 * time.Hour)
	if todayTrunc.Before(endDate) {
		endDate = todayTrunc
	}

	totalGS := 0
	for d := sp.StartDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dailyPeriod := PeriodForDate(seasonStart, d)
		gs, err := c.getTeamGSForPeriod(teamID, dailyPeriod)
		if err != nil {
			return 0, fmt.Errorf("period %d (%s): %w", dailyPeriod, d.Format("2006-01-02"), err)
		}
		totalGS += gs
	}
	return totalGS, nil
}

// getTeamGSForPeriod returns the GS for a single daily period.
func (c *Client) getTeamGSForPeriod(teamID string, period int) (int, error) {
	data := gsRosterRequest{
		LeagueID:            c.leagueID,
		Reload:              "1",
		Period:              strconv.Itoa(period),
		TeamID:              teamID,
		View:                "STATS",
		ScoringCategoryType: "1",
		StatsType:           "2",
	}

	fullRequest := map[string]interface{}{
		"msgs": []auth_client.FantraxMessage{
			{
				Method: "getTeamRosterInfo",
				Data:   data,
			},
		},
		"uiv":    3,
		"refUrl": fmt.Sprintf("https://www.fantrax.com/fantasy/league/%s/team/roster;reload=1;period=%d;teamId=%s", c.leagueID, period, teamID),
		"dt":     0,
		"at":     0,
		"av":     "0.0",
		"tz":     "UTC",
		"v":      "179.0.1",
	}

	jsonStr, err := json.Marshal(fullRequest)
	if err != nil {
		return 0, fmt.Errorf("marshal roster request: %w", err)
	}

	req, err := http.NewRequest("POST", standingsURL+"?leagueId="+c.leagueID, bytes.NewBuffer(jsonStr))
	if err != nil {
		return 0, fmt.Errorf("create roster request: %w", err)
	}

	resp, err := c.auth.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send roster request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("roster API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read roster response: %w", err)
	}

	var rosterResp models.TeamRosterResponse
	if err := json.Unmarshal(body, &rosterResp); err != nil {
		return 0, fmt.Errorf("unmarshal roster response: %w", err)
	}

	if len(rosterResp.Responses) == 0 {
		return 0, fmt.Errorf("no response data in roster")
	}

	tables := rosterResp.Responses[0].Data.Tables
	return sumGSFromTables(tables)
}

// sumGSFromTables finds the pitching table (scGroup=20) and sums the GS column.
func sumGSFromTables(tables []models.RosterTable) (int, error) {
	for _, table := range tables {
		if !isPitchingGroup(table.SCGroup) {
			continue
		}

		gsIdx := -1
		for i, col := range table.Header.Cells {
			if col.ShortName == "GS" {
				gsIdx = i
				break
			}
		}
		if gsIdx == -1 {
			return 0, nil
		}

		totalGS := 0
		for _, row := range table.Rows {
			if gsIdx >= len(row.Cells) {
				continue
			}
			raw := row.Cells[gsIdx].Content
			if raw == "" {
				continue
			}
			val, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			totalGS += int(math.Round(val))
		}
		return totalGS, nil
	}

	return 0, nil
}

// isPitchingGroup checks if scGroup represents the pitching group (20).
// SCGroup is interface{} in the model — it may be string "20" or float64 20.
func isPitchingGroup(scGroup interface{}) bool {
	switch v := scGroup.(type) {
	case string:
		return v == "20"
	case float64:
		return v == 20
	case int:
		return v == 20
	default:
		return false
	}
}

// FindJustEndedPeriod returns the period whose end date is yesterday, or nil.
func FindJustEndedPeriod(periods []ScoringPeriod, today time.Time) *ScoringPeriod {
	yesterday := today.AddDate(0, 0, -1)
	ymd := yesterday.Format("2006-01-02")
	for i := range periods {
		if periods[i].EndDate.Format("2006-01-02") == ymd {
			return &periods[i]
		}
	}
	return nil
}

// FindCurrentPeriod returns the period that contains today (start <= today <= end), or nil.
func FindCurrentPeriod(periods []ScoringPeriod, today time.Time) *ScoringPeriod {
	todayYMD := today.Format("2006-01-02")
	for i := range periods {
		if periods[i].StartDate.Format("2006-01-02") <= todayYMD && todayYMD <= periods[i].EndDate.Format("2006-01-02") {
			return &periods[i]
		}
	}
	return nil
}

// FindMostRecentPastPeriod returns the most recent period whose end date is before today.
func FindMostRecentPastPeriod(periods []ScoringPeriod, today time.Time) *ScoringPeriod {
	todayYMD := today.Format("2006-01-02")
	var best *ScoringPeriod
	for i := range periods {
		if periods[i].EndDate.Format("2006-01-02") < todayYMD {
			if best == nil || periods[i].EndDate.After(best.EndDate) {
				best = &periods[i]
			}
		}
	}
	return best
}
