// Package positions is the single source of truth for Fantrax position-ID
// semantics: which numeric ID maps to which slot name, what a multi-position
// slot accepts, which IDs denote a pitcher, and how a hitter's eligibility
// buckets for reporting.
//
// It depends only on the upstream auth_client constants — filling the two IDs
// auth_client omits (second base "003" and the infield-utility slot INF "008")
// — so any package can import it without an import cycle.
package positions

import "github.com/pmurley/go-fantrax/auth_client"

// Fantrax numeric position IDs. Most come from auth_client; SecondBase ("003")
// and INF ("008") are not exported upstream, so they are defined here.
const (
	C          = auth_client.PosC    // "001" catcher
	FirstBase  = auth_client.Pos1B   // "002"
	SecondBase = "003"               // not exported by auth_client
	ThirdBase  = auth_client.Pos3B   // "004"
	SS         = auth_client.PosSS   // "005"
	INF        = "008"               // infield utility (1B/2B/3B/SS); not in auth_client
	OF         = auth_client.PosOF   // "012"
	UT         = auth_client.PosUtil // "014" accepts any hitter
	SP         = auth_client.PosSP   // "015"
	RP         = auth_client.PosRP   // "016"
	P          = auth_client.PosP    // "017" any pitcher (SP or RP)
	RP2        = auth_client.PosRP2  // "043"
	RP3        = auth_client.PosRP3  // "044"
)

// SlotName returns the display name for a position/slot ID ("OF", "SP", …), or
// "" when the ID is unknown.
func SlotName(posID string) string {
	switch posID {
	case C:
		return "C"
	case FirstBase:
		return "1B"
	case SecondBase:
		return "2B"
	case ThirdBase:
		return "3B"
	case SS:
		return "SS"
	case INF:
		return "INF"
	case OF:
		return "OF"
	case UT:
		return "UT"
	case SP:
		return "SP"
	case RP, RP2, RP3:
		return "RP"
	case P:
		return "P"
	}
	return ""
}

// infAccepts holds the hitter position IDs the INF slot accepts (not catcher).
var infAccepts = map[string]bool{FirstBase: true, SecondBase: true, ThirdBase: true, SS: true}

// AcceptsINF reports whether a player position ID qualifies for the INF slot.
func AcceptsINF(posID string) bool { return infAccepts[posID] }

// pitcherSlots holds the position IDs that denote a pitcher slot.
var pitcherSlots = map[string]bool{SP: true, RP: true, P: true, RP2: true, RP3: true}

// IsPitcherSlot reports whether a position ID denotes a pitcher slot.
func IsPitcherSlot(posID string) bool { return pitcherSlots[posID] }

// HitterBucket assigns a hitter to a reporting bucket from its eligibility IDs,
// with precedence C > INF > OF > UT (the scarcest defensive role a player
// qualifies for wins). Empty eligibility falls back to UT.
func HitterBucket(eligibility []string) string {
	has := func(id string) bool {
		for _, p := range eligibility {
			if p == id {
				return true
			}
		}
		return false
	}
	switch {
	case has(C):
		return "C"
	case has(FirstBase), has(SecondBase), has(ThirdBase), has(SS), has(INF):
		return "INF"
	case has(OF):
		return "OF"
	default:
		return "UT"
	}
}
