package upgrader

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
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
	// EsphomeArgs are extra arguments appended after the fixed ones.
	EsphomeArgs []string
	// LogPrefix sets whether to prefix live output lines with [device-name].
	LogPrefix bool
	// WorkDir is the directory to run esphome from (where the YAMLs live).
	WorkDir string
}

// Upgrade runs "esphome run <file> --no-logs --device <host>" for each device
// in parallel, respecting the concurrency semaphore, with retry on failure.
// Results are delivered in the order devices finish.
func Upgrade(devices []discovery.Device, opts RunOptions) []Result {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = 5 * time.Second
	}

	sem := make(chan struct{}, opts.Concurrency)
	results := make([]Result, len(devices))
	var wg sync.WaitGroup

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = runWithRetry(d, opts, func(d discovery.Device) []string {
				return []string{"run", d.File, "--no-logs", "--device", d.Host}
			})
		}(i, dev)
	}

	wg.Wait()
	return results
}

// CheckVersions runs "esphome logs <file> --device <host>" for each device in
// parallel, grabs the first "ESPHome version" line and cancels the rest.
// timeout controls how long to wait per device before giving up.
func CheckVersions(devices []discovery.Device, opts RunOptions, timeout time.Duration) []VersionResult {
	if opts.Concurrency <= 0 {
		opts.Concurrency = len(devices) // version checks are just log reads, run all in parallel
	}

	results := make([]VersionResult, len(devices))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = fetchVersion(d, opts, timeout)
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

// runWithRetry executes esphome with the given args builder, retrying on failure.
func runWithRetry(d discovery.Device, opts RunOptions, argsFor func(discovery.Device) []string) Result {
	maxAttempts := 1 + opts.Retries
	start := time.Now()
	var lastErr string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		args := argsFor(d)

		if opts.DryRun {
			fmt.Printf("[dry-run] esphome %v  (dir: %s)\n", args, opts.WorkDir)
			return Result{Device: d, Success: true, Attempts: attempt, Duration: time.Since(start)}
		}

		cmd := exec.Command("esphome", args...)
		cmd.Dir = opts.WorkDir

		if attempt > 1 {
			fmt.Printf("[%s] retrying (attempt %d/%d) after %s...\n",
				d.Name, attempt, maxAttempts, opts.RetryDelay)
			time.Sleep(opts.RetryDelay)
		}

		err := runStreaming(cmd, d.Name, opts.LogPrefix)
		if err == nil {
			return Result{Device: d, Success: true, Attempts: attempt, Duration: time.Since(start)}
		}
		lastErr = err.Error()
	}

	return Result{Device: d, Success: false, Attempts: maxAttempts, Duration: time.Since(start), Err: lastErr}
}

// runStreaming runs cmd and streams its combined stdout/stderr to stdout,
// optionally prefixing each line with "[name] ".
func runStreaming(cmd *exec.Cmd, name string, prefix bool) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // merge stderr into the same pipe

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if prefix {
			fmt.Printf("[%s] %s\n", name, line)
		} else {
			fmt.Println(line)
		}
	}
	// Drain any remaining output after scanner stops
	io.Copy(io.Discard, stdout)

	return cmd.Wait()
}

// fetchVersion runs "esphome logs" and extracts the ESPHome version line.
func fetchVersion(d discovery.Device, opts RunOptions, timeout time.Duration) VersionResult {
	if opts.DryRun {
		return VersionResult{Device: d, Version: "v0.0.0-dry-run"}
	}

	args := []string{"logs", d.File, "--device", d.Host}
	cmd := exec.Command("esphome", args...)
	cmd.Dir = opts.WorkDir

	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err != nil {
		return VersionResult{Device: d, Err: err.Error()}
	}

	if err := cmd.Start(); err != nil {
		return VersionResult{Device: d, Err: err.Error()}
	}

	// Kill the process after timeout regardless of result
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(timeout):
			cmd.Process.Kill()
		case <-done:
		}
	}()

	version := ""
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if v := extractVersion(line); v != "" {
			version = v
			break
		}
	}
	close(done)
	io.Copy(io.Discard, stdout)
	cmd.Wait()

	if version == "" {
		return VersionResult{Device: d, Err: "unreachable or no version line found"}
	}
	return VersionResult{Device: d, Version: version}
}

// extractVersion parses "ESPHome version X.Y.Z" from a log line.
func extractVersion(line string) string {
	const marker = "ESPHome version "
	idx := -1
	for i := 0; i <= len(line)-len(marker); i++ {
		if line[i:i+len(marker)] == marker {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(marker):]
	// Take up to first space or end
	end := len(rest)
	for i, c := range rest {
		if c == ' ' || c == '\r' || c == '\n' {
			end = i
			break
		}
	}
	v := rest[:end]
	if v == "" {
		return ""
	}
	return "v" + v
}
