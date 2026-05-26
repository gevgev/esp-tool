package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if summary != string(want) {
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
