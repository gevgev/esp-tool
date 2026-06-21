package main

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// moduleRoot walks up from the current working directory until it finds the
// directory containing go.mod and returns that directory as the module root.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal("moduleRoot: Getwd:", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("moduleRoot: go.mod not found walking up from", dir)
		}
		dir = parent
	}
}

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns the captured string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("captureStdout: os.Pipe:", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var sb strings.Builder
	io.Copy(&sb, r)
	return sb.String()
}

// ---------------------------------------------------------------------------
// loadDevices
// ---------------------------------------------------------------------------

func TestLoadDevices_ValidDir_ReturnsAllDevices(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "testdata", "devices")

	devices, err := loadDevices(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 6 {
		t.Errorf("want 6 devices from testdata/devices, got %d", len(devices))
	}
}

func TestLoadDevices_Filter_MatchesSingleDevice(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "testdata", "devices")

	devices, err := loadDevices(dir, "espvibration1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("want 1 device, got %d", len(devices))
	}
	if devices[0].Name != "espvibration1" {
		t.Errorf("want device %q, got %q", "espvibration1", devices[0].Name)
	}
}

func TestLoadDevices_Filter_MultipleMatches(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "testdata", "devices")

	devices, err := loadDevices(dir, "espvibration1,step-motor-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("want 2 devices, got %d", len(devices))
	}
}

func TestLoadDevices_Filter_NoMatch_ReturnsError(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "testdata", "devices")

	_, err := loadDevices(dir, "nonexistent-device-xyz")
	if err == nil {
		t.Error("expected error when no devices match filter, got nil")
	}
}

func TestLoadDevices_EmptyDir_ReturnsError(t *testing.T) {
	dir := t.TempDir() // empty — no YAML files
	_, err := loadDevices(dir, "")
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

// ---------------------------------------------------------------------------
// upgrade command — golden file regression test
//
// This test locks in the exact stdout format of "upgrade --dry-run" so that
// any future change to the output pipeline (e.g. PlainWriter in Phase 1B)
// must produce byte-identical results for the deterministic sections.
//
// Non-deterministic [dry-run] lines (printed by concurrent goroutines) are
// verified by format-matching rather than order-dependent comparison.
//
// To regenerate the golden file after an intentional format change:
//
//	UPDATE_GOLDEN=1 go test ./cmd/esp-tool/
// ---------------------------------------------------------------------------

func TestUpgradeCmd_DryRun_GoldenOutput(t *testing.T) {
	root := moduleRoot(t)

	// Run from the repo root so the "testdata/devices" path embedded in the
	// output is always the same relative string, independent of the machine.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal("Chdir to module root:", err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	got := captureStdout(t, func() {
		cmd := rootCmd()
		cmd.SetArgs([]string{"upgrade", "--dry-run", "--dir", "testdata/devices"})
		if err := cmd.Execute(); err != nil {
			t.Errorf("rootCmd.Execute: %v", err)
		}
	})

	// ── Part 1: format-check every [dry-run] command line ───────────────────
	// The six expected lines (order is non-deterministic due to goroutines).
	wantDryRunLines := []string{
		"[dry-run] esphome [run air-quality-external.yaml --no-logs --device air-quality-external.local]  (dir: testdata/devices)",
		"[dry-run] esphome [run bluetooth-proxy-2.yaml --no-logs --device bluetooth-proxy-2.local]  (dir: testdata/devices)",
		"[dry-run] esphome [run espvibration1.yaml --no-logs --device espvibration1.local]  (dir: testdata/devices)",
		"[dry-run] esphome [run lux-living-christmas.yaml --no-logs --device lux-living-christmas.local]  (dir: testdata/devices)",
		"[dry-run] esphome [run ocamera.yaml --no-logs --device ocamera.local]  (dir: testdata/devices)",
		"[dry-run] esphome [run step-motor-1.yaml --no-logs --device step-motor-1.local]  (dir: testdata/devices)",
	}
	for _, want := range wantDryRunLines {
		if !strings.Contains(got, want) {
			t.Errorf("output missing expected dry-run line:\n  %s\nfull output:\n%s", want, got)
		}
	}

	// ── Part 2: golden file comparison for the deterministic summary section ─
	// Extract from the blank line before "Upgrade ESPHome..." to the end.
	summaryMarker := "\nUpgrade ESPHome devices"
	idx := strings.Index(got, summaryMarker)
	if idx < 0 {
		t.Fatal("output does not contain summary section starting with 'Upgrade ESPHome devices'")
	}
	summary := strings.TrimRight(got[idx:], "\n") + "\n"

	goldenFile := filepath.Join(root, "testdata", "golden", "upgrade_dry_run_summary.txt")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenFile), 0755); err != nil {
			t.Fatal("MkdirAll:", err)
		}
		if err := os.WriteFile(goldenFile, []byte(summary), 0644); err != nil {
			t.Fatal("WriteFile golden:", err)
		}
		t.Logf("golden file written: %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("golden file %s not found — run with UPDATE_GOLDEN=1 to create it:\n"+
			"  UPDATE_GOLDEN=1 go test ./cmd/esp-tool/", goldenFile)
	}
	// Normalize line endings before comparing so the test passes on Windows,
	// where git may check out the golden file with CRLF even though the
	// in-process output always uses LF.
	normalize := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	if normalize(summary) != normalize(string(want)) {
		t.Errorf("upgrade summary differs from golden file %s\n--- want ---\n%s\n--- got ---\n%s",
			goldenFile, want, summary)
	}
}

