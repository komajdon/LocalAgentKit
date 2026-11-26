//go:build windows

package sandbox

import (
	"os/exec"
	"strconv"
)

// setProcessTree kills the whole process tree on cancellation.
func setProcessTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
