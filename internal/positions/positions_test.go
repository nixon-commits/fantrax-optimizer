package positions

import "testing"

func TestSlotName(t *testing.T) {
	cases := map[string]string{
		C: "C", FirstBase: "1B", SecondBase: "2B", ThirdBase: "3B", SS: "SS",
		INF: "INF", OF: "OF", UT: "UT", SP: "SP", RP: "RP", RP2: "RP", RP3: "RP",
		P: "P", "999": "",
	}
	for id, want := range cases {
		if got := SlotName(id); got != want {
			t.Errorf("SlotName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestAcceptsINF(t *testing.T) {
	for _, id := range []string{FirstBase, SecondBase, ThirdBase, SS} {
		if !AcceptsINF(id) {
			t.Errorf("AcceptsINF(%q) = false, want true", id)
		}
	}
	for _, id := range []string{C, OF, UT, SP} {
		if AcceptsINF(id) {
			t.Errorf("AcceptsINF(%q) = true, want false (catcher/outfield/util not infield)", id)
		}
	}
}

func TestIsPitcherSlot(t *testing.T) {
	for _, id := range []string{SP, RP, P, RP2, RP3} {
		if !IsPitcherSlot(id) {
			t.Errorf("IsPitcherSlot(%q) = false, want true", id)
		}
	}
	for _, id := range []string{C, FirstBase, OF, UT} {
		if IsPitcherSlot(id) {
			t.Errorf("IsPitcherSlot(%q) = true, want false", id)
		}
	}
}

func TestHitterBucket(t *testing.T) {
	cases := []struct {
		name        string
		eligibility []string
		want        string
	}{
		{"catcher wins over outfield", []string{C, OF}, "C"},
		{"infield wins over outfield", []string{ThirdBase, OF}, "INF"},
		{"INF slot id buckets as INF", []string{INF}, "INF"},
		{"outfield only", []string{OF}, "OF"},
		{"empty falls back to UT", nil, "UT"},
		{"util only falls back to UT", []string{UT}, "UT"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := HitterBucket(tt.eligibility); got != tt.want {
				t.Errorf("HitterBucket(%v) = %q, want %q", tt.eligibility, got, tt.want)
			}
		})
	}
}
