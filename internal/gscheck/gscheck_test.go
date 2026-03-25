package gscheck

import (
	"strings"
	"testing"
)

func TestBuildReport(t *testing.T) {
	violations := []Violation{
		{TeamName: "Team Alpha", GSUsed: 14},
		{TeamName: "Team Beta", GSUsed: 13},
	}
	periodLabel := "Scoring Period 5 (2026-03-30 – 2026-04-05)"
	gsCap := 12

	title, summary := BuildReport(violations, periodLabel, gsCap)

	if !strings.Contains(title, "2 violation(s)") {
		t.Errorf("title missing violation count: %s", title)
	}
	if !strings.Contains(title, periodLabel) {
		t.Errorf("title missing period label: %s", title)
	}

	if !strings.Contains(summary, "Team Alpha (14 GS, +2)") {
		t.Errorf("summary missing Team Alpha: %s", summary)
	}
	if !strings.Contains(summary, "Team Beta (13 GS, +1)") {
		t.Errorf("summary missing Team Beta: %s", summary)
	}
	if !strings.Contains(summary, "cap 12") {
		t.Errorf("summary missing cap: %s", summary)
	}
}

func TestBuildReport_SingleViolation(t *testing.T) {
	violations := []Violation{
		{TeamName: "Violators", GSUsed: 15},
	}

	title, _ := BuildReport(violations, "Period 1", 10)
	if !strings.Contains(title, "1 violation(s)") {
		t.Errorf("title should show 1 violation: %s", title)
	}
}
