package tools

import (
	"context"
	"fmt"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/sandbox"
)

const (
	shellTimeout   = 5 * time.Minute
	maxCommandLen  = 10_000
	maxShellOutput = 256 * 1024
)

type shellTool struct {
	ws   workspace
	opts sandbox.Options
}

// NewShell builds the shell tool. opts.WorkDir and opts.Hidden are taken from
// workDir and protected; an unset mode means strict.
func NewShell(workDir string, opts sandbox.Options, protected ...string) domain.Tool {
	opts.WorkDir = workDir
	opts.Hidden = protected
	return &shellTool{workspace{dir: workDir, protected: protected}, opts}
}

func (t *shellTool) Name() string        { return "shell" }
func (t *shellTool) Description() string { return "Run a shell command and return output" }
func (t *shellTool) Parameters() string {
	return `{"command": "bash command to run", "dir": "working directory (optional, defaults to conversation work dir)"}`
}

func (t *shellTool) Execute(ctx context.Context, args domain.Args) string {
	command := args.String("command")
	if len(command) > maxCommandLen {
		return fmt.Sprintf("ERROR: command exceeds maximum length (%d > %d chars)", len(command), maxCommandLen)
	}

	dir := args.String("dir")
	if dir == "" {
		dir = t.ws.dir
	}
	// This only constrains where the command starts; reach is limited by the
	// sandbox, when one is active.
	dir, err := t.ws.directory(dir)
	if err != nil {
		return "ERROR: " + err.Error()
	}

	runCtx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()

	cmd, status, err := sandbox.Command(runCtx, t.opts, dir, command)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	out := &sandbox.Output{Max: maxShellOutput}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Run()

	header := "[" + status.String() + "]\n"
	if ctx.Err() != nil {
		return header + "ERROR: command canceled\n" + out.String()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return header + fmt.Sprintf("ERROR: command timed out after %s\n%s", shellTimeout, out)
	}
	if err != nil {
		return header + fmt.Sprintf("EXIT ERROR: %v\n%s", err, out)
	}
	return header + out.String()
}
