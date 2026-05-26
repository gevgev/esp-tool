package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDiagnosticsModel_InitialState(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha", "beta", "gamma"})
	if m.total != 3 {
		t.Errorf("total: got %d, want 3", m.total)
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		s, ok := m.states[name]
		if !ok {
			t.Errorf("state for %q not found", name)
			continue
		}
		if s.Status != dStatusChecking {
			t.Errorf("%s should start as dStatusChecking", name)
		}
	}
}

func TestDiagnosticsModel_DevicesSortedAlphabetically(t *testing.T) {
	m := NewDiagnosticsModel([]string{"zeta", "alpha", "beta"})
	if m.deviceNames[0] != "alpha" || m.deviceNames[1] != "beta" || m.deviceNames[2] != "zeta" {
		t.Errorf("expected sorted names, got %v", m.deviceNames)
	}
}

func TestDiagnosticsModel_ResultMsg_UpdatesState(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha", "beta"})

	updated, _ := m.Update(DiagnosticsResultMsg{
		Device: "alpha", Version: "v2026.4.3", Warnings: 1,
		IssueLines: []string{"⚠ Chip rev ≥3.0"},
	})
	m = updated.(DiagnosticsModel)

	s := m.states["alpha"]
	if s.Status != dStatusDone {
		t.Errorf("alpha status: want dStatusDone, got %d", s.Status)
	}
	if s.Version != "v2026.4.3" {
		t.Errorf("alpha version: got %q", s.Version)
	}
	if s.Warnings != 1 {
		t.Errorf("alpha warnings: got %d, want 1", s.Warnings)
	}
	if m.states["beta"].Status != dStatusChecking {
		t.Error("beta should still be checking")
	}
	if m.done {
		t.Error("should not be done yet — beta still pending")
	}
}

func TestDiagnosticsModel_AllResults_SetsDone(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha", "beta"})

	updated, _ := m.Update(DiagnosticsResultMsg{Device: "alpha", Version: "v2026.4.3"})
	m = updated.(DiagnosticsModel)
	updated, _ = m.Update(DiagnosticsResultMsg{Device: "beta", Err: "unreachable"})
	m = updated.(DiagnosticsModel)

	if !m.done {
		t.Error("expected model to be done after all results received")
	}
}

func TestDiagnosticsModel_AllDoneMsg_SetsDone(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	updated, _ := m.Update(DiagnosticsAllDoneMsg{})
	m = updated.(DiagnosticsModel)
	if !m.done {
		t.Error("expected model to be done after DiagnosticsAllDoneMsg")
	}
}

func TestDiagnosticsModel_QuitKey(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a tea.Cmd from q key press")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestDiagnosticsModel_TickMsg_IncrementsSpinner(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	tick0 := m.tick
	updated, _ := m.Update(TickMsg{Time: time.Now()})
	m = updated.(DiagnosticsModel)
	if m.tick != tick0+1 {
		t.Errorf("tick: got %d, want %d", m.tick, tick0+1)
	}
}

func TestDiagnosticsModel_View_ShowsDeviceNames(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha", "beta"})
	m.totalW = 120
	m.totalH = 30
	view := m.View()
	if !strings.Contains(view, "alpha") {
		t.Error("view should contain 'alpha'")
	}
	if !strings.Contains(view, "beta") {
		t.Error("view should contain 'beta'")
	}
}

func TestDiagnosticsModel_View_ShowsHealthyAfterResult(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	m.totalW = 120
	m.totalH = 30
	updated, _ := m.Update(DiagnosticsResultMsg{Device: "alpha", Version: "v2026.4.3"})
	m = updated.(DiagnosticsModel)
	view := m.View()
	if !strings.Contains(view, "Healthy") {
		t.Errorf("view should contain 'Healthy' for device with no issues; got:\n%s", view)
	}
	if !strings.Contains(view, "v2026.4.3") {
		t.Errorf("view should contain version; got:\n%s", view)
	}
}

func TestDiagnosticsModel_View_ShowsCrash(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	m.totalW = 120
	m.totalH = 30
	updated, _ := m.Update(DiagnosticsResultMsg{
		Device: "alpha", Version: "v2026.4.3",
		Crashes: 1, IssueLines: []string{"✗ Crash on previous boot — Hardware WDT"},
	})
	m = updated.(DiagnosticsModel)
	view := m.View()
	if !strings.Contains(view, "crash") {
		t.Errorf("view should indicate crash; got:\n%s", view)
	}
	if !strings.Contains(view, "Hardware WDT") {
		t.Errorf("view should show crash detail; got:\n%s", view)
	}
}

func TestDiagnosticsModel_View_ShowsWarnings(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	m.totalW = 120
	m.totalH = 30
	updated, _ := m.Update(DiagnosticsResultMsg{
		Device: "alpha", Version: "v2026.4.3",
		Warnings: 2, IssueLines: []string{"⚠ Chip rev ≥3.0", "⚠ Bootloader too old"},
	})
	m = updated.(DiagnosticsModel)
	view := m.View()
	if !strings.Contains(view, "2 warnings") {
		t.Errorf("view should show '2 warnings'; got:\n%s", view)
	}
	if !strings.Contains(view, "Chip rev") {
		t.Errorf("view should show warning detail; got:\n%s", view)
	}
}

func TestDiagnosticsModel_View_ShowsUnreachable(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	m.totalW = 120
	m.totalH = 30
	updated, _ := m.Update(DiagnosticsResultMsg{Device: "alpha", Err: "unreachable or no data within timeout"})
	m = updated.(DiagnosticsModel)
	view := m.View()
	if !strings.Contains(view, "Unreachable") {
		t.Errorf("view should show 'Unreachable'; got:\n%s", view)
	}
}

func TestDiagnosticsModel_View_EmptyBeforeWindowSize(t *testing.T) {
	m := NewDiagnosticsModel([]string{"alpha"})
	if m.View() != "" {
		t.Error("expected empty view before window size is set")
	}
}

func TestDiagnosticsProgram_MarkDone_Safe(t *testing.T) {
	dp := &DiagnosticsProgram{done: make(chan struct{})}
	dp.MarkDone()
	dp.MarkDone() // second call must not panic
}
