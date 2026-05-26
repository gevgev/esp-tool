package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ggevorgyan/esp-tool/internal/diagnostics"
	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/output"
	"github.com/ggevorgyan/esp-tool/internal/report"
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
	return root
}

// ─── upgrade ──────────────────────────────────────────────────────────────────

func upgradeCmd() *cobra.Command {
	var (
		dir         string
		concurrency int
		retries     int
		retryDelay  time.Duration
		dryRun      bool
		filter      string
		logPrefix   bool
		verbose     bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Rebuild and OTA-flash all ESPHome devices",
		Long: `Runs "esphome run <file> --no-logs --device <name>.local" for every
device YAML found in --dir, in parallel (bounded by --jobs).

On failure each device is retried up to --retries additional times with a
--retry-delay pause between attempts. A colored summary table is printed when
all devices finish.`,
		Example: `  # Upgrade all devices from the current directory
  esp-tool upgrade

  # Upgrade from a specific directory with more parallelism and retries
  esp-tool upgrade --dir ~/git/esp32/esphome/esphome --jobs 6 --retries 3

  # Dry-run: print commands without executing
  esp-tool upgrade --dry-run

  # Upgrade only one device (comma-separated names for multiple)
  esp-tool upgrade --filter lux-living-christmas`,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			fmt.Printf("Discovered %d devices in %s\n", len(devices), dir)
			for _, d := range devices {
				fmt.Printf("  %s  →  esphome run %s --device %s\n", d.Name, d.File, d.Host)
			}
			fmt.Println()

			opts := upgrader.RunOptions{
				Concurrency: concurrency,
				Retries:     retries,
				RetryDelay:  retryDelay,
				DryRun:      dryRun,
				LogPrefix:   logPrefix,
				WorkDir:     dir,
				Verbose:     verbose,
			}

			// Phase 1B: always use PlainWriter for now.
			// Phase 1F will select between PlainWriter and TUIWriter based on
			// output.ShouldUseTUI() and the --plain / --no-tui flags.
			writer := output.NewPlainWriter(os.Stdout, logPrefix)

			start := time.Now()
			results := upgrader.Upgrade(devices, opts, writer)
			elapsed := time.Since(start)

			report.PrintUpgradeSummary(results, elapsed)

			// Exit with non-zero if any device failed
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print commands without executing them")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit upgrade to")
	cmd.Flags().BoolVar(&logPrefix, "prefix", true, "Prefix live output lines with [device-name]")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, retries, timing)")

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
