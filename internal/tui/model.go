// Package tui provides the bubbletea-based TUI for the esp-tool upgrade
// command. It implements the Elm-architecture Model/Update/View loop:
//
//   - messages.go defines every tea.Msg type the model handles.
//   - writer.go implements output.OutputWriter by sending those messages via
//     prog.Send(), routing device goroutine output into the event loop.
//   - model.go (this file) owns all display state and the Update/View logic.
//
// Phase 1D: functional plain-text view. Phase 1E adds lipgloss panel styling.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Status represents a device's current upgrade status.
type Status int

const (
	StatusQueued   Status = iota // waiting for a concurrency slot
	StatusRunning                // esphome process is running
	StatusRetrying               // sleeping before next attempt (guideline #7: ↺)
	StatusSuccess                // completed with exit code 0
	StatusFailed                 // exhausted all retries with non-zero exit
)

// DeviceState holds all runtime state for a single device.
type DeviceState struct {
	Status    Status
	Attempts  int
	StartedAt time.Time
	Duration  time.Duration // final when completed
	LastLine  string        // last output line, shown in Active Jobs panel
	ErrLines  []string      // populated on failure by parseErrors (best-effort)
	RetryAt   time.Time     // when the next attempt will begin (for ↺ display)
}

// TailLine is one entry in the global output tail log.
type TailLine struct {
	Device string
	Line   string
}

// maxTailLines is the maximum number of lines kept in Model.GlobalTail.
const maxTailLines = 200

// ErrorEntry is one device's failure entry in the Errors panel.
type ErrorEntry struct {
	Device   string
	Snippets []string
}

// Model is the bubbletea Model for the upgrade TUI.
// Static config fields are set once by NewModel and never change.
// Dynamic state fields are updated by Update() as tea.Msgs arrive.
type Model struct {
	// Static config
	Devices     []string // ordered device names from discovery
	Jobs        int      // --jobs (concurrency)
	Retries     int      // --retries
	WorkDir     string   // --dir
	DryRun      bool
	LogFilePath string // non-empty when --log-file is set

	// Dynamic state
	States      map[string]*DeviceState
	Errors      []ErrorEntry  // failed-device error entries for Errors panel
	GlobalTail  []TailLine    // bounded log of all device output lines (max maxTailLines)
	Done        bool          // AllDoneMsg has been received
	HasFailures bool          // at least one device failed
	startedAt   time.Time     // for elapsed-time calculation
	Elapsed     time.Duration // updated on every TickMsg

	// Layout (updated by WindowSizeMsg)
	TermWidth  int
	TermHeight int

	// Animation
	Tick int // spinner frame index; incremented by TickMsg

	// UI state (Phase 1G)
	ShowHelp     bool // toggled by '?'; shows keyboard-shortcut overlay
	ScrollOffset int  // index of the first visible device in the device list
}

// NewModel constructs a Model with all devices in StatusQueued.
// Additional config fields (DryRun, LogFilePath) can be set after construction.
func NewModel(devices []string, jobs, retries int, workDir string) Model {
	states := make(map[string]*DeviceState, len(devices))
	for _, d := range devices {
		states[d] = &DeviceState{Status: StatusQueued}
	}
	// Make a defensive copy of the devices slice so callers can't mutate it.
	devCopy := make([]string, len(devices))
	copy(devCopy, devices)
	return Model{
		Devices:   devCopy,
		Jobs:      jobs,
		Retries:   retries,
		WorkDir:   workDir,
		States:    states,
		startedAt: time.Now(),
	}
}

// Init implements tea.Model. Returns the first tick command to start the
// spinner animation and elapsed-time updates.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// tickCmd returns a Cmd that fires a TickMsg after 100ms (spinner/clock rate).
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// Update implements tea.Model. It is the single-threaded event handler for all
// tea.Msgs, including device events sent by TUIWriter goroutines.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Keyboard ────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.ShowHelp = !m.ShowHelp
		case "up", "k":
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
		case "down", "j":
			if m.ScrollOffset < len(m.Devices)-1 {
				m.ScrollOffset++
			}
		case "home", "g":
			m.ScrollOffset = 0
		case "end", "G":
			if len(m.Devices) > 0 {
				m.ScrollOffset = len(m.Devices) - 1
			}
		}

	// ── Terminal resize ──────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height

	// ── Spinner / elapsed clock ──────────────────────────────────────────────
	case TickMsg:
		m.Tick++
		m.Elapsed = time.Since(m.startedAt)
		return m, tickCmd()

	// ── Device output line ───────────────────────────────────────────────────
	case DeviceOutputMsg:
		if st, ok := m.States[msg.Device]; ok {
			// First line after retry sleep → device is actively running again.
			if st.Status == StatusRetrying {
				st.Status = StatusRunning
			}
			st.LastLine = msg.Line
		}
		// Accumulate in the global tail log (bounded at maxTailLines).
		m.GlobalTail = append(m.GlobalTail, TailLine{Device: msg.Device, Line: msg.Line})
		if len(m.GlobalTail) > maxTailLines {
			m.GlobalTail = m.GlobalTail[len(m.GlobalTail)-maxTailLines:]
		}

	// ── Device status change (started / completed) ───────────────────────────
	case DeviceStatusMsg:
		if st, ok := m.States[msg.Device]; ok {
			st.Status = msg.Status
			st.Attempts = msg.Attempts
			st.Duration = msg.Duration
			if msg.Status == StatusRunning {
				st.StartedAt = time.Now()
			}
			if msg.Status == StatusFailed && len(msg.ErrLines) > 0 {
				st.ErrLines = msg.ErrLines
				m.Errors = append(m.Errors, ErrorEntry{
					Device:   msg.Device,
					Snippets: msg.ErrLines,
				})
			}
		}

	// ── Retry sleep notification ─────────────────────────────────────────────
	case DeviceRetryMsg:
		if st, ok := m.States[msg.Device]; ok {
			st.Status = StatusRetrying
			st.RetryAt = time.Now().Add(msg.Delay)
		}

	// ── All goroutines done ──────────────────────────────────────────────────
	case AllDoneMsg:
		m.Done = true
		m.HasFailures = m.countFailed() > 0
		if !m.HasFailures {
			// Success: auto-exit, plain summary printed after TUI closes (PRD §4 G4).
			return m, tea.Quit
		}
		// Failures: stay open so operator can read errors; user presses q to exit.
		return m, nil
	}

	return m, nil
}

