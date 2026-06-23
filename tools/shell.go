package tools

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"agent-gui/domain"
)

const shellTimeout = 5 * time.Minute

type shellTool struct{ workDir string }

func NewShell(workDir string) domain.Tool { return &shellTool{workDir} }

func (t *shellTool) Name() string        { return "shell" }
func (t *shellTool) Description() string { return "Run a shell command and return output" }
func (t *shellTool) Parameters() string {
	return `{"command": "bash command to run", "dir": "working directory (optional, defaults to conversation work dir)"}`
}

const maxCommandLen = 10_000

func (t *shellTool) Execute(args map[string]string) string {
	command := args["command"]
	if len(command) > maxCommandLen {
		return fmt.Sprintf("ERROR: command exceeds maximum length (%d > %d chars)", len(command), maxCommandLen)
	}

	dir := args["dir"]
	if dir == "" {
		dir = t.workDir
	} else {
		dir = resolve(dir, t.workDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("ERROR: command timed out after %s", shellTimeout)
	}
	if err != nil {
		return fmt.Sprintf("EXIT ERROR: %v\n%s", err, string(out))
	}
	return string(out)
}
