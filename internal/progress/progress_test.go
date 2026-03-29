package progress_test

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/nixon-commits/rosterbot/internal/progress"
)

// --- Non-interactive mode tests ---

func TestNonInteractiveHeader(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Header("Steamer", "2026-03-30", true)
	// Non-interactive uses log.Printf, not the writer — just verify no panic.
}

func TestNonInteractiveHeaderNoDryRun(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Header("Steamer", "2026-03-30", false)
	// Non-interactive uses log.Printf — verify no panic.
}

func TestNonInteractiveDone(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Done("Roster", "16 hitters (13 active)")
	// Non-interactive uses log.Printf — verify no panic.
}

func TestNonInteractiveWarn(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Warn("Handedness", "unavailable — matchup adjustments disabled")
	// Non-interactive uses log.Printf — verify no panic.
}

func TestNonInteractiveStartIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Start("Roster")

	out := buf.String()
	if out != "" {
		t.Errorf("expected Start to be no-op in non-interactive mode, got: %s", out)
	}
}

func TestNonInteractiveFinishIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)
	p.Finish()

	out := buf.String()
	if out != "" {
		t.Errorf("expected Finish to be no-op in non-interactive mode, got: %s", out)
	}
}

// --- Interactive mode tests ---

func TestInteractiveHeader(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Header("Steamer", "2026-03-30", true)

	out := buf.String()
	if !strings.Contains(out, "Steamer · 2026-03-30 · dry-run") {
		t.Errorf("expected formatted header, got: %s", out)
	}
	if !strings.Contains(out, "──────") {
		t.Errorf("expected separator line, got: %s", out)
	}
}

func TestInteractiveHeaderNoDryRun(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Header("Steamer", "2026-03-30", false)

	out := buf.String()
	if strings.Contains(out, "dry-run") {
		t.Errorf("expected no dry-run, got: %s", out)
	}
	if !strings.Contains(out, "Steamer · 2026-03-30") {
		t.Errorf("expected header without dry-run, got: %s", out)
	}
}

func TestInteractiveDoneShowsCheckmark(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Start("Roster")
	buf.Reset()
	p.Done("Roster", "16 hitters (13 active)")

	out := buf.String()
	if !strings.Contains(out, "✓") {
		t.Errorf("expected checkmark, got: %s", out)
	}
	if !strings.Contains(out, "Roster") {
		t.Errorf("expected phase name, got: %s", out)
	}
	if !strings.Contains(out, "16 hitters (13 active)") {
		t.Errorf("expected detail, got: %s", out)
	}
}

func TestInteractiveWarnShowsWarningMarker(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Start("Handedness")
	buf.Reset()
	p.Warn("Handedness", "unavailable — matchup adjustments disabled")

	out := buf.String()
	if !strings.Contains(out, "⚠") {
		t.Errorf("expected warning marker, got: %s", out)
	}
}

func TestInteractiveBarProgress(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Start("Roster")
	p.Done("Roster", "loaded")

	out := buf.String()
	if !strings.Contains(out, "10%") {
		t.Errorf("expected 10%% after Roster phase, got: %s", out)
	}
}

func TestInteractiveFinishShowsSeparator(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(true, &buf)
	p.Finish()

	out := buf.String()
	if !strings.Contains(out, "──────") {
		t.Errorf("expected closing separator, got: %s", out)
	}
}

// --- Verbose mode tests ---

func TestVerboseLogf(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	origFlags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(origFlags)

	p := progress.NewVerbose()
	p.Logf("hitter roster: %d hitters", 16)

	out := logBuf.String()
	if !strings.Contains(out, "hitter roster: 16 hitters") {
		t.Errorf("expected log output in verbose mode, got: %s", out)
	}
}

func TestVerboseHeaderIsNoOp(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	origFlags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(origFlags)

	p := progress.NewVerbose()
	p.Header("Steamer", "2026-03-30", true)

	if logBuf.Len() > 0 {
		t.Errorf("expected Header to be no-op in verbose mode, got: %s", logBuf.String())
	}
}

func TestVerboseDoneIsNoOp(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	origFlags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(origFlags)

	p := progress.NewVerbose()
	p.Done("Roster", "16 hitters")

	if logBuf.Len() > 0 {
		t.Errorf("expected Done to be no-op in verbose mode, got: %s", logBuf.String())
	}
}

func TestNonVerboseLogfIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	p := progress.New(false, &buf)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	origFlags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(origFlags)

	p.Logf("should not appear")

	if logBuf.Len() > 0 {
		t.Errorf("expected Logf to be no-op in non-verbose mode, got: %s", logBuf.String())
	}
}
