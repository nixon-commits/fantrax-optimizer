package recap

import (
	"math"
	"testing"
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

func dayRoster(date time.Time, players ...fantrax.DayPlayerFP) fantrax.DayRoster {
	return fantrax.DayRoster{Date: date, Players: players}
}

func TestComputeSeasonMeanFromDays(t *testing.T) {
	d1 := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)

	days := []fantrax.DayRoster{
		// Day 1: 2 active starters with games (40 + 10), 1 active no-game-no-pts (excluded), 1 inactive (excluded)
		dayRoster(d1,
			fantrax.DayPlayerFP{Active: true, HadGame: true, FPts: 40},
			fantrax.DayPlayerFP{Active: true, HadGame: true, FPts: 10},
			fantrax.DayPlayerFP{Active: true, HadGame: false, FPts: 0},
			fantrax.DayPlayerFP{Active: false, HadGame: true, FPts: 99},
		),
		// Day 2: all-bench day — should NOT count toward denominator
		dayRoster(d2,
			fantrax.DayPlayerFP{Active: false, HadGame: true, FPts: 50},
			fantrax.DayPlayerFP{Active: true, HadGame: false, FPts: 0},
		),
		// Day 3: negative FPts on an active starter who had a game (counts)
		dayRoster(d3,
			fantrax.DayPlayerFP{Active: true, HadGame: true, FPts: -3},
			fantrax.DayPlayerFP{Active: true, HadGame: true, FPts: 13},
		),
	}

	mean, played := computeSeasonMeanFromDays(days)
	if played != 2 {
		t.Fatalf("days played: want 2, got %d", played)
	}
	// Day1 sum = 50; Day3 sum = 10. Mean = 30.
	if math.Abs(mean-30.0) > 1e-9 {
		t.Errorf("mean: want 30, got %.6f", mean)
	}
}

func TestComputeSeasonMeanFromDaysEmpty(t *testing.T) {
	if mean, played := computeSeasonMeanFromDays(nil); mean != 0 || played != 0 {
		t.Errorf("nil → want (0,0), got (%.2f, %d)", mean, played)
	}
}

func TestComputeSeasonMeanFromDaysAllInactive(t *testing.T) {
	d := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	days := []fantrax.DayRoster{
		dayRoster(d,
			fantrax.DayPlayerFP{Active: false, HadGame: true, FPts: 50},
			fantrax.DayPlayerFP{Active: false, HadGame: false, FPts: 0},
		),
	}
	if mean, played := computeSeasonMeanFromDays(days); mean != 0 || played != 0 {
		t.Errorf("all-inactive → want (0,0), got (%.2f, %d)", mean, played)
	}
}

func TestComputeSeasonMeanFromDaysSingleDay(t *testing.T) {
	d := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	days := []fantrax.DayRoster{
		dayRoster(d, fantrax.DayPlayerFP{Active: true, HadGame: true, FPts: 42.5}),
	}
	mean, played := computeSeasonMeanFromDays(days)
	if played != 1 || math.Abs(mean-42.5) > 1e-9 {
		t.Errorf("single day → want (42.5, 1), got (%.2f, %d)", mean, played)
	}
}

func TestComputeStandingsHistory_WithMedian(t *testing.T) {
	// 4-team league, 2 weeks. Lower-middle median used (even N).
	//
	// Week 1 H2H: A beats B, C beats D
	// Week 1 pts: A=100, C=90, B=80, D=70
	//   sorted=[70,80,90,100] → lower-middle median = 80
	//   A(100)>80 → median W; C(90)>80 → median W
	//   B(80)=80 → median T; D(70)<80 → median L
	// W1 records: A=2W0L, C=2W0L (tied → pts break: A>C), B=0W1L1T, D=0W2L
	//
	// Week 2 H2H: A beats C, B beats D
	// Week 2 pts: A=95, C=85, B=75, D=65
	//   sorted=[65,75,85,95] → lower-middle median = 75
	//   A(95)>75 → median W; C(85)>75 → median W
	//   B(75)=75 → median T; D(65)<75 → median L
	// W2 additions: A+2W, C+1W1L, B+1W1T, D+2L
	// Cumulative: A=4W0L, C=3W1L, B=1W1L2T, D=0W4L
	recaps := []*Recap{
		{
			WeekNumber: 1,
			Matchups: []MatchupResult{
				{HomeTeamID: "A", HomeTeamName: "Alpha", HomePts: 100, AwayTeamID: "B", AwayTeamName: "Beta", AwayPts: 80, WinnerID: "A", LoserID: "B"},
				{HomeTeamID: "C", HomeTeamName: "Gamma", HomePts: 90, AwayTeamID: "D", AwayTeamName: "Delta", AwayPts: 70, WinnerID: "C", LoserID: "D"},
			},
			Teams: []TeamWeek{
				{TeamID: "A", TeamName: "Alpha", ActualPts: 100},
				{TeamID: "B", TeamName: "Beta", ActualPts: 80},
				{TeamID: "C", TeamName: "Gamma", ActualPts: 90},
				{TeamID: "D", TeamName: "Delta", ActualPts: 70},
			},
		},
		{
			WeekNumber: 2,
			Matchups: []MatchupResult{
				{HomeTeamID: "A", HomeTeamName: "Alpha", HomePts: 95, AwayTeamID: "C", AwayTeamName: "Gamma", AwayPts: 85, WinnerID: "A", LoserID: "C"},
				{HomeTeamID: "B", HomeTeamName: "Beta", HomePts: 75, AwayTeamID: "D", AwayTeamName: "Delta", AwayPts: 65, WinnerID: "B", LoserID: "D"},
			},
			Teams: []TeamWeek{
				{TeamID: "A", TeamName: "Alpha", ActualPts: 95},
				{TeamID: "B", TeamName: "Beta", ActualPts: 75},
				{TeamID: "C", TeamName: "Gamma", ActualPts: 85},
				{TeamID: "D", TeamName: "Delta", ActualPts: 65},
			},
		},
	}

	hist := ComputeStandingsHistory(recaps)
	if len(hist) != 2 {
		t.Fatalf("want 2 snapshots, got %d", len(hist))
	}

	// After week 1: A=2W, C=2W (tied → A ranks above by pts), B=0W, D=0W
	w1 := hist[0].Standings
	if w1[0].TeamID != "A" || w1[0].Wins != 2 {
		t.Errorf("W1 #1 want A 2W, got %s %dW", w1[0].TeamID, w1[0].Wins)
	}
	if w1[1].TeamID != "C" || w1[1].Wins != 2 {
		t.Errorf("W1 #2 want C 2W, got %s %dW", w1[1].TeamID, w1[1].Wins)
	}
	if w1[2].TeamID != "B" {
		t.Errorf("W1 #3 want B, got %s", w1[2].TeamID)
	}
	if w1[3].TeamID != "D" {
		t.Errorf("W1 #4 want D, got %s", w1[3].TeamID)
	}

	// After week 2: A=4W, C=3W, B=1W, D=0W
	w2 := hist[1].Standings
	wantW2 := []struct {
		id   string
		wins int
	}{{"A", 4}, {"C", 3}, {"B", 1}, {"D", 0}}
	for i, want := range wantW2 {
		if w2[i].TeamID != want.id || w2[i].Wins != want.wins {
			t.Errorf("W2 rank %d: want %s %dW, got %s %dW", i+1, want.id, want.wins, w2[i].TeamID, w2[i].Wins)
		}
	}
}
