package upgrader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/output"
)

// Result holds the outcome of upgrading a single device.
type Result struct {
	Device       discovery.Device
	Success      bool
	Attempts     int
	Duration     time.Duration
	Err          string   // last error message if failed
	ErrLines     []string // parsed error snippets (nil on success); guideline #8: best-effort only
	CompileError bool     // true when failure was a config/compile error; retries were skipped
}

// RunOptions controls parallelism and retry behaviour.
type RunOptions struct {
	// Concurrency is the maximum number of simultaneous esphome processes.
	// Default 4 — compiling is CPU/RAM intensive.
	Concurrency int
	// Retries is how many additional attempts to make after the first failure.
	Retries int
	// RetryDelay is how long to wait between retry attempts.
	RetryDelay time.Duration
	// DryRun prints the command that would be run without executing it.
	DryRun bool
	// LogPrefix sets whether to prefix live output lines with [device-name].
	LogPrefix bool
	// WorkDir is the directory to run esphome from (where the YAMLs live).
	WorkDir string
	// Verbose enables diagnostic logging to stderr.
	Verbose bool
	// LogFile receives all device output lines as they arrive ("streaming to disk").
	// Each line is written as "[device] line\n". nil disables log file output.
	// Wired up in Phase 1F via the --log-file flag.
	LogFile io.Writer
	// Ctx is an optional context for cancellation. When the context is cancelled
	// (e.g. because the user pressed q in the TUI), in-flight esphome processes
	// are killed via their process group and no new attempts are started.
	// nil is treated as context.Background() (no cancellation).
	Ctx context.Context
	// PerDeviceTimeout limits each individual attempt to this duration.
	// If an esphome process does not complete within this window it is killed and
	// the attempt is counted as a failure (subject to normal retry logic).
	// Zero means no timeout (default).
	PerDeviceTimeout time.Duration
}

// optCtx returns opts.Ctx if set, otherwise context.Background().
func optCtx(opts RunOptions) context.Context {
	if opts.Ctx != nil {
		return opts.Ctx
	}
	return context.Background()
}

// deviceBuffers holds per-device output storage managed inside runWithRetry.
//
//   - display: bounded ring buffer (~500 lines) for the TUI output-tail panel.
//     Created unconditionally so Phase 1D can read without nil checks.
//   - full: unbounded; accumulates all lines across all attempts; parsed by
//     parseErrors on failure then immediately discarded (GC eligible).
//   - logFile: non-nil if --log-file is set; lines written as they arrive.
type deviceBuffers struct {
	display *RingBuffer
	full    []string
	logFile io.Writer
}

// newDeviceBuffers initialises a fresh deviceBuffers for one device.
func newDeviceBuffers(logFile io.Writer) *deviceBuffers {
	return &deviceBuffers{
		display: NewRingBuffer(displayCapacity),
		logFile: logFile,
	}
}

// newLogger returns a logger that writes to stderr with timestamps when
// verbose is true, and discards all output otherwise.
func newLogger(verbose bool) *log.Logger {
	if verbose {
		return log.New(os.Stderr, "[verbose] ", log.Ltime|log.Lmicroseconds)
	}
	return log.New(io.Discard, "", 0)
}

// killGroup terminates an entire process group (the process and all its
// children). This is necessary because esphome spawns child processes that
// would otherwise keep the pipe's write end open and cause hangs.
func killGroup(p *os.Process, vlog *log.Logger, name string) {
	if p == nil {
		return
	}
	vlog.Printf("[%s] killing process group %d", name, p.Pid)
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		// Fall back to killing just the direct process
		vlog.Printf("[%s] process group kill failed (%v), falling back to direct kill", name, err)
		p.Kill()
	}
}

