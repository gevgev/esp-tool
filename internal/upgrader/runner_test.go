package upgrader

import (
	"testing"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/discovery"
)

// ---------------------------------------------------------------------------
// extractVersion
// ---------------------------------------------------------------------------

func TestExtractVersion_TypicalLine(t *testing.T) {
	line := "[09:57:03.106][I][app:154]: ESPHome version 2026.4.3 compiled on ..."
	got := extractVersion(line)
	want := "v2026.4.3"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestExtractVersion_NoMarker(t *testing.T) {
	line := "[I][wifi:123]: Connected to AP mynetwork"
	if got := extractVersion(line); got != "" {
		t.Errorf("want empty string, got %q", got)
	}
}

func TestExtractVersion_EmptyLine(t *testing.T) {
	if got := extractVersion(""); got != "" {
		t.Errorf("want empty string for empty input, got %q", got)
	}
}

func TestExtractVersion_VersionAtEndOfLine(t *testing.T) {
	line := "INFO ESPHome version 2025.12.1"
	got := extractVersion(line)
	if got != "v2025.12.1" {
		t.Errorf("want %q, got %q", "v2025.12.1", got)
	}
}

func TestExtractVersion_VersionWithTrailingSpace(t *testing.T) {
	line := "ESPHome version 2026.5.1 something after"
	got := extractVersion(line)
	if got != "v2026.5.1" {
		t.Errorf("version should stop at space, want %q, got %q", "v2026.5.1", got)
	}
}

// ---------------------------------------------------------------------------
// RunOptions defaults
// ---------------------------------------------------------------------------

func TestUpgrade_ConcurrencyDefault(t *testing.T) {
	// Passing opts with Concurrency=0 and DryRun=true exercises the
	// concurrency defaulting path without spawning any real processes.
	devices := []discovery.Device{
		{Name: "dev-a", File: "dev-a.yaml", Host: "dev-a.local"},
	}
	opts := RunOptions{
		Concurrency: 0, // should default to 4
		DryRun:      true,
	}
	results := Upgrade(devices, opts)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("dry-run should always report success")
	}
}

func TestUpgrade_DryRunReturnsSuccess(t *testing.T) {
	devices := []discovery.Device{
		{Name: "alpha", File: "alpha.yaml", Host: "alpha.local"},
		{Name: "beta", File: "beta.yaml", Host: "beta.local"},
		{Name: "gamma", File: "gamma.yaml", Host: "gamma.local"},
	}
	opts := RunOptions{DryRun: true, Concurrency: 2}
	results := Upgrade(devices, opts)

	if len(results) != len(devices) {
		t.Fatalf("want %d results, got %d", len(devices), len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("device %s: dry-run should succeed", r.Device.Name)
		}
		if r.Attempts != 1 {
			t.Errorf("device %s: dry-run should report 1 attempt, got %d", r.Device.Name, r.Attempts)
		}
	}
}

func TestUpgrade_PreservesOrder(t *testing.T) {
	// With DryRun the goroutines complete immediately; the results slice must
	// still preserve the input order regardless of goroutine scheduling.
	names := []string{"z-device", "a-device", "m-device", "b-device"}
	devices := make([]discovery.Device, len(names))
	for i, n := range names {
		devices[i] = discovery.Device{Name: n, File: n + ".yaml", Host: n + ".local"}
	}
	opts := RunOptions{DryRun: true, Concurrency: len(devices)}
	results := Upgrade(devices, opts)

	for i, r := range results {
		if r.Device.Name != names[i] {
			t.Errorf("index %d: want device %q, got %q", i, names[i], r.Device.Name)
		}
	}
}

func TestUpgrade_EmptyDeviceList(t *testing.T) {
	results := Upgrade(nil, RunOptions{DryRun: true})
	if len(results) != 0 {
		t.Errorf("want empty results for empty device list, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// CheckVersions (dry-run path)
// ---------------------------------------------------------------------------

func TestCheckVersions_DryRunReturnsPlaceholder(t *testing.T) {
	devices := []discovery.Device{
		{Name: "cam", File: "cam.yaml", Host: "cam.local"},
	}
	results := CheckVersions(devices, RunOptions{DryRun: true}, 5*time.Second)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Version != "v0.0.0-dry-run" {
		t.Errorf("want dry-run placeholder version, got %q", results[0].Version)
	}
	if results[0].Err != "" {
		t.Errorf("dry-run should have no error, got %q", results[0].Err)
	}
}

func TestCheckVersions_PreservesOrder(t *testing.T) {
	names := []string{"c", "a", "b"}
	devices := make([]discovery.Device, len(names))
	for i, n := range names {
		devices[i] = discovery.Device{Name: n, File: n + ".yaml", Host: n + ".local"}
	}
	results := CheckVersions(devices, RunOptions{DryRun: true}, 5*time.Second)
	for i, r := range results {
		if r.Device.Name != names[i] {
			t.Errorf("index %d: want %q, got %q", i, names[i], r.Device.Name)
		}
	}
}