// ---------------------------------------------------------------------------
// captureStderr helper
// ---------------------------------------------------------------------------

// captureStderr redirects os.Stderr to a pipe for the duration of fn and
// returns the captured string. Used to verify verbose fallback messages.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("captureStderr: os.Pipe:", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var sb strings.Builder
	io.Copy(&sb, r)
	return sb.String()
}

// ---------------------------------------------------------------------------
// Phase 1F — flag and log-file tests
// ---------------------------------------------------------------------------

func TestUpgradeCmd_PlainFlag_DryRunSucceeds(t *testing.T) {
	root := moduleRoot(t)
	captureStdout(t, func() {
		cmd := rootCmd()
		cmd.SetArgs([]string{
			"upgrade", "--dry-run", "--plain",
			"--dir", filepath.Join(root, "testdata", "devices"),
		})
		if err := cmd.Execute(); err != nil {
			t.Errorf("rootCmd.Execute with --plain: %v", err)
		}
	})
}

func TestUpgradeCmd_NoTuiFlag_DryRunSucceeds(t *testing.T) {
	root := moduleRoot(t)
	captureStdout(t, func() {
		cmd := rootCmd()
		cmd.SetArgs([]string{
			"upgrade", "--dry-run", "--no-tui",
			"--dir", filepath.Join(root, "testdata", "devices"),
		})
		if err := cmd.Execute(); err != nil {
			t.Errorf("rootCmd.Execute with --no-tui: %v", err)
		}
	})
}

func TestUpgradeCmd_LogFile_CreatesFile(t *testing.T) {
	root := moduleRoot(t)
	logPath := filepath.Join(t.TempDir(), "upgrade.log")

	captureStdout(t, func() {
		cmd := rootCmd()
		cmd.SetArgs([]string{
			"upgrade", "--dry-run",
			"--dir", filepath.Join(root, "testdata", "devices"),
			"--log-file", logPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Errorf("rootCmd.Execute with --log-file: %v", err)
		}
	})

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("--log-file should create %s even in dry-run mode", logPath)
	}
}

