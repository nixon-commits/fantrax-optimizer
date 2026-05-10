package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nixon-commits/rosterbot/internal/config"
	"github.com/nixon-commits/rosterbot/internal/espn"
	"github.com/nixon-commits/rosterbot/internal/fantrax"
	"github.com/nixon-commits/rosterbot/internal/waivers"
)

// initWaiversPlatform returns a waivers.Platform built for the configured
// fantasy provider. Adapters live here (not in internal/fantrax or
// internal/espn) so neither package needs to know about the waivers shape —
// each platform exposes its native types and this layer converts.
func initWaiversPlatform(cfg *config.Config) (waivers.Platform, error) {
	switch cfg.Platform {
	case config.PlatformFantrax:
		ft, err := fantrax.NewClient(cfg.LeagueID, cfg.TeamID)
		if err != nil {
			return nil, fmt.Errorf("fantrax client: %w", err)
		}
		return &fantraxWaiversAdapter{client: ft}, nil
	case config.PlatformESPN:
		ec, err := espn.NewClient(cfg.ESPNLeagueID, cfg.ESPNTeamID, cfg.ESPNYear, cfg.ESPNSWID, cfg.ESPNS2)
		if err != nil {
			return nil, fmt.Errorf("espn client: %w", err)
		}
		return &espnWaiversAdapter{client: ec}, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", cfg.Platform)
	}
}

// ---------------------------------------------------------------------------
// Fantrax adapter
// ---------------------------------------------------------------------------

type fantraxWaiversAdapter struct {
	client *fantrax.Client
}

func (a *fantraxWaiversAdapter) GetFreeAgents() ([]waivers.FreeAgent, error) {
	pool, err := a.client.GetFullPlayerPool()
	if err != nil {
		return nil, err
	}
	out := make([]waivers.FreeAgent, 0, len(pool))
	for _, p := range pool {
		if !isFantraxFreeAgent(p.FantasyStatus) {
			continue
		}
		if p.MinorsEligible {
			continue
		}
		out = append(out, waivers.FreeAgent{
			Name:      p.Name,
			MLBTeam:   p.MLBTeamShortName,
			Positions: parseFantraxPositions(p.MultiPositions),
			Display:   p.MultiPositions,
		})
	}
	return out, nil
}

func (a *fantraxWaiversAdapter) GetHitterScoringWeights() (map[string]float64, error) {
	w, err := a.client.GetScoringWeights()
	if err != nil {
		return nil, err
	}
	return map[string]float64(w), nil
}

func (a *fantraxWaiversAdapter) GetPitcherScoringWeights() (map[string]float64, error) {
	w, err := a.client.GetPitcherScoringWeights()
	if err != nil {
		return nil, err
	}
	return map[string]float64(w), nil
}

func (a *fantraxWaiversAdapter) GetHitterRoster() ([]waivers.RosteredPlayer, error) {
	roster, err := a.client.GetHitterRoster()
	if err != nil {
		return nil, err
	}
	return convertFantraxRoster(roster), nil
}

func (a *fantraxWaiversAdapter) GetPitcherRoster() ([]waivers.RosteredPlayer, error) {
	roster, err := a.client.GetPitcherRoster()
	if err != nil {
		return nil, err
	}
	return convertFantraxRoster(roster), nil
}

func convertFantraxRoster(roster []fantrax.Player) []waivers.RosteredPlayer {
	out := make([]waivers.RosteredPlayer, 0, len(roster))
	for _, p := range roster {
		out = append(out, waivers.RosteredPlayer{
			Name:      p.Name,
			MLBTeam:   p.MLBTeam,
			InMinors:  p.InMinors,
			IsInjured: p.IsInjured,
		})
	}
	return out
}

// isFantraxFreeAgent mirrors the prior internal logic in waivers.filterFreeAgents:
// FantasyStatus values "FA" / "" or any "W*" (waivers tier) count as free.
func isFantraxFreeAgent(status string) bool {
	if status == "FA" || status == "" {
		return true
	}
	return strings.HasPrefix(status, "W")
}

