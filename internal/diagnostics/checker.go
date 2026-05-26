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
	// Reboot sends a soft-restart to each device via the ESPHome API before
	// capturing logs. This ensures fresh boot messages are always captured,
	// even for long-running devices whose log buffers no longer contain them.
	Reboot bool
	// RebootWait is how long to wait after triggering reboots before connecting.
	// Default 12 s — enough for most ESP32 devices to fully boot.
	RebootWait time.Duration
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

// restartScript is a Python one-liner that presses the first button entity
// (restart or safe_mode) on an ESPHome device via the native encrypted API.
// It is invoked as: python3 -c <script> <host> <base64-api-key>
//
// aioesphomeapi is installed wherever the esphome CLI is installed, so it is
// always available on machines that already run ESPHome upgrades.
const restartScript = `
import asyncio, sys

async def restart(host, key_b64):
    from aioesphomeapi import APIClient
    # aioesphomeapi expects the raw base64 string for noise_psk (it decodes internally)
    cli = APIClient(host, 6053, None, noise_psk=key_b64)
    await cli.connect(login=True)
    entities, _ = await cli.list_entities_services()
    pressed = False
    for e in entities:
        if "button" in type(e).__name__.lower():
            await cli.button_command(e.key)
            pressed = True
            break
    await asyncio.sleep(0.3)
    await cli.disconnect()
    if not pressed:
        sys.exit(1)

asyncio.run(restart(sys.argv[1], sys.argv[2]))
`

// rebootDevice sends a soft-restart to one device by pressing its first button
// entity (restart or safe_mode) via aioesphomeapi. Requires an API key.
func rebootDevice(d discovery.Device, vlog *log.Logger) error {
	if d.APIKey == "" {
		return fmt.Errorf("no inline API key in YAML (uses !secret?)")
	}
	vlog.Printf("[%s] sending reboot via ESPHome API (host: %s)", d.Name, d.Host)
	cmd := exec.Command("python3", "-c", restartScript, d.Host, d.APIKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reboot failed: %v — %s", err, strings.TrimSpace(string(out)))
	}
	vlog.Printf("[%s] reboot command sent", d.Name)
	return nil
}

// Check connects to every device in parallel, collects the initial log dump
// for CheckOptions.Timeout, parses it for known issues, and returns results
// in the same order as devices.
//
// When CheckOptions.Reboot is true, all devices are soft-rebooted first via
// the ESPHome API so that fresh boot messages are always in the log buffer.
//
// onResult is an optional callback invoked from a goroutine as each device
// completes; pass nil for plain-text mode (no live notification).
func Check(devices []discovery.Device, opts CheckOptions, onResult func(Result)) []Result {
	if opts.Concurrency <= 0 {
		opts.Concurrency = len(devices)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.RebootWait <= 0 {
		opts.RebootWait = 12 * time.Second
	}
	vlog := newLogger(opts.Verbose)
	vlog.Printf("starting diagnostics for %d devices (timeout per device: %s)", len(devices), opts.Timeout)

	// Phase 1 (optional): reboot all devices in parallel, then wait.
	if opts.Reboot {
		fmt.Printf("Rebooting %d devices...\n", len(devices))
		var rwg sync.WaitGroup
		for _, dev := range devices {
			rwg.Add(1)
			go func(d discovery.Device) {
				defer rwg.Done()
				if err := rebootDevice(d, vlog); err != nil {
					fmt.Printf("  [%s] reboot skipped: %v\n", d.Name, err)
				} else {
					fmt.Printf("  [%s] reboot sent ✓\n", d.Name)
				}
			}(dev)
		}
		rwg.Wait()
		fmt.Printf("Waiting %s for devices to boot...\n\n", opts.RebootWait)
		time.Sleep(opts.RebootWait)
	}

	// Phase 2: collect log dumps in parallel.
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
			r := fetchDiagnostics(d, opts, vlog)
			results[idx] = r
			if onResult != nil {
				onResult(r)
			}
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

	// Wait for the full timeout to capture the initial boot dump.
	//
	// Key: when timeout fires we must NOT discard lines — the scanner goroutine
	// has been collecting them the whole time. We kill the process group (which
	// closes the pipe's write end and triggers EOF in the scanner), then wait
	// for the goroutine to drain and report before reading its results.
	var lines []string
	select {
	case r := <-scanCh:
		// Process exited on its own before the timeout (rare for esphome logs).
		lines = r.lines
		vlog.Printf("[%s] scanner finished before timeout (%d lines)", d.Name, len(lines))
	case <-time.After(opts.Timeout):
		vlog.Printf("[%s] timeout (%s), killing process group", d.Name, opts.Timeout)
		killGroup(cmd.Process, vlog, d.Name)
		// SIGKILL closes the child's write-end → scanner goroutine hits EOF.
		// Give it up to 3 s to drain buffered data and send results.
		select {
		case r := <-scanCh:
			lines = r.lines
			vlog.Printf("[%s] scanner drained after kill (%d lines)", d.Name, len(lines))
		case <-time.After(3 * time.Second):
			vlog.Printf("[%s] scanner drain timed out; some lines may be missing", d.Name)
		}
	}

	// Kill if not already dead (handles the early-EOF path above).
	killGroup(cmd.Process, vlog, d.Name)
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
