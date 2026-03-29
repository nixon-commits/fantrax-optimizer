package fantrax

import (
	"testing"

	"github.com/pmurley/go-fantrax/models"
)

func ptr[T any](v T) *T { return &v }

func TestExtractHitterStats(t *testing.T) {
	roster := []models.RosterPlayer{
		{
			PlayerID: "abc",
			Name:     "Test Player",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: ptr(5.75),
					GamesPlayed:          ptr(2),
				},
			},
		},
	}

	result := extractHitterStats(roster)

	stat, ok := result["abc"]
	if !ok {
		t.Fatal("expected player 'abc' in result")
	}
	if stat.FPtsPerGame != 5.75 {
		t.Errorf("FPtsPerGame: got %v, want 5.75", stat.FPtsPerGame)
	}
	if stat.GamesPlayed != 2 {
		t.Errorf("GamesPlayed: got %v, want 2", stat.GamesPlayed)
	}
}

func TestExtractHitterStats_NilFPG(t *testing.T) {
	roster := []models.RosterPlayer{
		{
			PlayerID: "xyz",
			Name:     "Nil Stats Player",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: nil,
					GamesPlayed:          ptr(0),
				},
			},
		},
	}

	result := extractHitterStats(roster)

	stat := result["xyz"]
	if stat.FPtsPerGame != 0 {
		t.Errorf("FPtsPerGame: got %v, want 0", stat.FPtsPerGame)
	}
	if stat.GamesPlayed != 0 {
		t.Errorf("GamesPlayed: got %v, want 0", stat.GamesPlayed)
	}
}

func TestExtractHitterStats_NilPlayerStats(t *testing.T) {
	roster := []models.RosterPlayer{
		{PlayerID: "a", Name: "No Stats", Stats: nil},
	}
	result := extractHitterStats(roster)
	if _, ok := result["a"]; ok {
		t.Error("expected player with nil Stats to be skipped")
	}
}

func TestExtractHitterStats_NilBatting(t *testing.T) {
	roster := []models.RosterPlayer{
		{PlayerID: "b", Name: "No Batting", Stats: &models.PlayerStats{Batting: nil}},
	}
	result := extractHitterStats(roster)
	if _, ok := result["b"]; ok {
		t.Error("expected player with nil Batting to be skipped")
	}
}

func TestExtractHitterStats_NilGamesPlayed(t *testing.T) {
	roster := []models.RosterPlayer{
		{
			PlayerID: "c",
			Name:     "Nil GP",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: ptr(5.0),
					GamesPlayed:          nil,
				},
			},
		},
	}
	result := extractHitterStats(roster)
	if _, ok := result["c"]; ok {
		t.Error("expected player with nil GamesPlayed to be skipped")
	}
}
