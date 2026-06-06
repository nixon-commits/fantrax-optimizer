// Package scoring converts raw baseball stat lines into fantasy points using
// the league's scoring weights. It is the single home for the stat→points
// algebra that the optimizer, projection blending, and backtest backfill all
// depend on.
//
// It deliberately imports nothing from the rest of the codebase, so every
// package can depend on it without an import cycle. fantrax.ScoringWeights is
// a type alias for Weights, so existing call sites keep compiling unchanged.
package scoring

// Weights maps a stat short-name (e.g. "HR", "SO", "QS") to its point value.
type Weights map[string]float64
