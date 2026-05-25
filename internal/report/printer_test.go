package report

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/diagnostics"
	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/upgrader"
)

// captureStdout redirects os.Stdout into a buffer for the duration of fn,
// then returns the captured string.
func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// ---------------------------------------------------------------------------
// deviceCategory
// ---------------------------------------------------------------------------

func TestDeviceCategory_Healthy(t *testing.T) {
	r := diagnostics.Result{Device: discovery.Device{Name: "d"}}
	if got := deviceCategory(r); got != "healthy" {
		t.Errorf("want %q, got %q", "healthy", got)
	}
}

func TestDeviceCategory_Unreachable(t *testing.T) {
	r := diagnostics.Result{Device: discovery.Device{Name: "d"}, Err: "timeout"}
	if got := deviceCategory(r); got != "unreachable" {
		t.Errorf("want %q, got %q", "unreachable", got)
	}
}

func TestDeviceCategory_Warning(t *testing.T) {
	r := diagnostics.Result{
		Device: discovery.Device{Name: "d"},
		Issues: []diagnostics.Issue{{Level: diagnostics.LevelWarning, Code: "CHIP_REV"}},
	}
	if got := deviceCategory(r); got != "warning" {
		t.Errorf("want %q, got %q", "warning", got)
	}
}

func TestDeviceCategory_Crash(t *testing.T) {
	r := diagnostics.Result{
		Device: discovery.Device{Name: "d"},
		Issues: []diagnostics.Issue{{Level: diagnostics.LevelCrash, Code: "CRASH"}},
	}
	if got := deviceCategory(r); got != "crash" {
		t.Errorf("want %q, got %q", "crash", got)
	}
}

