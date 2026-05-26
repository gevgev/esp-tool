package tui

// truncate clips s to at most maxRunes visible characters.
// If s is longer than maxRunes, it is trimmed and the last character is
// replaced with '…' so callers know content was cut.  This keeps rendered
// panel lines to a single terminal row and prevents lipgloss boxes from
// growing beyond their allocated height.
func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// clampLines returns the first maxLines lines of s (split on "\n").
// If s has fewer lines, it is returned unchanged.
func clampLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := splitLines(s)
	if len(lines) <= maxLines {
		return s
	}
	return joinLines(lines[:maxLines])
}

// splitLines splits s on "\n" without using strings.Split to avoid an
// extra import in this tiny file.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i, ch := range s {
		if ch == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func joinLines(lines []string) string {
	total := 0
	for _, l := range lines {
		total += len(l) + 1
	}
	b := make([]byte, 0, total)
	for i, l := range lines {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, l...)
	}
	return string(b)
}