func TestUpgradeCmd_VerboseFallback_PrintsMessageOnNonTTY(t *testing.T) {
	// In the test runner, stdout is a pipe (non-TTY), so ShouldUseTUI returns
	// false.  With --verbose the fallback message must appear on stderr.
	root := moduleRoot(t)

	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			cmd := rootCmd()
			cmd.SetArgs([]string{
				"upgrade", "--dry-run", "--verbose",
				"--dir", filepath.Join(root, "testdata", "devices"),
			})
			cmd.Execute() //nolint:errcheck
		})
	})

	if !strings.Contains(stderr, "TUI unavailable") {
		t.Errorf("--verbose should print TUI fallback message on non-TTY stderr; got:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// versions / diagnostics — --jobs flag
//
// versions and diagnostics don't have a --dry-run mode, so exercising the
// flag end-to-end would require a real "esphome" binary on PATH. These
// tests instead verify flag registration: default value and shorthand,
// matching upgrade/validate.
// ---------------------------------------------------------------------------

func TestVersionsCmd_JobsFlag_DefaultAndShorthand(t *testing.T) {
	root := rootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() != "versions" {
			continue
		}
		f := cmd.Flags().Lookup("jobs")
		if f == nil {
			t.Fatal("versions command has no --jobs flag")
		}
		if f.Shorthand != "j" {
			t.Errorf("--jobs shorthand = %q, want %q", f.Shorthand, "j")
		}
		if f.DefValue != "4" {
			t.Errorf("--jobs default = %q, want %q", f.DefValue, "4")
		}
		return
	}
	t.Fatal("versions command not found")
}

func TestDiagnosticsCmd_JobsFlag_DefaultAndShorthand(t *testing.T) {
	root := rootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() != "diagnostics" {
			continue
		}
		f := cmd.Flags().Lookup("jobs")
		if f == nil {
			t.Fatal("diagnostics command has no --jobs flag")
		}
		if f.Shorthand != "j" {
			t.Errorf("--jobs shorthand = %q, want %q", f.Shorthand, "j")
		}
		if f.DefValue != "4" {
			t.Errorf("--jobs default = %q, want %q", f.DefValue, "4")
		}
		return
	}
	t.Fatal("diagnostics command not found")
}

// TestVersionsCmd_JobsFlag_FlowsThroughViperInt and the diagnostics
// equivalent below exercise the actual data path RunE uses
// (viperInt(cmd, v, "jobs") -> RunOptions/CheckOptions.Concurrency)
// rather than just checking that the flag is registered. Because "jobs" is
// already a generically supported config-file key (used by upgrade and
// validate), it works as a config-file fallback for versions/diagnostics
// too as soon as they call viperInt for it — these tests confirm that.
func TestVersionsCmd_JobsFlag_FlowsThroughViperInt(t *testing.T) {
	v := viper.New()
	cmd := versionsCmd(v)
	if err := cmd.Flags().Parse([]string{"--jobs", "7"}); err != nil {
		t.Fatal(err)
	}
	if got := viperInt(cmd, v, "jobs"); got != 7 {
		t.Errorf("viperInt(jobs) after --jobs 7 = %d, want 7", got)
	}
}

func TestVersionsCmd_JobsFlag_ConfigFileFallback(t *testing.T) {
	v := viper.New()
	v.Set("jobs", 6)
	cmd := versionsCmd(v)
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatal(err)
	}
	if got := viperInt(cmd, v, "jobs"); got != 6 {
		t.Errorf("viperInt(jobs) with config jobs=6 and no CLI flag = %d, want 6", got)
	}
}

func TestDiagnosticsCmd_JobsFlag_FlowsThroughViperInt(t *testing.T) {
	v := viper.New()
	cmd := diagnosticsCmd(v)
	if err := cmd.Flags().Parse([]string{"--jobs", "7"}); err != nil {
		t.Fatal(err)
	}
	if got := viperInt(cmd, v, "jobs"); got != 7 {
		t.Errorf("viperInt(jobs) after --jobs 7 = %d, want 7", got)
	}
}