// parseFantraxPositions splits the comma-separated MultiPositions field into
// canonical short names. Fantrax already returns these as the league's stat
// keys ("OF", "1B", "SP"), so the only work is splitting and trimming.
func parseFantraxPositions(multi string) []string {
	if multi == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.Split(multi, ",") {
		if t := strings.TrimSpace(tok); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// ESPN adapter
// ---------------------------------------------------------------------------

// espnWaiversAdapter caches the league settings so the parallel scoring-weight
// calls in waivers.Run don't issue two HTTP roundtrips for the same data.
type espnWaiversAdapter struct {
	client *espn.Client

	settingsOnce sync.Once
	settings     *espn.Settings
	settingsErr  error
}

func (a *espnWaiversAdapter) GetFreeAgents() ([]waivers.FreeAgent, error) {
	fas, err := a.client.GetFreeAgents(0)
	if err != nil {
		return nil, err
	}
	out := make([]waivers.FreeAgent, 0, len(fas))
	for _, p := range fas {
		out = append(out, waivers.FreeAgent{
			Name:      p.Name,
			MLBTeam:   p.MLBTeam,
			Positions: append([]string(nil), p.Positions...),
			Display:   strings.Join(p.Positions, ","),
		})
	}
	return out, nil
}

func (a *espnWaiversAdapter) GetHitterScoringWeights() (map[string]float64, error) {
	s, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	return s.HitterWeights, nil
}

func (a *espnWaiversAdapter) GetPitcherScoringWeights() (map[string]float64, error) {
	s, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	return s.PitcherWeights, nil
}

func (a *espnWaiversAdapter) GetHitterRoster() ([]waivers.RosteredPlayer, error) {
	roster, err := a.client.GetTeamRoster(0)
	if err != nil {
		return nil, err
	}
	return splitESPNRoster(roster, false), nil
}

func (a *espnWaiversAdapter) GetPitcherRoster() ([]waivers.RosteredPlayer, error) {
	roster, err := a.client.GetTeamRoster(0)
	if err != nil {
		return nil, err
	}
	return splitESPNRoster(roster, true), nil
}

// loadSettings fetches league settings exactly once per adapter lifetime.
// Both scoring-weight methods funnel through here so the parallel errgroup
// in Run doesn't double-fetch.
func (a *espnWaiversAdapter) loadSettings() (*espn.Settings, error) {
	a.settingsOnce.Do(func() {
		a.settings, a.settingsErr = a.client.GetSettings()
	})
	return a.settings, a.settingsErr
}

// splitESPNRoster returns either hitters or pitchers from the team roster,
// based on the player's eligible positions. A player with both pitcher and
// hitter eligibility (rare — Ohtani-style two-ways) appears in both halves.
func splitESPNRoster(roster []espn.Player, wantPitchers bool) []waivers.RosteredPlayer {
	out := make([]waivers.RosteredPlayer, 0, len(roster))
	for _, p := range roster {
		isP := isPitcherPositions(p.Positions)
		isH := isHitterPositions(p.Positions)
		if wantPitchers && !isP {
			continue
		}
		if !wantPitchers && !isH {
			continue
		}
		out = append(out, waivers.RosteredPlayer{
			Name:      p.Name,
			MLBTeam:   p.MLBTeam,
			IsInjured: p.IsInjured(),
			// ESPN doesn't carry a separate "in minors" flag the way Fantrax
			// does; minors-eligible players in ESPN are slotted to the IL or
			// not rostered at all. Leave InMinors=false.
		})
	}
	return out
}

func isPitcherPositions(positions []string) bool {
	for _, pos := range positions {
		if pos == "SP" || pos == "RP" {
			return true
		}
	}
	return false
}

func isHitterPositions(positions []string) bool {
	for _, pos := range positions {
		switch pos {
		case "SP", "RP", "P":
			continue
		default:
			return true
		}
	}
	return false
}
