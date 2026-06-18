package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"

	"github.com/ggevorgyan/esp-tool/internal/diagnostics"
	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/output"
	"github.com/ggevorgyan/esp-tool/internal/report"
	"github.com/ggevorgyan/esp-tool/internal/syncer"
	"github.com/ggevorgyan/esp-tool/internal/tui"
	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

// configFileName is the name of the optional project/user config file.
const configFileName = ".esp-tool"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// initConfig loads .esp-tool.yaml from the current working directory, then
// falls back to ~/.esp-tool.yaml.  It is called via cobra.OnInitialize so it
// runs before any subcommand's RunE.
func initConfig(v *viper.Viper) {
	v.SetConfigName(configFileName)
	v.SetConfigType("yaml")

	// 1. Project-level: cwd (highest priority among config files)
	if cwd, err := os.Getwd(); err == nil {
		v.AddConfigPath(cwd)
	}

	// 2. User-level: home directory (fallback)
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(home)
	}

	// Best-effort read; silently ignore "config not found" — the file is optional.
	_ = v.ReadInConfig()
}

// viperString returns the string value for the named flag, preferring:
//  1. an explicit CLI flag (cmd.Flags().Changed)
//  2. a value from the config file (v.IsSet)
//  3. the cobra flag default (GetString fallback)
func viperString(cmd *cobra.Command, v *viper.Viper, flag string) string {
	if cmd.Flags().Changed(flag) {
		val, _ := cmd.Flags().GetString(flag)
		return val
	}
	if v.IsSet(flag) {
		return v.GetString(flag)
	}
	val, _ := cmd.Flags().GetString(flag)
	return val
}

// viperInt is like viperString but for int flags.
func viperInt(cmd *cobra.Command, v *viper.Viper, flag string) int {
	if cmd.Flags().Changed(flag) {
		val, _ := cmd.Flags().GetInt(flag)
		return val
	}
	if v.IsSet(flag) {
		return v.GetInt(flag)
	}
	val, _ := cmd.Flags().GetInt(flag)
	return val
}

// viperBool is like viperString but for bool flags.
func viperBool(cmd *cobra.Command, v *viper.Viper, flag string) bool {
	if cmd.Flags().Changed(flag) {
		val, _ := cmd.Flags().GetBool(flag)
		return val
	}
	if v.IsSet(flag) {
		return v.GetBool(flag)
	}
	val, _ := cmd.Flags().GetBool(flag)
	return val
}

// viperStringOr returns the config file value for key, or def if unset. Used
// for settings (e.g. ssh-host) that are config-only on commands that don't
// expose a matching CLI flag.
func viperStringOr(v *viper.Viper, key, def string) string {
	if v.IsSet(key) {
		return v.GetString(key)
	}
	return def
}

// viperIntOr is like viperStringOr but for int settings.
func viperIntOr(v *viper.Viper, key string, def int) int {
	if v.IsSet(key) {
		return v.GetInt(key)
	}
	return def
}

// viperDuration is like viperString but for duration flags.
func viperDuration(cmd *cobra.Command, v *viper.Viper, flag string) time.Duration {
	if cmd.Flags().Changed(flag) {
		val, _ := cmd.Flags().GetDuration(flag)
		return val
	}
	if v.IsSet(flag) {
		// Viper stores durations as strings when read from YAML; cast handles both.
		return v.GetDuration(flag)
	}
	val, _ := cmd.Flags().GetDuration(flag)
	return val
}

// configFilePath returns the resolved path of the config file viper loaded,
// or "" if none was found. Used only for --verbose output.
func configFilePath(v *viper.Viper) string {
	cf := v.ConfigFileUsed()
	if cf == "" {
		return ""
	}
	if abs, err := filepath.Abs(cf); err == nil {
		return abs
	}
	return cf
}

func rootCmd() *cobra.Command {
	v := viper.New()

	root := &cobra.Command{
		Use:   "esp-tool",
		Short: "ESPHome device manager — upgrade firmware and check versions",
		Long: `esp-tool automates ESPHome firmware upgrades and version checks.

It auto-discovers devices by scanning *.yaml files in the target directory,
parses the esphome.name field from each, and derives the OTA hostname as
<name>.local — so adding a new device YAML is all that's needed.

Configuration can be set in .esp-tool.yaml (project) or ~/.esp-tool.yaml
(user). Command-line flags always override config file values.`,
	}

	// Load config before any subcommand runs.
	cobra.OnInitialize(func() { initConfig(v) })

	root.AddCommand(upgradeCmd(v))
	root.AddCommand(versionsCmd(v))
	root.AddCommand(diagnosticsCmd(v))
	root.AddCommand(validateCmd(v))
	root.AddCommand(syncCmd(v))
	return root
}

