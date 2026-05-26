package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── message types ────────────────────────────────────────────────────────────

// VersionResultMsg is sent (via VersionsProgram.Send) when one device's
// version check completes.  errStr is non-empty on failure / timeout.
type VersionResultMsg struct {
	Device  string
	Version string
	ErrStr  string
}

// VersionsAllDoneMsg is sent after CheckVersions returns.
type VersionsAllDoneMsg struct{}

// ─── device states ────────────────────────────────────────────────────────────

type versionsDeviceStatus int

const (
	vStatusChecking versionsDeviceStatus = iota
	vStatusDone
)

type versionsDeviceState struct {
	Status  versionsDeviceStatus
	Version string // non-empty on success
	ErrStr  string // non-empty on failure / timeout
}

// ─── model ────────────────────────────────────────────────────────────────────

// VersionsModel is the bubbletea model for the "versions" TUI.
type VersionsModel struct {
	// static config
	deviceNames []string
	total       int

	// dynamic state
	states    map[string]*versionsDeviceState
	done      bool
	startedAt time.Time
	elapsed   time.Duration
	tick      int // spinner frame counter
	totalW    int
	totalH    int
}

// NewVersionsModel creates a VersionsModel for the given device names.
func NewVersionsModel(deviceNames []string) VersionsModel {
	states := make(map[string]*versionsDeviceState, len(deviceNames))
	for _, n := range deviceNames {
		states[n] = &versionsDeviceState{Status: vStatusChecking}
	}
	sorted := make([]string, len(deviceNames))
	copy(sorted, deviceNames)
	sort.Strings(sorted)
	return VersionsModel{
		deviceNames: sorted,
		total:       len(deviceNames),
		states:      states,
		startedAt:   time.Now(),
	}
}

// ─── bubbletea interface ──────────────────────────────────────────────────────

func (m VersionsModel) Init() tea.Cmd {
	return vTickCmd()
}

func (m VersionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.totalW = msg.Width
		m.totalH = msg.Height

	case TickMsg:
		m.tick++
		m.elapsed = time.Since(m.startedAt).Round(time.Second)
		if m.done {
			return m, tea.Quit
		}
		return m, vTickCmd()

	case VersionResultMsg:
		if s, ok := m.states[msg.Device]; ok {
			s.Status = vStatusDone
			s.Version = msg.Version
			s.ErrStr = msg.ErrStr
		}
		// Check if all devices are now done.
		allDone := true
		for _, s := range m.states {
			if s.Status == vStatusChecking {
				allDone = false
				break
			}
		}
		if allDone {
			m.done = true
		}

	case VersionsAllDoneMsg:
		m.done = true

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

// spinner frames (reuse the same braille set from the upgrade TUI).
var vSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m VersionsModel) View() string {
	if m.totalW == 0 {
		return ""
	}

	// ── styles ────────────────────────────────────────────────────────────────
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10"))

	failStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15"))

	// ── header ────────────────────────────────────────────────────────────────
	doneCount := 0
	for _, s := range m.states {
		if s.Status == vStatusDone {
			doneCount++
		}
	}
	header := headerStyle.Render(
		fmt.Sprintf("Checking firmware versions — %d/%d done  elapsed: %s",
			doneCount, m.total, m.elapsed))

	// ── device list ───────────────────────────────────────────────────────────
	spinner := vSpinnerFrames[m.tick%len(vSpinnerFrames)]
	var rows []string
	for _, name := range m.deviceNames {
		s := m.states[name]
		var icon, value string
		switch {
		case s.Status == vStatusChecking:
			icon = dimStyle.Render(spinner)
			value = dimStyle.Render("checking…")
		case s.ErrStr != "":
			icon = failStyle.Render("✗")
			value = failStyle.Render("Unreachable")
		default:
			icon = successStyle.Render("✓")
			value = successStyle.Render(s.Version)
		}
		row := fmt.Sprintf("  %s  %-42s  %s", icon, nameStyle.Render(name), value)
		rows = append(rows, truncate(row, m.totalW-1))
	}

	// ── footer ────────────────────────────────────────────────────────────────
	footer := dimStyle.Render("q — quit")
	if m.done {
		footer = dimStyle.Render("Done. Press q to exit.")
	}

	return header + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
}

// vTickCmd schedules a TickMsg every 100ms.
func vTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// ─── VersionsProgram ─────────────────────────────────────────────────────────

// VersionsProgram wraps a bubbletea program for the versions TUI.
// It exposes Send() for goroutines to deliver live results and MarkDone() to
// guard against late sends after the program exits.
type VersionsProgram struct {
	prog *tea.Program
	done chan struct{}
}

// Send delivers a VersionResultMsg to the bubbletea event loop.  Safe to call
// from any goroutine; drops the message if the program has already exited.
func (vp *VersionsProgram) Send(name, version, errStr string) {
	select {
	case <-vp.done:
		return
	default:
		vp.prog.Send(VersionResultMsg{Device: name, Version: version, ErrStr: errStr})
	}
}

// SendAllDone tells the model that version checking is complete.
func (vp *VersionsProgram) SendAllDone() {
	select {
	case <-vp.done:
	default:
		vp.prog.Send(VersionsAllDoneMsg{})
	}
}

// MarkDone signals that the program has exited; subsequent Send calls are no-ops.
func (vp *VersionsProgram) MarkDone() {
	select {
	case <-vp.done:
	default:
		close(vp.done)
	}
}

// StartVersions initialises a bubbletea program for the versions TUI.
// Returns a *VersionsProgram for live result delivery and a blocking run func.
func StartVersions(model VersionsModel) (*VersionsProgram, func() error) {
	prog := tea.NewProgram(model, tea.WithAltScreen())
	vp := &VersionsProgram{
		prog: prog,
		done: make(chan struct{}),
	}
	return vp, func() error {
		_, err := prog.Run()
		return err
	}
}
