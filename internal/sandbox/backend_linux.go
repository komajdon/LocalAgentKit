//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// systemDirs are mounted read-only. Home directories are deliberately absent.
var systemDirs = []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/etc", "/opt"}

// credentialPaths, relative to the home directory, are masked when the work
// directory contains them (for example when it is the home directory).
var credentialPaths = []string{".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker",
	".config/gcloud", ".config/gh", ".netrc", ".git-credentials", ".npmrc"}

var probe struct {
	sync.Once
	path string
	err  error
}

func platformBackend() (*backend, error) {
	probe.Do(func() {
		path, err := exec.LookPath("bwrap")
		if err != nil {
			probe.err = fmt.Errorf("bubblewrap (bwrap) is not installed")
			return
		}
		// Installed is not enough: distributions can forbid the user
		// namespaces it needs, so run it once.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		args := append(baseArgs(false), "true")
		if out, err := exec.CommandContext(ctx, path, args...).CombinedOutput(); err != nil {
			probe.err = fmt.Errorf("bubblewrap cannot create a sandbox: %s", strings.TrimSpace(string(out)))
			return
		}
		probe.path = path
	})
	if probe.err != nil {
		return nil, probe.err
	}
	return &backend{name: "bubblewrap", wrap: wrap}, nil
}

func baseArgs(network bool) []string {
	args := []string{"--unshare-all", "--die-with-parent", "--new-session"}
	if network {
		args = append(args, "--share-net")
	}
	for _, d := range systemDirs {
		info, err := os.Lstat(d)
		switch {
		case err != nil:
		case info.Mode()&os.ModeSymlink != 0:
			if target, err := os.Readlink(d); err == nil {
				args = append(args, "--symlink", target, d)
			}
		default:
			args = append(args, "--ro-bind", d, d)
		}
	}
	return append(args, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp")
}

func wrap(o Options, dir string, argv []string) (string, []string, error) {
	work, err := filepath.EvalSymlinks(o.WorkDir)
	if err != nil {
		return "", nil, fmt.Errorf("work directory: %w", err)
	}
	if work == string(filepath.Separator) {
		return "", nil, fmt.Errorf("the filesystem root cannot be used as a sandboxed work directory")
	}
	args := baseArgs(o.AllowNetwork)
	if o.AllowNetwork {
		// /etc/resolv.conf is often a link into /run.
		args = append(args, "--ro-bind-try", "/run/systemd/resolve", "/run/systemd/resolve")
	}
	args = append(args, "--bind", work, work)
	if work != filepath.Clean(o.WorkDir) {
		args = append(args, "--symlink", work, filepath.Clean(o.WorkDir))
	}
	// Mask protected directories and credential files that fall inside the work
	// directory.
	hidden := append([]string(nil), o.Hidden...)
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, c := range credentialPaths {
			hidden = append(hidden, filepath.Join(home, c))
		}
	}
	for _, h := range hidden {
		resolved, err := filepath.EvalSymlinks(h)
		if err != nil || !within(work, resolved) {
			continue
		}
		if info, err := os.Stat(resolved); err == nil && info.IsDir() {
			args = append(args, "--tmpfs", resolved)
		} else {
			args = append(args, "--ro-bind", "/dev/null", resolved)
		}
	}
	// A home directory that is not mounted would break tools that write there.
	if resolved, err := filepath.EvalSymlinks(home); err != nil || !within(work, resolved) {
		args = append(args, "--setenv", "HOME", "/tmp")
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, fmt.Errorf("command directory: %w", err)
	}
	// Without this, writes to unmounted paths would appear to succeed and then
	// vanish with the sandbox. /tmp stays writable and private.
	args = append(args, "--remount-ro", "/", "--chdir", resolvedDir, "--")
	return probe.path, append(args, argv...), nil
}

// within reports whether path is dir or lies below it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}
