package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/ggevorgyan/esp-tool/internal/diagnostics"
	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/output"
	"github.com/ggevorgyan/esp-tool/internal/report"
	"github.com/ggevorgyan/esp-tool/internal/tui"
	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "esp-tool",
		Short: "ESPHome device manager — upgrade firmware and check versions",
		Long: `esp-tool automates ESPHome firmware upgrades and version checks.

It auto-discovers devices by scanning *.yaml files in the target directory,
parses the esphome.name field from each, and derives the OTA hostname as
<name>.local — so adding a new device YAML is all that's needed.`,
	}

	root.AddCommand(upgradeCmd())
	root.AddCommand(versionsCmd())
	root.AddCommand(diagnosticsCmd())
	root.AddCommand(validateCmd())
	return root
}

// ─── upgrade ──────────────────────────────────────────────────────────────────

func upgradeCmd() *cobra.Command {
	var (
		dir              string
		concurrency      int
		retries          int
		retryDelay       time.Duration
		perDeviceTimeout time.Duration
		dryRun           bool
		filter           string
		retryFailed      bool   // --retry-failed: load failed devices from last run
		logPrefix        bool
		verbose          bool
		plain            bool   // set by --plain or --no-tui; both map to same var (guideline #3)
		logFile          string // path to --log-file destination
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Rebuild and OTA-flash all ESPHome devices",
		Long: `Runs "esphome run <file> --no-logs --device <name>.local" for every
device YAML found in --dir, in parallel (bounded by --jobs).

On failure each device is retried up to --retries additional times with a
--retry-delay pause between attempts. A colored summary table is printed when
all devices finish.

After each run the outcome is saved to .esp-tool-last-run.json in --dir.
Use --retry-failed to re-run only the devices that failed in the previous run.

The TUI activates automatically when stdout is a TTY ≥ 80×24. Use --plain
(or --no-tui) to force plain-text output, or --log-file to stream all device
output to a file in addition to the display.`,
		Example: `  # Upgrade all devices from the current directory
  esp-tool upgrade

  # Upgrade from a specific directory with more parallelism and retries
  esp-tool upgrade --dir ~/git/esp32/esphome/esphome --jobs 6 --retries 3

  # Dry-run: print commands without executing
  esp-tool upgrade --dry-run

  # Force plain-text output (no TUI)
  esp-tool upgrade --plain

  # Stream all device output to a log file
  esp-tool upgrade --log-file /tmp/upgrade.log

  # Upgrade only one device (comma-separated names for multiple)
  esp-tool upgrade --filter lux-living-christmas

  # Re-run only the devices that failed in the previous run
  esp-tool upgrade --retry-failed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --retry-failed and --filter are mutually exclusive.
			if retryFailed && filter != "" {
				return fmt.Errorf("--retry-failed and --filter cannot be used together; --retry-failed already filters to failed devices")
			}

			// If --retry-failed, derive filter from the previous run's failed list.
			if retryFailed {
				lr, err := upgrader.LoadLastRun(dir)
				if err != nil {
					return err
				}
				if len(lr.Failed) == 0 {
					fmt.Printf("No failed devices in last run (%s) — nothing to retry.\n",
						lr.Time.Local().Format("2006-01-02 15:04:05"))
					return nil
				}
				filter = strings.Join(lr.Failed, ",")
				fmt.Printf("Retrying %d failed device(s) from last run (%s): %s\n\n",
					len(lr.Failed),
					lr.Time.Local().Format("2006-01-02 15:04:05"),
					strings.Join(lr.Failed, ", "))
			}

			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			fmt.Printf("Discovered %d devices in %s\n", len(devices), dir)
			for _, d := range devices {
				fmt.Printf("  %s  →  esphome run %s --device %s\n", d.Name, d.File, d.Host)
			}
			fmt.Println()

			// Open log file if --log-file was given.
			// MutexWriter serialises concurrent writes from device goroutines
			// (guideline #4: mutex-protected log writer from day one).
			var logWriter io.Writer
			if logFile != "" {
				f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return fmt.Errorf("--log-file: %w", err)
				}
				defer f.Close()
				logWriter = output.NewMutexWriter(f)
			}

			opts := upgrader.RunOptions{
				Concurrency:      concurrency,
				Retries:          retries,
				RetryDelay:       retryDelay,
				PerDeviceTimeout: perDeviceTimeout,
				DryRun:           dryRun,
				LogPrefix:        logPrefix,
				WorkDir:          dir,
				Verbose:          verbose,
				LogFile:          logWriter,
			}

			// Determine whether to use the TUI (guideline #2: print reason under --verbose).
			useTUI, tuiReason := output.ShouldUseTUI(plain)
			if verbose && !useTUI && tuiReason != "" {
				fmt.Fprintf(os.Stderr, "TUI unavailable, falling back to plain mode: %s\n", tuiReason)
			}

			if !useTUI {
				// ── Plain mode ────────────────────────────────────────────────
				writer := output.NewPlainWriter(os.Stdout, logPrefix)
				start := time.Now()
				results := upgrader.Upgrade(devices, opts, writer)
				elapsed := time.Since(start)

				// Persist last-run state so --retry-failed works next time.
				if saveErr := upgrader.SaveLastRun(dir, results); saveErr != nil && verbose {
					fmt.Fprintf(os.Stderr, "warning: could not save last-run state: %v\n", saveErr)
				}

				report.PrintUpgradeSummary(results, elapsed)
				for _, r := range results {
					if !r.Success {
						os.Exit(1)
					}
				}
				return nil
			}

			// ── TUI mode ──────────────────────────────────────────────────────
			deviceNames := make([]string, len(devices))
			for i, d := range devices {
				deviceNames[i] = d.Name
			}
			m := tui.NewModel(deviceNames, concurrency, retries, dir)
			m.DryRun = dryRun
			m.LogFilePath = logFile

			// tui.Start encapsulates the bubbletea program so main.go does not
			// need to import bubbletea directly.
			w, runTUI := tui.Start(m)

			// Context for cancellation: cancelled when the TUI exits so that
			// any in-flight esphome processes are killed (guideline #6).
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts.Ctx = ctx

			var (
				results []upgrader.Result
				wg      sync.WaitGroup
				start   = time.Now()
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				results = upgrader.Upgrade(devices, opts, w)
				w.SendAllDone()
			}()

			// Block until the user presses q or all devices finish (auto-quit).
			if err := runTUI(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			}

			// Guard against late prog.Send() calls from goroutines that are still
			// finishing up (guideline #6); then cancel context to kill esphome
			// processes and wait for Upgrade() to drain.
			w.MarkDone()
			cancel()
			wg.Wait()
			elapsed := time.Since(start)

			// Persist last-run state so --retry-failed works next time.
			if saveErr := upgrader.SaveLastRun(dir, results); saveErr != nil && verbose {
				fmt.Fprintf(os.Stderr, "warning: could not save last-run state: %v\n", saveErr)
			}

			report.PrintUpgradeSummary(results, elapsed)
			for _, r := range results {
				if !r.Success {
					os.Exit(1)
				}
			}
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().IntVarP(&concurrency, "jobs", "j", 4, "Maximum simultaneous esphome processes")
	cmd.Flags().IntVarP(&retries, "retries", "r", 2, "Number of retry attempts after the first failure")
	cmd.Flags().DurationVar(&retryDelay, "retry-delay", 5*time.Second, "Wait time between retry attempts")
	cmd.Flags().DurationVar(&perDeviceTimeout, "timeout", 0, "Per-attempt timeout; 0 means no limit (e.g. 10m)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print commands without executing them")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit upgrade to")
	cmd.Flags().BoolVar(&retryFailed, "retry-failed", false, "Re-run only devices that failed in the previous upgrade run")
	cmd.Flags().BoolVar(&logPrefix, "prefix", true, "Prefix live output lines with [device-name]")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, retries, timing)")
	// --plain and --no-tui are true cobra aliases: both point to the same bool
	// variable so either flag suppresses the TUI (guideline #3).
	cmd.Flags().BoolVar(&plain, "plain", false, "Disable TUI and use plain-text output")
	cmd.Flags().BoolVar(&plain, "no-tui", false, "Disable TUI and use plain-text output")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Append all device output to this file (streamed line-by-line)")

	return cmd
}

// ─── versions ─────────────────────────────────────────────────────────────────

func versionsCmd() *cobra.Command {
	var (
		dir     string
		timeout time.Duration
		filter  string
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Check the running firmware version on all ESPHome devices",
		Long: `Connects to each device's live log stream in parallel, grabs the first
"ESPHome version" line, and exits. Prints a colored summary table.

Replaces check-esp-versions.sh.`,
		Example: `  # Check all devices in the current directory
  esp-tool versions

  # Check from a specific directory with a longer timeout
  esp-tool versions --dir ~/git/esp32/esphome/esphome --timeout 20s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			fmt.Printf("Checking firmware versions for %d devices...\n", len(devices))

			opts := upgrader.RunOptions{
				WorkDir: dir,
				Verbose: verbose,
			}

			start := time.Now()
			results := upgrader.CheckVersions(devices, opts, timeout)
			elapsed := time.Since(start)

			report.PrintVersionSummary(results, elapsed)
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().DurationVar(&timeout, "timeout", 12*time.Second, "Per-device timeout for version check")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit check to")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, timeouts, timing)")

	return cmd
}

