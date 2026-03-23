package projections

// PitcherRollingSource provides a fallback pitcher projection based on
// recent stats. Values are stored as per-game rates and converted to
// season-equivalent totals (162 games) for scoring comparability.
type PitcherRollingSource struct {
	stats map[string]*PitcherProjection
}

// NewPitcherRollingSource creates an empty pitcher rolling source.
func NewPitcherRollingSource() *PitcherRollingSource {
	return &PitcherRollingSource{stats: make(map[string]*PitcherProjection)}
}

// AddPitcher records a pitcher's rolling per-game rates, annualized to 162 games.
func (s *PitcherRollingSource) AddPitcher(
	name string,
	gamesPlayed int,
	ip, k, bba, ha, er, hra, w, l, sv, hld float64,
) {
	if gamesPlayed <= 0 {
		return
	}
	scale := 162.0 / float64(gamesPlayed)
	s.stats[NormalizeName(name)] = &PitcherProjection{
		G: 162, GS: 0, IP: ip * scale, K: k * scale,
		BBA: bba * scale, HA: ha * scale, ER: er * scale, HRA: hra * scale,
		W: w * scale, L: l * scale, SV: sv * scale, HLD: hld * scale,
	}
}

// GetPitcherProjection implements PitcherSource.
func (s *PitcherRollingSource) GetPitcherProjection(name, _ string) (*PitcherProjection, bool) {
	p, ok := s.stats[NormalizeName(name)]
	return p, ok
}

// PitcherChainedSource tries pitcher sources in order, returning the first match.
type PitcherChainedSource struct {
	sources []PitcherSource
}

// NewPitcherChainedSource returns a source that tries each delegate in order.
func NewPitcherChainedSource(sources ...PitcherSource) *PitcherChainedSource {
	return &PitcherChainedSource{sources: sources}
}

func (c *PitcherChainedSource) GetPitcherProjection(name, mlbTeam string) (*PitcherProjection, bool) {
	for _, s := range c.sources {
		if p, ok := s.GetPitcherProjection(name, mlbTeam); ok {
			return p, true
		}
	}
	return nil, false
}
