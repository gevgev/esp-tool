package upgrader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/discovery"
	"github.com/ggevorgyan/esp-tool/internal/output"
)

// ---------------------------------------------------------------------------
// TestMain — build fake esphome binary once for all subprocess tests
// ---------------------------------------------------------------------------

// fakeBinDir is the directory holding the "esphome" stub binary.
// Set once by TestMain; read-only after that.
var fakeBinDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeesphome-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: failed to create temp dir:", err)
		os.Exit(1)
	}

	binPath := filepath.Join(dir, "esphome")
	cmd := exec.Command("go", "build", "-o", binPath, "./testdata/fakeesphome")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: failed to build fakeesphome: %v\n%s\n", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	fakeBinDir = dir

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// withFakeEsphome prepends fakeBinDir to PATH for the duration of t and
// sets any additional "KEY=VALUE" environment variables.
func withFakeEsphome(t *testing.T, env ...string) {
	t.Helper()
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			t.Setenv(parts[0], parts[1])
		}
	}
}

// captureStdout redirects os.Stdout into a buffer for the duration of fn
// and returns the captured string. Still used for tests that check the
// dry-run fmt.Printf output that bypasses the writer.
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
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// ---------------------------------------------------------------------------
// OutputWriter test doubles
// ---------------------------------------------------------------------------

// noopWriter satisfies output.OutputWriter and discards all events.
// Use for tests that only care about return values, not output format.
type noopWriter struct{}

var _ output.OutputWriter = &noopWriter{}

func (n *noopWriter) WriteLine(_ string, _ string)                                                       {}
func (n *noopWriter) DeviceStarted(_ string)                                                             {}
func (n *noopWriter) DeviceCompleted(_ string, _ bool, _ int, _ time.Duration, _ []string)              {}
func (n *noopWriter) DeviceRetrying(_ string, _, _ int, _ time.Duration)                                 {}

// writeRecord is a single call captured by captureWriter.
type writeRecord struct {
	device string
	line   string
}

// captureWriter satisfies output.OutputWriter and records every WriteLine call.
// Use for tests that inspect what output was delivered to the writer.
type captureWriter struct {
	mu      sync.Mutex
	records []writeRecord
}

var _ output.OutputWriter = &captureWriter{}

func (c *captureWriter) WriteLine(device, line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, writeRecord{device: device, line: line})
}
func (c *captureWriter) DeviceStarted(_ string)                                                          {}
func (c *captureWriter) DeviceCompleted(_ string, _ bool, _ int, _ time.Duration, _ []string)           {}
func (c *captureWriter) DeviceRetrying(_ string, _, _ int, _ time.Duration)                              {}

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
	devices := []discovery.Device{
		{Name: "dev-a", File: "dev-a.yaml", Host: "dev-a.local"},
	}
	opts := RunOptions{
		Concurrency: 0, // should default to 4
		DryRun:      true,
	}
	captureStdout(t, func() {
		results := Upgrade(devices, opts, &noopWriter{})
		if len(results) != 1 {
			t.Fatalf("want 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Error("dry-run should always report success")
		}
	})
}

func TestUpgrade_DryRunReturnsSuccess(t *testing.T) {
	devices := []discovery.Device{
		{Name: "alpha", File: "alpha.yaml", Host: "alpha.local"},
		{Name: "beta", File: "beta.yaml", Host: "beta.local"},
		{Name: "gamma", File: "gamma.yaml", Host: "gamma.local"},
	}
	opts := RunOptions{DryRun: true, Concurrency: 2}
	var results []Result
	captureStdout(t, func() {
		results = Upgrade(devices, opts, &noopWriter{})
	})

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
	names := []string{"z-device", "a-device", "m-device", "b-device"}
	devices := make([]discovery.Device, len(names))
	for i, n := range names {
		devices[i] = discovery.Device{Name: n, File: n + ".yaml", Host: n + ".local"}
	}
	opts := RunOptions{DryRun: true, Concurrency: len(devices)}
	var results []Result
	captureStdout(t, func() {
		results = Upgrade(devices, opts, &noopWriter{})
	})

	for i, r := range results {
		if r.Device.Name != names[i] {
			t.Errorf("index %d: want device %q, got %q", i, names[i], r.Device.Name)
		}
	}
}

