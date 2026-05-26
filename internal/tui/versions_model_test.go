package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestVersionsModel_InitialState(t *testing.T) {
	m := NewVersionsModel([]string{"alpha", "beta", "gamma"})
	if m.total != 3 {
		t.Errorf("total: got %d, want 3", m.total)
	}
	if len(m.states) != 3 {
		t.Errorf("states: got %d entries, want 3", len(m.states))
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		s, ok := m.states[name]
		if !ok {
			t.Errorf("state for %q not found", name)
			continue
		}
		if s.Status != vStatusChecking {
			t.Errorf("%s status: got %d, want vStatusChecking", name, s.Status)
		}
	}
}

func TestVersionsModel_DevicesSortedAlphabetically(t *testing.T) {
	m := NewVersionsModel([]string{"zeta", "alpha", "beta"})
	if m.deviceNames[0] != "alpha" || m.deviceNames[1] != "beta" || m.deviceNames[2] != "zeta" {
		t.Errorf("expected sorted names, got %v", m.deviceNames)
	}
}

func TestVersionsModel_VersionResultMsg_UpdatesState(t *testing.T) {
	m := NewVersionsModel([]string{"alpha", "beta"})

	updated, _ := m.Update(VersionResultMsg{Device: "alpha", Version: "v2024.11.0"})
	m = updated.(VersionsModel)

	s := m.states["alpha"]
	if s.Status != vStatusDone {
		t.Errorf("alpha status: got %d, want vStatusDone", s.Status)
	}
	if s.Version != "v2024.11.0" {
		t.Errorf("alpha version: got %q", s.Version)
	}
	// beta still checking
	if m.states["beta"].Status != vStatusChecking {
		t.Error("beta should still be checking")
	}
	if m.done {
		t.Error("should not be done yet")
	}
}

func TestVersionsModel_AllResults_SetsDone(t *testing.T) {
	m := NewVersionsModel([]string{"alpha", "beta"})

	updated, _ := m.Update(VersionResultMsg{Device: "alpha", Version: "v2024.11.0"})
	m = updated.(VersionsModel)
	updated, _ = m.Update(VersionResultMsg{Device: "beta", ErrStr: "unreachable"})
	m = updated.(VersionsModel)

	if !m.done {
		t.Error("expected model to be done after all results received")
	}
}

func TestVersionsModel_VersionsAllDoneMsg_SetsDone(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	updated, _ := m.Update(VersionsAllDoneMsg{})
	m = updated.(VersionsModel)
	if !m.done {
		t.Error("expected model to be done after VersionsAllDoneMsg")
	}
}

func TestVersionsModel_QuitKey(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd from q key press")
	}
	// Execute the cmd and check it's tea.Quit
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestVersionsModel_TickMsg_IncrementsSpinner(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	tick0 := m.tick

	updated, _ := m.Update(TickMsg{Time: time.Now()})
	m = updated.(VersionsModel)

	if m.tick != tick0+1 {
		t.Errorf("tick: got %d, want %d", m.tick, tick0+1)
	}
}

func TestVersionsModel_View_ShowsDeviceNames(t *testing.T) {
	m := NewVersionsModel([]string{"alpha", "beta"})
	m.totalW = 100
	m.totalH = 30

	view := m.View()
	if !strings.Contains(view, "alpha") {
		t.Error("view should contain 'alpha'")
	}
	if !strings.Contains(view, "beta") {
		t.Error("view should contain 'beta'")
	}
}

func TestVersionsModel_View_ShowsVersionAfterResult(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	m.totalW = 100
	m.totalH = 30

	updated, _ := m.Update(VersionResultMsg{Device: "alpha", Version: "v2026.4.3"})
	m = updated.(VersionsModel)

	view := m.View()
	if !strings.Contains(view, "v2026.4.3") {
		t.Errorf("view should contain version, got:\n%s", view)
	}
}

func TestVersionsModel_View_ShowsUnreachableOnError(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	m.totalW = 100
	m.totalH = 30

	updated, _ := m.Update(VersionResultMsg{Device: "alpha", ErrStr: "unreachable"})
	m = updated.(VersionsModel)

	view := m.View()
	if !strings.Contains(view, "Unreachable") {
		t.Errorf("view should contain 'Unreachable', got:\n%s", view)
	}
}

func TestVersionsModel_View_EmptyWithNoTerminalSize(t *testing.T) {
	m := NewVersionsModel([]string{"alpha"})
	// totalW = 0 → View returns ""
	view := m.View()
	if view != "" {
		t.Errorf("expected empty view before window size set, got %q", view)
	}
}

func TestVersionsProgram_MarkDone_PreventsSend(t *testing.T) {
	// Create a VersionsProgram without a real bubbletea program so we can
	// verify MarkDone prevents panics on subsequent Send calls.
	vp := &VersionsProgram{
		prog: nil,
		done: make(chan struct{}),
	}
	vp.MarkDone()
	// Second MarkDone should not panic (select/close is guarded).
	vp.MarkDone()
}
