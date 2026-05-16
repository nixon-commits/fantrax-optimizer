package recap

import (
	"math"
	"time"

	"github.com/pmurley/go-fantrax/models"
)

// WPInputs is the per-matchup data needed to compute a WP curve. All slices
// are length 7 (one per day in the matchup week).
type WPInputs struct {
	HomeTeamID    string
	AwayTeamID    string
	HomeMeanDaily float64     // expected daily FPts for home (drives weekly projection)
	AwayMeanDaily float64     // expected daily FPts for away
	Dates         []time.Time // length 7, one per matchup day (chronological)
	HomeActuals   []float64   // length 7, observed home FPts per day
	AwayActuals   []float64   // length 7, observed away FPts per day
	WeekNumber    int         // carried through for output identification
}

// ComputeWPCurve returns the 8-point WP trace for one matchup using the same
// formula the Fantrax UI runs client-side (ported as
// models.CalculateWinProbability). Points[0] is the pre-week baseline,
// derived from the projection ratio alone (so a 60/40 projected favorite
// starts at ~84/16, not 50/50). Points[1..7] are end-of-Day-i states using
// cumulative actuals, a *live-adjusted* weekly projection
// (`actual_so_far + remaining_days × HomeMeanDaily`, mirroring how Fantrax
// recomputes `calculatedProjectedTotalsMap` intra-week), and a uniform
// `games left = 7 - i` assumption for both teams. Without the live
// adjustment, two teams with similar season averages would have an
// identically-balanced projection ratio every day and the chart would be
// flat at 50% mid-week.
//
// The formula is pure and deterministic; identical inputs always produce
// identical curves.
func ComputeWPCurve(in WPInputs) MatchupWPCurve {
	if len(in.Dates) != 7 || len(in.HomeActuals) != 7 || len(in.AwayActuals) != 7 {
		// Degenerate inputs — return an empty curve. The orchestrator
		// soft-fails by hiding charts/sparklines.
		return MatchupWPCurve{HomeTeamID: in.HomeTeamID, AwayTeamID: in.AwayTeamID}
	}

	points := make([]WPPoint, 8)
	var hSum, aSum float64
	for i := 0; i <= 7; i++ {
		if i > 0 {
			hSum += in.HomeActuals[i-1]
			aSum += in.AwayActuals[i-1]
		}
		daysLeft := 7 - i

		// Live-adjusted projection: locked-in points + remaining-day rate.
		// At i=0 reduces to mean × 7; at i=7 reduces to the actual total.
		homeProj := hSum + float64(daysLeft)*in.HomeMeanDaily
		awayProj := aSum + float64(daysLeft)*in.AwayMeanDaily

		hp, _ := models.CalculateWinProbability(hSum, aSum, homeProj, awayProj, daysLeft, daysLeft)

		// Date semantics: Points[0] uses the first matchup day's date as a
		// stand-in (the chart treats it as the leftmost X-axis tick);
		// Points[i] for i in 1..7 uses Dates[i-1].
		var date time.Time
		if i == 0 {
			date = in.Dates[0]
		} else {
			date = in.Dates[i-1]
		}
		points[i] = WPPoint{
			Date:        date,
			HomeWP:      float64(hp) / 100.0,
			HomeRunning: hSum,
			AwayRunning: aSum,
		}
	}

	curve := MatchupWPCurve{
		HomeTeamID: in.HomeTeamID,
		AwayTeamID: in.AwayTeamID,
		Points:     points,
	}
	curve.LeadChanges = LeadChangeCount(points)
	return curve
}

// LeadChangeCount returns the number of times the leader (defined as
// HomeWP > 0.5) flips across consecutive points. Days at exactly 0.5 do not
// trigger a transition either way.
func LeadChangeCount(points []WPPoint) int {
	if len(points) < 2 {
		return 0
	}
	count := 0
	prev := points[0].HomeWP
	for i := 1; i < len(points); i++ {
		cur := points[i].HomeWP
		// "Side" is HomeWP > 0.5 (true=home leads). Skip points at 0.5 by
		// carrying prev forward: a tie point alone does not count.
		if (prev > 0.5) != (cur > 0.5) {
			count++
		}
		prev = cur
	}
	return count
}

// MinWinnerWP returns the lowest mid-week win probability for the eventual
// winner. Mid-week is defined as Points[1..6] (Days 1..6) — index 0 (pre-
// week) and index 7 (final) are excluded.
//
// homeWon = true means the eventual winner was the home team; ok=false
// when the curve is too short to evaluate (need 8 points).
func MinWinnerWP(points []WPPoint, homeWon bool) (float64, bool) {
	if len(points) < 8 {
		return 0, false
	}
	min := math.Inf(1)
	for i := 1; i <= 6; i++ {
		wp := points[i].HomeWP
		if !homeWon {
			wp = 1.0 - wp
		}
		if wp < min {
			min = wp
		}
	}
	return min, true
}
