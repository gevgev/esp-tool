package tui

import (
	"fmt"
	"strings"
)

// renderErrors renders the bottom-right panel showing per-device error snippets.
//
// Content is hard-capped at l.ErrorsH-3 lines (title + 2 border rows = 3
// overhead) and each snippet line is truncated to l.RightW-6 runes to prevent
// the panel from growing beyond its allocated height.
func renderErrors(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render(fmt.Sprintf("Errors (%d)", len(m.Errors))) + "\n")

	// Maximum content rows inside the box (excluding title, top/bottom borders).
	maxContent := l.ErrorsH - 3
	if maxContent < 1 {
		maxContent = 1
	}
	maxLineW := l.RightW - 6
	if maxLineW < 10 {
		maxLineW = 10
	}

	lineCount := 0
	for _, e := range m.Errors {
		if lineCount >= maxContent {
			break
		}
		sb.WriteString("  " + styleBoldRed.Render(e.Device) + "\n")
		lineCount++

		for _, snippet := range e.Snippets {
			for _, line := range strings.Split(snippet, "\n") {
				if lineCount >= maxContent {
					break
				}
				sb.WriteString("    " + styleDim.Render(truncate(line, maxLineW)) + "\n")
				lineCount++
			}
			if lineCount >= maxContent {
				break
			}
		}
	}

	inner := strings.TrimRight(sb.String(), "\n")

	w := l.RightW - 2
	if w < 0 {
		w = 0
	}
	h := l.ErrorsH - 2
	if h < 0 {
		h = 0
	}

	return stylePanelBox.Width(w).Height(h).Render(inner)
}