// Upgrade runs "esphome run <file> --no-logs --device <host>" for each device
// in parallel, respecting the concurrency semaphore, with retry on failure.
// Results are returned in the same order as devices.
//
// writer receives all output events — use output.NewPlainWriter for non-TUI
// mode, or a TUIWriter (Phase 1D) for the bubbletea interface.
func Upgrade(devices []discovery.Device, opts RunOptions, writer output.OutputWriter) []Result {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 5 * time.Second
	}
	vlog := newLogger(opts.Verbose)

	sem := make(chan struct{}, opts.Concurrency)
	results := make([]Result, len(devices))
	var wg sync.WaitGroup

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			vlog.Printf("[%s] goroutine started, waiting for semaphore (%d/%d slots in use)",
				d.Name, len(sem), cap(sem))
			sem <- struct{}{}
			defer func() { <-sem }()
			vlog.Printf("[%s] semaphore acquired", d.Name)

			// Signal that this device has started (TUI uses this for the active panel).
			writer.DeviceStarted(d.Name)

			results[idx] = runWithRetry(d, opts, writer, vlog)

			// Signal completion — errLines is nil on success, populated by
			// parseErrors on failure (Phase 1C). PlainWriter ignores this;
			// TUIWriter (Phase 1D) uses it to populate the Errors panel.
			writer.DeviceCompleted(d.Name, results[idx].Success, results[idx].Attempts, results[idx].Duration, results[idx].ErrLines)
		}(i, dev)
	}

	wg.Wait()
	return results
}

// CheckVersions runs "esphome logs <file> --device <host>" for each device in
// parallel, grabs the first "ESPHome version" line, then kills the process.
// timeout controls how long to wait per device before giving up.
func CheckVersions(devices []discovery.Device, opts RunOptions, timeout time.Duration) []VersionResult {
	if opts.Concurrency <= 0 {
		opts.Concurrency = len(devices)
	}
	vlog := newLogger(opts.Verbose)
	vlog.Printf("starting version check for %d devices (timeout: %s)", len(devices), timeout)

	results := make([]VersionResult, len(devices))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			vlog.Printf("[%s] goroutine started, waiting for semaphore", d.Name)
			sem <- struct{}{}
			defer func() { <-sem }()
			vlog.Printf("[%s] semaphore acquired", d.Name)

			results[idx] = fetchVersion(d, opts, timeout, vlog)
		}(i, dev)
	}

	wg.Wait()
	return results
}

// VersionResult holds the firmware version (or error) for one device.
type VersionResult struct {
	Device  discovery.Device
	Version string // e.g. "v2024.11.0", empty if unreachable
	Err     string
}

