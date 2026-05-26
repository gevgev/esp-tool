package tui

import (
	"fmt"
	"strings"
)

// renderActiveJobs renders the top-right panel showing in-flight devices only.
// Queued and completed devices are omitted — this panel is for live progress.
func renderActiveJobs(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render("Active Jobs") + "\n")

	for _, name := range m.Devices {
		st := m.States[name]
		if st.Status != StatusRunning && st.Status != StatusRetrying {
			continue
		}
		icon := styledStatus(st.Status, m.Tick)
		lastLine := st.LastLine
		if lastLine == "" {
			lastLine = styleDim.Render("starting…")
		}
		sb.WriteString(fmt.Sprintf(" %s  %s\n", icon, styleCyan.Render(name)))
		sb.WriteString(fmt.Sprintf("    %s\n", styleDim.Render(lastLine)))
	}

	inner := strings.TrimRight(sb.String(), "\n")

	w := l.RightW - 2
	if w < 0 {
		w = 0
	}
	h := l.ActiveH - 2
	if h < 0 {
		h = 0
	}

	return stylePanelBox.Width(w).Height(h).Render(inner)
}
