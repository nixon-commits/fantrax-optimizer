package scoring

// PitcherLine is a neutral set of raw pitcher counting stats for one scope. QS
// is supplied by the caller: FanGraphs provides it for projections, while the
// game-log adapter derives it (IP ≥ 6 AND ER ≤ 3). Events a source doesn't
// track (e.g. CG/SHO/PKO from an MLB game log) are left zero.
type PitcherLine struct {
	IP  float64
	K   float64
	BB  float64
	H   float64
	ER  float64
	HR  float64
	W   float64
	L   float64
	QS  float64
	SV  float64
	HLD float64
	BS  float64
	HBP float64
	WP  float64
	BK  float64
	CG  float64
	SHO float64
	PKO float64
}

// ApplyPitcher scores a PitcherLine against the league weights. Like
// ApplyHitter, it does no per-game normalization — divide by games for a
// per-game value. Stats with no configured weight are skipped.
func ApplyPitcher(l PitcherLine, w Weights) float64 {
	statMap := map[string]float64{
		"IP": l.IP, "K": l.K, "BB": l.BB, "H": l.H,
		"ER": l.ER, "HR": l.HR, "W": l.W, "L": l.L,
		"QS": l.QS, "SV": l.SV, "HLD": l.HLD, "BS": l.BS,
		"HBP": l.HBP, "WP": l.WP, "BK": l.BK,
		"CG": l.CG, "SHO": l.SHO, "PKO": l.PKO,
	}

	var total float64
	for stat, val := range statMap {
		if pts, ok := w[stat]; ok {
			total += val * pts
		}
	}
	return total
}