// View implements tea.Model. Routes to the lipgloss panel view when the
// terminal is large enough, otherwise falls back to the Phase 1D plain-text
// view for small or piped terminals.
func (m Model) View() string {
	if m.TermWidth >= 80 && m.TermHeight >= 24 {
		return m.viewPanels()
	}
	return m.viewFallback()
}

// viewPanels assembles the full lipgloss layout: header, two-column body, and
// output tail / help panel at the bottom.
func (m Model) viewPanels() string {
	l := LayoutFor(m.TermWidth, m.TermHeight)

	header := renderHeader(m, l)
	deviceList := renderDeviceList(m, l)
	activeJobs := renderActiveJobs(m, l)
	errorsPanel := renderErrors(m, l)

	// '?' toggles the help overlay in place of the output tail.
	var bottom string
	if m.ShowHelp {
		bottom = renderHelp(m, l)
	} else {
		bottom = renderOutputTail(m, l)
	}

	rightCol := lipgloss.JoinVertical(lipgloss.Left, activeJobs, errorsPanel)
	body := lipgloss.JoinHorizontal(lipgloss.Top, deviceList, rightCol)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, bottom)
}

// viewFallback is the Phase 1D minimal plain-text view for small/piped terminals.
func (m Model) viewFallback() string {
	var sb strings.Builder

	// ── Header line ──────────────────────────────────────────────────────────
	elapsed := m.Elapsed.Round(time.Second)
	completed := m.countCompleted()
	fmt.Fprintf(&sb,
		"esp-tool upgrade  ⚡%d jobs  ↺%d retries  %d/%d devices  %s\n\n",
		m.Jobs, m.Retries, completed, len(m.Devices), elapsed,
	)

	// ── Device list ──────────────────────────────────────────────────────────
	for _, name := range m.Devices {
		st := m.States[name]
		icon := statusIcon(st.Status, m.Tick)
		line := fmt.Sprintf("  %s  %s", icon, name)

		// Show retry countdown for "↺ retry in Xs" (guideline #7).
		if st.Status == StatusRetrying {
			remaining := time.Until(st.RetryAt).Round(time.Second)
			if remaining > 0 {
				line += fmt.Sprintf("  ↺ retry in %s", remaining)
			}
		}

		// Show elapsed/duration.
		switch st.Status {
		case StatusRunning, StatusRetrying:
			if !st.StartedAt.IsZero() {
				line += fmt.Sprintf("  %s…", time.Since(st.StartedAt).Round(time.Second))
			}
		case StatusSuccess, StatusFailed:
			if st.Duration > 0 {
				line += fmt.Sprintf("  [%s]", st.Duration.Round(time.Second))
			}
		}
		sb.WriteString(line + "\n")
	}

	// ── Errors panel ─────────────────────────────────────────────────────────
	if len(m.Errors) > 0 {
		fmt.Fprintf(&sb, "\nErrors (%d):\n", len(m.Errors))
		for _, e := range m.Errors {
			fmt.Fprintf(&sb, "  %s:\n", e.Device)
			for _, snippet := range e.Snippets {
				for _, l := range strings.Split(snippet, "\n") {
					fmt.Fprintf(&sb, "    %s\n", l)
				}
			}
		}
	}

	// ── Footer ───────────────────────────────────────────────────────────────
	sb.WriteString("\n")
	if m.Done && m.HasFailures {
		sb.WriteString("Press [q] to exit and view summary\n")
	} else if !m.Done {
		sb.WriteString("[q] quit\n")
	}

	return sb.String()
}

// spinnerFrames are the braille animation frames per PRD §7.2.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// statusIcon returns the display icon for a device status.
// For StatusRunning it cycles through the braille spinner frames.
func statusIcon(s Status, tick int) string {
	switch s {
	case StatusRunning:
		return spinnerFrames[tick%len(spinnerFrames)]
	case StatusRetrying:
		return "↺"
	case StatusSuccess:
		return "✓"
	case StatusFailed:
		return "✗"
	default: // StatusQueued
		return "◷"
	}
}

// countCompleted returns the number of devices in a terminal state.
func (m Model) countCompleted() int {
	n := 0
	for _, st := range m.States {
		if st.Status == StatusSuccess || st.Status == StatusFailed {
			n++
		}
	}
	return n
}

// countFailed returns the number of devices in StatusFailed.
func (m Model) countFailed() int {
	n := 0
	for _, st := range m.States {
		if st.Status == StatusFailed {
			n++
		}
	}
	return n
}
