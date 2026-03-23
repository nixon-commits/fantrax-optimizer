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
