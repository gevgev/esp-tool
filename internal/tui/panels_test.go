package tui

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newSizedModel(devices ...string) Model {
	if len(devices) == 0 {
		devices = []string{"alpha", "beta"}
	}
	m := NewModel(devices, 4, 2, "/tmp/devices")
	m.TermWidth = 120
	m.TermHeight = 40
	return m
}

// ---------------------------------------------------------------------------
// renderHeader
// ---------------------------------------------------------------------------

func TestRenderHeader_ContainsDeviceCount(t *testing.T) {
	m := newSizedModel("alpha", "beta", "gamma")
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	if !strings.Contains(got, "3") {
		t.Errorf("header should contain device count 3; got:\n%s", got)
	}
}

func TestRenderHeader_ContainsJobCount(t *testing.T) {
	m := newSizedModel("alpha")
	m.Jobs = 6
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	if !strings.Contains(got, "6") {
		t.Errorf("header should contain jobs count 6; got:\n%s", got)
	}
}

func TestRenderHeader_ContainsRetryCount(t *testing.T) {
	m := newSizedModel("alpha")
	m.Retries = 3
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	if !strings.Contains(got, "3") {
		t.Errorf("header should contain retry count 3; got:\n%s", got)
	}
}

func TestRenderHeader_ContainsProgressFraction(t *testing.T) {
	// 1 of 2 devices done
	m := newSizedModel("alpha", "beta")
	m.States["alpha"].Status = StatusSuccess
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	// Should contain "1" and "2" for "1/2 completed"
	if !strings.Contains(got, "1") || !strings.Contains(got, "2") {
		t.Errorf("header should contain progress fraction; got:\n%s", got)
	}
}

func TestRenderHeader_ContainsProgressBar(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	// Progress bar contains filled or empty block characters
	if !strings.Contains(got, "█") && !strings.Contains(got, "░") {
		t.Errorf("header should contain progress bar (█ or ░); got:\n%s", got)
	}
}

func TestRenderHeader_ContainsElapsed(t *testing.T) {
	m := newSizedModel("alpha")
	m.Elapsed = 65 * time.Second
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	// Elapsed time formatted as Xm Ys or just Xs
	if !strings.Contains(got, "s") {
		t.Errorf("header should contain elapsed time with 's'; got:\n%s", got)
	}
}

func TestRenderHeader_WidthRespected(t *testing.T) {
	m := newSizedModel("alpha")
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	// Each line should not exceed the terminal width significantly.
	for _, line := range strings.Split(got, "\n") {
		// Strip ANSI (rough check: visible content length ≤ width + some slack)
		if len(line) > l.TotalW+50 { // +50 slack for ANSI escape codes
			t.Errorf("header line exceeds width (%d): %q", l.TotalW, line)
		}
	}
}

// ---------------------------------------------------------------------------
// renderDeviceList
// ---------------------------------------------------------------------------

func TestRenderDeviceList_ContainsAllDeviceNames(t *testing.T) {
	m := newSizedModel("alpha", "beta", "gamma")
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(got, name) {
			t.Errorf("device list should contain %q; got:\n%s", name, got)
		}
	}
}

func TestRenderDeviceList_ShowsQueuedIcon(t *testing.T) {
	m := newSizedModel("alpha")
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "◷") {
		t.Errorf("queued device should show ◷; got:\n%s", got)
	}
}

func TestRenderDeviceList_ShowsSuccessIcon(t *testing.T) {
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusSuccess
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "✓") {
		t.Errorf("success device should show ✓; got:\n%s", got)
	}
}

func TestRenderDeviceList_ShowsFailedIcon(t *testing.T) {
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusFailed
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "✗") {
		t.Errorf("failed device should show ✗; got:\n%s", got)
	}
}

func TestRenderDeviceList_ShowsRetryingIcon(t *testing.T) {
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusRetrying
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "↺") {
		t.Errorf("retrying device should show ↺; got:\n%s", got)
	}
}

func TestRenderDeviceList_ShowsRetryBadge(t *testing.T) {
	// Device that succeeded but took 2 attempts: show ↺2 badge.
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusSuccess
	m.States["alpha"].Attempts = 2
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "2") {
		t.Errorf("device with 2 attempts should show retry badge; got:\n%s", got)
	}
}

