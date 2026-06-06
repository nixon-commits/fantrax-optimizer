package backtest

import (
	"testing"
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestBuildHitterSeries_GroupsByPlayerAndDropsPitchers(t *testing.T) {
	days := []fantrax.DayRoster{
		{Date: d("2026-05-01"), Players: []fantrax.DayPlayerFP{
			{PlayerID: "h1", FPts: 5, HadGame: true, IsPitcher: false},
			{PlayerID: "p1", FPts: 9, HadGame: true, IsPitcher: true},
		}},
		{Date: d("2026-05-02"), Players: []fantrax.DayPlayerFP{
			{PlayerID: "h1", FPts: 7, HadGame: true, IsPitcher: false},
		}},
	}
	got := BuildHitterSeries(days)
	if _, ok := got["p1"]; ok {
		t.Fatalf("pitcher leaked into hitter series")
	}
	if len(got["h1"]) != 2 {
		t.Fatalf("h1 series len = %d, want 2", len(got["h1"]))
	}
	if got["h1"][1].FP != 7 || !got["h1"][1].Played {
		t.Fatalf("h1 day2 = %+v, want FP 7 played", got["h1"][1])
	}
}
