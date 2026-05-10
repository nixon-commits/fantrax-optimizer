package cmd

import (
	"fmt"
	"time"

	"github.com/nixon-commits/rosterbot/internal/config"
	"github.com/nixon-commits/rosterbot/internal/espn"
	"github.com/nixon-commits/rosterbot/internal/fantrax"
	"github.com/nixon-commits/rosterbot/internal/recap"
)

// initRecapPlatform builds a recap.Platform for the configured fantasy
// provider. Adapters live here (not inside internal/recap or internal/espn)
// so neither package needs to know about the other.
func initRecapPlatform(cfg *config.Config) (recap.Platform, error) {
	switch cfg.Platform {
	case config.PlatformFantrax:
		ft, err := fantrax.NewClient(cfg.LeagueID, cfg.TeamID)
		if err != nil {
			return nil, fmt.Errorf("fantrax client: %w", err)
		}
		return &fantraxRecapAdapter{client: ft}, nil
	case config.PlatformESPN:
		ec, err := espn.NewClient(cfg.ESPNLeagueID, cfg.ESPNTeamID, cfg.ESPNYear, cfg.ESPNSWID, cfg.ESPNS2)
		if err != nil {
			return nil, fmt.Errorf("espn client: %w", err)
		}
		return newESPNRecapAdapter(ec, todayET()), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", cfg.Platform)
	}
}

// ---------------------------------------------------------------------------
// Fantrax adapter — passthrough wrapper around *fantrax.Client. Exists so
// the recap package never imports anything platform-specific. Each method is
// a thin delegation; preserving byte-identical Fantrax behavior is the
// invariant.
// ---------------------------------------------------------------------------

type fantraxRecapAdapter struct {
	client *fantrax.Client
}

func (a *fantraxRecapAdapter) GetSeasonDateRange() (time.Time, time.Time, error) {
	return a.client.GetSeasonDateRange()
}

func (a *fantraxRecapAdapter) GetTeams() (map[string]string, map[string]string, error) {
	_, names, logos, err := a.client.GetScoringPeriodsAndTeams()
	return names, logos, err
}

func (a *fantraxRecapAdapter) GetActiveSlots() ([]fantrax.Slot, error) {
	return a.client.GetActiveSlots()
}

func (a *fantraxRecapAdapter) GetPitcherSlots() ([]fantrax.Slot, error) {
	return a.client.GetPitcherSlots()
}

func (a *fantraxRecapAdapter) GetAllMatchupEntries() ([]fantrax.MatchupEntry, error) {
	return a.client.GetAllMatchupEntries()
}

func (a *fantraxRecapAdapter) GetMatchupWeekNumberForDate(date time.Time) (int, error) {
	return a.client.GetMatchupWeekNumberForDate(date)
}

func (a *fantraxRecapAdapter) GetMatchupWeekByNumber(n int) (time.Time, time.Time, error) {
	return a.client.GetMatchupWeekByNumber(n)
}

func (a *fantraxRecapAdapter) DailyFantasyPoints(
	teamID string,
	start, end, seasonStart time.Time,
	cacheDir string,
	cacheTTL time.Duration,
) ([]fantrax.DayRoster, error) {
	return a.client.DailyFantasyPoints(teamID, start, end, seasonStart, cacheDir, cacheTTL)
}

func (a *fantraxRecapAdapter) GetTeamPitcherStarts(
	teamID string,
	start, end, seasonStart time.Time,
) ([]fantrax.DatedPitcherStart, error) {
	return a.client.GetTeamPitcherStarts(teamID, start, end, seasonStart)
}