// ─── diagnostics ──────────────────────────────────────────────────────────────

func diagnosticsCmd() *cobra.Command {
	var (
		dir        string
		timeout    time.Duration
		filter     string
		verbose    bool
		reboot     bool
		rebootWait time.Duration
	)

	cmd := &cobra.Command{
		Use:   "diagnostics",
		Short: "Check device health by scanning boot logs for warnings and crashes",
		Long: `Connects to each device's live log stream in parallel, collects the
initial boot dump (version, chip info, warnings), and prints a per-device
health table.

Detects:
  ✗ Crash on previous boot (hardware WDT, exception, etc.)
  ⚠ Bootloader too old for OTA rollback (needs one-time USB flash)
  ⚠ Bootloader supports SRAM1 (+40 KB IRAM, opt-in flag available)
  ⚠ Chip rev ≥3.0 (binary size can be reduced with minimum_chip_revision)
  ⚠ GPIO strapping pin in use
  ⚠ Multiple OTA platform configs merged`,
		Example: `  # Check all devices in the current directory
  esp-tool diagnostics

  # Check from a specific directory
  esp-tool diagnostics --dir ~/git/esp32/esphome/esphome

  # Check a subset of devices with verbose output
  esp-tool diagnostics --filter espvibration1,lux-living-christmas --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			fmt.Printf("Running diagnostics on %d devices...\n", len(devices))

			opts := diagnostics.CheckOptions{
				Timeout:    timeout,
				WorkDir:    dir,
				Verbose:    verbose,
				Reboot:     reboot,
				RebootWait: rebootWait,
			}

			start := time.Now()
			results := diagnostics.Check(devices, opts)
			elapsed := time.Since(start)

			report.PrintDiagnosticsSummary(results, elapsed)
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().DurationVar(&timeout, "timeout", 15*time.Second, "Per-device timeout for log collection")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit check to")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, timeouts, timing)")
	cmd.Flags().BoolVarP(&reboot, "reboot", "r", false, "Soft-reboot each device before capturing logs (ensures fresh boot messages)")
	cmd.Flags().DurationVar(&rebootWait, "reboot-wait", 12*time.Second, "Time to wait after rebooting before collecting logs")

	return cmd
}

// ─── validate ─────────────────────────────────────────────────────────────────

func validateCmd() *cobra.Command {
	var (
		dir         string
		concurrency int
		timeout     time.Duration
		filter      string
		dryRun      bool
		verbose     bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate ESPHome YAML configs without compiling or flashing",
		Long: `Runs "esphome config <file>" for every device YAML found in --dir,
in parallel. Reports which configs are valid and which have errors.

Useful as a pre-flight check before upgrading — catches YAML syntax errors,
unknown component keys, and invalid option values before any device is touched.`,
		Example: `  # Validate all devices in the current directory
  esp-tool validate

  # Validate from a specific directory
  esp-tool validate --dir ~/git/esp32/esphome/esphome

  # Validate a single device
  esp-tool validate --filter lux-living-christmas

  # Dry-run: print commands without executing
  esp-tool validate --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			fmt.Printf("Validating configs for %d devices in %s\n", len(devices), dir)

			opts := upgrader.RunOptions{
				Concurrency: concurrency,
				DryRun:      dryRun,
				WorkDir:     dir,
				Verbose:     verbose,
			}

			start := time.Now()
			results := upgrader.ValidateDevices(devices, opts, timeout)
			elapsed := time.Since(start)

			report.PrintValidateSummary(results, elapsed)

			for _, r := range results {
				if !r.Valid {
					os.Exit(1)
				}
			}
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().IntVarP(&concurrency, "jobs", "j", 4, "Maximum simultaneous esphome processes")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Per-device timeout for config validation")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit validation to")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print commands without executing them")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr")

	return cmd
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func loadDevices(dir, filter string) ([]discovery.Device, error) {
	devices, err := discovery.Scan(dir)
	if err != nil {
		return nil, fmt.Errorf("device discovery: %w", err)
	}

	if filter == "" {
		return devices, nil
	}

	// Build a set of names to include
	names := make(map[string]bool)
	for _, name := range strings.Split(filter, ",") {
		names[strings.TrimSpace(name)] = true
	}

	var filtered []discovery.Device
	for _, d := range devices {
		if names[d.Name] {
			filtered = append(filtered, d)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no devices matched filter %q", filter)
	}
	return filtered, nil
}
