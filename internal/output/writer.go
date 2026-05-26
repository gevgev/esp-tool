// Package output defines the OutputWriter abstraction that decouples the
// execution engine (upgrader) from the display layer (plain text or TUI).
//
// PlainWriter (this package) writes directly to an io.Writer, preserving the
// exact output format of the original fmt.Printf calls in runner.go.
//
// TUIWriter (internal/tui, Phase 1D) sends tea.Msgs to the bubbletea program.
package output

import "time"

// OutputWriter is the interface between the upgrade execution engine and the
// display layer. All methods must be safe to call concurrently from multiple
// device goroutines.
type OutputWriter interface {
	// WriteLine is called for every line of output from a device's esphome process.
	WriteLine(device, line string)

	// DeviceStarted is called once per device when its goroutine acquires the
	// concurrency semaphore and begins its first attempt.
	DeviceStarted(device string)

	// DeviceCompleted is called when a device finishes all attempts.
	// success is true when the final attempt exited with code 0.
	// errLines contains parsed error snippets extracted from the device's output
	// buffer on failure (nil on success). Populated by parseErrors in Phase 1C;
	// always nil in Phase 1B.
	DeviceCompleted(device string, success bool, attempts int, duration time.Duration, errLines []string)

	// DeviceRetrying is called before each retry sleep, after a failed attempt.
	// attempt is the upcoming attempt number (≥ 2); maxAttempts is the total
	// attempt limit; delay is the configured retry wait duration.
	DeviceRetrying(device string, attempt, maxAttempts int, delay time.Duration)
}
