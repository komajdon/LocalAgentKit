//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"
)

// setProcessTree puts the command in its own process group so cancellation
// also kills the processes it spawned.
func setProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
