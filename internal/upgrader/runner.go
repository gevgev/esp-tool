package upgrader

import (
	"bufio"
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
)

// Result holds the outcome of upgrading a single device.
type Result struct {
	Device   discovery.Device
	Success  bool
	Attempts int
	Duration time.Duration
	Err      string // last error message if failed
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
func Upgrade(devices []discovery.Device, opts RunOptions) []Result {
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

			results[idx] = runWithRetry(d, opts, vlog)
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
func runWithRetry(d discovery.Device, opts RunOptions, vlog *log.Logger) Result {
	maxAttempts := 1 + opts.Retries
	start := time.Now()
	var lastErr string

	args := []string{"run", d.File, "--no-logs", "--device", d.Host}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if opts.DryRun {
			fmt.Printf("[dry-run] esphome %v  (dir: %s)\n", args, opts.WorkDir)
			return Result{Device: d, Success: true, Attempts: attempt, Duration: time.Since(start)}
		}

		if attempt > 1 {
			vlog.Printf("[%s] retry in %s (attempt %d/%d)", d.Name, opts.RetryDelay, attempt, maxAttempts)
			fmt.Printf("[%s] retrying (attempt %d/%d) after %s...\n",
				d.Name, attempt, maxAttempts, opts.RetryDelay)
			time.Sleep(opts.RetryDelay)
		}

		cmd := exec.Command("esphome", args...)
		cmd.Dir = opts.WorkDir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		vlog.Printf("[%s] attempt %d/%d — running: esphome %s", d.Name, attempt, maxAttempts, strings.Join(args, " "))

		err := runStreaming(cmd, d.Name, opts.LogPrefix, vlog)
		if err == nil {
			vlog.Printf("[%s] attempt %d succeeded in %s", d.Name, attempt, time.Since(start).Round(time.Second))
			return Result{Device: d, Success: true, Attempts: attempt, Duration: time.Since(start)}
		}
		vlog.Printf("[%s] attempt %d failed: %v", d.Name, attempt, err)
		lastErr = err.Error()
	}

	return Result{Device: d, Success: false, Attempts: maxAttempts, Duration: time.Since(start), Err: lastErr}
}

// runStreaming runs cmd and streams its combined stdout+stderr to stdout,
// optionally prefixing each line with "[name] ".
func runStreaming(cmd *exec.Cmd, name string, prefix bool, vlog *log.Logger) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // redirect stderr into the same pipe

	if err := cmd.Start(); err != nil {
		return err
	}
	vlog.Printf("[%s] process started (pid: %d)", name, cmd.Process.Pid)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if prefix {
			fmt.Printf("[%s] %s\n", name, line)
		} else {
			fmt.Println(line)
		}
	}
	io.Copy(io.Discard, stdout)

	err = cmd.Wait()
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
