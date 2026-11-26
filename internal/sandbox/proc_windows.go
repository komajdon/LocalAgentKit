//go:build windows

package sandbox

import (
	"os/exec"
	"strconv"
)

// ProcessTree makes cmd.Cancel kill the whole process tree of a command made
// with exec.CommandContext.
func ProcessTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
