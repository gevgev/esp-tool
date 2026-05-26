package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestModel returns a Model with two devices and minimal config.
func newTestModel(devices ...string) Model {
	if len(devices) == 0 {
		devices = []string{"alpha", "beta"}
	}
	return NewModel(devices, 4, 2, "/tmp")
}

// update is a shorthand for calling m.Update(msg) and type-asserting the result.
func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	newM, cmd := m.Update(msg)
	return newM.(Model), cmd
}

// ---------------------------------------------------------------------------
// Initial state
// ---------------------------------------------------------------------------

func TestModel_NewModel_AllDevicesQueued(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		st, ok := m.States[name]
		if !ok {
			t.Errorf("device %q not in States", name)
			continue
		}
		if st.Status != StatusQueued {
			t.Errorf("%s: want StatusQueued, got %v", name, st.Status)
		}
	}
}

func TestModel_NewModel_PreservesDeviceOrder(t *testing.T) {
	names := []string{"z-device", "a-device", "m-device"}
	m := NewModel(names, 4, 2, "/tmp")
	if len(m.Devices) != len(names) {
		t.Fatalf("want %d devices, got %d", len(names), len(m.Devices))
	}
	for i, n := range names {
		if m.Devices[i] != n {
			t.Errorf("index %d: want %q, got %q", i, n, m.Devices[i])
		}
	}
}

func TestModel_NewModel_StaticConfig(t *testing.T) {
	m := NewModel([]string{"dev"}, 6, 3, "/my/dir")
	if m.Jobs != 6 {
		t.Errorf("Jobs: want 6, got %d", m.Jobs)
	}
	if m.Retries != 3 {
		t.Errorf("Retries: want 3, got %d", m.Retries)
	}
	if m.WorkDir != "/my/dir" {
		t.Errorf("WorkDir: want /my/dir, got %q", m.WorkDir)
	}
}

// ---------------------------------------------------------------------------
// DeviceStatusMsg — running
// ---------------------------------------------------------------------------

func TestModel_DeviceStatusMsg_Running_TransitionsState(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusRunning})
	if m.States["alpha"].Status != StatusRunning {
		t.Errorf("want StatusRunning, got %v", m.States["alpha"].Status)
	}
}

func TestModel_DeviceStatusMsg_Running_SetsStartedAt(t *testing.T) {
	m := newTestModel("alpha")
	before := time.Now()
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusRunning})
	if m.States["alpha"].StartedAt.Before(before) {
		t.Error("StartedAt should be set to approximately now when device starts")
	}
}

// ---------------------------------------------------------------------------
// DeviceOutputMsg
// ---------------------------------------------------------------------------

func TestModel_DeviceOutputMsg_UpdatesLastLine(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusRunning})
	m, _ = update(m, DeviceOutputMsg{Device: "alpha", Line: "INFO uploading firmware"})
	if m.States["alpha"].LastLine != "INFO uploading firmware" {
		t.Errorf("want last line to be set, got %q", m.States["alpha"].LastLine)
	}
}

func TestModel_DeviceOutputMsg_RetryingToRunning(t *testing.T) {
	// First line after a retry sleep transitions stateRetrying → stateRunning.
	m := newTestModel("alpha")
	m, _ = update(m, DeviceRetryMsg{Device: "alpha", Attempt: 2, Max: 3, Delay: 5 * time.Second})
	if m.States["alpha"].Status != StatusRetrying {
		t.Fatalf("want StatusRetrying before first line, got %v", m.States["alpha"].Status)
	}
	m, _ = update(m, DeviceOutputMsg{Device: "alpha", Line: "INFO retry attempt started"})
	if m.States["alpha"].Status != StatusRunning {
		t.Errorf("want StatusRunning after first line post-retry, got %v", m.States["alpha"].Status)
	}
}

// ---------------------------------------------------------------------------
// DeviceRetryMsg
// ---------------------------------------------------------------------------

func TestModel_DeviceRetryMsg_TransitionsToRetrying(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceRetryMsg{Device: "alpha", Attempt: 2, Max: 3, Delay: 5 * time.Second})
	if m.States["alpha"].Status != StatusRetrying {
		t.Errorf("want StatusRetrying, got %v", m.States["alpha"].Status)
	}
}

func TestModel_DeviceRetryMsg_SetsRetryAt(t *testing.T) {
	m := newTestModel("alpha")
	delay := 5 * time.Second
	before := time.Now()
	m, _ = update(m, DeviceRetryMsg{Device: "alpha", Attempt: 2, Max: 3, Delay: delay})
	retryAt := m.States["alpha"].RetryAt
	if retryAt.Before(before.Add(delay - time.Millisecond)) {
		t.Errorf("RetryAt should be approximately now+delay, got %v", retryAt)
	}
}

// ---------------------------------------------------------------------------
// DeviceStatusMsg — success
// ---------------------------------------------------------------------------

