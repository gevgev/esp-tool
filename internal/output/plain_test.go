package output_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ggevorgyan/esp-tool/internal/output"
)

// ---------------------------------------------------------------------------
// PlainWriter — WriteLine
// ---------------------------------------------------------------------------

func TestPlainWriter_WriteLine_WithPrefix(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, true)
	w.WriteLine("my-device", "INFO Starting upload")

	got := buf.String()
	if !strings.Contains(got, "[my-device]") {
		t.Errorf("want '[my-device]' prefix in output, got: %q", got)
	}
	if !strings.Contains(got, "INFO Starting upload") {
		t.Errorf("want line content in output, got: %q", got)
	}
}

func TestPlainWriter_WriteLine_NoPrefix(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.WriteLine("my-device", "INFO Starting upload")

	got := buf.String()
	if strings.Contains(got, "[my-device]") {
		t.Errorf("want no device prefix in output, got: %q", got)
	}
	if !strings.Contains(got, "INFO Starting upload") {
		t.Errorf("want line content in output, got: %q", got)
	}
}

func TestPlainWriter_WriteLine_EndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.WriteLine("dev", "some line")

	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("want trailing newline, got: %q", got)
	}
}

func TestPlainWriter_WriteLine_PrefixFormat(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, true)
	w.WriteLine("cam", "INFO Upload complete")

	// Exact format: "[cam] INFO Upload complete\n"
	want := "[cam] INFO Upload complete\n"
	got := buf.String()
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// PlainWriter — DeviceStarted
// ---------------------------------------------------------------------------

func TestPlainWriter_DeviceStarted_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.DeviceStarted("silent-device")
	if buf.Len() != 0 {
		t.Errorf("DeviceStarted should produce no output in PlainWriter, got: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// PlainWriter — DeviceCompleted
// ---------------------------------------------------------------------------

func TestPlainWriter_DeviceCompleted_Success_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.DeviceCompleted("cam", true, 1, time.Second, nil)
	// Summary is handled by report.PrintUpgradeSummary — PlainWriter is a no-op.
	if buf.Len() != 0 {
		t.Errorf("DeviceCompleted success should produce no output in PlainWriter, got: %q", buf.String())
	}
}

func TestPlainWriter_DeviceCompleted_Failure_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	// errLines non-nil: parseErrors populates this in Phase 1C; still a no-op here.
	w.DeviceCompleted("cam", false, 3, 5*time.Second, []string{"ERROR: Connection refused"})
	if buf.Len() != 0 {
		t.Errorf("DeviceCompleted failure should produce no output in PlainWriter (summary handled by report), got: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// PlainWriter — DeviceRetrying
// ---------------------------------------------------------------------------

func TestPlainWriter_DeviceRetrying_ContainsDeviceName(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.DeviceRetrying("cam", 2, 3, 5*time.Second)
	got := buf.String()
	if !strings.Contains(got, "cam") {
		t.Errorf("retry message should contain device name, got: %q", got)
	}
}

func TestPlainWriter_DeviceRetrying_ContainsAttemptCounts(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.DeviceRetrying("dev", 2, 3, 5*time.Second)
	got := buf.String()
	if !strings.Contains(got, "2") || !strings.Contains(got, "3") {
		t.Errorf("retry message should contain attempt counts, got: %q", got)
	}
}

func TestPlainWriter_DeviceRetrying_Format(t *testing.T) {
	var buf bytes.Buffer
	w := output.NewPlainWriter(&buf, false)
	w.DeviceRetrying("beta", 2, 3, 5*time.Second)

	// Preserve the exact format from the original runner.go fmt.Printf.
	want := "[beta] retrying (attempt 2/3) after 5s...\n"
	got := buf.String()
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// PlainWriter — concurrent safety (run with -race)
// ---------------------------------------------------------------------------

func TestPlainWriter_ConcurrentWrites_NoRace(t *testing.T) {
	// Use io.Discard so we don't need a thread-safe buffer; we're testing
	// that PlainWriter's own mutex prevents races, not the underlying writer.
	w := output.NewPlainWriter(io.Discard, true)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			w.WriteLine(fmt.Sprintf("dev-%d", n), "concurrent line")
			w.DeviceRetrying(fmt.Sprintf("dev-%d", n), 2, 3, time.Millisecond)
			w.DeviceStarted(fmt.Sprintf("dev-%d", n))
			w.DeviceCompleted(fmt.Sprintf("dev-%d", n), true, 1, time.Millisecond, nil)
		}(i)
	}
	wg.Wait()
}