// runWithRetry executes esphome run with retry logic on failure.
func runWithRetry(d discovery.Device, opts RunOptions, writer output.OutputWriter, vlog *log.Logger) Result {
	maxAttempts := 1 + opts.Retries
	start := time.Now()
	var lastErr string
	ctx := optCtx(opts)

	args := []string{"run", d.File, "--no-logs", "--device", d.Host}

	// Dry-run is checked before the attempt loop so it never spawns a process.
	if opts.DryRun {
		fmt.Printf("[dry-run] esphome %v  (dir: %s)\n", args, opts.WorkDir)
		return Result{Device: d, Success: true, Attempts: 1, Duration: time.Since(start)}
	}

	// One buffer per device, shared across all attempts. This gives parseErrors
	// the full picture on failure and lets the ring buffer accumulate for the TUI.
	bufs := newDeviceBuffers(opts.LogFile)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check for cancellation before starting each attempt.
		select {
		case <-ctx.Done():
			vlog.Printf("[%s] context cancelled before attempt %d", d.Name, attempt)
			return Result{Device: d, Success: false, Attempts: attempt - 1, Duration: time.Since(start), Err: "cancelled"}
		default:
		}

		if attempt > 1 {
			vlog.Printf("[%s] retry in %s (attempt %d/%d)", d.Name, opts.RetryDelay, attempt, maxAttempts)
			// DeviceRetrying replaces the old fmt.Printf; the TUI shows "↺ retry in Xs"
			// (guideline #7). PlainWriter emits the same text as before.
			writer.DeviceRetrying(d.Name, attempt, maxAttempts, opts.RetryDelay)
			// Sleep for RetryDelay, but wake immediately if context is cancelled.
			select {
			case <-time.After(opts.RetryDelay):
			case <-ctx.Done():
				vlog.Printf("[%s] context cancelled during retry sleep", d.Name)
				return Result{Device: d, Success: false, Attempts: attempt - 1, Duration: time.Since(start), Err: "cancelled"}
			}
		}

		cmd := exec.Command("esphome", args...)
		cmd.Dir = opts.WorkDir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		vlog.Printf("[%s] attempt %d/%d — running: esphome %s", d.Name, attempt, maxAttempts, strings.Join(args, " "))

		// Wrap the parent context with a per-attempt deadline when requested.
		attemptCtx := ctx
		var attemptCancel context.CancelFunc
		if opts.PerDeviceTimeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(ctx, opts.PerDeviceTimeout)
		} else {
			attemptCancel = func() {}
		}

		err := runStreaming(cmd, d.Name, writer, bufs, vlog, attemptCtx)
		attemptCancel() // release deadline resources whether or not it fired

		if err == nil {
			vlog.Printf("[%s] attempt %d succeeded in %s", d.Name, attempt, time.Since(start).Round(time.Second))
			bufs.full = nil // discard full buffer immediately — GC eligible (guideline #8)
			return Result{Device: d, Success: true, Attempts: attempt, Duration: time.Since(start)}
		}

		// Record human-readable failure reason (include "timed out" when applicable).
		if attemptCtx.Err() == context.DeadlineExceeded {
			lastErr = fmt.Sprintf("timed out after %s", opts.PerDeviceTimeout)
			vlog.Printf("[%s] attempt %d timed out after %s", d.Name, attempt, opts.PerDeviceTimeout)
		} else {
			vlog.Printf("[%s] attempt %d failed: %v", d.Name, attempt, err)
			lastErr = err.Error()
		}

		// Detect compile/config errors — retrying won't help without a code change.
		if isCompilationError(bufs.full) {
			vlog.Printf("[%s] compile/config error detected — skipping remaining retries", d.Name)
			errLines := parseErrors(bufs.full)
			bufs.full = nil
			return Result{
				Device:       d,
				Success:      false,
				Attempts:     attempt,
				Duration:     time.Since(start),
				Err:          "compile/config error (retries skipped)",
				ErrLines:     errLines,
				CompileError: true,
			}
		}
	}

	// All attempts exhausted. Parse errors from the full buffer before discarding.
	errLines := parseErrors(bufs.full)
	bufs.full = nil // discard regardless of parse result
	return Result{
		Device:   d,
		Success:  false,
		Attempts: maxAttempts,
		Duration: time.Since(start),
		Err:      lastErr,
		ErrLines: errLines,
	}
}

// runStreaming runs cmd and streams its combined stdout+stderr through writer
// and into bufs. For each line:
//   - bufs.display.Push(line)         — ring buffer for TUI output-tail panel
//   - bufs.full = append(...)         — full buffer for post-failure parseErrors
//   - bufs.logFile write (if set)     — streaming to disk (--log-file, Phase 1F)
//   - writer.WriteLine(name, line)    — route to TUI or plain output
//
// ctx cancellation kills the process group (guideline #6); the function returns
// after the process has been killed and cmd.Wait() has drained.
func runStreaming(cmd *exec.Cmd, name string, writer output.OutputWriter, bufs *deviceBuffers, vlog *log.Logger, ctx context.Context) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // redirect stderr into the same pipe

	if err := cmd.Start(); err != nil {
		return err
	}
	vlog.Printf("[%s] process started (pid: %d)", name, cmd.Process.Pid)

	// Watch for context cancellation and kill the process group.
	// processDone is closed after cmd.Wait() so the watcher goroutine always exits.
	processDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			vlog.Printf("[%s] context cancelled — killing process group", name)
			killGroup(cmd.Process, vlog, name)
		case <-processDone:
			// process exited normally; nothing to kill
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if bufs != nil {
			bufs.display.Push(line)
			bufs.full = append(bufs.full, line)
			if bufs.logFile != nil {
				fmt.Fprintf(bufs.logFile, "[%s] %s\n", name, line)
			}
		}
		writer.WriteLine(name, line)
	}
	io.Copy(io.Discard, stdout)

	err = cmd.Wait()
	close(processDone) // signal watcher goroutine that the process is done
	if err != nil {
		vlog.Printf("[%s] process exited with error: %v", name, err)
	} else {
		vlog.Printf("[%s] process exited successfully", name)
	}
	return err
}