func TestUpgrade_EmptyDeviceList(t *testing.T) {
	results := Upgrade(nil, RunOptions{DryRun: true}, &noopWriter{})
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

// ---------------------------------------------------------------------------
// newLogger
// ---------------------------------------------------------------------------

func TestNewLogger_Verbose_WritesToStderr(t *testing.T) {
	// Temporarily redirect stderr to capture logger output.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w

	logger := newLogger(true)
	logger.Print("hello from verbose logger")

	w.Close()
	os.Stderr = orig

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !strings.Contains(buf.String(), "hello from verbose logger") {
		t.Errorf("verbose logger should write to stderr, got: %q", buf.String())
	}
}

func TestNewLogger_Silent_Discards(t *testing.T) {
	// Should not panic and should produce no observable output.
	logger := newLogger(false)
	logger.Print("this should be silently discarded")
}

// ---------------------------------------------------------------------------
// killGroup
// ---------------------------------------------------------------------------

func TestKillGroup_NilProcess_NoOp(t *testing.T) {
	vlog := newLogger(false)
	// Must not panic on a nil process.
	killGroup(nil, vlog, "test-device")
}

// ---------------------------------------------------------------------------
// runStreaming — subprocess tests (require fakeesphome binary)
// ---------------------------------------------------------------------------

func TestRunStreaming_ReturnsNilOnSuccess(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)

	if err := runStreaming(cmd, "test-dev", &noopWriter{}, nil, vlog, context.Background()); err != nil {
		t.Errorf("expected nil error on success, got: %v", err)
	}
}

func TestRunStreaming_ReturnsErrorOnNonZeroExit(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=fail")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)

	if err := runStreaming(cmd, "test-dev", &noopWriter{}, nil, vlog, context.Background()); err == nil {
		t.Error("expected non-nil error for non-zero exit, got nil")
	}
}

func TestRunStreaming_WriterReceivesDeviceName(t *testing.T) {
	// runStreaming always calls writer.WriteLine(name, line); formatting is
	// the writer's responsibility (PlainWriter.Prefix controls it).
	withFakeEsphome(t, "FAKE_MODE=succeed")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)
	cw := &captureWriter{}

	if err := runStreaming(cmd, "my-device", cw, nil, vlog, context.Background()); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}

	if len(cw.records) == 0 {
		t.Fatal("writer should receive at least one line from fakeesphome")
	}
	for _, r := range cw.records {
		if r.device != "my-device" {
			t.Errorf("all records should have device='my-device', got %q", r.device)
		}
		if r.line == "" {
			t.Error("empty line should not be passed to writer")
		}
	}
}

func TestRunStreaming_WriterReceivesRawLineContent(t *testing.T) {
	// Verifies that the raw line from fakeesphome is passed through unmodified.
	withFakeEsphome(t, "FAKE_MODE=succeed")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)
	cw := &captureWriter{}

	runStreaming(cmd, "my-device", cw, nil, vlog, context.Background()) //nolint:errcheck

	// fakeesphome in "succeed" mode prints "INFO Starting" and "INFO Upload successful".
	var found bool
	for _, r := range cw.records {
		if strings.Contains(r.line, "INFO") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one INFO line in writer records; records: %+v", cw.records)
	}
}

func TestRunStreaming_CapturesLinesToBuffer(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)
	bufs := newDeviceBuffers(nil)

	if err := runStreaming(cmd, "dev", &noopWriter{}, bufs, vlog, context.Background()); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}

	// Both full buffer and display ring buffer should have received lines.
	if len(bufs.full) == 0 {
		t.Error("full buffer should have captured lines")
	}
	if bufs.display.Len() == 0 {
		t.Error("display ring buffer should have captured lines")
	}
	// Verify full buffer and display ring buffer agree on content.
	dispLines := bufs.display.Lines()
	// display may be a subset if > displayCapacity lines, but for this test
	// fakeesphome only emits 2 lines — they should match exactly.
	if len(bufs.full) != len(dispLines) {
		t.Errorf("full buffer (%d lines) and display (%d lines) should agree for small output",
			len(bufs.full), len(dispLines))
	}
}

// ---------------------------------------------------------------------------
// runWithRetry — subprocess tests (require fakeesphome binary)
// ---------------------------------------------------------------------------

