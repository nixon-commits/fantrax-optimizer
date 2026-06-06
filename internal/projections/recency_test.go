package projections

import (
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// series: played 10 FP on each of the last 3 days, plus an old cold 0 FP day.
func sampleSeries() []DayFP {
	return []DayFP{
		{Date: day("2026-04-01"), FP: 0, Played: true}, // old cold game
		{Date: day("2026-05-28"), FP: 10, Played: true},
		{Date: day("2026-05-29"), FP: 10, Played: true},
		{Date: day("2026-05-30"), FP: 10, Played: true},
		{Date: day("2026-05-31"), FP: 99, Played: true}, // eval day — must be excluded
	}
}

func TestWeightedRecent_YTD_AveragesAllPriorGames(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), YTDWeight)
	// (0+10+10+10)/4 = 7.5 over 4 games; the eval-day 99 is excluded.
	if math.Abs(got.FPtsPerGame-7.5) > 1e-9 {
		t.Fatalf("FPtsPerGame = %v, want 7.5", got.FPtsPerGame)
	}
	if got.GamesPlayed != 4 {
		t.Fatalf("GamesPlayed = %d, want 4", got.GamesPlayed)
	}
}

func TestWeightedRecent_Window_DropsOldGames(t *testing.T) {
	// last-7-day window: only the three May 28–30 games qualify (April 1 is too old).
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), WindowWeight(7))
	if math.Abs(got.FPtsPerGame-10.0) > 1e-9 {
		t.Fatalf("FPtsPerGame = %v, want 10.0", got.FPtsPerGame)
	}
	if got.GamesPlayed != 3 {
		t.Fatalf("GamesPlayed = %d, want 3", got.GamesPlayed)
	}
}

func TestWeightedRecent_LeakageGuard_ExcludesEvalDayAndLater(t *testing.T) {
	// A future game must never count, even with YTD weighting.
	s := append(sampleSeries(), DayFP{Date: day("2026-06-05"), FP: 100, Played: true})
	got := WeightedRecent(s, day("2026-05-31"), YTDWeight)
	if got.GamesPlayed != 4 {
		t.Fatalf("GamesPlayed = %d, want 4 (eval-day and future excluded)", got.GamesPlayed)
	}
}

func TestWeightedRecent_Decay_WeightsRecentMore(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-05-31"), DecayWeight(7))
	// Decay pulls the average above the flat 7.5 (recent 10s outweigh the old 0).
	if !(got.FPtsPerGame > 7.5) {
		t.Fatalf("decay FPtsPerGame = %v, want > 7.5", got.FPtsPerGame)
	}
}

func TestWeightedRecent_NoPriorGames_ReturnsZeroValue(t *testing.T) {
	got := WeightedRecent(sampleSeries(), day("2026-04-01"), YTDWeight)
	if got.GamesPlayed != 0 || got.FPtsPerGame != 0 {
		t.Fatalf("got %+v, want zero value", got)
	}
}
