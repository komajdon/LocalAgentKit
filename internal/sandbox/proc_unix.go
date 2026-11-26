//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"
)

// ProcessTree puts a command made with exec.CommandContext in its own process
// group, so that cmd.Cancel also kills the processes it spawned.
func ProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
