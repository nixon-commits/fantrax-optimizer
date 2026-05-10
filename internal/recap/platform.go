package recap

import (
	"time"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

// Platform is the read-only data surface recap requires from a fantasy
// provider. Both Fantrax and ESPN implement it via thin adapters in cmd/.
//
// The interface intentionally returns existing `fantrax.*` types as the
// lingua franca. Those types are structurally platform-neutral (Player,
// Slot, MatchupEntry, DayRoster) — the names just live in the fantrax
// package today because that's where they were first introduced. When the
// `internal/league` extraction lands, the types move there and this import
// goes away. ESPN adapters manufacture fantrax-typed values from ESPN
// data; `internal/espn` itself never imports `internal/fantrax`.
type Platform interface {
	// GetSeasonDateRange returns the calendar bounds of the current season.
	GetSeasonDateRange() (start, end time.Time, err error)

	// GetTeams returns teamID → display name and teamID → logo URL.
	// Recap does not need the scoring period list separately (that data is
	// derived from per-day fetches) — the existing fantrax method bundles
	// these so we surface them as one call to keep the adapter thin.
	GetTeams() (names map[string]string, logos map[string]string, err error)

	// GetActiveSlots returns the league's active hitter slot definitions
	// in display order. Slot IDs are platform-specific strings; recap and
	// backtest only require that they match the SlotPosID values inside
	// DailyFantasyPoints output.
	GetActiveSlots() ([]fantrax.Slot, error)

	// GetPitcherSlots returns the league's pitcher slot definitions.
	GetPitcherSlots() ([]fantrax.Slot, error)

	// GetAllMatchupEntries returns every matchup pairing in the season.
	// Recap uses these to figure out who played whom in the recap window.
	GetAllMatchupEntries() ([]fantrax.MatchupEntry, error)

	// GetMatchupWeekNumberForDate returns the 1-indexed matchup week
	// number containing the given date, or 0 if unknown.
	GetMatchupWeekNumberForDate(date time.Time) (int, error)

	// GetMatchupWeekByNumber returns the calendar bounds [start, end] of
	// the given 1-indexed matchup week. Used by RunSite to enumerate every
	// completed week.
	GetMatchupWeekByNumber(n int) (start, end time.Time, err error)

	// DailyFantasyPoints returns one DayRoster per day in [start, end] with
	// per-player FPts deltas. seasonStart is needed to compute the period
	// number for each day; cacheDir + cacheTTL let the adapter persist
	// per-period snapshots since past periods are immutable.
	DailyFantasyPoints(
		teamID string,
		start, end, seasonStart time.Time,
		cacheDir string,
		cacheTTL time.Duration,
	) ([]fantrax.DayRoster, error)

	// GetTeamPitcherStarts returns every active-slot SP appearance for the
	// team in [start, end] with each start's date and FPts.
	GetTeamPitcherStarts(
		teamID string,
		start, end, seasonStart time.Time,
	) ([]fantrax.DatedPitcherStart, error)
}
