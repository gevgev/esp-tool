package upgrader

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseErrors
// ---------------------------------------------------------------------------

func TestParseErrors_EmptyInput_ReturnsGeneric(t *testing.T) {
	// Even with no output at all, a failure must produce at least one snippet.
	got := parseErrors(nil)
	if len(got) == 0 {
		t.Fatal("want at least one snippet for empty input, got none")
	}
}

func TestParseErrors_NilInput(t *testing.T) {
	got := parseErrors(nil)
	if len(got) == 0 {
		t.Fatal("want at least one snippet for nil input, got none")
	}
}

func TestParseErrors_NoKeywords_ReturnsGenericFallback(t *testing.T) {
	lines := []string{
		"INFO Starting build",
		"INFO Compiling esphome",
		"INFO Done",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want generic fallback when no keywords match, got empty")
	}
	// The fallback should be a human-readable sentinel, not empty.
	if got[0] == "" {
		t.Error("fallback snippet should not be empty string")
	}
}

func TestParseErrors_ERRORKeyword(t *testing.T) {
	lines := []string{
		"INFO Building",
		"ERROR Connection refused to 192.168.1.50",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want at least one snippet for ERROR line, got none")
	}
	if !strings.Contains(got[0], "Connection refused") {
		t.Errorf("snippet should contain error line content, got: %q", got[0])
	}
}

func TestParseErrors_FAILEDKeyword(t *testing.T) {
	lines := []string{
		"FAILED: upload step after 3 retries",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet for FAILED line, got none")
	}
	if !strings.Contains(got[0], "FAILED") {
		t.Errorf("snippet should contain FAILED, got: %q", got[0])
	}
}

func TestParseErrors_TracebackKeyword(t *testing.T) {
	lines := []string{
		"Traceback (most recent call last):",
		`  File "/usr/lib/python3/dist-packages/esphome/upload.py", line 42, in upload`,
		"ConnectionRefusedError: [Errno 111] Connection refused",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet for Traceback, got none")
	}
	if !strings.Contains(got[0], "Traceback") {
		t.Errorf("first snippet should contain Traceback, got: %q", got[0])
	}
}

func TestParseErrors_SnippetIncludesNextLine(t *testing.T) {
	// Each snippet includes the matching line plus the next non-empty line,
	// giving enough context to understand the error.
	lines := []string{
		"ERROR OTA upload failed",
		"  reason: checksum mismatch",
		"INFO retrying...",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want at least one snippet, got none")
	}
	// Snippet should contain both the ERROR line and the next line.
	if !strings.Contains(got[0], "OTA upload failed") {
		t.Errorf("snippet should contain matching line, got: %q", got[0])
	}
	if !strings.Contains(got[0], "checksum mismatch") {
		t.Errorf("snippet should include next line context, got: %q", got[0])
	}
}

func TestParseErrors_LimitsToFourSnippets(t *testing.T) {
	// Architecture §3.3: at most 4 snippets returned.
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "ERROR something went wrong"
	}
	got := parseErrors(lines)
	if len(got) > 4 {
		t.Errorf("want at most 4 snippets, got %d", len(got))
	}
}

func TestParseErrors_RefusedKeyword(t *testing.T) {
	lines := []string{
		"[I][wifi]: Connecting to AP...",
		"refused: host did not respond within 10s",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet for 'refused', got none")
	}
}

func TestParseErrors_ExceptionKeyword(t *testing.T) {
	lines := []string{
		"Exception: port is already open",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet for Exception, got none")
	}
}

func TestParseErrors_UnreachableKeyword(t *testing.T) {
	lines := []string{
		"INFO Checking device",
		"host unreachable: ping failed after 30s",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet for 'unreachable', got none")
	}
}

func TestParseErrors_PreservesOrderOfFirstMatches(t *testing.T) {
	lines := []string{
		"INFO Start",
		"ERROR First error",
		"INFO In between",
		"FAILED Second match",
	}
	got := parseErrors(lines)
	if len(got) < 2 {
		t.Fatalf("want at least 2 snippets, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "First error") {
		t.Errorf("first snippet should be first match in order, got: %q", got[0])
	}
	if !strings.Contains(got[1], "Second match") {
		t.Errorf("second snippet should be second match in order, got: %q", got[1])
	}
}

func TestParseErrors_SnippetNextLineEmptyNotIncluded(t *testing.T) {
	// An empty next line should not be appended (would look odd in the TUI).
	lines := []string{
		"ERROR upload timed out",
		"", // empty next line
		"INFO cleanup",
	}
	got := parseErrors(lines)
	if len(got) == 0 {
		t.Fatal("want snippet, got none")
	}
	// Should not have a trailing newline from the empty second line.
	if strings.HasSuffix(got[0], "\n") {
		t.Errorf("snippet should not end with newline when next line is empty, got: %q", got[0])
	}
}

func TestParseErrors_BestEffort_EmptyReturnIsAllowed(t *testing.T) {
	// Guideline #8: parseErrors is best-effort. The return value is advisory;
	// the exit code is truth. We do enforce a generic fallback message, but
	// this test documents the contract: callers must never treat an empty/nil
	// errLines as proof of success.
	got := parseErrors([]string{"INFO everything looks fine"})
	// With no keywords, we get the fallback — the function always returns non-empty.
	if got == nil {
		t.Error("parseErrors should never return nil (use fallback)")
	}
}
