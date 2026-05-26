package tui

import (
	"fmt"
	"strings"
	"time"
)

// renderHeader renders the top status bar spanning the full terminal width.
// It shows: jobs, retries, progress fraction, a progress bar, and elapsed time.
func renderHeader(m Model, l Layout) string {
	total := len(m.Devices)
	completed := m.countCompleted()

	// Scale the progress bar to ~1/6 of the terminal width (min 4, max 40).
	barWidth := l.TotalW / 6
	if barWidth < 4 {
		barWidth = 4
	}
	if barWidth > 40 {
		barWidth = 40
	}

	filled := 0
	if total > 0 {
		filled = barWidth * completed / total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	elapsed := m.Elapsed.Round(time.Second)

	line := fmt.Sprintf(
		"⚡ esp-tool   jobs:%d   retries:%d   %d/%d devices   [%s]   %s",
		m.Jobs, m.Retries, completed, total, bar, elapsed,
	)

	return styleBold.Render(line)
}
