//go:build !windows

// process_unix_test.go — Unix-specific assertions for process helpers.
package diagnostics

import (
	"os/exec"
	"testing"
)

// TestSetProcGroup_SetsSetpgid verifies that on Unix, setProcGroup configures
// the command to run in its own process group (Setpgid: true).
func TestSetProcGroup_SetsSetpgid(t *testing.T) {
	cmd := exec.Command("true")
	setProcGroup(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be non-nil after setProcGroup")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected SysProcAttr.Setpgid to be true after setProcGroup")
	}
}
