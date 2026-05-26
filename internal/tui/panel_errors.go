package tui

import (
	"fmt"
	"strings"
)

// renderErrors renders the bottom-right panel showing per-device error snippets.
func renderErrors(m Model, l Layout) string {
	var sb strings.Builder
	sb.WriteString(stylePanelTitle.Render(fmt.Sprintf("Errors (%d)", len(m.Errors))) + "\n")

	for _, e := range m.Errors {
		sb.WriteString("  " + styleBoldRed.Render(e.Device) + "\n")
		for _, snippet := range e.Snippets {
			for _, line := range strings.Split(snippet, "\n") {
				sb.WriteString("    " + styleDim.Render(line) + "\n")
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
