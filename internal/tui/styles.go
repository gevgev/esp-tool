package tui

import "github.com/charmbracelet/lipgloss"

// Shared lipgloss styles for all panel renderers.
// These are package-level vars, safe for concurrent read-only use after init.
//
// Color palette uses 4-bit ANSI for broad terminal compatibility:
//   0=black 1=red 2=green 3=yellow 4=blue 5=magenta 6=cyan 7=white
//   8-15 = bright variants; 240-255 = grey scale

var (
	// Text styles
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleBoldRed = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	styleCyan    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Panel container style — rounded border, dim border color.
	// Width and Height are set per-call since they vary per panel.
	stylePanelBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	// Section title inside a panel (bold, full-width bar)
	stylePanelTitle = lipgloss.NewStyle().Bold(true)
)

// styledStatus returns the icon for a device status with the appropriate color.
func styledStatus(s Status, tick int) string {
	switch s {
	case StatusRunning:
		return styleCyan.Render(spinnerFrames[tick%len(spinnerFrames)])
	case StatusRetrying:
		return styleYellow.Render("↺")
	case StatusSuccess:
		return styleGreen.Render("✓")
	case StatusFailed:
		return styleRed.Render("✗")
	default: // StatusQueued
		return styleDim.Render("◷")
	}
}
