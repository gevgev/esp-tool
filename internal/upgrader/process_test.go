// process_test.go — cross-platform contract tests for killGroup and setProcGroup.
//
// This file runs on ALL platforms (no build tag).  Platform-specific behaviour
// is exercised in process_unix_test.go (!windows) and process_windows_test.go
// (windows).
package upgrader

import (
	"io"
	"log"
	"os"
	"os/exec"
	"testing"
)

// TestKillGroup_NilProcess verifies that killGroup never panics when passed a
// nil process pointer.  This guard matters on all platforms.
func TestKillGroup_NilProcess(t *testing.T) {
	vlog := log.New(io.Discard, "", 0)
	// Must not panic.
	killGroup(nil, vlog, "test-device")
}

// TestSetProcGroup_NoPanic verifies that setProcGroup does not panic when
// called with a freshly-constructed Cmd (before Start).
func TestSetProcGroup_NoPanic(t *testing.T) {
	cmd := exec.Command(currentPlatformNoOp())
	setProcGroup(cmd)
	// No assertion needed — a panic would fail the test.
}

// TestKillGroup_AlreadyExited verifies that killGroup handles a process that
// has already exited without panicking or returning an error to the caller.
func TestKillGroup_AlreadyExited(t *testing.T) {
	cmd := exec.Command(currentPlatformNoOp())
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// Process has exited; killGroup must not panic.
	vlog := log.New(io.Discard, "", 0)
	killGroup(cmd.Process, vlog, "test-device")
}

// currentPlatformNoOp returns a command that exits immediately with code 0
// on the current platform (used for test process fixtures).
func currentPlatformNoOp() string {
	if _, err := exec.LookPath("true"); err == nil {
		return "true" // Unix
	}
	// Windows: "cmd /c exit 0" — but we only invoke currentPlatformNoOp() for
	// the binary name, so use the Windows shell.
	if _, err := os.Stat(`C:\Windows\System32\cmd.exe`); err == nil {
		return `C:\Windows\System32\cmd.exe`
	}
	return "true" // fallback
}