// ─── upgrade ──────────────────────────────────────────────────────────────────

func upgradeCmd(v *viper.Viper) *cobra.Command {
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
		autoSync         bool   // --auto-sync: run sync after a successful upgrade
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
			// Apply config file values for any flags that were not explicitly set.
			dir = viperString(cmd, v, "dir")
			concurrency = viperInt(cmd, v, "jobs")
			retries = viperInt(cmd, v, "retries")
			retryDelay = viperDuration(cmd, v, "retry-delay")
			perDeviceTimeout = viperDuration(cmd, v, "timeout")
			filter = viperString(cmd, v, "filter")

			if verbose {
				if cf := configFilePath(v); cf != "" {
					fmt.Fprintf(os.Stderr, "config: loaded from %s\n", cf)
				}
			}

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
				maybeAutoSync(cmd, v, dir, filter, verbose)
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
			maybeAutoSync(cmd, v, dir, filter, verbose)
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
	cmd.Flags().BoolVar(&autoSync, "auto-sync", true, "Sync ESPHome Device Builder state after upgrading (requires ssh-host or db-file in .esp-tool.yaml)")

	return cmd
}

// ─── versions ─────────────────────────────────────────────────────────────────

func versionsCmd(v *viper.Viper) *cobra.Command {
	var (
		dir     string
		timeout time.Duration
		filter  string
		plain   bool
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Check the running firmware version on all ESPHome devices",
		Long: `Connects to each device's live log stream in parallel, grabs the first
"ESPHome version" line, and exits. Prints a colored summary table.

When stdout is a TTY ≥ 80×24, shows a live spinner per device that updates
as each result arrives. Use --plain to force plain-text output.

Replaces check-esp-versions.sh.`,
		Example: `  # Check all devices in the current directory
  esp-tool versions

  # Check from a specific directory with a longer timeout
  esp-tool versions --dir ~/git/esp32/esphome/esphome --timeout 20s

  # Force plain-text output (no TUI)
  esp-tool versions --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir = viperString(cmd, v, "dir")
			timeout = viperDuration(cmd, v, "timeout")
			filter = viperString(cmd, v, "filter")

			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			opts := upgrader.RunOptions{
				WorkDir: dir,
				Verbose: verbose,
			}

			// Determine whether to use the TUI.
			useTUI, tuiReason := output.ShouldUseTUI(plain)
			if verbose && !useTUI && tuiReason != "" {
				fmt.Fprintf(os.Stderr, "TUI unavailable, falling back to plain mode: %s\n", tuiReason)
			}

			if !useTUI {
				// ── Plain mode ────────────────────────────────────────────────
				fmt.Printf("Checking firmware versions for %d devices...\n", len(devices))
				start := time.Now()
				results := upgrader.CheckVersions(devices, opts, timeout, nil)
				elapsed := time.Since(start)
				report.PrintVersionSummary(results, elapsed)
				return nil
			}

			// ── TUI mode ──────────────────────────────────────────────────────
			deviceNames := make([]string, len(devices))
			for i, d := range devices {
				deviceNames[i] = d.Name
			}
			m := tui.NewVersionsModel(deviceNames)
			vp, runTUI := tui.StartVersions(m)

			var (
				results []upgrader.VersionResult
				wg      sync.WaitGroup
				start   = time.Now()
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				results = upgrader.CheckVersions(devices, opts, timeout, func(name, version, errStr string) {
					vp.Send(name, version, errStr)
				})
				vp.SendAllDone()
			}()

			if err := runTUI(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			}
			vp.MarkDone()
			wg.Wait()
			elapsed := time.Since(start)

			report.PrintVersionSummary(results, elapsed)
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().DurationVar(&timeout, "timeout", 12*time.Second, "Per-device timeout for version check")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit check to")
	cmd.Flags().BoolVar(&plain, "plain", false, "Disable TUI and use plain-text output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, timeouts, timing)")

	return cmd
}

// ─── diagnostics ──────────────────────────────────────────────────────────────

func diagnosticsCmd(v *viper.Viper) *cobra.Command {
	var (
		dir        string
		timeout    time.Duration
		filter     string
		plain      bool
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

When stdout is a TTY ≥ 80×24, shows a live TUI with a spinner per device
that resolves to ✓ Healthy / ⚠ N warnings / ✗ crash as each result arrives.
Use --plain to force plain-text output.

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
  esp-tool diagnostics --filter espvibration1,lux-living-christmas --verbose

  # Force plain-text output (no TUI)
  esp-tool diagnostics --plain`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir = viperString(cmd, v, "dir")
			timeout = viperDuration(cmd, v, "timeout")
			filter = viperString(cmd, v, "filter")

			devices, err := loadDevices(dir, filter)
			if err != nil {
				return err
			}

			opts := diagnostics.CheckOptions{
				Timeout:    timeout,
				WorkDir:    dir,
				Verbose:    verbose,
				Reboot:     reboot,
				RebootWait: rebootWait,
			}

			// Determine whether to use the TUI.
			useTUI, tuiReason := output.ShouldUseTUI(plain)
			if verbose && !useTUI && tuiReason != "" {
				fmt.Fprintf(os.Stderr, "TUI unavailable, falling back to plain mode: %s\n", tuiReason)
			}

			if !useTUI {
				// ── Plain mode ────────────────────────────────────────────────
				fmt.Printf("Running diagnostics on %d devices...\n", len(devices))
				start := time.Now()
				results := diagnostics.Check(devices, opts, nil)
				elapsed := time.Since(start)
				report.PrintDiagnosticsSummary(results, elapsed)
				return nil
			}

			// ── TUI mode ──────────────────────────────────────────────────────
			deviceNames := make([]string, len(devices))
			for i, d := range devices {
				deviceNames[i] = d.Name
			}
			m := tui.NewDiagnosticsModel(deviceNames)
			dp, runTUI := tui.StartDiagnostics(m)

			var (
				results []diagnostics.Result
				wg      sync.WaitGroup
				start   = time.Now()
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				results = diagnostics.Check(devices, opts, func(r diagnostics.Result) {
					dp.Send(diagResultToMsg(r))
				})
				dp.SendAllDone()
			}()

			if err := runTUI(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			}
			dp.MarkDone()
			wg.Wait()
			elapsed := time.Since(start)

			report.PrintDiagnosticsSummary(results, elapsed)
			return nil
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().DurationVar(&timeout, "timeout", 15*time.Second, "Per-device timeout for log collection")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit check to")
	cmd.Flags().BoolVar(&plain, "plain", false, "Disable TUI and use plain-text output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print diagnostic logs to stderr (process lifecycle, timeouts, timing)")
	cmd.Flags().BoolVarP(&reboot, "reboot", "r", false, "Soft-reboot each device before capturing logs (ensures fresh boot messages)")
	cmd.Flags().DurationVar(&rebootWait, "reboot-wait", 12*time.Second, "Time to wait after rebooting before collecting logs")

	return cmd
}

// diagResultToMsg converts a diagnostics.Result into a tui.DiagnosticsResultMsg.
// This translation lives in main.go so the tui package never imports diagnostics.
func diagResultToMsg(r diagnostics.Result) tui.DiagnosticsResultMsg {
	msg := tui.DiagnosticsResultMsg{
		Device:  r.Device.Name,
		Version: r.Version,
		Err:     r.Err,
	}
	for _, issue := range r.Issues {
		switch issue.Level {
		case diagnostics.LevelCrash:
			msg.Crashes++
			msg.IssueLines = append(msg.IssueLines, "✗ "+issue.Message)
		case diagnostics.LevelWarning:
			msg.Warnings++
			msg.IssueLines = append(msg.IssueLines, "⚠ "+issue.Message)
		}
	}
	return msg
}

// ─── validate ─────────────────────────────────────────────────────────────────

func validateCmd(v *viper.Viper) *cobra.Command {
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
			dir = viperString(cmd, v, "dir")
			concurrency = viperInt(cmd, v, "jobs")
			timeout = viperDuration(cmd, v, "timeout")
			filter = viperString(cmd, v, "filter")

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

// ─── sync ─────────────────────────────────────────────────────────────────────

// syncOptions configures one sync run, shared by the `sync` command and
// `upgrade --auto-sync`.
type syncOptions struct {
	Dir        string
	Filter     string
	DBFile     string
	SSHHost    string
	SSHPort    int
	SSHUser    string
	SSHKey     string
	RemoteFile string // optional override; auto-discovered under /addon_configs if empty
	DryRun     bool
	Verbose    bool
}

// runSync patches the Device Builder state file's expected_config_hash to
// match deployed_config_hash for every device that succeeded in the last
// "esp-tool upgrade" run in opts.Dir. It reads/writes the state file either
// locally (opts.DBFile) or over SSH (opts.SSHHost), auto-discovering the
// remote file path under /addon_configs when opts.RemoteFile is empty.
func runSync(opts syncOptions) error {
	devices, err := loadDevices(opts.Dir, opts.Filter)
	if err != nil {
		return err
	}

	lr, err := upgrader.LoadLastRun(opts.Dir)
	if err != nil {
		return err
	}
	failed := make(map[string]bool, len(lr.Failed))
	for _, name := range lr.Failed {
		failed[name] = true
	}

	var files []string
	for _, d := range devices {
		if !failed[d.Name] {
			files = append(files, d.File)
		}
	}
	if len(files) == 0 {
		fmt.Println("No successfully flashed devices to sync.")
		return nil
	}

	var (
		data       []byte
		sshClient  *ssh.Client
		remoteFile = opts.RemoteFile
	)
	if opts.DBFile != "" {
		data, err = os.ReadFile(opts.DBFile)
		if err != nil {
			return fmt.Errorf("read --db-file: %w", err)
		}
	} else {
		sshClient, err = syncer.Dial(opts.SSHHost, opts.SSHPort, opts.SSHUser, opts.SSHKey, 10*time.Second)
		if err != nil {
			return err
		}
		defer sshClient.Close()

		if remoteFile == "" {
			remoteFile, err = syncer.DiscoverRemoteFile(sshClient)
			if err != nil {
				return err
			}
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "  discovered remote state file: %s\n", remoteFile)
			}
		}

		data, err = syncer.ReadFile(sshClient, remoteFile)
		if err != nil {
			return err
		}
	}

	newData, res, err := syncer.Patch(data, files)
	if err != nil {
		return err
	}

	for _, f := range res.Patched {
		fmt.Printf("  %s synced\n", f)
	}
	if opts.Verbose {
		for f, reason := range res.Skipped {
			fmt.Fprintf(os.Stderr, "  %s skipped: %s\n", f, reason)
		}
	}

	if opts.DryRun {
		fmt.Printf("[dry-run] would sync %d device(s), skip %d — no changes written\n", len(res.Patched), len(res.Skipped))
		return nil
	}

	if len(res.Patched) == 0 {
		fmt.Println("Nothing to sync — all devices already up to date.")
		return nil
	}

	if opts.DBFile != "" {
		if err := syncer.WriteAtomic(opts.DBFile, newData); err != nil {
			return err
		}
	} else {
		if err := syncer.WriteFileAtomic(sshClient, remoteFile, newData); err != nil {
			return err
		}
	}

	fmt.Printf("Synced %d device(s); %d skipped\n", len(res.Patched), len(res.Skipped))
	return nil
}

