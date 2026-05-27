//go:build windows

package upgrader

import (
	"log"
	"os"
	"os/exec"
)

// setProcGroup is a no-op on Windows. POSIX process groups (Setpgid) do not
// exist on Windows. Child process cleanup relies on the OS reclaiming orphans
// when the parent exits, or on killGroup's direct p.Kill() call.
func setProcGroup(cmd *exec.Cmd) {}

// killGroup terminates the direct child process on Windows. Negative-PID
// group kills are a POSIX-only concept; on Windows we fall back to killing
// the direct esphome process. Any grandchildren esphome spawned will be
// orphaned — in practice esphome on Windows runs under WSL2 where the Linux
// binary (and its process-group kill) is used instead.
func killGroup(p *os.Process, vlog *log.Logger, name string) {
	if p == nil {
		return
	}
	vlog.Printf("[%s] killing process %d (Windows: direct kill, no process groups)", name, p.Pid)
	p.Kill()
}
