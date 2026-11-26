//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const sandboxExec = "/usr/bin/sandbox-exec"

func platformBackend() (*backend, error) {
	if err := exec.Command(sandboxExec, "-p", "(version 1)(allow default)", "/usr/bin/true").Run(); err != nil {
		return nil, fmt.Errorf("sandbox-exec is not usable: %v", err)
	}
	return &backend{name: "sandbox-exec", wrap: wrap}, nil
}

// wrap builds a profile in which later rules override earlier ones. Paths are
// passed as parameters, never spliced into the profile text.
func wrap(o Options, _ string, argv []string) (string, []string, error) {
	work, err := filepath.EvalSymlinks(o.WorkDir)
	if err != nil {
		return "", nil, fmt.Errorf("work directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	var profile strings.Builder
	profile.WriteString(`(version 1)
(allow default)
(deny file-write*)
(allow file-write* (subpath (param "WORK")) (subpath "/private/tmp") (subpath "/private/var/folders") (subpath "/dev"))
(deny file-read* (subpath (param "HOME")))
(allow file-read* (subpath (param "WORK")))
(allow file-read-metadata)
`)
	args := []string{"-D", "WORK=" + work, "-D", "HOME=" + home}
	for i, h := range o.Hidden {
		if resolved, err := filepath.EvalSymlinks(h); err == nil {
			name := fmt.Sprintf("HIDDEN%d", i)
			args = append(args, "-D", name+"="+resolved)
			fmt.Fprintf(&profile, "(deny file-read* file-write* (subpath (param %q)))\n", name)
		}
	}
	if !o.AllowNetwork {
		profile.WriteString("(deny network*)\n")
	}
	args = append(args, "-p", profile.String())
	return sandboxExec, append(args, argv...), nil
}
