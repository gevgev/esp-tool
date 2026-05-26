package tui

import (
	"fmt"
	"strings"
)

// renderActiveJobs renders the top-right panel showing in-flight devices only.
// Queued and completed devices are omitted — this panel is for live progress.
//
// Content is hard-capped at l.ActiveH-3 lines (title + 2 border rows = 3
// overhead) and each LastLine is truncated to l.RightW-6 runes.  This
// prevents the panel from growing beyond its allocated height when firmware
// upload paths are very long — which would cause the total layout height to
// exceed TermHeight and push the header off-screen.
func renderActiveJobs(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render("Active Jobs") + "\n")

	// Maximum content rows inside the box (excluding title, top/bottom borders).
	maxContent := l.ActiveH - 3
	if maxContent < 1 {
		maxContent = 1
	}
	// Maximum visible width for a LastLine string (indent + icon + spacing = ~6 chars).
	maxLineW := l.RightW - 6
	if maxLineW < 10 {
		maxLineW = 10
	}

	lineCount := 0
	for _, name := range m.Devices {
		if lineCount >= maxContent {
			break
		}
		st := m.States[name]
		if st.Status != StatusRunning && st.Status != StatusRetrying {
			continue
		}

		// Device-name row — show attempt badge only when retrying (attempt > 1).
		icon := styledStatus(st.Status, m.Tick)
		attemptBadge := ""
		if st.CurrentAttempt > 1 {
			attemptBadge = "  " + styleDim.Render(fmt.Sprintf("[attempt %d/%d]", st.CurrentAttempt, m.Retries+1))
		}
		sb.WriteString(fmt.Sprintf(" %s  %s%s\n", icon, styleCyan.Render(name), attemptBadge))
		lineCount++

		// LastLine row — only if we still have room.
		if lineCount < maxContent {
			lastLine := st.LastLine
			if lastLine == "" {
				lastLine = styleDim.Render("starting…")
			} else {
				lastLine = styleDim.Render(truncate(lastLine, maxLineW))
			}
			sb.WriteString(fmt.Sprintf("    %s\n", lastLine))
			lineCount++
		}
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