func TestModel_DeviceStatusMsg_Success_TransitionsState(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{
		Device:   "alpha",
		Status:   StatusSuccess,
		Attempts: 1,
		Duration: 30 * time.Second,
	})
	if m.States["alpha"].Status != StatusSuccess {
		t.Errorf("want StatusSuccess, got %v", m.States["alpha"].Status)
	}
	if m.States["alpha"].Duration != 30*time.Second {
		t.Errorf("want Duration=30s, got %v", m.States["alpha"].Duration)
	}
}

// ---------------------------------------------------------------------------
// DeviceStatusMsg — failure
// ---------------------------------------------------------------------------

func TestModel_DeviceStatusMsg_Failed_StoresErrLines(t *testing.T) {
	m := newTestModel("alpha")
	errLines := []string{"ERROR: Connection refused", "  detail line"}
	m, _ = update(m, DeviceStatusMsg{
		Device:   "alpha",
		Status:   StatusFailed,
		ErrLines: errLines,
	})
	if m.States["alpha"].Status != StatusFailed {
		t.Errorf("want StatusFailed, got %v", m.States["alpha"].Status)
	}
	if len(m.States["alpha"].ErrLines) != 2 {
		t.Errorf("want 2 ErrLines stored, got %d", len(m.States["alpha"].ErrLines))
	}
}

func TestModel_DeviceStatusMsg_Failed_PopulatesErrorsPanel(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{
		Device:   "alpha",
		Status:   StatusFailed,
		ErrLines: []string{"ERROR: OTA failed"},
	})
	if len(m.Errors) == 0 {
		t.Fatal("Errors panel should have an entry for failed device")
	}
	if m.Errors[0].Device != "alpha" {
		t.Errorf("error entry device: want alpha, got %q", m.Errors[0].Device)
	}
}

func TestModel_DeviceStatusMsg_Failed_NoErrLines_NoErrorEntry(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{
		Device: "alpha",
		Status: StatusFailed,
		// ErrLines is nil
	})
	// Failure with nil errLines should not add an Errors panel entry
	// (parseErrors always returns non-nil in practice, but defensive check).
	if len(m.Errors) != 0 {
		t.Errorf("want 0 Errors entries when ErrLines is nil, got %d", len(m.Errors))
	}
}

// ---------------------------------------------------------------------------
// AllDoneMsg — auto-quit on success
// ---------------------------------------------------------------------------

func TestModel_AllDoneMsg_AllSuccess_ReturnsQuit(t *testing.T) {
	m := newTestModel("alpha", "beta")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusSuccess})
	m, _ = update(m, DeviceStatusMsg{Device: "beta", Status: StatusSuccess})
	_, cmd := update(m, AllDoneMsg{})
	if cmd == nil {
		t.Fatal("want non-nil Cmd (tea.Quit) when all succeed, got nil")
	}
	// Execute the cmd and verify it returns a QuitMsg.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("want tea.QuitMsg, got %T", msg)
	}
}

func TestModel_AllDoneMsg_AllSuccess_SetsDone(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusSuccess})
	m, _ = update(m, AllDoneMsg{})
	if !m.Done {
		t.Error("want Done=true after AllDoneMsg")
	}
	if m.HasFailures {
		t.Error("want HasFailures=false when all succeeded")
	}
}

func TestModel_AllDoneMsg_HasFailure_NoAutoQuit(t *testing.T) {
	m := newTestModel("alpha", "beta")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusSuccess})
	m, _ = update(m, DeviceStatusMsg{Device: "beta", Status: StatusFailed, ErrLines: []string{"ERROR"}})
	m, cmd := update(m, AllDoneMsg{})
	if cmd != nil {
		// cmd might be the tick cmd — verify it's NOT tea.Quit
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Error("should NOT auto-quit when there are failures (wait for q)")
		}
	}
	if !m.Done {
		t.Error("want Done=true")
	}
	if !m.HasFailures {
		t.Error("want HasFailures=true")
	}
}

// ---------------------------------------------------------------------------
// Keyboard — q / ctrl+c
// ---------------------------------------------------------------------------

func TestModel_KeyQ_ReturnsQuit(t *testing.T) {
	m := newTestModel()
	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("want non-nil Cmd on 'q', got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("want tea.QuitMsg from 'q', got %T", cmd())
	}
}

func TestModel_KeyCtrlC_ReturnsQuit(t *testing.T) {
	m := newTestModel()
	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("want non-nil Cmd on ctrl+c, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("want tea.QuitMsg from ctrl+c, got %T", cmd())
	}
}

// ---------------------------------------------------------------------------
// TickMsg
// ---------------------------------------------------------------------------

func TestModel_TickMsg_IncrementsCounter(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, TickMsg{Time: time.Now()})
	if m.Tick != 1 {
		t.Errorf("want Tick=1 after one TickMsg, got %d", m.Tick)
	}
	m, _ = update(m, TickMsg{Time: time.Now()})
	if m.Tick != 2 {
		t.Errorf("want Tick=2 after two TickMsgs, got %d", m.Tick)
	}
}

