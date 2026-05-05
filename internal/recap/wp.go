package recap

import (
	"hash/fnv"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// wpNumSims is the Monte Carlo iteration count per WP point. 5000 gives a
// standard error of ~0.007 at p=0.5 — invisible at chart resolution.
const wpNumSims = 5000

// LeagueDailySigma returns the sample standard deviation of daily team
// scores. Returns 0 for fewer than 2 points (caller should treat as
// "WP simulation unavailable" and skip the curve).
func LeagueDailySigma(days []TeamDay) float64 {
	n := len(days)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, d := range days {
		sum += d.Pts
	}
	mean := sum / float64(n)
	var ss float64
	for _, d := range days {
		dev := d.Pts - mean
		ss += dev * dev
	}
	return math.Sqrt(ss / float64(n-1))
}

// wpRNG returns a deterministic *rand.Rand seeded from the matchup identity
// + week number, so every run produces identical curves.
func wpRNG(homeID, awayID string, week int) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(homeID))
	h.Write([]byte("|"))
	h.Write([]byte(awayID))
	h.Write([]byte("|"))
	h.Write([]byte(strconv.Itoa(week)))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// WPInputs is the per-matchup data needed to compute a WP curve. All slices
// are length 7 (one per day in the matchup week).
type WPInputs struct {
	HomeTeamID    string
	AwayTeamID    string
	HomeMeanDaily float64     // expected daily FPts for home
	AwayMeanDaily float64     // expected daily FPts for away
	Sigma         float64     // league-wide daily-score stddev
	Dates         []time.Time // length 7, one per matchup day (chronological)
	HomeActuals   []float64   // length 7, observed home FPts per day
	AwayActuals   []float64   // length 7, observed away FPts per day
	WeekNumber    int         // for RNG seed
}

// ComputeWPCurve returns the 8-point WP trace for one matchup. Points[0] is
// the pre-week baseline (always 0.5 — equal teams projected forward by
// definition); Points[1..7] are end-of-Day-i states using observed actuals
// + Monte Carlo projection of remaining days.
//
// Determinism: identical inputs always produce identical curves (RNG seeded
// from match identity + week number).
func ComputeWPCurve(in WPInputs) MatchupWPCurve {
	if len(in.Dates) != 7 || len(in.HomeActuals) != 7 || len(in.AwayActuals) != 7 {
		// Degenerate inputs — return an empty curve. The orchestrator
		// soft-fails by hiding charts/sparklines.
		return MatchupWPCurve{HomeTeamID: in.HomeTeamID, AwayTeamID: in.AwayTeamID}
	}
	rng := wpRNG(in.HomeTeamID, in.AwayTeamID, in.WeekNumber)

	points := make([]WPPoint, 8)
	var hSum, aSum float64
	for i := 0; i <= 7; i++ {
		if i > 0 {
			hSum += in.HomeActuals[i-1]
			aSum += in.AwayActuals[i-1]
		}
		daysLeft := 7 - i

		var wp float64
		switch {
		case daysLeft == 0:
			switch {
			case hSum > aSum:
				wp = 1.0
			case hSum < aSum:
				wp = 0.0
			default:
				wp = 0.5
			}
		default:
			wins := 0
			for s := 0; s < wpNumSims; s++ {
				simH := hSum
				simA := aSum
				for d := 0; d < daysLeft; d++ {
					simH += rng.NormFloat64()*in.Sigma + in.HomeMeanDaily
					simA += rng.NormFloat64()*in.Sigma + in.AwayMeanDaily
				}
				if simH > simA {
					wins++
				}
			}
			wp = float64(wins) / float64(wpNumSims)
		}

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
			HomeWP:      wp,
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
