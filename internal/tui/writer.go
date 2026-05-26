package tui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ggevorgyan/esp-tool/internal/output"
)

// sender is the subset of *tea.Program that TUIWriter uses.
// Abstracting it as an interface enables unit testing without a real terminal.
type sender interface {
	Send(msg tea.Msg)
}

// TUIWriter implements output.OutputWriter by sending tea.Msgs to the
// bubbletea program's event loop. prog.Send() is goroutine-safe, so multiple
// device goroutines can call these methods concurrently.
//
// Call MarkDone() after tea.Program.Run() returns to prevent device goroutines
// that are still finishing from sending messages into a dead program
// (guideline #6: guard against late prog.Send() after quit).
type TUIWriter struct {
	prog sender
	once sync.Once
	done chan struct{} // closed by MarkDone; subsequent sends are dropped
}

// Compile-time check: TUIWriter must satisfy output.OutputWriter.
var _ output.OutputWriter = (*TUIWriter)(nil)

// NewTUIWriter constructs a TUIWriter that sends events to prog.
func NewTUIWriter(prog sender) *TUIWriter {
	return &TUIWriter{
		prog: prog,
		done: make(chan struct{}),
	}
}

// send delivers msg to the program unless MarkDone has been called.
// The select is best-effort: a message may arrive in the nanosecond window
// between the done-check and prog.Send(). This is harmless — bubbletea's
// program drops messages after it exits.
func (w *TUIWriter) send(msg tea.Msg) {
	select {
	case <-w.done:
		return // program has quit — drop the message (guideline #6)
	default:
		w.prog.Send(msg)
	}
}

// MarkDone closes the done channel so subsequent send calls are dropped.
// Safe to call multiple times — sync.Once guarantees idempotence.
// Call this immediately after tea.Program.Run() returns.
func (w *TUIWriter) MarkDone() {
	w.once.Do(func() { close(w.done) })
}

// SendAllDone delivers AllDoneMsg to the program. Called by the goroutine in
// main.go after Upgrade() returns (all device goroutines are finished).
func (w *TUIWriter) SendAllDone() {
	w.send(AllDoneMsg{})
}

// ── output.OutputWriter implementation ──────────────────────────────────────

// WriteLine sends a DeviceOutputMsg for one output line from a device's process.
func (w *TUIWriter) WriteLine(device, line string) {
	w.send(DeviceOutputMsg{Device: device, Line: line})
}

// DeviceStarted sends DeviceStatusMsg{StatusRunning} when a device's goroutine
// acquires the concurrency semaphore and begins its first attempt.
func (w *TUIWriter) DeviceStarted(device string) {
	w.send(DeviceStatusMsg{Device: device, Status: StatusRunning})
}

// DeviceCompleted sends DeviceStatusMsg{StatusSuccess or StatusFailed} when a
// device finishes all retry attempts. errLines (from parseErrors) is forwarded
// to the Errors panel; nil on success.
func (w *TUIWriter) DeviceCompleted(device string, success bool, attempts int, duration time.Duration, errLines []string) {
	status := StatusSuccess
	if !success {
		status = StatusFailed
	}
	w.send(DeviceStatusMsg{
		Device:   device,
		Status:   status,
		Attempts: attempts,
		Duration: duration,
		ErrLines: errLines,
	})
}

// DeviceRetrying sends DeviceRetryMsg before each retry sleep.
// The model transitions the device to StatusRetrying (guideline #7: ↺ retry in Xs).
func (w *TUIWriter) DeviceRetrying(device string, attempt, maxAttempts int, delay time.Duration) {
	w.send(DeviceRetryMsg{
		Device:  device,
		Attempt: attempt,
		Max:     maxAttempts,
		Delay:   delay,
	})
}
