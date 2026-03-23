package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Username     string
	Password     string
	LeagueID     string
	TeamID       string
	DryRun       bool
	Dates        []time.Time
	ILSlots      int
	MinorsSlots  int
}

func Load(dryRun bool, dates []time.Time) (*Config, error) {
	// Load .env if present (local dev); ignore error if missing (GHA uses env directly)
	_ = godotenv.Load()

	cfg := &Config{
		Username:    os.Getenv("FANTRAX_USERNAME"),
		Password:    os.Getenv("FANTRAX_PASSWORD"),
		LeagueID:    os.Getenv("FANTRAX_LEAGUE_ID"),
		TeamID:      os.Getenv("FANTRAX_TEAM_ID"),
		DryRun:      dryRun,
		Dates:       dates,
		ILSlots:     envInt("FANTRAX_IL_SLOTS", 0),
		MinorsSlots: envInt("FANTRAX_MINORS_SLOTS", 0),
	}

	var missing []string
	for name, val := range map[string]string{
		"FANTRAX_USERNAME":  cfg.Username,
		"FANTRAX_PASSWORD":  cfg.Password,
		"FANTRAX_LEAGUE_ID": cfg.LeagueID,
		"FANTRAX_TEAM_ID":   cfg.TeamID,
	} {
		if val == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, nil
}

func envInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