func TestDiagnosticsCmd_JobsFlag_ConfigFileFallback(t *testing.T) {
	v := viper.New()
	v.Set("jobs", 6)
	cmd := diagnosticsCmd(v)
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatal(err)
	}
	if got := viperInt(cmd, v, "jobs"); got != 6 {
		t.Errorf("viperInt(jobs) with config jobs=6 and no CLI flag = %d, want 6", got)
	}
}

// ---------------------------------------------------------------------------
// CLI flag consistency across commands
//
// These tests guard against the same shorthand meaning different things in
// different subcommands, and against a flag name shared by multiple
// commands silently drifting in type or shorthand. New flags should make
// these tests pass by construction, not require special-casing.
//
// Scope: these checks only cover local flags on the four subcommands
// (upgrade, versions, diagnostics, validate) — root-level flags such as
// --version are out of scope since none currently carry a shorthand that
// could collide. They also check signature only (type + shorthand), not
// semantics or default values — e.g. --timeout intentionally has a
// different default per command today; that is not a contract violation
// this test is meant to catch.
// ---------------------------------------------------------------------------

// flagSig captures the parts of a flag's signature that must stay consistent
// wherever the same long name is reused across commands.
type flagSig struct {
	command   string
	shorthand string
	valueType string
}

// collectFlagSigs walks every subcommand's local flags and returns, for each
// long flag name, the list of signatures seen across all commands.
func collectFlagSigs(t *testing.T) map[string][]flagSig {
	t.Helper()
	root := rootCmd()
	sigs := make(map[string][]flagSig)
	for _, cmd := range root.Commands() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			sigs[f.Name] = append(sigs[f.Name], flagSig{
				command:   cmd.Name(),
				shorthand: f.Shorthand,
				valueType: f.Value.Type(),
			})
		})
	}
	return sigs
}

func TestFlagConsistency_NoShorthandCollision(t *testing.T) {
	sigs := collectFlagSigs(t)

	// shorthand -> long name -> commands using it
	byShorthand := make(map[string]map[string][]string)
	for name, occurrences := range sigs {
		for _, sig := range occurrences {
			if sig.shorthand == "" {
				continue
			}
			if byShorthand[sig.shorthand] == nil {
				byShorthand[sig.shorthand] = make(map[string][]string)
			}
			byShorthand[sig.shorthand][name] = append(byShorthand[sig.shorthand][name], sig.command)
		}
	}

	shorthands := make([]string, 0, len(byShorthand))
	for s := range byShorthand {
		shorthands = append(shorthands, s)
	}
	sort.Strings(shorthands)

	for _, shorthand := range shorthands {
		names := byShorthand[shorthand]
		if len(names) <= 1 {
			continue
		}
		longNames := make([]string, 0, len(names))
		for n := range names {
			longNames = append(longNames, n)
		}
		sort.Strings(longNames)
		t.Errorf("shorthand -%s is used for multiple different flags across commands: %v (%v)",
			shorthand, longNames, names)
	}
}

func TestFlagConsistency_SharedNameSameTypeAndShorthand(t *testing.T) {
	sigs := collectFlagSigs(t)

	names := make([]string, 0, len(sigs))
	for name := range sigs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		occurrences := sigs[name]
		if len(occurrences) <= 1 {
			continue // only defined on one command — nothing to compare
		}
		first := occurrences[0]
		for _, sig := range occurrences[1:] {
			if sig.valueType != first.valueType {
				t.Errorf("flag --%s has inconsistent type across commands: %s=%s vs %s=%s",
					name, first.command, first.valueType, sig.command, sig.valueType)
			}
			if sig.shorthand != first.shorthand {
				t.Errorf("flag --%s has inconsistent shorthand across commands: %s=%q vs %s=%q",
					name, first.command, first.shorthand, sig.command, sig.shorthand)
			}
		}
	}
}
