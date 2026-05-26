package tui

import (
	"fmt"
	"strings"
)

// renderOutputTail renders the bottom-spanning panel with the most recent
// output lines from all devices (bounded by GlobalTail capacity).
//
// Each line is truncated to l.TotalW-6 runes so it occupies exactly one
// terminal row and cannot inflate the panel height beyond OutputH.
func renderOutputTail(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render("Output") + "\n")

	// Show as many tail lines as fit in the inner content area:
	// OutputH total = 2 border rows + 1 title row + N content rows → N = OutputH-3
	maxLines := l.OutputH - 3
	if maxLines < 1 {
		maxLines = 1
	}

	// Maximum visible width for a single output line (device prefix + spacing ≈ 6 chars).
	maxLineW := l.TotalW - 6
	if maxLineW < 10 {
		maxLineW = 10
	}

	tail := m.GlobalTail
	if len(tail) > maxLines {
		tail = tail[len(tail)-maxLines:]
	}

	for _, tl := range tail {
		prefix := styleCyan.Render(tl.Device)
		line := truncate(tl.Line, maxLineW)
		sb.WriteString(fmt.Sprintf(" %s  %s\n", prefix, line))
	}

	inner := strings.TrimRight(sb.String(), "\n")

	w := l.TotalW - 2
	if w < 0 {
		w = 0
	}
	h := l.OutputH - 2
	if h < 0 {
		h = 0
	}

	return stylePanelBox.Width(w).Height(h).Render(inner)
}
