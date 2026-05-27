//go:build windows

// process_windows_test.go — Windows-specific assertions for process helpers.
package upgrader

import (
	"os/exec"
	"testing"
)

// TestSetProcGroup_NoopOnWindows verifies that on Windows, setProcGroup leaves
// SysProcAttr nil (POSIX process groups are not supported).
func TestSetProcGroup_NoopOnWindows(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "0")
	setProcGroup(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatal("expected SysProcAttr to remain nil on Windows after setProcGroup")
	}
}
