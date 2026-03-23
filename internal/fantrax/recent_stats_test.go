package fantrax

import (
	"testing"

	"github.com/pmurley/go-fantrax/models"
)

func ptr[T any](v T) *T { return &v }

func TestAggregateRecentStats(t *testing.T) {
	fp1 := 8.5
	gp1 := 1
	fp2 := 3.0
	gp2 := 1

	period1 := []models.RosterPlayer{
		{
			PlayerID: "abc",
			Name:     "Test Player",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: &fp1,
					GamesPlayed:          &gp1,
				},
			},
		},
	}
	period2 := []models.RosterPlayer{
		{
			PlayerID: "abc",
			Name:     "Test Player",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: &fp2,
					GamesPlayed:          &gp2,
				},
			},
		},
	}

	result := aggregateRecentStats([][]models.RosterPlayer{period1, period2})

	stat, ok := result["abc"]
	if !ok {
		t.Fatal("expected player 'abc' in result")
	}
	if stat.TotalFP != 11.5 {
		t.Errorf("TotalFP: got %v, want 11.5", stat.TotalFP)
	}
	if stat.GamesPlayed != 2 {
		t.Errorf("GamesPlayed: got %v, want 2", stat.GamesPlayed)
	}
}

func TestAggregateRecentStats_NilStats(t *testing.T) {
	gp0 := 0

	period := []models.RosterPlayer{
		{
			PlayerID: "xyz",
			Name:     "Nil Stats Player",
			Stats: &models.PlayerStats{
				Batting: &models.BattingStats{
					FantasyPointsPerGame: nil,
					GamesPlayed:          &gp0,
				},
			},
		},
	}

	result := aggregateRecentStats([][]models.RosterPlayer{period})

	stat := result["xyz"]
	if stat.TotalFP != 0 {
		t.Errorf("TotalFP: got %v, want 0", stat.TotalFP)
	}
	if stat.GamesPlayed != 0 {
		t.Errorf("GamesPlayed: got %v, want 0", stat.GamesPlayed)
	}
}

func TestAggregateRecentStats_NilPlayerStats(t *testing.T) {
	period := []models.RosterPlayer{
		{PlayerID: "a", Name: "No Stats", Stats: nil},
	}
	result := aggregateRecentStats([][]models.RosterPlayer{period})
	if _, ok := result["a"]; ok {
		t.Error("expected player with nil Stats to be skipped")
	}
}

func TestAggregateRecentStats_NilBatting(t *testing.T) {
	period := []models.RosterPlayer{
		{PlayerID: "b", Name: "No Batting", Stats: &models.PlayerStats{Batting: nil}},
	}
	result := aggregateRecentStats([][]models.RosterPlayer{period})
	if _, ok := result["b"]; ok {
		t.Error("expected player with nil Batting to be skipped")
	}
}

func TestAggregateRecentStats_NilGamesPlayed(t *testing.T) {
	period := []models.RosterPlayer{
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
	result := aggregateRecentStats([][]models.RosterPlayer{period})
	if _, ok := result["c"]; ok {
		t.Error("expected player with nil GamesPlayed to be skipped")
	}
}
