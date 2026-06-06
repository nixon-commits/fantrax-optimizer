package scoring

import "testing"

const eps = 1e-9

func almostEqual(a, b float64) bool {
	d := a - b
	return d < eps && d > -eps
}

func TestApplyHitter(t *testing.T) {
	// A weight per scored category, including the derived 1B/XBH/TB.
	w := Weights{
		"1B": 1, "2B": 2, "3B": 3, "HR": 4,
		"RBI": 1, "R": 1, "BB": 1, "SB": 2, "CS": -1,
		"HBP": 1, "SO": -0.5, "GIDP": -1, "XBH": 1, "TB": 1,
	}

	tests := []struct {
		name string
		line HitterLine
		want float64
	}{
		{
			name: "derives 1B/XBH/TB from H and extra-base hits",
			// H=10 → 1B = 10-2-1-1 = 6. XBH = 2+1+1 = 4. TB = 6 + 4 + 3 + 4 = 17.
			line: HitterLine{H: 10, Doubles: 2, Triples: 1, HR: 1, RBI: 5, R: 4, BB: 3, SB: 1, CS: 1, HBP: 1, SO: 6, GIDP: 1},
			// 1B:6 + 2B:4 + 3B:3 + HR:4 + RBI:5 + R:4 + BB:3 + SB:2 + CS:-1
			// + HBP:1 + SO:-3 + GIDP:-1 + XBH:4 + TB:17 = 48
			want: 48,
		},
		{
			name: "clamps negative singles to zero",
			// H=2 but XBH=3 → raw 1B = -1, floored to 0. XBH=3, TB = 0 + 2 + 3 + 4 = 9.
			line: HitterLine{H: 2, Doubles: 1, Triples: 1, HR: 1},
			// 1B:0 + 2B:2 + 3B:3 + HR:4 + XBH:3 + TB:9 = 21
			want: 21,
		},
		{
			name: "empty line scores zero",
			line: HitterLine{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyHitter(tt.line, w); !almostEqual(got, tt.want) {
				t.Errorf("ApplyHitter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyHitter_SkipsUnweightedStats(t *testing.T) {
	// Only HR is weighted; everything else must be ignored.
	w := Weights{"HR": 4}
	line := HitterLine{H: 10, Doubles: 5, Triples: 2, HR: 3, RBI: 9, BB: 4}
	if got := ApplyHitter(line, w); !almostEqual(got, 12) {
		t.Errorf("ApplyHitter() = %v, want 12 (3 HR × 4)", got)
	}
}

func TestApplyHitter_LinearityForPerGame(t *testing.T) {
	// Scoring is linear, so the per-game value can be taken either by dividing
	// the line or dividing the result. The adapters rely on this.
	w := Weights{"1B": 1, "2B": 2, "HR": 4, "TB": 1, "XBH": 1}
	line := HitterLine{H: 12, Doubles: 3, HR: 2}
	const g = 4
	perGameLine := HitterLine{H: line.H / g, Doubles: line.Doubles / g, HR: line.HR / g}
	if a, b := ApplyHitter(line, w)/g, ApplyHitter(perGameLine, w); !almostEqual(a, b) {
		t.Errorf("linearity broken: result/G=%v, perGameLine=%v", a, b)
	}
}

func TestApplyPitcher(t *testing.T) {
	w := Weights{
		"IP": 1, "K": 1, "BB": -0.5, "H": -0.5, "ER": -1, "HR": -1,
		"W": 4, "L": -2, "QS": 3, "SV": 5, "HLD": 2, "BS": -2,
		"HBP": -0.5, "WP": -0.5, "BK": -0.5, "CG": 5, "SHO": 5, "PKO": 1,
	}

	tests := []struct {
		name string
		line PitcherLine
		want float64
	}{
		{
			name: "quality start line",
			// IP:6 + K:8 + BB:-1 + H:-2.5 + ER:-2 + HR:-1 + W:4 + QS:3 = 14.5
			line: PitcherLine{IP: 6, K: 8, BB: 2, H: 5, ER: 2, HR: 1, W: 1, QS: 1},
			want: 14.5,
		},
		{
			name: "untracked CG/SHO/PKO stay zero",
			// Same as above but with zero CG/SHO/PKO — must match the QS line.
			line: PitcherLine{IP: 6, K: 8, BB: 2, H: 5, ER: 2, HR: 1, W: 1, QS: 1, CG: 0, SHO: 0, PKO: 0},
			want: 14.5,
		},
		{
			name: "reliever save",
			// IP:1 + K:2 + SV:5 + HLD:0 = 8
			line: PitcherLine{IP: 1, K: 2, SV: 1},
			want: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyPitcher(tt.line, w); !almostEqual(got, tt.want) {
				t.Errorf("ApplyPitcher() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPitcher_SkipsUnweightedStats(t *testing.T) {
	w := Weights{"K": 1}
	line := PitcherLine{IP: 6, K: 7, ER: 3, W: 1}
	if got := ApplyPitcher(line, w); !almostEqual(got, 7) {
		t.Errorf("ApplyPitcher() = %v, want 7 (7 K × 1)", got)
	}
}
