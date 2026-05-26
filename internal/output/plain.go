// Package output defines the OutputWriter abstraction that decouples the
// execution engine (upgrader) from the display layer (plain text or TUI).
//
// PlainWriter (this file) writes directly to an io.Writer, preserving the
// exact output format of the original fmt.Printf calls in runner.go.
//
// TUIWriter (internal/tui, Phase 1D) sends tea.Msgs to the bubbletea program.
package output

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// PlainWriter is an OutputWriter for non-TUI mode. It writes directly to Out
// using the same format as the original fmt.Printf calls in runner.go.
// All methods are safe for concurrent use — a single mutex serialises writes.
//
// The mutex is intentional from day one (guideline #4): even though os.Stdout
// POSIX-guarantees atomicity for small writes, --log-file targets need it, and
// the TUIWriter that replaces PlainWriter in Phase 1D sends through a channel
// that also requires serialisation.
type PlainWriter struct {
	mu     sync.Mutex
	Out    io.Writer
	Prefix bool // if true, each line is prefixed with "[device] "
}

// NewPlainWriter constructs a PlainWriter that writes to out.
// When prefix is true each device output line is prefixed with "[device] ".
func NewPlainWriter(out io.Writer, prefix bool) *PlainWriter {
	return &PlainWriter{Out: out, Prefix: prefix}
}

// WriteLine writes one output line from a device.
// Format with prefix: "[device] line\n"
// Format without:     "line\n"
func (w *PlainWriter) WriteLine(device, line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Prefix {
		fmt.Fprintf(w.Out, "[%s] %s\n", device, line)
	} else {
		fmt.Fprintln(w.Out, line)
	}
}

// DeviceStarted is a no-op for PlainWriter.
// The first output line from the device serves as the implicit start signal;
// no separate announcement is printed in plain mode.
func (w *PlainWriter) DeviceStarted(_ string) {}

// DeviceCompleted is a no-op for PlainWriter.
// The per-device outcome is reported by report.PrintUpgradeSummary after
// Upgrade() returns; the writer itself does not emit completion lines.
// (errLines will be populated by parseErrors in Phase 1C; always nil in 1B.)
func (w *PlainWriter) DeviceCompleted(_ string, _ bool, _ int, _ time.Duration, _ []string) {}

// DeviceRetrying prints a retry notice to Out, preserving the exact format
// of the original fmt.Printf("[%s] retrying…") call in runner.go.
func (w *PlainWriter) DeviceRetrying(device string, attempt, maxAttempts int, delay time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintf(w.Out, "[%s] retrying (attempt %d/%d) after %s...\n", device, attempt, maxAttempts, delay)
}
