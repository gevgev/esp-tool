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
