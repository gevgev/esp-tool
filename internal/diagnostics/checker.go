package diagnostics

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

// IssueLevel ranks the severity of a detected issue.
type IssueLevel int

const (
	LevelCrash   IssueLevel = iota // previous-boot crash (red ✗)
	LevelWarning                   // actionable firmware/hardware warning (yellow ⚠)
)

// Issue is a single parsed diagnostic from a device's log output.
type Issue struct {
	Level   IssueLevel
	Code    string // stable short key, e.g. "CRASH", "BOOTLOADER_OLD"
	Message string // human-readable description with remediation hint
}

// Result holds the complete diagnostic report for one device.
type Result struct {
	Device  discovery.Device
	Version string        // firmware version compiled into the device; empty if unreachable
	Issues  []Issue       // empty means healthy
	Elapsed time.Duration
	Err     string // non-empty if we could not connect at all
}

// CheckOptions controls the diagnostic run.
type CheckOptions struct {
	// Concurrency caps simultaneous esphome log connections.
	// Defaults to len(devices) — log streaming is not CPU-intensive.
	Concurrency int
	// Timeout is how long to wait for the initial boot dump per device.
	// The dump arrives within a second of handshake; 15 s is generous.
	Timeout time.Duration
	// WorkDir is the directory containing the YAML files (esphome CWD).
	WorkDir string
	// Verbose enables per-device lifecycle logging to stderr.
	Verbose bool
}

// issuePatterns lists every condition we look for, in priority order.
// Patterns are matched against raw output lines (CLI + device log).
var issuePatterns = []struct {
	contains string
	level    IssueLevel
	code     string
	message  string
}{
	// ── device-side log messages (appear after handshake) ─────────────────
	{
		"CRASH DETECTED ON PREVIOUS BOOT",
		LevelCrash,
		"CRASH",
		"Crash on previous boot", // message refined with Reason: line below
	},
	{
		"Bootloader too old for OTA rollback",
		LevelWarning,
		"BOOTLOADER_OLD",
		"Bootloader too old — flash via USB once to enable OTA rollback + 40 KB IRAM",
	},
	{
		"Bootloader supports SRAM1 as IRAM",
		LevelWarning,
		"SRAM1_AVAILABLE",
		`Bootloader supports SRAM1 (+40 KB IRAM) — set sram1_as_iram: true under esp32 > framework > advanced`,
	},
	{
		"Chip rev >= 3.0 detected",
		LevelWarning,
		"CHIP_REV",
		`Chip rev ≥3.0 — set minimum_chip_revision: "3.0" under esp32 > framework > advanced to reduce binary size`,
	},
	// ── CLI-level config warnings (appear before device connection) ───────
	{
		"is a strapping PIN",
		LevelWarning,
		"STRAPPING_PIN",
		"GPIO strapping pin in use — verify pull-up/down won't cause boot issues (see esphome.io/guides/faq)",
	},
	{
		"Found and merged multiple configurations for ota platform",
		LevelWarning,
		"OTA_MULTI_CONFIG",
		"Multiple OTA platform configs merged — consider consolidating the ota: section in your YAML",
	},
}

// Check connects to every device in parallel, collects the initial log dump
// for CheckOptions.Timeout, parses it for known issues, and returns results
// in the same order as devices.
func Check(devices []discovery.Device, opts CheckOptions) []Result {
	if opts.Concurrency <= 0 {
		opts.Concurrency = len(devices)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	vlog := newLogger(opts.Verbose)
	vlog.Printf("starting diagnostics for %d devices (timeout per device: %s)", len(devices), opts.Timeout)

	results := make([]Result, len(devices))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.Concurrency)

	for i, dev := range devices {
		wg.Add(1)
		go func(idx int, d discovery.Device) {
			defer wg.Done()
			vlog.Printf("[%s] waiting for semaphore", d.Name)
			sem <- struct{}{}
			defer func() { <-sem }()
			vlog.Printf("[%s] semaphore acquired", d.Name)
			results[idx] = fetchDiagnostics(d, opts, vlog)
		}(i, dev)
	}

	wg.Wait()
	return results
}