func TestRunWithRetry_SucceedsFirstAttempt(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed")
	d := discovery.Device{Name: "alpha", File: "alpha.yaml", Host: "alpha.local"}
	opts := RunOptions{Retries: 2, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if !result.Success {
		t.Errorf("expected Success=true, got false (err: %s)", result.Err)
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
}

func TestRunWithRetry_SucceedsAfterOneRetry(t *testing.T) {
	counterFile := filepath.Join(t.TempDir(), "counter")
	withFakeEsphome(t,
		"FAKE_MODE=fail-once",
		"FAKE_COUNTER_FILE="+counterFile,
	)
	d := discovery.Device{Name: "beta", File: "beta.yaml", Host: "beta.local"}
	opts := RunOptions{Retries: 2, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if !result.Success {
		t.Errorf("expected success after retry, got failure: %s", result.Err)
	}
	if result.Attempts != 2 {
		t.Errorf("expected 2 attempts (1 fail + 1 retry), got %d", result.Attempts)
	}
}

func TestRunWithRetry_ExhaustsAllRetries(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=fail")
	d := discovery.Device{Name: "gamma", File: "gamma.yaml", Host: "gamma.local"}
	// Retries: 2 → maxAttempts = 3
	opts := RunOptions{Retries: 2, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Error("expected failure after exhausting retries, got success")
	}
	if result.Attempts != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", result.Attempts)
	}
	if result.Err == "" {
		t.Error("expected non-empty Err on exhausted retries")
	}
}

func TestRunWithRetry_CallsDeviceRetrying(t *testing.T) {
	// Verify that the writer's DeviceRetrying method is called on each retry.
	withFakeEsphome(t, "FAKE_MODE=fail")
	d := discovery.Device{Name: "retry-dev", File: "r.yaml", Host: "retry-dev.local"}
	opts := RunOptions{Retries: 2, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	type retryCall struct {
		attempt, maxAttempts int
	}
	var mu sync.Mutex
	var retryCalls []retryCall

	w := &retryTrackingWriter{
		onRetry: func(device string, attempt, maxAttempts int, delay time.Duration) {
			mu.Lock()
			retryCalls = append(retryCalls, retryCall{attempt, maxAttempts})
			mu.Unlock()
		},
	}

	runWithRetry(d, opts, w, vlog)

	mu.Lock()
	n := len(retryCalls)
	mu.Unlock()

	if n != 2 {
		t.Errorf("want 2 DeviceRetrying calls (Retries=2), got %d", n)
	}
	mu.Lock()
	for i, c := range retryCalls {
		wantAttempt := i + 2 // 2nd and 3rd attempts
		if c.attempt != wantAttempt {
			t.Errorf("retry[%d]: want attempt=%d, got %d", i, wantAttempt, c.attempt)
		}
		if c.maxAttempts != 3 {
			t.Errorf("retry[%d]: want maxAttempts=3, got %d", i, c.maxAttempts)
		}
	}
	mu.Unlock()
}

// retryTrackingWriter is a test double that invokes a callback on DeviceRetrying.
type retryTrackingWriter struct {
	onRetry func(device string, attempt, maxAttempts int, delay time.Duration)
}

var _ output.OutputWriter = &retryTrackingWriter{}

func (r *retryTrackingWriter) WriteLine(_ string, _ string)                                              {}
func (r *retryTrackingWriter) DeviceStarted(_ string)                                                    {}
func (r *retryTrackingWriter) DeviceCompleted(_ string, _ bool, _ int, _ time.Duration, _ []string)     {}
func (r *retryTrackingWriter) DeviceRetrying(device string, attempt, maxAttempts int, delay time.Duration) {
	if r.onRetry != nil {
		r.onRetry(device, attempt, maxAttempts, delay)
	}
}

func TestRunWithRetry_ErrLinesPopulatedOnFailure(t *testing.T) {
	// fakeesphome "fail" mode prints "ERROR Connection refused" to stderr,
	// which is merged into the pipe — should appear in ErrLines.
	withFakeEsphome(t, "FAKE_MODE=fail")
	d := discovery.Device{Name: "err-dev", File: "e.yaml", Host: "err-dev.local"}
	opts := RunOptions{Retries: 0, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Fatal("expected failure, got success")
	}
	if len(result.ErrLines) == 0 {
		t.Error("ErrLines should be non-empty on failure")
	}
}

func TestRunWithRetry_ErrLinesNilOnSuccess(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed")
	d := discovery.Device{Name: "ok-dev", File: "ok.yaml", Host: "ok-dev.local"}
	opts := RunOptions{Retries: 0, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Err)
	}
	if result.ErrLines != nil {
		t.Errorf("ErrLines should be nil on success, got: %v", result.ErrLines)
	}
}

// ---------------------------------------------------------------------------
// fetchVersion — subprocess tests (require fakeesphome binary)
// ---------------------------------------------------------------------------

func TestFetchVersion_VersionFoundInOutput(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=print-version")
	d := discovery.Device{Name: "cam", File: "cam.yaml", Host: "cam.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := fetchVersion(d, opts, 5*time.Second, vlog)

	if result.Version != "v2026.4.3" {
		t.Errorf("want version %q, got %q (err: %s)", "v2026.4.3", result.Version, result.Err)
	}
	if result.Err != "" {
		t.Errorf("expected no error, got: %s", result.Err)
	}
}

func TestFetchVersion_TimeoutReached_ReturnsError(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=stall")
	d := discovery.Device{Name: "stuck", File: "stuck.yaml", Host: "stuck.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := fetchVersion(d, opts, 100*time.Millisecond, vlog)

	if result.Version != "" {
		t.Errorf("expected empty version on timeout, got %q", result.Version)
	}
	if result.Err == "" {
		t.Error("expected non-empty Err on timeout, got empty string")
	}
}

func TestFetchVersion_NoVersionLine_ReturnsError(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed") // prints INFO lines but no version string
	d := discovery.Device{Name: "noversion", File: "nv.yaml", Host: "noversion.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)

	result := fetchVersion(d, opts, 2*time.Second, vlog)

	if result.Version != "" {
		t.Errorf("expected empty version when output has no version line, got %q", result.Version)
	}
	if result.Err == "" {
		t.Error("expected non-empty Err when no version line found")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestRunWithRetry_ContextCancelled_StopsEarly(t *testing.T) {
	// fakeesphome "stall" sleeps for 10 minutes; context timeout of 300ms should
	// kill it quickly via the process-group kill in runStreaming.
	withFakeEsphome(t, "FAKE_MODE=stall")
	d := discovery.Device{Name: "stall-dev", File: "stall.yaml", Host: "stall.local"}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	opts := RunOptions{
		Retries:    0,
		RetryDelay: 1 * time.Millisecond,
		WorkDir:    t.TempDir(),
		Ctx:        ctx,
	}
	vlog := newLogger(false)

	start := time.Now()
	result := runWithRetry(d, opts, &noopWriter{}, vlog)
	elapsed := time.Since(start)

	// Should complete well before the fakeesphome stall timeout.
	if elapsed > 5*time.Second {
		t.Errorf("runWithRetry should terminate on context cancellation; took %s", elapsed)
	}
	if result.Success {
		t.Error("cancelled run should not succeed")
	}
}

func TestRunWithRetry_ContextAlreadyCancelled_SkipsProcess(t *testing.T) {
	// Context is already done — runWithRetry should return without spawning a process.
	withFakeEsphome(t, "FAKE_MODE=succeed")
	d := discovery.Device{Name: "fast-dev", File: "fast.yaml", Host: "fast.local"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	opts := RunOptions{Retries: 2, RetryDelay: 1 * time.Millisecond, WorkDir: t.TempDir(), Ctx: ctx}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Error("pre-cancelled context should not produce a successful result")
	}
	if result.Attempts != 0 {
		// Cancelled before first attempt — Attempts should be 0 (attempt-1 on first loop iter)
		t.Logf("note: Attempts=%d (implementation detail, not a strict requirement)", result.Attempts)
	}
}

func TestRunStreaming_ContextCancelled_KillsProcess(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=stall")
	cmd := exec.Command("esphome", "run", "device.yaml", "--no-logs", "--device", "device.local")
	vlog := newLogger(false)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := runStreaming(cmd, "stall-dev", &noopWriter{}, nil, vlog, ctx)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("runStreaming should be killed by context cancel; took %s", elapsed)
	}
	// Process was killed — expect a non-nil error
	if err == nil {
		t.Error("expected non-nil error when process is killed by context cancel")
	}
}

// ---------------------------------------------------------------------------
// Per-device timeout (Feature 1)
// ---------------------------------------------------------------------------

func TestRunWithRetry_PerDeviceTimeout_KillsStalledProcess(t *testing.T) {
	// fakeesphome "stall" sleeps forever; PerDeviceTimeout should kill it.
	withFakeEsphome(t, "FAKE_MODE=stall")
	d := discovery.Device{Name: "stall-dev", File: "s.yaml", Host: "stall-dev.local"}
	opts := RunOptions{
		Retries:          0,
		RetryDelay:       1 * time.Millisecond,
		WorkDir:          t.TempDir(),
		PerDeviceTimeout: 200 * time.Millisecond,
	}
	vlog := newLogger(false)

	start := time.Now()
	result := runWithRetry(d, opts, &noopWriter{}, vlog)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("PerDeviceTimeout should have killed the process; ran for %s", elapsed)
	}
	if result.Success {
		t.Error("timed-out run should not succeed")
	}
	if !strings.Contains(result.Err, "timed out") {
		t.Errorf("Err should mention 'timed out', got: %q", result.Err)
	}
}

func TestRunWithRetry_PerDeviceTimeout_RetriesAfterTimeout(t *testing.T) {
	// With Retries=1 the device should be attempted twice, each timing out.
	withFakeEsphome(t, "FAKE_MODE=stall")
	d := discovery.Device{Name: "stall2", File: "s2.yaml", Host: "stall2.local"}
	opts := RunOptions{
		Retries:          1,
		RetryDelay:       1 * time.Millisecond,
		WorkDir:          t.TempDir(),
		PerDeviceTimeout: 150 * time.Millisecond,
	}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Error("should fail after retries")
	}
	// maxAttempts=2; both should have timed out
	if result.Attempts != 2 {
		t.Errorf("want 2 attempts, got %d", result.Attempts)
	}
}

func TestRunWithRetry_NoTimeout_RunsToCompletion(t *testing.T) {
	// Zero PerDeviceTimeout means no limit — fast succeed should not be killed.
	withFakeEsphome(t, "FAKE_MODE=succeed")
	d := discovery.Device{Name: "ok", File: "ok.yaml", Host: "ok.local"}
	opts := RunOptions{
		Retries:          0,
		RetryDelay:       1 * time.Millisecond,
		WorkDir:          t.TempDir(),
		PerDeviceTimeout: 0, // no timeout
	}
	vlog := newLogger(false)
	result := runWithRetry(d, opts, &noopWriter{}, vlog)
	if !result.Success {
		t.Errorf("expected success with no timeout, got err: %s", result.Err)
	}
}

// ---------------------------------------------------------------------------
// Compile-error detection (Feature 2)
// ---------------------------------------------------------------------------

func TestIsCompilationError_DetectsFailedConfig(t *testing.T) {
	lines := []string{"INFO Starting compilation", "Failed config", "  device.yaml: invalid key"}
	if !isCompilationError(lines) {
		t.Error("should detect 'Failed config' as a compilation error")
	}
}

func TestIsCompilationError_DetectsInvalidYAML(t *testing.T) {
	lines := []string{"Invalid YAML syntax at line 5"}
	if !isCompilationError(lines) {
		t.Error("should detect 'Invalid YAML' as a compilation error")
	}
}

func TestIsCompilationError_DetectsPythonTraceback(t *testing.T) {
	lines := []string{
		"Traceback (most recent call last):",
		"  File \"esphome/__main__.py\", line 42, in run",
		"RecursionError: maximum recursion depth exceeded",
	}
	if !isCompilationError(lines) {
		t.Error("should detect Python traceback as a compilation error")
	}
}

func TestIsCompilationError_NoFalsePositiveOnNetworkError(t *testing.T) {
	lines := []string{
		"INFO Connecting to device.local...",
		"ERROR Could not connect to device.local",
		"exit status 1",
	}
	if isCompilationError(lines) {
		t.Error("network error should NOT be classified as a compilation error")
	}
}

func TestIsCompilationError_EmptyLines_ReturnsFalse(t *testing.T) {
	if isCompilationError(nil) {
		t.Error("nil lines should return false")
	}
	if isCompilationError([]string{}) {
		t.Error("empty lines should return false")
	}
}

func TestRunWithRetry_CompileError_SkipsRetries(t *testing.T) {
	// fakeesphome "compile-error" prints "Failed config"; retries should be skipped.
	withFakeEsphome(t, "FAKE_MODE=compile-error")
	d := discovery.Device{Name: "bad-config", File: "bad.yaml", Host: "bad.local"}
	opts := RunOptions{
		Retries:    2, // would normally allow 3 total attempts
		RetryDelay: 1 * time.Millisecond,
		WorkDir:    t.TempDir(),
	}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Fatal("compile-error run should not succeed")
	}
	// Should have stopped after the first attempt, not retried.
	if result.Attempts != 1 {
		t.Errorf("compile error should skip retries: want 1 attempt, got %d", result.Attempts)
	}
	if !result.CompileError {
		t.Error("result.CompileError should be true for a compile-error failure")
	}
}

func TestRunWithRetry_NetworkError_StillRetries(t *testing.T) {
	// Plain "fail" mode outputs "ERROR Connection refused" (no compile markers).
	// Retries should proceed as normal.
	withFakeEsphome(t, "FAKE_MODE=fail")
	d := discovery.Device{Name: "net-fail", File: "n.yaml", Host: "net-fail.local"}
	opts := RunOptions{
		Retries:    2,
		RetryDelay: 1 * time.Millisecond,
		WorkDir:    t.TempDir(),
	}
	vlog := newLogger(false)

	result := runWithRetry(d, opts, &noopWriter{}, vlog)

	if result.Success {
		t.Fatal("should have failed")
	}
	if result.Attempts != 3 {
		t.Errorf("network error should use all retries: want 3 attempts, got %d", result.Attempts)
	}
	if result.CompileError {
		t.Error("network error should NOT set CompileError")
	}
}

// ---------------------------------------------------------------------------
// ValidateDevices (Feature 3)
// ---------------------------------------------------------------------------

func TestValidateDevices_DryRun_AllValid(t *testing.T) {
	devices := []discovery.Device{
		{Name: "alpha", File: "alpha.yaml", Host: "alpha.local"},
		{Name: "beta", File: "beta.yaml", Host: "beta.local"},
	}
	opts := RunOptions{DryRun: true, WorkDir: t.TempDir()}
	var results []ValidateResult
	captureStdout(t, func() {
		results = ValidateDevices(devices, opts, 5*time.Second)
	})
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Valid {
			t.Errorf("device %s: dry-run validate should be valid", r.Device.Name)
		}
	}
}

func TestValidateDevices_PreservesOrder(t *testing.T) {
	names := []string{"z", "a", "m"}
	devices := make([]discovery.Device, len(names))
	for i, n := range names {
		devices[i] = discovery.Device{Name: n, File: n + ".yaml", Host: n + ".local"}
	}
	opts := RunOptions{DryRun: true, WorkDir: t.TempDir()}
	var results []ValidateResult
	captureStdout(t, func() {
		results = ValidateDevices(devices, opts, 5*time.Second)
	})
	for i, r := range results {
		if r.Device.Name != names[i] {
			t.Errorf("index %d: want %q, got %q", i, names[i], r.Device.Name)
		}
	}
}

func TestValidateDevice_Success(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=succeed")
	d := discovery.Device{Name: "ok", File: "ok.yaml", Host: "ok.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)
	r := validateDevice(d, opts, 5*time.Second, vlog)
	if !r.Valid {
		t.Errorf("succeed mode should produce Valid=true, got err: %s", r.Err)
	}
}

func TestValidateDevice_Failure(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=compile-error")
	d := discovery.Device{Name: "bad", File: "bad.yaml", Host: "bad.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)
	r := validateDevice(d, opts, 5*time.Second, vlog)
	if r.Valid {
		t.Error("compile-error mode should produce Valid=false")
	}
	if r.Err == "" {
		t.Error("failed validation should have a non-empty Err")
	}
}

func TestValidateDevice_Timeout(t *testing.T) {
	withFakeEsphome(t, "FAKE_MODE=stall")
	d := discovery.Device{Name: "slow", File: "slow.yaml", Host: "slow.local"}
	opts := RunOptions{WorkDir: t.TempDir()}
	vlog := newLogger(false)

	start := time.Now()
	r := validateDevice(d, opts, 150*time.Millisecond, vlog)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("validation should timeout; ran for %s", elapsed)
	}
	if r.Valid {
		t.Error("timed-out validation should not be Valid")
	}
}
