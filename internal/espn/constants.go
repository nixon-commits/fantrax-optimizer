// Package espn is a thin client for ESPN's unofficial Fantasy Baseball v3 API.
//
// Read-only MVP that powers the waivers command for ESPN H2H Points leagues.
// The clean Platform abstraction (shared types with internal/fantrax) is
// deliberately deferred — see /Users/jnixon/.claude/plans for the design
// decision. This package owns its own types and never imports internal/fantrax.
package espn

// Slot IDs ported from cwendt94/espn-api/baseball/constant.py.
// ESPN uses these numeric codes for lineup slots, eligibility and Free Agent
// queries. Slots 18, 21, 22 appear in some responses but their meaning is
// undocumented; we ignore them.
const (
	SlotC     = 0
	Slot1B    = 1
	Slot2B    = 2
	Slot3B    = 3
	SlotSS    = 4
	SlotOF    = 5
	Slot2BSS  = 6
	Slot1B3B  = 7
	SlotLF    = 8
	SlotCF    = 9
	SlotRF    = 10
	SlotDH    = 11
	SlotUTIL  = 12
	SlotP     = 13
	SlotSP    = 14
	SlotRP    = 15
	SlotBench = 16
	SlotIL    = 17
	SlotIF    = 19
)

// SlotName maps the ESPN slot ID to the canonical short name used by the
// rest of the project. Pitcher/hitter eligibility logic in the waivers
// package keys off these names ("SP", "RP", "P", "OF", ...).
var SlotName = map[int]string{
	SlotC:    "C",
	Slot1B:   "1B",
	Slot2B:   "2B",
	Slot3B:   "3B",
	SlotSS:   "SS",
	SlotOF:   "OF",
	Slot2BSS: "2B/SS",
	Slot1B3B: "1B/3B",
	SlotLF:   "LF",
	SlotCF:   "CF",
	SlotRF:   "RF",
	SlotDH:   "DH",
	SlotUTIL: "UTIL",
	SlotP:    "P",
	SlotSP:   "SP",
	SlotRP:   "RP",
	SlotIF:   "IF",
}

// PositionName maps ESPN's defaultPositionId (the "natural" position field
// on player records, not the lineup slot) to a canonical short name.
// ESPN uses 1-11; cwendt94's mapping: 1=SP, 2=C, 3=1B, 4=2B, 5=3B, 6=SS,
// 7=LF, 8=CF, 9=RF, 10=DH, 11=RP. We collapse LF/CF/RF to OF since the
// project's scoring/eligibility model treats them uniformly.
var PositionName = map[int]string{
	1:  "SP",
	2:  "C",
	3:  "1B",
	4:  "2B",
	5:  "3B",
	6:  "SS",
	7:  "OF",
	8:  "OF",
	9:  "OF",
	10: "DH",
	11: "RP",
}

// MLBTeam maps ESPN's proTeamId (1-30) to the 3-letter MLBAM abbreviation
// used by Statcast joins and the FanGraphs/Steamer projection lookup. ID 0
// represents free agency. Pulled from cwendt94/espn-api PRO_TEAM_MAP.
var MLBTeam = map[int]string{
	0:  "FA",
	1:  "BAL",
	2:  "BOS",
	3:  "LAA",
	4:  "CHW",
	5:  "CLE",
	6:  "DET",
	7:  "KC",
	8:  "MIL",
	9:  "MIN",
	10: "NYY",
	11: "OAK",
	12: "SEA",
	13: "TEX",
	14: "TOR",
	15: "ATL",
	16: "CHC",
	17: "CIN",
	18: "HOU",
	19: "LAD",
	20: "WSH",
	21: "NYM",
	22: "PHI",
	23: "PIT",
	24: "STL",
	25: "SD",
	26: "SF",
	27: "COL",
	28: "MIA",
	29: "ARI",
	30: "TB",
}

// HitterStatID and PitcherStatID maps translate ESPN's numeric statId to the
// short-name keys this project's scoring engine recognizes (the same keys
// produced by *fantrax.Client.GetScoringWeights and consumed by
// internal/projections.ExpectedPtsFromProj / PitcherExpectedPtsFromProj).
//
// ESPN uses different stat IDs for batter and pitcher versions of the same
// abstract stat (e.g. statId 10 is hitter walks, statId 39 is pitcher walks
// — both must surface as "BB" in their respective scoring map). Splitting
// the table by domain makes that intent explicit and lets a single league
// scoring item land in the correct (and only the correct) weights map.
//
// IDs ported from cwendt94/espn-api/baseball/constant.py STATS_MAP. Stat IDs
// the project's scoring model doesn't recognize are intentionally omitted —
// they get silently dropped rather than producing a phantom weight key the
// projection engine would never consult.

// HitterStatID maps ESPN batting/hitter statIds to short names.
var HitterStatID = map[int]string{
	0:  "AB",
	1:  "H",
	2:  "AVG",
	3:  "2B",
	4:  "3B",
	5:  "HR",
	6:  "XBH",
	7:  "1B",
	8:  "TB",
	9:  "SLG",
	10: "BB",  // hitter walks
	11: "IBB", // hitter intentional walks
	12: "HBP", // hit by pitch (hitter)
	13: "SF",
	14: "SH",
	15: "SAC",
	16: "PA",
	17: "OBP",
	18: "OPS",
	19: "RC",
	20: "R",
	21: "RBI",
	23: "SB",
	24: "CS",
	25: "SB-CS",
	26: "GIDP", // grounded into double play
	27: "SO",   // hitter strikeouts
	31: "CYC",  // hit for the cycle
}

// PitcherStatID maps ESPN pitching statIds to the short-name keys the
// project's PitcherExpectedPtsFromProj recognizes. ESPN-only stats that
// the project's scoring engine doesn't model (rate stats like K/9, totals
// like TBF, no-hit/perfect-game bonuses) are intentionally absent — they
// get dropped rather than produce phantom keys nothing consumes.
//
// Note: statId 34 (OUTS) is special-cased in league.go since ESPN scores
// per-out but the project's engine scores per-IP — the parser multiplies
// the points by 3 and stores them under "IP" instead.
var PitcherStatID = map[int]string{
	34: "IP",  // OUTS — converted to IP-equivalent in league.go (×3 on points)
	37: "H",   // hits allowed
	39: "BB",  // walks issued
	42: "HBP", // hit batters
	45: "ER",  // earned runs
	46: "HR",  // home runs allowed
	48: "K",   // strikeouts (pitcher)
	50: "WP",  // wild pitch
	51: "BK",  // balk (project uses "BK" not "BLK")
	52: "PKO", // pickoff (project uses "PKO" not "PK")
	53: "W",
	54: "L",
	57: "SV",
	58: "BS", // blown save
	60: "HLD",
	62: "CG",
	63: "QS",
}

// IsPitcherStatID returns true when the given ESPN statId belongs to the
// pitching domain. Used to split a single ESPN scoring config into separate
// hitter/pitcher weight maps that match the project's two-source model.
func IsPitcherStatID(id int) bool {
	_, ok := PitcherStatID[id]
	return ok
}

// IsHitterStatID returns true when the given ESPN statId belongs to the
// hitting domain.
func IsHitterStatID(id int) bool {
	_, ok := HitterStatID[id]
	return ok
}
