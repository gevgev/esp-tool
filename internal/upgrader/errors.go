package upgrader

import "strings"

// errorKeywords are the patterns that parseErrors scans for.
// Order matters: first match wins for each line.
// Words are chosen to have low false-positive rates in ESPHome log output
// while catching the most common failure signals.
var errorKeywords = []string{
	"ERROR",
	"FAILED",
	"Exception",
	"Traceback",
	"refused",
	"timeout",
	"unreachable",
	"CRIT",
}

// parseErrors scans captured output lines for recognisable error patterns and
// returns up to 4 snippets, each consisting of the matching line plus the next
// non-empty line (for context).
//
// When no keywords match, a generic fallback message is returned so callers
// always have something human-readable to display.
//
// This function is explicitly best-effort (guideline #8): an empty or generic
// result does NOT imply success — the process exit code is the authoritative
// signal. parseErrors is only called after a non-zero exit has already been
// confirmed.
func parseErrors(lines []string) []string {
	const maxSnippets = 4

	var snippets []string
	for i, line := range lines {
		if !containsAny(line, errorKeywords) {
			continue
		}
		snippet := line
		// Include the next non-empty line for context (e.g. Python traceback detail).
		if i+1 < len(lines) && lines[i+1] != "" {
			snippet += "\n" + lines[i+1]
		}
		snippets = append(snippets, snippet)
		if len(snippets) >= maxSnippets {
			break
		}
	}

	if len(snippets) == 0 {
		return []string{"Process exited with error (no parseable error line found)"}
	}
	return snippets
}

// containsAny reports whether line contains any of the given substrings.
func containsAny(line string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}