// maybeAutoSync runs sync after a successful "esp-tool upgrade" when
// --auto-sync is enabled and a sync target (ssh-host or db-file) is
// configured. It never fails the upgrade — sync errors are reported as
// warnings on stderr.
func maybeAutoSync(cmd *cobra.Command, v *viper.Viper, dir, filter string, verbose bool) {
	if !viperBool(cmd, v, "auto-sync") {
		return
	}
	sshHost := viperStringOr(v, "ssh-host", "")
	dbFile := viperStringOr(v, "db-file", "")
	if sshHost == "" && dbFile == "" {
		if verbose {
			fmt.Fprintln(os.Stderr, "auto-sync: no ssh-host or db-file configured in .esp-tool.yaml — skipping")
		}
		return
	}

	fmt.Println()
	fmt.Println("Auto-sync: updating ESPHome Device Builder state for synced devices...")
	err := runSync(syncOptions{
		Dir:        dir,
		Filter:     filter,
		DBFile:     dbFile,
		SSHHost:    sshHost,
		SSHPort:    viperIntOr(v, "ssh-port", 22),
		SSHUser:    viperStringOr(v, "ssh-user", "root"),
		SSHKey:     viperStringOr(v, "ssh-key", ""),
		RemoteFile: viperStringOr(v, "ssh-remote-file", ""),
		Verbose:    verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: auto-sync failed: %v\n", err)
	}
}

func syncCmd(v *viper.Viper) *cobra.Command {
	var (
		dir        string
		filter     string
		dbFile     string
		sshHost    string
		sshPort    int
		sshUser    string
		sshKey     string
		remoteFile string
		dryRun     bool
		verbose    bool
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Mark esp-tool-flashed devices as in-sync in the ESPHome Device Builder",
		Long: `The ESPHome Device Builder add-on (2026.6.0+) shows an "out of sync" dot
for every device because it tracks compiles it initiated itself, separate
from flashes done externally via esp-tool.

sync reads the Builder's .device-builder-devices.json state file and, for
every device that succeeded in the last "esp-tool upgrade" run, sets
expected_config_hash = deployed_config_hash so the dot clears.

Use --db-file when running directly on the HA host (or against a mounted
copy of the file). Use --ssh-host to patch the file remotely over SSH — the
host must already be trusted in ~/.ssh/known_hosts, and auth comes from
ssh-agent or --ssh-key. The Device Builder's data directory is named after
an opaque per-install slug, so the remote state file path is auto-discovered
under /addon_configs unless --ssh-remote-file overrides it.

Both --db-file and --ssh-host can be set once in .esp-tool.yaml so that bare
"esp-tool sync" just works — see the Configuration file section of the README.`,
		Example: `  # Patch the file directly (run on the HA host)
  esp-tool sync --db-file /addon_configs/a8a2938f_esphome/data/.device-builder-devices.json

  # Patch over SSH from a Mac — remote file path is auto-discovered
  esp-tool sync --ssh-host homeassistant.local --ssh-user root

  # With ssh-host set in .esp-tool.yaml, no flags are needed at all
  esp-tool sync

  # Preview what would change without writing anything
  esp-tool sync --db-file ./.device-builder-devices.json --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir = viperString(cmd, v, "dir")
			filter = viperString(cmd, v, "filter")
			dbFile = viperString(cmd, v, "db-file")
			sshHost = viperString(cmd, v, "ssh-host")
			sshPort = viperInt(cmd, v, "ssh-port")
			sshUser = viperString(cmd, v, "ssh-user")
			sshKey = viperString(cmd, v, "ssh-key")
			remoteFile = viperString(cmd, v, "ssh-remote-file")

			if dbFile != "" && sshHost != "" {
				return fmt.Errorf("specify only one of --db-file or --ssh-host (or set one in .esp-tool.yaml)")
			}
			if dbFile == "" && sshHost == "" {
				return fmt.Errorf("no sync target configured — pass --db-file or --ssh-host, or set ssh-host in .esp-tool.yaml")
			}

			return runSync(syncOptions{
				Dir:        dir,
				Filter:     filter,
				DBFile:     dbFile,
				SSHHost:    sshHost,
				SSHPort:    sshPort,
				SSHUser:    sshUser,
				SSHKey:     sshKey,
				RemoteFile: remoteFile,
				DryRun:     dryRun,
				Verbose:    verbose,
			})
		},
	}

	wd, _ := os.Getwd()
	cmd.Flags().StringVarP(&dir, "dir", "d", wd, "Directory containing ESPHome YAML files")
	cmd.Flags().StringVar(&filter, "filter", "", "Comma-separated device names to limit sync to")
	cmd.Flags().StringVar(&dbFile, "db-file", "", "Local path to .device-builder-devices.json")
	cmd.Flags().StringVar(&sshHost, "ssh-host", "", "HA host to SSH into (alternative to --db-file)")
	cmd.Flags().IntVar(&sshPort, "ssh-port", 22, "SSH port")
	cmd.Flags().StringVar(&sshUser, "ssh-user", "root", "SSH user")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key (falls back to ssh-agent if unset)")
	cmd.Flags().StringVar(&remoteFile, "ssh-remote-file", "", "Path to .device-builder-devices.json on the remote host (auto-discovered under /addon_configs if unset)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be synced without writing changes")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print skip reasons to stderr")

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
