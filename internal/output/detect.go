package output

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	minTermWidth  = 80
	minTermHeight = 24
)

// ShouldUseTUI reports whether the TUI should be activated.
//
// Returns (true, "") when all conditions are satisfied.
// Returns (false, reason) otherwise — reason is a human-readable string
// suitable for display under --verbose (guideline #2: "TUI unavailable,
// falling back to plain mode: <reason>").
//
// Conditions required for TUI:
//  1. plain == false  (neither --plain nor --no-tui was given)
//  2. os.Stdout is a TTY
//  3. Terminal is at least 80×24
func ShouldUseTUI(plain bool) (use bool, reason string) {
	if plain {
		return false, "plain mode requested (--plain / --no-tui)"
	}

	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return false, "stdout is not a terminal"
	}

	w, h, err := term.GetSize(fd)
	if err != nil {
		return false, fmt.Sprintf("could not determine terminal size: %v", err)
	}
	if w < minTermWidth || h < minTermHeight {
		return false, fmt.Sprintf(
			"terminal too small (%d×%d, need at least %d×%d)",
			w, h, minTermWidth, minTermHeight,
		)
	}

	return true, ""
}
