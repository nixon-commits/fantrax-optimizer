package projections

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFanGraphsSource_ParsesJSON(t *testing.T) {
	fixture := []map[string]interface{}{
		{"PlayerName": "Aaron Judge", "Team": "NYY", "G": 141.0, "PA": 633.0, "H": 143.0,
			"1B": 77.0, "2B": 23.0, "3B": 1.0, "HR": 42.0,
			"R": 109.0, "RBI": 102.0, "BB": 112.0, "SB": 9.0, "CS": 2.0, "HBP": 6.0, "SO": 156.0, "GDP": nil},
		{"PlayerName": "Freddie Freeman", "Team": "LAD", "G": 138.0, "PA": 590.0, "H": 160.0,
			"1B": 100.0, "2B": 35.0, "3B": 1.0, "HR": 24.0,
			"R": 95.0, "RBI": 90.0, "BB": 70.0, "SB": 10.0, "CS": 3.0, "HBP": 6.0, "SO": 100.0, "GDP": 10.0},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fixture)
	}))
	defer srv.Close()

	orig := fangraphsBattingURL
	fangraphsBattingURL = srv.URL
	defer func() { fangraphsBattingURL = orig }()

	src, err := NewFanGraphsSource()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, ok := src.GetProjection("Aaron Judge", "NYY")
	if !ok {
		t.Fatal("expected projection for Aaron Judge")
	}
	if p.HR != 42 {
		t.Errorf("expected HR=42, got %v", p.HR)
	}
	if p.G != 141 {
		t.Errorf("expected G=141, got %v", p.G)
	}
}

func TestFanGraphsSource_CaseInsensitiveTeam(t *testing.T) {
	src := &FanGraphsSource{projections: map[string]*Projection{
		"freddie freeman|LAD": {G: 138, HR: 24},
	}}

	p, ok := src.GetProjection("Freddie Freeman", "lad")
	if !ok {
		t.Fatal("expected projection with lowercase team")
	}
	if p.HR != 24 {
		t.Errorf("expected HR=24, got %v", p.HR)
	}
}

func TestFanGraphsSource_NameFallback(t *testing.T) {
	src := &FanGraphsSource{projections: map[string]*Projection{
		"manny machado|SD": {G: 140, HR: 26},
	}}

	// Different team (traded) - should still find by name.
	p, ok := src.GetProjection("Manny Machado", "LAD")
	if !ok {
		t.Fatal("expected name-only fallback to work")
	}
	if p.HR != 26 {
		t.Errorf("expected HR=26, got %v", p.HR)
	}
}

func TestChainedSource_FallsThrough(t *testing.T) {
	primary := &FanGraphsSource{projections: map[string]*Projection{
		"aaron judge|NYY": {G: 141, HR: 40},
	}}

	rolling := NewRollingSource()
	rolling.AddPlayer("mystery player", 14, 2.0, 0.5, 0.1, 0.3, 1.5, 1.2, 1.0, 0.3, 0.0, 0.1)

	chained := NewChainedSource(primary, rolling)

	_, ok := chained.GetProjection("Aaron Judge", "NYY")
	if !ok {
		t.Error("expected primary source hit")
	}

	_, ok2 := chained.GetProjection("mystery player", "COL")
	if !ok2 {
		t.Error("expected rolling fallback hit")
	}

	_, ok3 := chained.GetProjection("nobody", "XYZ")
	if ok3 {
		t.Error("expected miss for unknown player")
	}
}
