//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

const sandboxExec = "/usr/bin/sandbox-exec"

func platformBackend() (*backend, error) {
	if err := exec.Command(sandboxExec, "-p", "(version 1)(allow default)", "/usr/bin/true").Run(); err != nil {
		return nil, fmt.Errorf("sandbox-exec is not usable: %v", err)
	}
	return &backend{name: "sandbox-exec", wrap: wrap}, nil
}

// scratch is this process's private temporary and home directory for sandboxed
// commands.
var scratch = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "lak-sandbox-*")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(dir)
})

func wrap(o Options, _ string, argv []string) (string, []string, error) {
	work, err := filepath.EvalSymlinks(o.WorkDir)
	if err != nil {
		return "", nil, fmt.Errorf("work directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	tmp, err := scratch()
	if err != nil {
		return "", nil, fmt.Errorf("sandbox temporary directory: %w", err)
	}
	var hidden []string
	for _, h := range o.Hidden {
		if resolved, err := filepath.EvalSymlinks(h); err == nil {
			hidden = append(hidden, resolved)
		}
	}
	rel, err := filepath.Rel(work, home)
	homeVisible := err == nil && filepath.IsLocal(rel)

	args := darwinProfile(work, home, tmp, hidden, o.AllowNetwork)
	args = append(args, darwinEnv(tmp, homeVisible)...)
	return sandboxExec, append(args, argv...), nil
}
