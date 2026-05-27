// process_test.go — cross-platform contract tests for killGroup and setProcGroup.
//
// This file runs on ALL platforms (no build tag).  Platform-specific behaviour
// is exercised in process_unix_test.go (!windows) and process_windows_test.go
// (windows).
package diagnostics

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
}

// TestKillGroup_AlreadyExited verifies that killGroup handles a process that
// has already exited without panicking.
func TestKillGroup_AlreadyExited(t *testing.T) {
	cmd := exec.Command(currentPlatformNoOp())
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	vlog := log.New(io.Discard, "", 0)
	killGroup(cmd.Process, vlog, "test-device")
}

func currentPlatformNoOp() string {
	if _, err := exec.LookPath("true"); err == nil {
		return "true"
	}
	if _, err := os.Stat(`C:\Windows\System32\cmd.exe`); err == nil {
		return `C:\Windows\System32\cmd.exe`
	}
	return "true"
}
