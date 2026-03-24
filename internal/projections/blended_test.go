package projections

import (
	"testing"

	"github.com/nixon-commits/rosterbot/internal/fantrax"
)

type stubSource struct {
	proj map[string]*Projection
}

func (s *stubSource) GetProjection(name, mlbTeam string) (*Projection, bool) {
	p, ok := s.proj[NormalizeName(name)]
	return p, ok
}

func TestBlendedSource_WithRecentStats(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{
		"test player": {G: 100, H: 100, HR: 20, RBI: 60, R: 50, BB: 40},
	}}
	scoring := fantrax.ScoringWeights{"HR": 4.0, "RBI": 1.0}
	// Steamer: (20/100)*4 + (60/100)*1 = 0.8 + 0.6 = 1.4
	// Recent: 10/5 = 2.0
	// Blended: 0.6*1.4 + 0.4*2.0 = 0.84 + 0.8 = 1.64

	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{
		"player1": {TotalFP: 10.0, GamesPlayed: 5},
	}, scoring, map[string]string{"test player": "player1"})

	pts, ok := src.GetPtsPerGame("Test Player", "NYY", scoring)
	if !ok {
		t.Fatal("expected true")
	}
	if pts < 1.63 || pts > 1.65 {
		t.Errorf("expected ~1.64, got %.4f", pts)
	}
}

func TestBlendedSource_NoRecentStats_FallsBackToSteamer(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{
		"test player": {G: 100, HR: 20},
	}}
	scoring := fantrax.ScoringWeights{"HR": 4.0}
	// Steamer only: (20/100)*4 = 0.8

	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, scoring,
		map[string]string{"test player": "player1"})

	pts, ok := src.GetPtsPerGame("Test Player", "NYY", scoring)
	if !ok {
		t.Fatal("expected true")
	}
	if pts < 0.79 || pts > 0.81 {
		t.Errorf("expected ~0.8, got %.4f", pts)
	}
}

func TestBlendedSource_NoSteamer_ReturnsFalse(t *testing.T) {
	inner := &stubSource{proj: map[string]*Projection{}}
	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, nil, map[string]string{})
	_, ok := src.GetPtsPerGame("Unknown Player", "NYY", fantrax.ScoringWeights{"HR": 4.0})
	if ok {
		t.Error("expected false for unknown player")
	}
}

func TestBlendedSource_GetProjection_Delegates(t *testing.T) {
	proj := &Projection{G: 100, HR: 20}
	inner := &stubSource{proj: map[string]*Projection{"test player": proj}}
	src := NewBlendedSource(inner, map[string]fantrax.RecentStat{}, nil, nil)
	p, ok := src.GetProjection("Test Player", "NYY")
	if !ok {
		t.Fatal("expected projection found")
	}
	if p.HR != 20 {
		t.Errorf("expected HR=20, got %.0f", p.HR)
	}
}