// fetchDiagnostics connects to one device via "esphome logs", collects all
// output lines until the timeout fires, then parses them for known issues.
func fetchDiagnostics(d discovery.Device, opts CheckOptions, vlog *log.Logger) Result {
	devStart := time.Now()
	args := []string{"logs", d.File, "--device", d.Host}
	vlog.Printf("[%s] running: esphome %s", d.Name, strings.Join(args, " "))

	cmd := exec.Command("esphome", args...)
	cmd.Dir = opts.WorkDir
	// Own process group so we can kill esphome AND its children atomically.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Device: d, Err: fmt.Sprintf("pipe: %v", err), Elapsed: time.Since(devStart)}
	}
	cmd.Stderr = cmd.Stdout // merge stderr into the same pipe

	if err := cmd.Start(); err != nil {
		return Result{Device: d, Err: fmt.Sprintf("start: %v", err), Elapsed: time.Since(devStart)}
	}
	vlog.Printf("[%s] pid %d", d.Name, cmd.Process.Pid)

	// Scanner goroutine — collects all lines until EOF (caused by killGroup).
	type scanDone struct{ lines []string }
	scanCh := make(chan scanDone, 1)
	go func() {
		var lines []string
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			lines = append(lines, line)
			vlog.Printf("[%s] %s", d.Name, line)
		}
		vlog.Printf("[%s] scanner EOF (%d lines)", d.Name, len(lines))
		scanCh <- scanDone{lines}
	}()

	// Wait for timeout — we always wait the full window so the boot dump is complete.
	var lines []string
	select {
	case r := <-scanCh:
		lines = r.lines
		vlog.Printf("[%s] scanner finished before timeout", d.Name)
	case <-time.After(opts.Timeout):
		vlog.Printf("[%s] timeout (%s) reached", d.Name, opts.Timeout)
	}

	// Always kill the process group, then drain and wait.
	killGroup(cmd.Process, vlog, d.Name)
	io.Copy(io.Discard, stdout)
	cmd.Wait()

	elapsed := time.Since(devStart)
	vlog.Printf("[%s] done in %s, %d lines collected", d.Name, elapsed.Round(time.Millisecond), len(lines))

	if len(lines) == 0 {
		return Result{Device: d, Elapsed: elapsed, Err: "unreachable or no data within timeout"}
	}

	version, issues := parseLogLines(lines)
	return Result{Device: d, Version: version, Issues: issues, Elapsed: elapsed}
}

// parseLogLines extracts the device firmware version and all known issues
// from the collected output lines.
func parseLogLines(lines []string) (version string, issues []Issue) {
	seen := make(map[string]bool)
	pendingCrash := false // true while we're looking for the Reason: follow-up line

	for _, line := range lines {
		// Device firmware version: "[HH:MM:SS.mmm][I][app:N]: ESPHome version X.Y.Z compiled on ..."
		// Note: "INFO ESPHome X.Y.Z" (CLI banner) does NOT contain "version " so it won't match.
		if version == "" {
			if v := extractDeviceVersion(line); v != "" {
				version = v
			}
		}

		// Crash reason refinement — the line immediately following CRASH DETECTED.
		if pendingCrash {
			if idx := strings.Index(line, "Reason:"); idx >= 0 {
				reason := strings.TrimSpace(line[idx+len("Reason:"):])
				for i := range issues {
					if issues[i].Code == "CRASH" {
						issues[i].Message = "Crash on previous boot — " + reason
					}
				}
			}
			pendingCrash = false
		}

		// Match known patterns (skip duplicates).
		for _, p := range issuePatterns {
			if seen[p.code] || !strings.Contains(line, p.contains) {
				continue
			}
			seen[p.code] = true
			issues = append(issues, Issue{Level: p.level, Code: p.code, Message: p.message})
			if p.code == "CRASH" {
				pendingCrash = true
			}
		}
	}
	return version, issues
}

// extractDeviceVersion parses "ESPHome version X.Y.Z" from a device log line.
// This matches device-side messages like:
//
//	[09:57:03.106][I][app:154]: ESPHome version 2026.4.3 compiled on ...
//
// It does NOT match the CLI banner "INFO ESPHome 2026.4.3" because that line
// contains "ESPHome 2026" not "ESPHome version 2026".
func extractDeviceVersion(line string) string {
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
	v := strings.TrimSpace(rest[:end])
	if v == "" {
		return ""
	}
	return "v" + v
}

// newLogger returns a logger writing to stderr when verbose, discard otherwise.
func newLogger(verbose bool) *log.Logger {
	if verbose {
		return log.New(os.Stderr, "[verbose] ", log.Ltime|log.Lmicroseconds)
	}
	return log.New(io.Discard, "", 0)
}

// killGroup terminates an entire process group (process + all children).
func killGroup(p *os.Process, vlog *log.Logger, name string) {
	if p == nil {
		return
	}
	vlog.Printf("[%s] killing process group %d", name, p.Pid)
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		vlog.Printf("[%s] group kill failed (%v), falling back to direct kill", name, err)
		p.Kill()
	}
}
