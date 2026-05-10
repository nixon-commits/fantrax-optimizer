package espn

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient("12345", 7, 2026, "{abc-swid}", "espn-s2-cookie-value")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.SetBaseURL(srv.URL)
	return c, srv
}

func TestNewClient_ValidatesAuth(t *testing.T) {
	if _, err := NewClient("123", 0, 2026, "", "s2"); err == nil {
		t.Error("expected error when SWID is empty")
	}
	if _, err := NewClient("123", 0, 2026, "swid", ""); err == nil {
		t.Error("expected error when espn_s2 is empty")
	}
	if _, err := NewClient("", 0, 2026, "swid", "s2"); err == nil {
		t.Error("expected error when leagueID is empty")
	}
}

func TestClient_AttachesCookies(t *testing.T) {
	var sawSWID, sawS2 string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, c := range r.Cookies() {
			if c.Name == "SWID" {
				sawSWID = c.Value
			}
			if c.Name == "espn_s2" {
				sawS2 = c.Value
			}
		}
		w.Write([]byte(`{"seasonId":2026,"settings":{"scoringSettings":{"scoringItems":[]}}}`))
	})
	c, _ := newTestClient(t, h)
	if _, err := c.GetSettings(); err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if sawSWID != "{abc-swid}" {
		t.Errorf("SWID cookie not sent; got %q", sawSWID)
	}
	if sawS2 != "espn-s2-cookie-value" {
		t.Errorf("espn_s2 cookie not sent; got %q", sawS2)
	}
}

func TestGetSettings_SplitsHitterAndPitcher(t *testing.T) {
	// Stat IDs per cwendt94/espn-api STATS_MAP:
	//   5  → HR (hitter)
	//   21 → RBI (hitter)
	//   27 → SO (hitter strikeouts)
	//   12 → HBP (hitter)         — used to verify zero-points entries are dropped
	//   48 → K  (pitcher strikeouts)
	//   53 → W  (pitcher wins)
	//   99 → unknown — must be ignored
	body := map[string]any{
		"seasonId": 2026,
		"settings": map[string]any{
			"scoringSettings": map[string]any{
				"scoringItems": []map[string]any{
					{"statId": 5, "points": 4.0},
					{"statId": 21, "points": 1.0},
					{"statId": 27, "points": -0.5},
					{"statId": 48, "points": 2.0},
					{"statId": 53, "points": 5.0},
					{"statId": 99, "points": 1.0},
					{"statId": 12, "points": 0.0},
				},
			},
		},
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "view=mSettings") {
			t.Errorf("expected view=mSettings in query, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(body)
	})
	c, _ := newTestClient(t, h)

	got, err := c.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.SeasonID != 2026 {
		t.Errorf("SeasonID = %d, want 2026", got.SeasonID)
	}
	wantHit := map[string]float64{"HR": 4.0, "RBI": 1.0, "SO": -0.5}
	wantPit := map[string]float64{"K": 2.0, "W": 5.0}
	if !mapsEqual(got.HitterWeights, wantHit) {
		t.Errorf("HitterWeights = %v, want %v", got.HitterWeights, wantHit)
	}
	if !mapsEqual(got.PitcherWeights, wantPit) {
		t.Errorf("PitcherWeights = %v, want %v", got.PitcherWeights, wantPit)
	}
}