func TestRenderDeviceList_ShowsCompletedDuration(t *testing.T) {
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusSuccess
	m.States["alpha"].Duration = 45 * time.Second
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "45s") {
		t.Errorf("completed device should show duration; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// renderActiveJobs
// ---------------------------------------------------------------------------

func TestRenderActiveJobs_ShowsRunningDevice(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	m.States["alpha"].Status = StatusRunning
	m.States["alpha"].LastLine = "INFO Uploading firmware"
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if !strings.Contains(got, "alpha") {
		t.Errorf("active jobs panel should show running device alpha; got:\n%s", got)
	}
}

func TestRenderActiveJobs_ShowsLastLine(t *testing.T) {
	m := newSizedModel("alpha")
	m.States["alpha"].Status = StatusRunning
	m.States["alpha"].LastLine = "INFO Uploading firmware"
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if !strings.Contains(got, "Uploading firmware") {
		t.Errorf("active jobs panel should show last output line; got:\n%s", got)
	}
}

func TestRenderActiveJobs_HidesQueuedDevices(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	// beta is still queued
	m.States["alpha"].Status = StatusRunning
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if strings.Contains(got, "beta") {
		t.Errorf("active jobs panel should NOT show queued device beta; got:\n%s", got)
	}
}

func TestRenderActiveJobs_EmptyWhenNoneRunning(t *testing.T) {
	m := newSizedModel("alpha")
	// alpha is queued (default)
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	// Should render without panicking and not show alpha
	if strings.Contains(got, "alpha") {
		t.Errorf("active jobs panel should not show queued device; got:\n%s", got)
	}
}

func TestRenderActiveJobs_NoBadge_OnFirstAttempt(t *testing.T) {
	m := newSizedModel("alpha")
	m.Retries = 2 // maxAttempts = 3, but first attempt — no badge
	m.States["alpha"].Status = StatusRunning
	m.States["alpha"].CurrentAttempt = 1
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if strings.Contains(got, "attempt") {
		t.Errorf("active jobs panel should NOT show attempt badge on first attempt; got:\n%s", got)
	}
}

func TestRenderActiveJobs_ShowsBadge_OnSecondAttempt(t *testing.T) {
	m := newSizedModel("alpha")
	m.Retries = 2 // maxAttempts = 3
	m.States["alpha"].Status = StatusRunning
	m.States["alpha"].CurrentAttempt = 2
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if !strings.Contains(got, "attempt 2/3") {
		t.Errorf("active jobs panel should show [attempt 2/3] on second attempt; got:\n%s", got)
	}
}

func TestRenderActiveJobs_ShowsBadge_DuringRetryWait(t *testing.T) {
	m := newSizedModel("alpha")
	m.Retries = 2 // maxAttempts = 3
	m.States["alpha"].Status = StatusRetrying
	// Runner sends attempt=2 (the upcoming retry) when attempt 1 fails.
	// Model sets CurrentAttempt = msg.Attempt = 2 (no +1).
	m.States["alpha"].CurrentAttempt = 2
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if !strings.Contains(got, "attempt 2/3") {
		t.Errorf("active jobs panel should show [attempt 2/3] while waiting to retry; got:\n%s", got)
	}
}

func TestRenderActiveJobs_NoBadge_WhenRetriesZero(t *testing.T) {
	m := newSizedModel("alpha")
	m.Retries = 0 // only 1 attempt possible; CurrentAttempt stays 1 → no badge
	m.States["alpha"].Status = StatusRunning
	m.States["alpha"].CurrentAttempt = 1
	l := LayoutFor(120, 40)
	got := renderActiveJobs(m, l)
	if strings.Contains(got, "attempt") {
		t.Errorf("active jobs panel should NOT show attempt badge when retries=0; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// renderErrors
// ---------------------------------------------------------------------------

func TestRenderErrors_ContainsErrorSnippets(t *testing.T) {
	m := newSizedModel("alpha")
	m.Errors = []ErrorEntry{
		{Device: "alpha", Snippets: []string{"ERROR Connection refused"}},
	}
	l := LayoutFor(120, 40)
	got := renderErrors(m, l)
	if !strings.Contains(got, "Connection refused") {
		t.Errorf("errors panel should contain snippet; got:\n%s", got)
	}
}

func TestRenderErrors_ContainsFailedDeviceName(t *testing.T) {
	m := newSizedModel("alpha")
	m.Errors = []ErrorEntry{
		{Device: "alpha", Snippets: []string{"ERROR something"}},
	}
	l := LayoutFor(120, 40)
	got := renderErrors(m, l)
	if !strings.Contains(got, "alpha") {
		t.Errorf("errors panel should contain device name; got:\n%s", got)
	}
}

func TestRenderErrors_EmptyWhenNoErrors(t *testing.T) {
	m := newSizedModel("alpha")
	l := LayoutFor(120, 40)
	got := renderErrors(m, l)
	// Should render cleanly without error snippets
	if strings.Contains(got, "ERROR") {
		t.Errorf("errors panel should be empty when no failures; got:\n%s", got)
	}
}

func TestRenderErrors_MultipleDevicesListed(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	m.Errors = []ErrorEntry{
		{Device: "alpha", Snippets: []string{"ERROR first"}},
		{Device: "beta", Snippets: []string{"ERROR second"}},
	}
	l := LayoutFor(120, 40)
	got := renderErrors(m, l)
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Errorf("errors panel should list both failed devices; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// renderOutputTail
// ---------------------------------------------------------------------------

func TestRenderOutputTail_ShowsRecentLines(t *testing.T) {
	m := newSizedModel("alpha")
	m.GlobalTail = []TailLine{
		{Device: "alpha", Line: "INFO Starting upload"},
		{Device: "alpha", Line: "INFO Upload complete"},
	}
	l := LayoutFor(120, 40)
	got := renderOutputTail(m, l)
	if !strings.Contains(got, "Starting upload") {
		t.Errorf("output tail should contain recent line; got:\n%s", got)
	}
}

func TestRenderOutputTail_ShowsDevicePrefix(t *testing.T) {
	m := newSizedModel("alpha")
	m.GlobalTail = []TailLine{
		{Device: "alpha", Line: "INFO uploading"},
	}
	l := LayoutFor(120, 40)
	got := renderOutputTail(m, l)
	if !strings.Contains(got, "alpha") {
		t.Errorf("output tail should contain device prefix; got:\n%s", got)
	}
}

func TestRenderOutputTail_EmptyTailRendersCleanly(t *testing.T) {
	m := newSizedModel("alpha")
	// GlobalTail is empty
	l := LayoutFor(120, 40)
	got := renderOutputTail(m, l)
	// Should render without panic
	_ = got
}

// ---------------------------------------------------------------------------
// Model.GlobalTail accumulation
// ---------------------------------------------------------------------------

func TestModel_DeviceOutputMsg_AppendsToGlobalTail(t *testing.T) {
	m := newSizedModel("alpha")
	m, _ = update(m, DeviceOutputMsg{Device: "alpha", Line: "INFO line 1"})
	m, _ = update(m, DeviceOutputMsg{Device: "alpha", Line: "INFO line 2"})
	if len(m.GlobalTail) != 2 {
		t.Fatalf("want 2 GlobalTail entries, got %d", len(m.GlobalTail))
	}
	if m.GlobalTail[0].Line != "INFO line 1" {
		t.Errorf("first tail entry: want %q, got %q", "INFO line 1", m.GlobalTail[0].Line)
	}
	if m.GlobalTail[1].Device != "alpha" {
		t.Errorf("tail entry device: want alpha, got %q", m.GlobalTail[1].Device)
	}
}

func TestModel_GlobalTail_BoundedAtMax(t *testing.T) {
	m := newSizedModel("alpha")
	// Send more lines than maxTailLines
	for i := 0; i < maxTailLines+10; i++ {
		m, _ = update(m, DeviceOutputMsg{Device: "alpha", Line: "INFO line"})
	}
	if len(m.GlobalTail) > maxTailLines {
		t.Errorf("GlobalTail should be bounded at %d, got %d", maxTailLines, len(m.GlobalTail))
	}
}

// ---------------------------------------------------------------------------
// View() with panels (smoke test)
// ---------------------------------------------------------------------------

func TestModel_View_WithSize_ContainsDeviceNames(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	m.TermWidth = 120
	m.TermHeight = 40
	view := m.View()
	for _, name := range []string{"alpha", "beta"} {
		if !strings.Contains(view, name) {
			t.Errorf("View() with terminal size should contain device %q; got:\n%s", name, view)
		}
	}
}

func TestModel_View_WithoutSize_FallsBackToPlain(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	m.TermWidth = 0
	m.TermHeight = 0
	view := m.View()
	// Should not panic and should contain device names
	for _, name := range []string{"alpha", "beta"} {
		if !strings.Contains(view, name) {
			t.Errorf("fallback View() should contain device %q; got:\n%s", name, view)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 1G — renderDeviceList scroll
// ---------------------------------------------------------------------------

func TestRenderDeviceList_ScrollOffset_HidesScrolledDevice(t *testing.T) {
	// With offset=1, "alpha" (index 0) should not appear in the rendered list.
	m := newSizedModel("alpha", "beta", "gamma")
	m.ScrollOffset = 1
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if strings.Contains(got, "alpha") {
		t.Errorf("ScrollOffset=1 should hide 'alpha'; got:\n%s", got)
	}
	if !strings.Contains(got, "beta") {
		t.Errorf("ScrollOffset=1 should show 'beta'; got:\n%s", got)
	}
}

func TestRenderDeviceList_ScrollOffset_ShowsUpIndicator(t *testing.T) {
	m := newSizedModel("alpha", "beta", "gamma")
	m.ScrollOffset = 1 // alpha is above the view
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if !strings.Contains(got, "▲") {
		t.Errorf("ScrollOffset=1 should show ▲ up-scroll indicator; got:\n%s", got)
	}
}

func TestRenderDeviceList_NoScrollIndicators_WhenAllFit(t *testing.T) {
	m := newSizedModel("alpha", "beta") // 2 devices fit easily in BodyH=31
	l := LayoutFor(120, 40)
	got := renderDeviceList(m, l)
	if strings.Contains(got, "▲") || strings.Contains(got, "▼") {
		t.Errorf("no scroll indicators should appear when all devices fit; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 1G — renderHelp
// ---------------------------------------------------------------------------

func TestRenderHelp_ContainsKeyBindings(t *testing.T) {
	m := newSizedModel("alpha")
	l := LayoutFor(120, 40)
	got := renderHelp(m, l)
	for _, key := range []string{"q", "?", "↑", "↓"} {
		if !strings.Contains(got, key) {
			t.Errorf("help panel should contain key %q; got:\n%s", key, got)
		}
	}
}

func TestRenderHelp_ContainsHomeEnd(t *testing.T) {
	m := newSizedModel("alpha")
	l := LayoutFor(120, 40)
	got := renderHelp(m, l)
	if !strings.Contains(got, "Home") || !strings.Contains(got, "End") {
		t.Errorf("help panel should mention Home/End keys; got:\n%s", got)
	}
}

func TestModel_View_ShowHelp_ContainsScrollShortcuts(t *testing.T) {
	m := newSizedModel("alpha", "beta")
	m.ShowHelp = true
	got := m.View()
	if !strings.Contains(got, "↑") && !strings.Contains(got, "↓") {
		t.Errorf("View() with ShowHelp=true should show scroll shortcuts; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Overflow / height-clamping tests (Issue #3 — header disappears)
// ---------------------------------------------------------------------------

// TestRenderActiveJobs_LongLastLine_DoesNotOverflow verifies that a very long
// firmware-path LastLine does not cause renderActiveJobs to render more lines
// than l.ActiveH (which would push the header off-screen).
func TestRenderActiveJobs_LongLastLine_DoesNotOverflow(t *testing.T) {
	const termW, termH = 120, 40
	m := newSizedModel("alpha", "beta", "gamma", "delta")
	longPath := "INFO Uploading /Users/ggevorgyan/git/esp32/esphome/.esphome/build/espvibration1/.pioenvs/espvibration1/firmware.bin (910224 bytes)"
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		m.States[name].Status = StatusRunning
		m.States[name].LastLine = longPath
	}
	l := LayoutFor(termW, termH)
	got := renderActiveJobs(m, l)
	lineCount := strings.Count(got, "\n") + 1
	if lineCount > l.ActiveH {
		t.Errorf("renderActiveJobs rendered %d lines but ActiveH=%d; long LastLine caused overflow",
			lineCount, l.ActiveH)
	}
}

// TestRenderActiveJobs_ManyDevices_DoesNotOverflow verifies that having more
// Running devices than content rows doesn't overflow the panel.
func TestRenderActiveJobs_ManyDevices_DoesNotOverflow(t *testing.T) {
	const termW, termH = 80, 24 // minimum supported TUI size
	devices := []string{"d1", "d2", "d3", "d4", "d5", "d6"}
	m := newSizedModel(devices...)
	m.TermWidth = termW
	m.TermHeight = termH
	for _, name := range devices {
		m.States[name].Status = StatusRunning
		m.States[name].LastLine = "INFO Compiling..."
	}
	l := LayoutFor(termW, termH)
	got := renderActiveJobs(m, l)
	lineCount := strings.Count(got, "\n") + 1
	if lineCount > l.ActiveH {
		t.Errorf("renderActiveJobs rendered %d lines but ActiveH=%d for %d devices",
			lineCount, l.ActiveH, len(devices))
	}
}

// TestRenderOutputTail_LongLine_Truncated verifies that a very long output line
// is truncated to a single row (does not wrap and inflate the panel height).
func TestRenderOutputTail_LongLine_Truncated(t *testing.T) {
	const termW, termH = 120, 40
	m := newSizedModel("alpha")
	longLine := strings.Repeat("x", 300) // 300 chars, much wider than any panel
	m.GlobalTail = []TailLine{{Device: "alpha", Line: longLine}}
	l := LayoutFor(termW, termH)
	got := renderOutputTail(m, l)
	lineCount := strings.Count(got, "\n") + 1
	if lineCount > l.OutputH {
		t.Errorf("renderOutputTail rendered %d lines but OutputH=%d; long line caused overflow",
			lineCount, l.OutputH)
	}
}

// TestRenderErrors_LongSnippet_DoesNotOverflow verifies error snippets are
// clipped to the panel's allocated height.
func TestRenderErrors_LongSnippet_DoesNotOverflow(t *testing.T) {
	const termW, termH = 80, 24
	m := newSizedModel("alpha", "beta", "gamma", "delta", "epsilon")
	for _, name := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		m.Errors = append(m.Errors, ErrorEntry{
			Device:   name,
			Snippets: []string{"ERROR: connection refused\nERROR: timeout\nERROR: ota failed"},
		})
	}
	l := LayoutFor(termW, termH)
	got := renderErrors(m, l)
	lineCount := strings.Count(got, "\n") + 1
	if lineCount > l.ErrorsH {
		t.Errorf("renderErrors rendered %d lines but ErrorsH=%d; content not clamped",
			lineCount, l.ErrorsH)
	}
}

// TestRenderHeader_ElapsedRoundedToSeconds verifies elapsed is shown without
// sub-second precision (Issue #4).
func TestRenderHeader_ElapsedRoundedToSeconds(t *testing.T) {
	m := newSizedModel("alpha")
	// Set elapsed to a value with significant sub-second component.
	m.Elapsed = 71652283542 // 1m11.652283542s in nanoseconds
	l := LayoutFor(120, 40)
	got := renderHeader(m, l)
	// Raw nanosecond-precision string would contain many digits after decimal point.
	// After rounding to seconds it should be "1m11s" with no decimal.
	if strings.Contains(got, ".6") || strings.Contains(got, ".65") {
		t.Errorf("header should not show sub-second precision; got:\n%s", got)
	}
	if !strings.Contains(got, "1m12s") {
		t.Errorf("header should show rounded elapsed '1m12s'; got:\n%s", got)
	}
}
