package recap

import "sort"

// TeamRecord tracks a team's cumulative season record.
type TeamRecord struct {
	TeamID    string
	TeamName  string
	Wins      int
	Losses    int
	Ties      int
	PointsFor float64
}

// StandingEntry is one team's rank at the end of a given week.
type StandingEntry struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	Rank     int    `json:"rank"` // 1 = first place
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
}

// WeekStandings is the full league standings snapshot after one matchup week.
type WeekStandings struct {
	WeekNumber int             `json:"week_number"`
	Standings  []StandingEntry `json:"standings"` // len = number of teams, sorted rank asc
}

// ComputeStandingsHistory derives the cumulative league standings after each
// completed matchup week. recaps must be in week-number ascending order. The
// returned slice has one entry per week and can be passed to the template for
// the bump chart.
//
// This league uses a median game: each week a team plays their H2H matchup
// AND an additional game against the league median score. Teams above the
// weekly median earn an extra win; teams below earn an extra loss; ties at
// the exact median are a tie. Both games are counted so W-L totals match the
// official Fantrax standings.
func ComputeStandingsHistory(recaps []*Recap) []WeekStandings {
	records := map[string]*TeamRecord{}

	history := make([]WeekStandings, 0, len(recaps))
	for _, r := range recaps {
		// H2H matchup results.
		for _, m := range r.Matchups {
			ensureRecord(records, m.HomeTeamID, m.HomeTeamName)
			ensureRecord(records, m.AwayTeamID, m.AwayTeamName)
			records[m.HomeTeamID].PointsFor += m.HomePts
			records[m.AwayTeamID].PointsFor += m.AwayPts

			switch {
			case m.IsTie:
				records[m.HomeTeamID].Ties++
				records[m.AwayTeamID].Ties++
			case m.WinnerID == m.HomeTeamID:
				records[m.HomeTeamID].Wins++
				records[m.AwayTeamID].Losses++
			default:
				records[m.AwayTeamID].Wins++
				records[m.HomeTeamID].Losses++
			}
		}

		// Median game: sort teams by weekly ActualPts, award extra W/L/T.
		// TeamWeek.ActualPts is the weekly fantasy points total already
		// computed by the recap for each team.
		if len(r.Teams) > 0 {
			median := weeklyMedian(r.Teams)
			for _, tw := range r.Teams {
				ensureRecord(records, tw.TeamID, tw.TeamName)
				switch {
				case tw.ActualPts > median:
					records[tw.TeamID].Wins++
				case tw.ActualPts < median:
					records[tw.TeamID].Losses++
				default:
					records[tw.TeamID].Ties++
				}
			}
		}

		history = append(history, WeekStandings{
			WeekNumber: r.WeekNumber,
			Standings:  rankRecords(records),
		})
	}
	return history
}

// weeklyMedian returns the median ActualPts across all teams for one week.
// For an even-length list the lower of the two middle values is used,
// matching how most leagues define "above median" (strictly greater than).
func weeklyMedian(teams []TeamWeek) float64 {
	if len(teams) == 0 {
		return 0
	}
	pts := make([]float64, len(teams))
	for i, t := range teams {
		pts[i] = t.ActualPts
	}
	sort.Float64s(pts)
	mid := len(pts) / 2
	if len(pts)%2 == 0 {
		return pts[mid-1] // lower middle — "above" means strictly greater
	}
	return pts[mid]
}

func ensureRecord(m map[string]*TeamRecord, id, name string) {
	if _, ok := m[id]; !ok {
		m[id] = &TeamRecord{TeamID: id, TeamName: name}
	}
}

// rankRecords returns a snapshot of team standings sorted by rank.
// Primary sort: wins desc. Tiebreaker: points for desc. Final: teamID asc
// for deterministic output.
func rankRecords(records map[string]*TeamRecord) []StandingEntry {
	teams := make([]*TeamRecord, 0, len(records))
	for _, r := range records {
		teams = append(teams, r)
	}
	sort.Slice(teams, func(i, j int) bool {
		a, b := teams[i], teams[j]
		if a.Wins != b.Wins {
			return a.Wins > b.Wins
		}
		if a.PointsFor != b.PointsFor {
			return a.PointsFor > b.PointsFor
		}
		return a.TeamID < b.TeamID
	})

	out := make([]StandingEntry, len(teams))
	for i, t := range teams {
		out[i] = StandingEntry{
			TeamID:   t.TeamID,
			TeamName: t.TeamName,
			Rank:     i + 1,
			Wins:     t.Wins,
			Losses:   t.Losses,
		}
	}
	return out
}