func TestGetFreeAgents_FiltersAndConverts(t *testing.T) {
	// kona_player_info on the league endpoint returns a top-level `players`
	// array, each entry wrapping the player record under a `player` envelope.
	body := map[string]any{
		"players": []map[string]any{
			{"player": map[string]any{
				"id": 100, "fullName": "Free Hitter", "proTeamId": 10,
				"defaultPositionId": 7, "eligibleSlots": []int{5, 8, 12, 16},
				"onTeamId": 0, "injuryStatus": "ACTIVE",
				"ownership": map[string]float64{"percentOwned": 12.5},
			}},
			{"player": map[string]any{
				"id": 200, "fullName": "Free Pitcher", "proTeamId": 15,
				"defaultPositionId": 1, "eligibleSlots": []int{14, 13, 16},
				"onTeamId": 0, "injuryStatus": "",
			}},
			{"player": map[string]any{
				"id": 300, "fullName": "Already Rostered", "proTeamId": 1,
				"onTeamId": 4, // non-zero — must be filtered out
			}},
		},
	}
	var sawFilter string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawFilter = r.Header.Get("X-Fantasy-Filter")
		json.NewEncoder(w).Encode(body)
	})
	c, _ := newTestClient(t, h)

	got, err := c.GetFreeAgents(0)
	if err != nil {
		t.Fatalf("GetFreeAgents: %v", err)
	}
	if sawFilter == "" {
		t.Error("X-Fantasy-Filter header was not set")
	}
	if !strings.Contains(sawFilter, "FREEAGENT") {
		t.Errorf("filter missing FREEAGENT status: %s", sawFilter)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 free agents, got %d", len(got))
	}
	if got[0].Name != "Free Hitter" || got[0].MLBTeam != "NYY" {
		t.Errorf("first FA = %+v, want Free Hitter/NYY", got[0])
	}
	if len(got[0].Positions) != 1 || got[0].Positions[0] != "OF" {
		t.Errorf("first FA positions = %v, want [OF]", got[0].Positions)
	}
	if got[1].Name != "Free Pitcher" || got[1].MLBTeam != "ATL" {
		t.Errorf("second FA = %+v, want Free Pitcher/ATL", got[1])
	}
	if len(got[1].Positions) != 1 || got[1].Positions[0] != "SP" {
		t.Errorf("second FA positions = %v, want [SP]", got[1].Positions)
	}
}

func TestGetTeamRoster_FiltersByTeamID(t *testing.T) {
	body := map[string]any{
		"teams": []map[string]any{
			{"id": 7, "roster": map[string]any{
				"entries": []map[string]any{
					{"lineupSlotId": 5, "playerId": 100, "playerPoolEntry": map[string]any{
						"player": map[string]any{
							"id": 100, "fullName": "My Hitter", "proTeamId": 10,
							"eligibleSlots": []int{5, 16}, "onTeamId": 7,
						},
					}},
				},
			}},
			{"id": 4, "roster": map[string]any{
				"entries": []map[string]any{
					{"lineupSlotId": 14, "playerId": 200, "playerPoolEntry": map[string]any{
						"player": map[string]any{
							"id": 200, "fullName": "Other Team Pitcher",
						},
					}},
				},
			}},
		},
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(body)
	})
	c, _ := newTestClient(t, h)

	got, err := c.GetTeamRoster(0) // 0 → use client default (7)
	if err != nil {
		t.Fatalf("GetTeamRoster: %v", err)
	}
	if len(got) != 1 || got[0].Name != "My Hitter" {
		t.Errorf("roster = %+v, want one player My Hitter", got)
	}
}

func TestGet_AuthFailureMessage(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	})
	c, _ := newTestClient(t, h)
	_, err := c.GetSettings()
	if err == nil || !strings.Contains(err.Error(), "SWID/espn_s2") {
		t.Errorf("expected auth-error hint, got %v", err)
	}
}

func TestGet_NonOKBubbles(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	})
	c, _ := newTestClient(t, h)
	_, err := c.GetSettings()
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}

func TestPlayerFromNode_InjuryStatus(t *testing.T) {
	p := playerFromNode(playerNode{
		ID: 1, FullName: "X", ProTeamID: 10, InjuryStatus: "IL10",
	}, 0)
	if !p.IsInjured() {
		t.Error("IL10 should be injured")
	}
	p2 := playerFromNode(playerNode{
		ID: 2, FullName: "Y", ProTeamID: 10, InjuryStatus: "ACTIVE",
	}, 0)
	if p2.IsInjured() {
		t.Error("ACTIVE should not be injured")
	}
}

func mapsEqual(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
