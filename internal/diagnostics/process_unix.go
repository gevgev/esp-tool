//go:build !windows

package diagnostics

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

// setProcGroup puts cmd in its own process group so that killGroup can
// terminate it and all its children atomically. Without this, killing only
// the parent leaves child processes alive, which keep the pipe's write end
// open and hang the scanner.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup terminates an entire process group (process + all children).
func killGroup(p *os.Process, vlog *log.Logger, name string) {
	if p == nil {
		return
	}
	vlog.Printf("[%s] killing process group %d", name, p.Pid)
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		vlog.Printf("[%s] group kill failed (%v), falling back to direct kill", name, err)
		p.Kill()
	}
}
