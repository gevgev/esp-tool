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

// DiagnosticsResultMsg carries one device's completed diagnostic result.
// The fields mirror diagnostics.Result but use plain types to avoid importing
// the diagnostics package from tui (keeping the import graph clean).
type DiagnosticsResultMsg struct {
	Device  string
	Version string // empty if unreachable
	Err     string // non-empty if could not connect
	// Health summary derived from issues slice before sending.
	Crashes  int
	Warnings int
	// IssueLines are the human-readable issue messages (one per line).
	IssueLines []string
}

// DiagnosticsAllDoneMsg is sent after diagnostics.Check returns.
type DiagnosticsAllDoneMsg struct{}

// ─── device states ────────────────────────────────────────────────────────────

type diagDeviceStatus int

const (
	dStatusChecking diagDeviceStatus = iota
	dStatusDone
)

type diagDeviceState struct {
	Status     diagDeviceStatus
	Version    string
	Err        string
	Crashes    int
	Warnings   int
	IssueLines []string
}

// ─── model ────────────────────────────────────────────────────────────────────

// DiagnosticsModel is the bubbletea model for the "diagnostics" TUI.
type DiagnosticsModel struct {
	// static config
	deviceNames []string
	total       int

	// dynamic state
	states    map[string]*diagDeviceState
	done      bool
	startedAt time.Time
	elapsed   time.Duration
	tick      int
	totalW    int
	totalH    int
}

// NewDiagnosticsModel creates a DiagnosticsModel for the given device names.
func NewDiagnosticsModel(deviceNames []string) DiagnosticsModel {
	states := make(map[string]*diagDeviceState, len(deviceNames))
	for _, n := range deviceNames {
		states[n] = &diagDeviceState{Status: dStatusChecking}
	}
	sorted := make([]string, len(deviceNames))
	copy(sorted, deviceNames)
	sort.Strings(sorted)
	return DiagnosticsModel{
		deviceNames: sorted,
		total:       len(deviceNames),
		states:      states,
		startedAt:   time.Now(),
	}
}

// ─── bubbletea interface ──────────────────────────────────────────────────────

func (m DiagnosticsModel) Init() tea.Cmd {
	return diagTickCmd()
}

func (m DiagnosticsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m, diagTickCmd()

	case DiagnosticsResultMsg:
		if s, ok := m.states[msg.Device]; ok {
			s.Status = dStatusDone
			s.Version = msg.Version
			s.Err = msg.Err
			s.Crashes = msg.Crashes
			s.Warnings = msg.Warnings
			s.IssueLines = msg.IssueLines
		}
		allDone := true
		for _, s := range m.states {
			if s.Status == dStatusChecking {
				allDone = false
				break
			}
		}
		if allDone {
			m.done = true
		}

	case DiagnosticsAllDoneMsg:
		m.done = true

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m DiagnosticsModel) View() string {
	if m.totalW == 0 {
		return ""
	}

	// ── styles ────────────────────────────────────────────────────────────────
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	healthyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	crashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	versionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	// ── header ────────────────────────────────────────────────────────────────
	doneCount := 0
	for _, s := range m.states {
		if s.Status == dStatusDone {
			doneCount++
		}
	}
	header := headerStyle.Render(
		fmt.Sprintf("Device diagnostics — %d/%d done  elapsed: %s",
			doneCount, m.total, m.elapsed))

	// ── device list ───────────────────────────────────────────────────────────
	spinner := vSpinnerFrames[m.tick%len(vSpinnerFrames)]
	maxLineW := m.totalW - 4
	if maxLineW < 20 {
		maxLineW = 20
	}

	var rows []string
	for _, name := range m.deviceNames {
		s := m.states[name]

		var icon, health, ver string
		switch {
		case s.Status == dStatusChecking:
			icon = dimStyle.Render(spinner)
			health = dimStyle.Render("checking…")
			ver = ""

		case s.Err != "":
			icon = crashStyle.Render("✗")
			health = crashStyle.Render("Unreachable")
			ver = ""

		case s.Crashes > 0:
			icon = crashStyle.Render("✗")
			health = crashStyle.Render(fmt.Sprintf("%d crash", s.Crashes))
			if s.Version != "" {
				ver = "  " + versionStyle.Render(s.Version)
			}

		case s.Warnings > 0:
			icon = warnStyle.Render("⚠")
			health = warnStyle.Render(fmt.Sprintf("%d warning", s.Warnings))
			if s.Warnings > 1 {
				health = warnStyle.Render(fmt.Sprintf("%d warnings", s.Warnings))
			}
			if s.Version != "" {
				ver = "  " + versionStyle.Render(s.Version)
			}

		default:
			icon = healthyStyle.Render("✓")
			health = healthyStyle.Render("Healthy")
			if s.Version != "" {
				ver = "  " + versionStyle.Render(s.Version)
			}
		}

		row := fmt.Sprintf("  %s  %-42s  %s%s",
			icon, nameStyle.Render(name), health, ver)
		rows = append(rows, truncate(row, maxLineW))

		// Show issue detail lines (indented) when done.
		if s.Status == dStatusDone && len(s.IssueLines) > 0 {
			for _, issue := range s.IssueLines {
				var issueStyle lipgloss.Style
				if strings.HasPrefix(issue, "✗") {
					issueStyle = crashStyle
				} else {
					issueStyle = warnStyle
				}
				detail := "       " + issueStyle.Render(truncate(issue, maxLineW-7))
				rows = append(rows, detail)
			}
		}
	}

	// ── footer ────────────────────────────────────────────────────────────────
	footer := dimStyle.Render("q — quit")
	if m.done {
		footer = dimStyle.Render("Done. Press q to exit.")
	}

	return header + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
}

func diagTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// ─── DiagnosticsProgram ───────────────────────────────────────────────────────

// DiagnosticsProgram wraps a bubbletea program for the diagnostics TUI.
type DiagnosticsProgram struct {
	prog *tea.Program
	done chan struct{}
}

// Send delivers a DiagnosticsResultMsg to the event loop. Safe to call from
// any goroutine; drops silently after MarkDone.
func (dp *DiagnosticsProgram) Send(msg DiagnosticsResultMsg) {
	select {
	case <-dp.done:
	default:
		dp.prog.Send(msg)
	}
}

// SendAllDone signals that all devices have been checked.
func (dp *DiagnosticsProgram) SendAllDone() {
	select {
	case <-dp.done:
	default:
		dp.prog.Send(DiagnosticsAllDoneMsg{})
	}
}

// MarkDone closes the guard channel so subsequent Send calls are no-ops.
func (dp *DiagnosticsProgram) MarkDone() {
	select {
	case <-dp.done:
	default:
		close(dp.done)
	}
}

// StartDiagnostics initialises a bubbletea program for the diagnostics TUI.
func StartDiagnostics(model DiagnosticsModel) (*DiagnosticsProgram, func() error) {
	prog := tea.NewProgram(model, tea.WithAltScreen())
	dp := &DiagnosticsProgram{
		prog: prog,
		done: make(chan struct{}),
	}
	return dp, func() error {
		_, err := prog.Run()
		return err
	}
}