// fetchVersion runs "esphome logs" and extracts the ESPHome version line.
// It always kills the entire process group when done (either on timeout, on
// version found, or on EOF) — this is critical because esphome spawns child
// processes that would otherwise keep the pipe open and cause an indefinite hang.
func fetchVersion(d discovery.Device, opts RunOptions, timeout time.Duration, vlog *log.Logger) VersionResult {
	if opts.DryRun {
		return VersionResult{Device: d, Version: "v0.0.0-dry-run"}
	}

	devStart := time.Now()
	args := []string{"logs", d.File, "--device", d.Host}
	vlog.Printf("[%s] running: esphome %s", d.Name, strings.Join(args, " "))

	cmd := exec.Command("esphome", args...)
	cmd.Dir = opts.WorkDir
	// Put esphome in its own process group so we can kill it AND all its
	// children at once. Without this, killing only the parent leaves child
	// processes alive, which keep the pipe's write end open and hang the scanner.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return VersionResult{Device: d, Err: fmt.Sprintf("pipe setup: %v", err)}
	}
	cmd.Stderr = cmd.Stdout // merge stderr into the same pipe

	if err := cmd.Start(); err != nil {
		return VersionResult{Device: d, Err: fmt.Sprintf("start: %v", err)}
	}
	vlog.Printf("[%s] process started (pid: %d)", d.Name, cmd.Process.Pid)

	// Scanner goroutine — sends the found version (or "") into the channel.
	// It runs independently; the pipe will be closed by killGroup, causing
	// the goroutine to exit cleanly even if we stop waiting for it.
	type scanResult struct{ version string }
	scanCh := make(chan scanResult, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			vlog.Printf("[%s] output: %s", d.Name, line)
			if v := extractVersion(line); v != "" {
				scanCh <- scanResult{v}
				return
			}
		}
		vlog.Printf("[%s] scanner reached EOF", d.Name)
		scanCh <- scanResult{}
	}()

	// Race the scanner against the timeout.
	var version string
	select {
	case r := <-scanCh:
		version = r.version
		if version != "" {
			vlog.Printf("[%s] version found: %s (after %s)", d.Name, version, time.Since(devStart).Round(time.Millisecond))
		} else {
			vlog.Printf("[%s] EOF without version line", d.Name)
		}
	case <-time.After(timeout):
		vlog.Printf("[%s] timeout (%s) reached", d.Name, timeout)
	}

	// Always kill the process group — whether we found the version, timed out,
	// or hit EOF. This unblocks the scanner goroutine if it is still running.
	killGroup(cmd.Process, vlog, d.Name)
	io.Copy(io.Discard, stdout) // drain so cmd.Wait() can proceed
	cmd.Wait()
	vlog.Printf("[%s] done in %s", d.Name, time.Since(devStart).Round(time.Millisecond))

	if version == "" {
		return VersionResult{Device: d, Err: "unreachable or version line not found within timeout"}
	}
	return VersionResult{Device: d, Version: version}
}

// isCompilationError returns true when the buffered output contains markers
// that indicate the esphome process failed during config validation or
// compilation — before any OTA upload was attempted.  Retrying such failures
// will never succeed without a code or config change, so runWithRetry uses
// this to skip remaining attempts and save time.
//
// The check is intentionally conservative: only explicit ESPHome/Python error
// markers are matched.  Transient failures (network drop, device unreachable)
// do not produce these patterns and will still be retried normally.
func isCompilationError(lines []string) bool {
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "failed config") ||
			strings.Contains(lower, "invalid yaml") ||
			strings.Contains(lower, "error in configuration") ||
			strings.Contains(lower, "recursionerror") ||
			strings.Contains(lower, "syntaxerror") {
			return true
		}
	}
	return false
	// NOTE: we intentionally do NOT check for "Traceback" here. A Python
	// traceback from esphome/git.py (e.g. shutil._rmtree_safe_fd) is a
	// transient concurrency error in ESPHome's git cache, NOT a config error,
	// and should still be retried.
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

