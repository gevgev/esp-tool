package tui

import (
	"fmt"
	"strings"
	"time"
)

// renderDeviceList renders the left panel showing all devices and their status.
func renderDeviceList(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render("Devices") + "\n")

	for _, name := range m.Devices {
		st := m.States[name]
		icon := styledStatus(st.Status, m.Tick)
		row := fmt.Sprintf(" %s  %s", icon, name)

		// Retry countdown for devices sleeping before next attempt.
		if st.Status == StatusRetrying {
			remaining := time.Until(st.RetryAt).Round(time.Second)
			if remaining > 0 {
				row += " " + styleDim.Render(fmt.Sprintf("↺ in %s", remaining))
			}
		}

		// For completed devices: retry badge (when >1 attempt) and final duration.
		if st.Status == StatusSuccess || st.Status == StatusFailed {
			if st.Attempts > 1 {
				row += " " + styleDim.Render(fmt.Sprintf("↺%d", st.Attempts))
			}
			if st.Duration > 0 {
				row += " " + styleDim.Render(st.Duration.Round(time.Second).String())
			}
		}

		sb.WriteString(row + "\n")
	}

	inner := strings.TrimRight(sb.String(), "\n")

	w := l.LeftW - 2
	if w < 0 {
		w = 0
	}
	h := l.BodyH - 2
	if h < 0 {
		h = 0
	}

	return stylePanelBox.Width(w).Height(h).Render(inner)
}
