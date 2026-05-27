//go:build !windows

package upgrader

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

// killGroup terminates an entire process group (the process and all its
// children). This is necessary because esphome spawns child processes that
// would otherwise keep the pipe's write end open and cause hangs.
func killGroup(p *os.Process, vlog *log.Logger, name string) {
	if p == nil {
		return
	}
	vlog.Printf("[%s] killing process group %d", name, p.Pid)
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		// Fall back to killing just the direct process.
		vlog.Printf("[%s] process group kill failed (%v), falling back to direct kill", name, err)
		p.Kill()
	}
}
