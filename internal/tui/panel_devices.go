package tui

import (
	"fmt"
	"strings"
	"time"
)

// renderDeviceList renders the left panel showing all devices and their status.
// When there are more devices than can fit, scroll indicators (▲/▼) appear and
// the visible window is controlled by m.ScrollOffset.
func renderDeviceList(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render("Devices") + "\n")

	total := len(m.Devices)

	if total > 0 {
		// contentH: inner height of the box minus the title row.
		// BodyH = outer panel height; subtract 2 for borders, 1 for title.
		contentH := l.BodyH - 3
		if contentH < 1 {
			contentH = 1
		}

		// Clamp scroll offset to a valid range.
		start := m.ScrollOffset
		if start < 0 {
			start = 0
		}
		if start >= total {
			start = total - 1
		}

		showScrollUp := start > 0
		showScrollDown := start+contentH < total

		// Reserve one content line per indicator that is visible.
		slotCount := contentH
		if showScrollUp {
			slotCount--
		}
		if showScrollDown {
			slotCount--
		}
		if slotCount < 0 {
			slotCount = 0
		}

		end := start + slotCount
		if end > total {
			end = total
		}

		// Up-scroll indicator.
		if showScrollUp {
			sb.WriteString(styleDim.Render(fmt.Sprintf(" ▲ %d more above", start)) + "\n")
		}

		// Visible devices.
		for _, name := range m.Devices[start:end] {
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

		// Down-scroll indicator.
		if showScrollDown {
			remaining := total - end
			sb.WriteString(styleDim.Render(fmt.Sprintf(" ▼ %d more below", remaining)) + "\n")
		}
	}

	inner := strings.TrimRight(sb.String(), "\n")

	w := max(0, l.LeftW-2)
	h := max(0, l.BodyH-2)
	return stylePanelBox.Width(w).Height(h).Render(inner)
}
