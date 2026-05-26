package tui

import "strings"

// renderHelp renders the bottom panel as a keyboard-shortcut reference.
// It replaces the output tail panel when the user presses '?'.
func renderHelp(m Model, l Layout) string {
	// The Model parameter is kept for future context-sensitive help (e.g. hiding
	// scroll shortcuts when there is only one device), but is unused for now.
	_ = m

	lines := []string{
		stylePanelTitle.Render("Keyboard shortcuts") + "   " + styleDim.Render("press ? to close"),
		"",
		"  q / ctrl+c   quit (auto-exits on all-success; waits on failure)",
		"  ?            toggle this help",
		"  ↑ / k        scroll device list up",
		"  ↓ / j        scroll device list down",
		"  Home / g     jump to top of device list",
		"  End  / G     jump to bottom of device list",
	}

	inner := strings.Join(lines, "\n")

	w := max(0, l.TotalW-2)
	h := max(0, l.OutputH-2)
	return stylePanelBox.Width(w).Height(h).Render(inner)
}
