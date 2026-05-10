package espn

import "fmt"

// settingsResponse is the trimmed schema of the mSettings view. ESPN returns
// many more fields; we only decode what waivers needs (scoring categories +
// season ID). Unknown fields are skipped by encoding/json.
type settingsResponse struct {
	SeasonID int `json:"seasonId"`
	Settings struct {
		ScoringSettings struct {
			ScoringItems []struct {
				StatID int     `json:"statId"`
				Points float64 `json:"points"`
			} `json:"scoringItems"`
		} `json:"scoringSettings"`
	} `json:"settings"`
}

// GetSettings fetches league-wide settings and returns the scoring categories
// split into hitter and pitcher weight maps. Stat IDs that the project's
// scoring model doesn't recognize (StatShortName lookup misses) are dropped
// — the project's projections engine has no use for them.
func (c *Client) GetSettings() (*Settings, error) {
	var raw settingsResponse
	if err := c.get(c.leagueURL([]string{"mSettings"}), "", &raw); err != nil {
		return nil, fmt.Errorf("get league settings: %w", err)
	}

	out := &Settings{
		HitterWeights:  map[string]float64{},
		PitcherWeights: map[string]float64{},
		SeasonID:       raw.SeasonID,
	}
	for _, item := range raw.Settings.ScoringSettings.ScoringItems {
		if item.Points == 0 {
			continue
		}
		// Split by domain — same statId space, but ESPN uses different IDs
		// for the hitter and pitcher version of the "same" stat (HBP, BB,
		// HR, etc.). The Hitter/Pitcher tables are mutually exclusive so a
		// single statId can never land in both maps.
		if name, ok := HitterStatID[item.StatID]; ok {
			out.HitterWeights[name] = item.Points
			continue
		}
		if name, ok := PitcherStatID[item.StatID]; ok {
			pts := item.Points
			// statId 34 (OUTS) → "IP" — ESPN scores per out, project's
			// PitcherExpectedPtsFromProj scores per IP. Three outs equal
			// one IP, so the equivalent IP weight is 3× the OUTS weight.
			if item.StatID == 34 {
				pts *= 3
			}
			out.PitcherWeights[name] = pts
		}
	}
	return out, nil
}