func TestDeviceCategory_CrashTakesPrecedenceOverWarning(t *testing.T) {
	r := diagnostics.Result{
		Device: discovery.Device{Name: "d"},
		Issues: []diagnostics.Issue{
			{Level: diagnostics.LevelWarning, Code: "CHIP_REV"},
			{Level: diagnostics.LevelCrash, Code: "CRASH"},
		},
	}
	if got := deviceCategory(r); got != "crash" {
		t.Errorf("crash should take precedence over warning, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// PrintUpgradeSummary output content
// ---------------------------------------------------------------------------

func TestPrintUpgradeSummary_AllSuccess(t *testing.T) {
	results := []upgrader.Result{
		{Device: discovery.Device{Name: "alpha"}, Success: true, Attempts: 1, Duration: 30 * time.Second},
		{Device: discovery.Device{Name: "beta"}, Success: true, Attempts: 1, Duration: 45 * time.Second},
	}
	out := captureStdout(func() { PrintUpgradeSummary(results, 1*time.Minute) })

	if !strings.Contains(out, "alpha") {
		t.Error("output should contain device name 'alpha'")
	}
	if !strings.Contains(out, "beta") {
		t.Error("output should contain device name 'beta'")
	}
	if !strings.Contains(out, "Upgrade successful") {
		t.Error("output should contain 'Upgrade successful'")
	}
	if strings.Contains(out, "Upgrade failed") {
		t.Error("output should not contain 'Upgrade failed' when all succeed")
	}
	if !strings.Contains(out, "2") {
		t.Error("summary should mention count of devices")
	}
}

func TestPrintUpgradeSummary_SomeFailed(t *testing.T) {
	results := []upgrader.Result{
		{Device: discovery.Device{Name: "ok"}, Success: true, Attempts: 1, Duration: 10 * time.Second},
		{Device: discovery.Device{Name: "fail"}, Success: false, Attempts: 3, Duration: 60 * time.Second},
	}
	out := captureStdout(func() { PrintUpgradeSummary(results, 1*time.Minute) })

	if !strings.Contains(out, "Upgrade successful") {
		t.Error("output should contain 'Upgrade successful' for passing device")
	}
	if !strings.Contains(out, "Upgrade failed") {
		t.Error("output should contain 'Upgrade failed' for failing device")
	}
	// Multiple-attempt label only for retried devices
	if !strings.Contains(out, "3 attempts") {
		t.Error("output should show attempt count for failed device with retries")
	}
}

func TestPrintUpgradeSummary_MultipleAttemptsLabel(t *testing.T) {
	results := []upgrader.Result{
		{Device: discovery.Device{Name: "stubborn"}, Success: true, Attempts: 2, Duration: 5 * time.Second},
	}
	out := captureStdout(func() { PrintUpgradeSummary(results, 10*time.Second) })
	if !strings.Contains(out, "2 attempts") {
		t.Errorf("want '2 attempts' in output, got:\n%s", out)
	}
}

func TestPrintUpgradeSummary_SingleAttemptNoLabel(t *testing.T) {
	results := []upgrader.Result{
		{Device: discovery.Device{Name: "quick"}, Success: true, Attempts: 1, Duration: 5 * time.Second},
	}
	out := captureStdout(func() { PrintUpgradeSummary(results, 10*time.Second) })
	if strings.Contains(out, "1 attempts") || strings.Contains(out, "attempt") {
		t.Error("single attempt should not show attempt count label")
	}
}

// ---------------------------------------------------------------------------
// PrintDiagnosticsSummary output content
// ---------------------------------------------------------------------------

func TestPrintDiagnosticsSummary_HealthyDevice(t *testing.T) {
	results := []diagnostics.Result{
		{Device: discovery.Device{Name: "healthy-box"}, Version: "v2026.4.3"},
	}
	out := captureStdout(func() { PrintDiagnosticsSummary(results, 15*time.Second) })

	if !strings.Contains(out, "healthy-box") {
		t.Error("output should contain device name")
	}
	if !strings.Contains(out, "v2026.4.3") {
		t.Error("output should contain firmware version")
	}
	if !strings.Contains(out, "Healthy") {
		t.Error("output should contain 'Healthy' label")
	}
}

func TestPrintDiagnosticsSummary_CrashedDevice(t *testing.T) {
	results := []diagnostics.Result{
		{
			Device:  discovery.Device{Name: "crasher"},
			Version: "v2026.1.0",
			Issues:  []diagnostics.Issue{{Level: diagnostics.LevelCrash, Code: "CRASH", Message: "Crash on previous boot — Hardware WDT"}},
		},
	}
	out := captureStdout(func() { PrintDiagnosticsSummary(results, 15*time.Second) })

	if !strings.Contains(out, "crasher") {
		t.Error("output should contain device name")
	}
	if !strings.Contains(out, "Crash on previous boot") {
		t.Error("output should contain crash message")
	}
	if !strings.Contains(out, "1 crash") {
		t.Error("summary line should report crash count")
	}
}

func TestPrintDiagnosticsSummary_UnreachableDevice(t *testing.T) {
	results := []diagnostics.Result{
		{Device: discovery.Device{Name: "ghost"}, Err: "unreachable or no data within timeout"},
	}
	out := captureStdout(func() { PrintDiagnosticsSummary(results, 15*time.Second) })

	if !strings.Contains(out, "ghost") {
		t.Error("output should contain device name")
	}
	if !strings.Contains(out, "Unreachable") {
		t.Error("output should contain 'Unreachable'")
	}
	if !strings.Contains(out, "unreachable") {
		t.Error("summary line should report unreachable count")
	}
}

func TestPrintDiagnosticsSummary_MixedResults(t *testing.T) {
	results := []diagnostics.Result{
		{Device: discovery.Device{Name: "ok"}, Version: "v2026.4.3"},
		{Device: discovery.Device{Name: "warn"}, Version: "v2026.4.3",
			Issues: []diagnostics.Issue{{Level: diagnostics.LevelWarning, Code: "CHIP_REV", Message: "Chip rev ≥3.0"}}},
		{Device: discovery.Device{Name: "dead"}, Err: "timeout"},
	}
	out := captureStdout(func() { PrintDiagnosticsSummary(results, 15*time.Second) })

	if !strings.Contains(out, "1 healthy") {
		t.Errorf("want '1 healthy' in summary, output:\n%s", out)
	}
	if !strings.Contains(out, "1 warning") {
		t.Errorf("want '1 warning' in summary, output:\n%s", out)
	}
	if !strings.Contains(out, "unreachable") {
		t.Errorf("want unreachable count in summary, output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// PrintVersionSummary output content
// ---------------------------------------------------------------------------

func TestPrintVersionSummary_AllReachable(t *testing.T) {
	results := []upgrader.VersionResult{
		{Device: discovery.Device{Name: "a"}, Version: "v2026.4.3"},
		{Device: discovery.Device{Name: "b"}, Version: "v2026.4.3"},
	}
	out := captureStdout(func() { PrintVersionSummary(results, 10*time.Second) })

	if !strings.Contains(out, "v2026.4.3") {
		t.Error("output should contain version string")
	}
	if strings.Contains(out, "Unreachable") {
		t.Error("should not contain 'Unreachable' when all devices respond")
	}
	if !strings.Contains(out, "All") || !strings.Contains(out, "reachable") {
		t.Error("summary should report all devices reachable")
	}
}

func TestPrintVersionSummary_SomeUnreachable(t *testing.T) {
	results := []upgrader.VersionResult{
		{Device: discovery.Device{Name: "a"}, Version: "v2026.4.3"},
		{Device: discovery.Device{Name: "b"}, Err: "timeout"},
	}
	out := captureStdout(func() { PrintVersionSummary(results, 10*time.Second) })

	if !strings.Contains(out, "Unreachable") {
		t.Error("output should contain 'Unreachable' for offline device")
	}
	if !strings.Contains(out, "1 reachable") {
		t.Errorf("want '1 reachable' in summary, got:\n%s", out)
	}
	if !strings.Contains(out, "1 unreachable") {
		t.Errorf("want '1 unreachable' in summary, got:\n%s", out)
	}
}

func TestPrintVersionSummary_ElapsedTimeShown(t *testing.T) {
	results := []upgrader.VersionResult{
		{Device: discovery.Device{Name: "a"}, Version: "v2026.4.3"},
	}
	elapsed := 42 * time.Second
	out := captureStdout(func() { PrintVersionSummary(results, elapsed) })

	want := fmt.Sprintf("%s", elapsed.Round(time.Second))
	if !strings.Contains(out, want) {
		t.Errorf("output should contain elapsed time %q, got:\n%s", want, out)
	}
}
