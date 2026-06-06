package scoring

// HitterLine is a neutral set of raw hitter counting stats for one scope — a
// season projection or a single game. Singles, XBH, and TB are derived by
// ApplyHitter, so callers never compute them.
type HitterLine struct {
	H       float64
	Doubles float64
	Triples float64
	HR      float64
	RBI     float64
	R       float64
	BB      float64
	SB      float64
	CS      float64
	HBP     float64
	SO      float64
	GIDP    float64
}

// ApplyHitter scores a HitterLine against the league weights, returning total
// fantasy points for the counts as given. It does no per-game normalization:
// scoring is linear, so divide the result by games for a per-game value
// (Σ(val/G·pts) == (Σ val·pts)/G).
//
// Derived stats: 1B = H - 2B - 3B - HR (floored at 0), XBH = 2B + 3B + HR,
// TB = 1B + 2·2B + 3·3B + 4·HR. Stats with no configured weight are skipped.
func ApplyHitter(l HitterLine, w Weights) float64 {
	singles := l.H - l.Doubles - l.Triples - l.HR
	if singles < 0 {
		singles = 0
	}
	xbh := l.Doubles + l.Triples + l.HR
	tb := singles + 2*l.Doubles + 3*l.Triples + 4*l.HR

	statMap := map[string]float64{
		"1B": singles, "2B": l.Doubles, "3B": l.Triples,
		"HR": l.HR, "RBI": l.RBI, "R": l.R,
		"BB": l.BB, "SB": l.SB, "CS": l.CS,
		"HBP": l.HBP, "SO": l.SO, "GIDP": l.GIDP,
		"XBH": xbh, "TB": tb,
	}

	var total float64
	for stat, val := range statMap {
		if pts, ok := w[stat]; ok {
			total += val * pts
		}
	}
	return total
}