// ValidateResult holds the outcome of validating a single device's YAML config.
type ValidateResult struct {
	Device   discovery.Device
	Valid    bool
	Err      string
	ErrLines []string // config error snippets extracted from esphome output
}

// ValidateDevices runs "esphome config <file>" for each device in parallel,
// checking that the YAML is valid without compiling or flashing.
// timeout controls how long to wait per device; zero means no timeout.
func ValidateDevices(devices []discovery.Device, opts RunOptions, timeout time.Duration) []ValidateResult {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4 // same default as Upgrade; higher values race on ESPHome's git cache
	}
	vlog := newLogger(opts.Verbose)
	vlog.Printf("starting config validation for %d devices", len(devices))

	results := make([]ValidateResult, len(devices))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			vlog.Printf("[%s] goroutine started, waiting for semaphore", d.Name)
			sem <- struct{}{}
			defer func() { <-sem }()
			vlog.Printf("[%s] semaphore acquired", d.Name)
			results[idx] = validateDevice(d, opts, timeout, vlog)
		}(i, dev)
	}

	wg.Wait()
	return results
}

// validateDevice runs "esphome config <file>" for a single device and returns
// whether the configuration is valid.
//
// It retries once on failure (with a 2 s pause) because ESPHome's
// external_components git cache can cause transient errors when multiple
// processes run concurrently on a cold cache.  A genuine config error will
// fail on both attempts; a transient cache race will pass on the second.
func validateDevice(d discovery.Device, opts RunOptions, timeout time.Duration, vlog *log.Logger) ValidateResult {
	if opts.DryRun {
		fmt.Printf("[dry-run] esphome config %s  (dir: %s)\n", d.File, opts.WorkDir)
		return ValidateResult{Device: d, Valid: true}
	}

	ctx := optCtx(opts)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := []string{"config", d.File}
	const maxAttempts = 2
	const retryDelay = 2 * time.Second

	var lastOut []byte
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		vlog.Printf("[%s] validate attempt %d/%d — running: esphome %s", d.Name, attempt, maxAttempts, strings.Join(args, " "))

		cmd := exec.CommandContext(ctx, "esphome", args...)
		cmd.Dir = opts.WorkDir

		out, err := cmd.CombinedOutput()
		if err == nil {
			vlog.Printf("[%s] validation passed on attempt %d", d.Name, attempt)
			return ValidateResult{Device: d, Valid: true}
		}

		lastOut = out
		lastErr = err
		vlog.Printf("[%s] validate attempt %d failed: %v", d.Name, attempt, err)

		if attempt < maxAttempts {
			// Brief pause lets ESPHome's git cache settle before retrying.
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				break
			}
		}
	}

	// Both attempts failed — build the most useful error display we can.
	lines := strings.Split(strings.TrimSpace(string(lastOut)), "\n")
	errLines := parseErrors(lines)
	// If parseErrors only produced the generic fallback, show the last few
	// non-empty raw lines instead so the user sees the actual esphome output.
	if len(errLines) == 1 && strings.HasPrefix(errLines[0], "Process exited with error") {
		errLines = lastNonEmptyLines(lines, 6)
		if len(errLines) == 0 {
			errLines = []string{fmt.Sprintf("esphome config exited with: %v (no output captured)", lastErr)}
		}
	}

	return ValidateResult{
		Device:   d,
		Valid:    false,
		Err:      lastErr.Error(),
		ErrLines: errLines,
	}
}

// lastNonEmptyLines returns the last n non-empty lines from lines.
func lastNonEmptyLines(lines []string, n int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			out = append([]string{lines[i]}, out...)
		}
	}
	return out
}

// extractVersion parses "ESPHome version X.Y.Z" from a log line.
func extractVersion(line string) string {
	const marker = "ESPHome version "
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(marker):]
	end := strings.IndexAny(rest, " \r\n")
	if end < 0 {
		end = len(rest)
	}
	v := rest[:end]
	if v == "" {
		return ""
	}
	return "v" + v
}
