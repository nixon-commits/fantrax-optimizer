package roster

import (
	"testing"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

// openSlots returns slot counts with plenty of room so alerts aren't suppressed.
func openSlots() fantrax.SlotCounts {
	return fantrax.SlotCounts{ILUsed: 0, ILCapacity: 5, MinorsUsed: 0, MinorsCapacity: 5}
}

func TestCheckRoster_HealthyInIL(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Healthy Guy", Status: "Injured Reserve", IsInjured: false},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != HealthyInIL {
		t.Errorf("expected type %s, got %s", HealthyInIL, alerts[0].Type)
	}
}

func TestCheckRoster_CalledUpInMinors(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Called Up", Status: "Minors", InMinors: false},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != CalledUpInMinors {
		t.Errorf("expected type %s, got %s", CalledUpInMinors, alerts[0].Type)
	}
}

func TestCheckRoster_InjuredInActive(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Hurt Player", Status: "Active", IsInjured: true},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != InjuredInActive {
		t.Errorf("expected type %s, got %s", InjuredInActive, alerts[0].Type)
	}
}

func TestCheckRoster_MinorInActive(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Minor Leaguer", Status: "Reserve", InMinors: true},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != MinorInActive {
		t.Errorf("expected type %s, got %s", MinorInActive, alerts[0].Type)
	}
}

func TestCheckRoster_NoAlerts(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Active Healthy", Status: "Active", IsInjured: false, InMinors: false},
		{ID: "p2", Name: "Reserve Healthy", Status: "Reserve", IsInjured: false, InMinors: false},
		{ID: "p3", Name: "IL Injured", Status: "Injured Reserve", IsInjured: true},
		{ID: "p4", Name: "Minors Player", Status: "Minors", InMinors: true},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestCheckRoster_MultipleAlerts(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Healthy in IL", Status: "Injured Reserve", IsInjured: false},
		{ID: "p2", Name: "Called Up", Status: "Minors", InMinors: false},
		{ID: "p3", Name: "Hurt Active", Status: "Active", IsInjured: true},
		{ID: "p4", Name: "Minor on Reserve", Status: "Reserve", InMinors: true},
		{ID: "p5", Name: "Clean Player", Status: "Active", IsInjured: false, InMinors: false},
	}
	alerts := CheckRoster(players, openSlots())
	if len(alerts) != 4 {
		t.Fatalf("expected 4 alerts, got %d", len(alerts))
	}

	types := make(map[AlertType]bool)
	for _, a := range alerts {
		types[a.Type] = true
	}
	for _, want := range []AlertType{HealthyInIL, CalledUpInMinors, InjuredInActive, MinorInActive} {
		if !types[want] {
			t.Errorf("missing alert type %s", want)
		}
	}
}

func TestCheckRoster_ILFull_SuppressesAlert(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Hurt Active", Status: "Active", IsInjured: true},
	}
	// IL is full across all players (hitters + pitchers).
	counts := fantrax.SlotCounts{ILUsed: 6, ILCapacity: 6, MinorsUsed: 0, MinorsCapacity: 10}
	alerts := CheckRoster(players, counts)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts when IL is full, got %d", len(alerts))
		for _, a := range alerts {
			t.Logf("  alert: %s %s", a.Type, a.Player.Name)
		}
	}
}

func TestCheckRoster_MinorsFull_SuppressesAlert(t *testing.T) {
	players := []fantrax.Player{
		{ID: "p1", Name: "Minor on Reserve", Status: "Reserve", InMinors: true},
	}
	// Minors is full across all players (hitters + pitchers).
	counts := fantrax.SlotCounts{ILUsed: 0, ILCapacity: 6, MinorsUsed: 10, MinorsCapacity: 10}
	alerts := CheckRoster(players, counts)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts when Minors is full, got %d", len(alerts))
		for _, a := range alerts {
			t.Logf("  alert: %s %s", a.Type, a.Player.Name)
		}
	}
}

func TestCheckRoster_SlotsFull_StillAlertsOnWrongDirection(t *testing.T) {
	// Even when IL is full, healthy players IN IL should still be alerted.
	players := []fantrax.Player{
		{ID: "p1", Name: "IL Healthy", Status: "Injured Reserve", IsInjured: false},
	}
	counts := fantrax.SlotCounts{ILUsed: 6, ILCapacity: 6, MinorsUsed: 10, MinorsCapacity: 10}
	alerts := CheckRoster(players, counts)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != HealthyInIL {
		t.Errorf("expected %s, got %s", HealthyInIL, alerts[0].Type)
	}
}
