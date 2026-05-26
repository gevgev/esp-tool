package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_ShortString_Unchanged(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("want %q, got %q", "hello", got)
	}
}

func TestTruncate_ExactLength_Unchanged(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Errorf("want %q, got %q", "hello", got)
	}
}

func TestTruncate_LongString_Clipped(t *testing.T) {
	got := truncate("hello world", 6)
	if got != "hello…" {
		t.Errorf("want %q, got %q", "hello…", got)
	}
}

func TestTruncate_MaxOne_ReturnsEllipsis(t *testing.T) {
	got := truncate("hello", 1)
	if got != "…" {
		t.Errorf("want %q, got %q", "…", got)
	}
}

func TestTruncate_MaxZero_ReturnsEmpty(t *testing.T) {
	got := truncate("hello", 0)
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestTruncate_EmptyString_Unchanged(t *testing.T) {
	got := truncate("", 10)
	if got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestTruncate_MultibyteSafeLength(t *testing.T) {
	// Each '…' is one rune; total input is 3 runes.
	s := "αβγ"
	got := truncate(s, 2)
	runes := []rune(got)
	if len(runes) != 2 {
		t.Errorf("want 2 runes, got %d: %q", len(runes), got)
	}
	if string(runes[len(runes)-1:]) != "…" {
		t.Errorf("last rune should be ellipsis, got %q", got)
	}
}

func TestTruncate_LongFirmwarePath(t *testing.T) {
	// Real-world example from ESPHome upgrade output.
	path := "INFO Uploading /Users/ggevorgyan/git/esp32/esphome/.esphome/build/espvibration1/.pioenvs/espvibration1/firmware.bin (910224 bytes)"
	got := truncate(path, 56)
	runes := []rune(got)
	if len(runes) > 56 {
		t.Errorf("result should be ≤56 runes, got %d: %q", len(runes), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clipped string should end with …, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// clampLines
// ---------------------------------------------------------------------------

func TestClampLines_FewLines_Unchanged(t *testing.T) {
	s := "a\nb\nc"
	got := clampLines(s, 5)
	if got != s {
		t.Errorf("want %q, got %q", s, got)
	}
}

func TestClampLines_ExactLines_Unchanged(t *testing.T) {
	s := "a\nb\nc"
	got := clampLines(s, 3)
	if got != s {
		t.Errorf("want %q, got %q", s, got)
	}
}

func TestClampLines_TooManyLines_Clipped(t *testing.T) {
	s := "a\nb\nc\nd\ne"
	got := clampLines(s, 3)
	want := "a\nb\nc"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestClampLines_MaxZero_ReturnsEmpty(t *testing.T) {
	got := clampLines("a\nb\nc", 0)
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestClampLines_SingleLine_Unchanged(t *testing.T) {
	got := clampLines("hello", 3)
	if got != "hello" {
		t.Errorf("want %q, got %q", "hello", got)
	}
}