func TestModel_TickMsg_ReturnsNextTickCmd(t *testing.T) {
	m := newTestModel()
	_, cmd := update(m, TickMsg{Time: time.Now()})
	if cmd == nil {
		t.Error("TickMsg should return a follow-up tick Cmd to keep animation running")
	}
}

// ---------------------------------------------------------------------------
// WindowSizeMsg
// ---------------------------------------------------------------------------

func TestModel_WindowSizeMsg_UpdatesDimensions(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.TermWidth != 120 || m.TermHeight != 40 {
		t.Errorf("want 120×40, got %d×%d", m.TermWidth, m.TermHeight)
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func TestModel_Init_ReturnsTickCmd(t *testing.T) {
	m := newTestModel()
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() should return a tick Cmd")
	}
}

// ---------------------------------------------------------------------------
// View — basic smoke test
// ---------------------------------------------------------------------------

func TestModel_View_ContainsAllDeviceNames(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	view := m.View()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(view, name) {
			t.Errorf("View() should contain device name %q", name)
		}
	}
}

func TestModel_View_WaitForQuit_ShownOnFailure(t *testing.T) {
	m := newTestModel("alpha")
	m, _ = update(m, DeviceStatusMsg{Device: "alpha", Status: StatusFailed, ErrLines: []string{"ERROR"}})
	m, _ = update(m, AllDoneMsg{})
	view := m.View()
	if !strings.Contains(view, "q") {
		t.Errorf("View() should mention 'q' to quit when there are failures; got:\n%s", view)
	}
}

func TestModel_View_SpinnerUsesFrames(t *testing.T) {
	// Verify that statusIcon returns different characters for different tick values.
	frames := map[string]bool{}
	for i := 0; i < len(spinnerFrames); i++ {
		frames[statusIcon(StatusRunning, i)] = true
	}
	if len(frames) < 2 {
		t.Errorf("want multiple distinct spinner frames, got: %v", frames)
	}
}

// ---------------------------------------------------------------------------
// Phase 1G — ShowHelp toggle
// ---------------------------------------------------------------------------

func TestModel_KeyQuestion_TogglesShowHelp(t *testing.T) {
	m := newTestModel("alpha")
	if m.ShowHelp {
		t.Fatal("ShowHelp should be false initially")
	}
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.ShowHelp {
		t.Error("pressing '?' should set ShowHelp=true")
	}
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.ShowHelp {
		t.Error("pressing '?' again should set ShowHelp=false")
	}
}

// ---------------------------------------------------------------------------
// Phase 1G — ScrollOffset
// ---------------------------------------------------------------------------

func TestModel_KeyDown_IncrementsScrollOffset(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.ScrollOffset != 1 {
		t.Errorf("↓ should increment ScrollOffset to 1, got %d", m.ScrollOffset)
	}
}

func TestModel_KeyUp_DecrementsScrollOffset(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m.ScrollOffset = 2
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.ScrollOffset != 1 {
		t.Errorf("↑ should decrement ScrollOffset to 1, got %d", m.ScrollOffset)
	}
}

func TestModel_ScrollOffset_NotNegative(t *testing.T) {
	m := newTestModel("alpha", "beta")
	// ScrollOffset starts at 0; pressing ↑ should not go below 0.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.ScrollOffset < 0 {
		t.Errorf("ScrollOffset should not go negative, got %d", m.ScrollOffset)
	}
}

func TestModel_ScrollOffset_BoundedByDeviceCount(t *testing.T) {
	m := newTestModel("alpha", "beta")
	// Press ↓ many times — should not exceed len(Devices)-1.
	for i := 0; i < 10; i++ {
		m, _ = update(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.ScrollOffset >= len(m.Devices) {
		t.Errorf("ScrollOffset should be < len(Devices)=%d, got %d", len(m.Devices), m.ScrollOffset)
	}
}

func TestModel_KeyHome_ResetsScrollOffset(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m.ScrollOffset = 2
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyHome})
	if m.ScrollOffset != 0 {
		t.Errorf("Home should reset ScrollOffset to 0, got %d", m.ScrollOffset)
	}
}

func TestModel_KeyEnd_SetsScrollOffsetToLast(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyEnd})
	want := len(m.Devices) - 1
	if m.ScrollOffset != want {
		t.Errorf("End should set ScrollOffset to %d, got %d", want, m.ScrollOffset)
	}
}

func TestModel_KeyVimJ_IncrementsScroll(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.ScrollOffset != 1 {
		t.Errorf("j should scroll down (offset=1), got %d", m.ScrollOffset)
	}
}

func TestModel_KeyVimK_DecrementsScroll(t *testing.T) {
	m := newTestModel("alpha", "beta", "gamma")
	m.ScrollOffset = 2
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.ScrollOffset != 1 {
		t.Errorf("k should scroll up (offset=1), got %d", m.ScrollOffset)
	}
}
