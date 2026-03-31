package transactions

import (
	"testing"
	"time"

	"github.com/nixon-commits/rosterbot/internal/hkb"
	"github.com/pmurley/go-fantrax/models"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Bobby Witt Jr.", "bobby witt"},
		{"Vladimir Guerrero Jr.", "vladimir guerrero"},
		{"Ken Griffey Sr.", "ken griffey"},
		{"Ronald Acuña Jr.", "ronald acuña"},
		{"Mike Trout", "mike trout"},
		{"  Juan Soto  ", "juan soto"},
		{"Cal Ripken III", "cal ripken"},
		{"Ken Griffey II", "ken griffey"},
	}
	for _, tt := range tests {
		if got := normalizeName(tt.input); got != tt.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildHKBLookup(t *testing.T) {
	players := []hkb.Player{
		{Name: "Bobby Witt Jr.", Value: 10000},
		{Name: "Juan Soto", Value: 8782},
	}
	lookup := buildHKBLookup(players)

	if p, ok := lookup["bobby witt"]; !ok || p.Value != 10000 {
		t.Error("expected Bobby Witt Jr. lookup to work")
	}
	if p, ok := lookup["juan soto"]; !ok || p.Value != 8782 {
		t.Error("expected Juan Soto lookup to work")
	}
}

func TestGroupTrades(t *testing.T) {
	now := time.Now()
	txs := []models.Transaction{
		{
			TradeGroupID:   "trade1",
			ToTeamName:     "Team Alpha",
			FromTeamName:   "Team Beta",
			PlayerName:     "Bobby Witt Jr.",
			PlayerPosition: "SS",
			ProcessedDate:  now,
		},
		{
			TradeGroupID:   "trade1",
			ToTeamName:     "Team Beta",
			FromTeamName:   "Team Alpha",
			PlayerName:     "Juan Soto",
			PlayerPosition: "OF",
			ProcessedDate:  now,
		},
		{
			TradeGroupID:   "trade1",
			ToTeamName:     "Team Beta",
			FromTeamName:   "Team Alpha",
			PlayerName:     "Mike Trout",
			PlayerPosition: "OF",
			ProcessedDate:  now,
		},
	}

	lookup := buildHKBLookup([]hkb.Player{
		{Name: "Bobby Witt Jr.", Value: 10000},
		{Name: "Juan Soto", Value: 8782},
	})

	trades := groupTrades(txs, lookup)
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	trade := trades[0]
	// Find which side is Team Alpha and which is Team Beta.
	var alpha, beta *TradeSide
	for i := range trade.Sides {
		switch trade.Sides[i].TeamName {
		case "Team Alpha":
			alpha = &trade.Sides[i]
		case "Team Beta":
			beta = &trade.Sides[i]
		}
	}
	if alpha == nil || beta == nil {
		t.Fatal("expected both Team Alpha and Team Beta sides")
	}

	if len(alpha.Players) != 1 {
		t.Errorf("Team Alpha should receive 1 player, got %d", len(alpha.Players))
	}
	if alpha.Total != 10000 {
		t.Errorf("Team Alpha total = %d, want 10000", alpha.Total)
	}

	if len(beta.Players) != 2 {
		t.Errorf("Team Beta should receive 2 players, got %d", len(beta.Players))
	}
	if beta.Total != 8782 {
		t.Errorf("Team Beta total = %d, want 8782 (Soto=8782, Trout=unranked=0)", beta.Total)
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1,000"},
		{10000, "10,000"},
		{1234567, "1,234,567"},
	}
	for _, tt := range tests {
		if got := formatValue(tt.input); got != tt.want {
			t.Errorf("formatValue(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatReport(t *testing.T) {
	trades := []Trade{
		{
			ProcessedDate: time.Now(),
			Sides: [2]TradeSide{
				{
					TeamName: "Team A",
					Players: []TradePlayer{
						{Name: "Player X", Position: "SS", Value: 5200, Ranked: true},
					},
					Total: 5200,
				},
				{
					TeamName: "Team B",
					Players: []TradePlayer{
						{Name: "Player Y", Position: "OF", Value: 0, Ranked: false},
					},
					Total: 0,
				},
			},
		},
	}

	report := formatReport(trades, true)
	if report == "" {
		t.Fatal("expected non-empty report")
	}
	if !contains(report, "Team A <-> Team B") {
		t.Error("report should contain trade header")
	}
	if !contains(report, "Player X (SS) 5,200") {
		t.Error("report should contain ranked player with value")
	}
	if !contains(report, "Player Y (OF) unranked") {
		t.Error("report should contain unranked player")
	}
	if !contains(report, colorGreen) {
		t.Error("report should contain green color for winning side")
	}
	if !contains(report, colorRed) {
		t.Error("report should contain red color for losing side")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
