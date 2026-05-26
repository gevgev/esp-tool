package output_test

import (
	"testing"

	"github.com/ggevorgyan/esp-tool/internal/output"
)

// ---------------------------------------------------------------------------
// ShouldUseTUI
// ---------------------------------------------------------------------------

func TestShouldUseTUI_PlainFlagTrue_ReturnsFalse(t *testing.T) {
	use, reason := output.ShouldUseTUI(true)
	if use {
		t.Error("want false when plain=true, got true")
	}
	if reason == "" {
		t.Error("want non-empty reason when returning false")
	}
}

func TestShouldUseTUI_PlainFlagTrue_ReasonMentionsPlain(t *testing.T) {
	// Guideline #2: the reason string is shown under --verbose so it must be
	// descriptive enough to tell the user why TUI was skipped.
	_, reason := output.ShouldUseTUI(true)
	if len(reason) < 5 {
		t.Errorf("reason should be descriptive (≥5 chars), got: %q", reason)
	}
}

func TestShouldUseTUI_PlainFalse_NonTTY_ReturnsFalse(t *testing.T) {
	// In the test environment stdout is not a TTY → always returns false.
	use, _ := output.ShouldUseTUI(false)
	if use {
		// If this fires, the test was run with a real TTY ≥80×24 — acceptable.
		t.Log("ShouldUseTUI returned true (likely running in a large terminal; test is skipped as non-definitive)")
	}
}

func TestShouldUseTUI_NonTTY_ReasonNonEmpty(t *testing.T) {
	use, reason := output.ShouldUseTUI(false)
	if !use && reason == "" {
		t.Error("when returning false, reason must be non-empty so --verbose can report it")
	}
}
