package recap

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestLeagueDailySigma(t *testing.T) {
	// Three points: 10, 20, 30 → mean=20, sample var=100, σ=10
	days := []TeamDay{
		{Pts: 10}, {Pts: 20}, {Pts: 30},
	}
	got := LeagueDailySigma(days)
	if math.Abs(got-10.0) > 1e-9 {
		t.Errorf("LeagueDailySigma: want 10, got %.6f", got)
	}
}

func TestLeagueDailySigmaTooFew(t *testing.T) {
	if got := LeagueDailySigma(nil); got != 0 {
		t.Errorf("nil → want 0, got %.6f", got)
	}
	if got := LeagueDailySigma([]TeamDay{{Pts: 50}}); got != 0 {
		t.Errorf("len=1 → want 0, got %.6f", got)
	}
}

func makeDates(start time.Time) []time.Time {
	out := make([]time.Time, 7)
	for i := 0; i < 7; i++ {
		out[i] = start.AddDate(0, 0, i)
	}
	return out
}

func TestComputeWPCurve_HomeDominates(t *testing.T) {
	start := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	dates := makeDates(start)
	in := WPInputs{
		HomeTeamID:    "A",
		AwayTeamID:    "B",
		HomeMeanDaily: 60,
		AwayMeanDaily: 40,
		Sigma:         15,
		Dates:         dates,
		HomeActuals:   []float64{60, 60, 60, 60, 60, 60, 60}, // 420
		AwayActuals:   []float64{40, 40, 40, 40, 40, 40, 40}, // 280
		WeekNumber:    1,
	}
	curve := ComputeWPCurve(in)

	if len(curve.Points) != 8 {
		t.Fatalf("Points: want 8, got %d", len(curve.Points))
	}
	final := curve.Points[7]
	if final.HomeWP != 1.0 {
		t.Errorf("final HomeWP: want 1.0, got %.4f", final.HomeWP)
	}
	if final.HomeRunning != 420 {
		t.Errorf("final HomeRunning: want 420, got %.2f", final.HomeRunning)
	}
	if final.AwayRunning != 280 {
		t.Errorf("final AwayRunning: want 280, got %.2f", final.AwayRunning)
	}
	// Curve should monotonically increase as home dominates from start.
	for i := 1; i < 8; i++ {
		if curve.Points[i].HomeWP < curve.Points[i-1].HomeWP-1e-6 {
			t.Errorf("non-monotone WP at i=%d: prev=%.4f cur=%.4f",
				i, curve.Points[i-1].HomeWP, curve.Points[i].HomeWP)
		}
	}
}

func TestComputeWPCurve_TiedFinalIsHalf(t *testing.T) {
	start := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	in := WPInputs{
		HomeTeamID:    "A",
		AwayTeamID:    "B",
		HomeMeanDaily: 50,
		AwayMeanDaily: 50,
		Sigma:         20,
		Dates:         makeDates(start),
		HomeActuals:   []float64{50, 50, 50, 50, 50, 50, 50},
		AwayActuals:   []float64{50, 50, 50, 50, 50, 50, 50},
		WeekNumber:    1,
	}
	curve := ComputeWPCurve(in)
	if final := curve.Points[7].HomeWP; final != 0.5 {
		t.Errorf("tied final: want 0.5, got %.4f", final)
	}
}

func TestComputeWPCurve_Determinism(t *testing.T) {
	start := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	in := WPInputs{
		HomeTeamID:    "A",
		AwayTeamID:    "B",
		HomeMeanDaily: 55,
		AwayMeanDaily: 50,
		Sigma:         18,
		Dates:         makeDates(start),
		HomeActuals:   []float64{55, 50, 60, 45, 55, 50, 60},
		AwayActuals:   []float64{50, 55, 45, 60, 50, 55, 45},
		WeekNumber:    3,
	}
	a := ComputeWPCurve(in)
	b := ComputeWPCurve(in)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("ComputeWPCurve is non-deterministic")
	}
}

func TestLeadChangeCount(t *testing.T) {
	cases := []struct {
		name string
		wps  []float64
		want int
	}{
		{"flat half", []float64{0.5, 0.5, 0.5, 0.5}, 0},
		{"home dominant", []float64{0.5, 0.7, 0.8, 0.9}, 1}, // crosses 0.5 once
		{"alternating", []float64{0.5, 0.6, 0.4, 0.6, 0.4, 0.6, 0.4, 0.6}, 7},
		{"never crosses", []float64{0.6, 0.7, 0.55, 0.8}, 0},
		{"goes to tie midway", []float64{0.6, 0.5, 0.4, 0.5, 0.4}, 1}, // crosses on .4
	}
	for _, c := range cases {
		points := make([]WPPoint, len(c.wps))
		for i, w := range c.wps {
			points[i] = WPPoint{HomeWP: w}
		}
		if got := LeadChangeCount(points); got != c.want {
			t.Errorf("%s: want %d changes, got %d", c.name, c.want, got)
		}
	}
}

func TestMinWinnerWP(t *testing.T) {
	cases := []struct {
		name    string
		wps     []float64 // length 8: idx 0=pre, idx 7=final
		homeWon bool
		wantMin float64
		wantOK  bool
	}{
		{
			name:    "winner trailed mid-week",
			wps:     []float64{0.5, 0.4, 0.3, 0.2, 0.4, 0.6, 0.7, 1.0},
			homeWon: true,
			wantMin: 0.2,
			wantOK:  true,
		},
		{
			name:    "winner never trailed",
			wps:     []float64{0.5, 0.6, 0.7, 0.8, 0.85, 0.9, 0.95, 1.0},
			homeWon: true,
			wantMin: 0.6,
			wantOK:  true,
		},
		{
			name:    "away winner — invert",
			wps:     []float64{0.5, 0.6, 0.7, 0.8, 0.4, 0.3, 0.2, 0.0},
			homeWon: false,
			wantMin: 0.2, // = 1 - 0.8 (away's lowest mid-week WP)
			wantOK:  true,
		},
	}

	for _, c := range cases {
		points := make([]WPPoint, len(c.wps))
		for i, w := range c.wps {
			points[i] = WPPoint{HomeWP: w}
		}
		got, ok := MinWinnerWP(points, c.homeWon)
		if ok != c.wantOK {
			t.Errorf("%s: ok mismatch: want %v got %v", c.name, c.wantOK, ok)
			continue
		}
		if !ok {
			continue
		}
		if math.Abs(got-c.wantMin) > 1e-9 {
			t.Errorf("%s: want %.4f, got %.4f", c.name, c.wantMin, got)
		}
	}
}

func TestMinWinnerWPShortCurve(t *testing.T) {
	if _, ok := MinWinnerWP(nil, true); ok {
		t.Errorf("nil curve: want ok=false")
	}
	short := []WPPoint{{HomeWP: 0.5}, {HomeWP: 0.7}}
	if _, ok := MinWinnerWP(short, true); ok {
		t.Errorf("short curve: want ok=false")
	}
}
