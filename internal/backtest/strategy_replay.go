package backtest

import (
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
	"github.com/nixon-commits/rosterbot/internal/projections"
)

// StrategyVariant is one named projection strategy the replay harness evaluates.
// Build returns the hitter Source the variant would use to set a lineup on the
// given evaluation date, using only data available before that date.
type StrategyVariant struct {
	Name  string
	Build func(asOf time.Time) (projections.Source, error)
}

// VariantResult is the aggregate scorecard for one variant across the window.
type VariantResult struct {
	Name        string
	RealizedPts float64 // total actual FPts of the lineups this variant set
	MeanGap     float64 // mean daily (realized − hindsight-optimal), hitter slots
	MAE         float64 // mean abs per-player projection error vs actual
	Bias        float64 // signed mean per-player error (projected − actual)
}

// BuildHitterSeries groups per-day hitter FPts into a per-player DayFP series.
func BuildHitterSeries(days []fantrax.DayRoster) map[string][]projections.DayFP {
	series := make(map[string][]projections.DayFP)
	for _, day := range days {
		for _, p := range day.Players {
			if p.IsPitcher {
				continue
			}
			series[p.PlayerID] = append(series[p.PlayerID], projections.DayFP{
				Date:   day.Date,
				FP:     p.FPts,
				Played: p.HadGame,
			})
		}
	}
	return series
}
