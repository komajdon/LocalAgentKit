package sandbox

import (
	"fmt"
	"strings"
)

// This file has no build tag so that the profile can be tested everywhere.

// deniedOnDarwin are where other people's and other programs' files live:
// home directories, mounted volumes and the temporary directories.
var deniedOnDarwin = []string{"/Users", "/Volumes", "/private/tmp", "/private/var/folders"}

// darwinProfile returns the sandbox-exec arguments that precede the command.
// work is the work directory, home the user's home, scratch a private
// temporary directory, shims a directory of helper programs that commands may
// run but not change, and hidden the protected directories; all must be
// resolved paths. Paths are passed as parameters, never spliced into the
// profile text. Later rules override earlier ones.
func darwinProfile(work, home, scratch, shims string, hidden []string, network bool) []string {
	args := []string{"-D", "WORK=" + work, "-D", "HOME=" + home, "-D", "SCRATCH=" + scratch, "-D", "SHIMS=" + shims}
	var p strings.Builder
	p.WriteString(`(version 1)
(allow default)
(deny file-write*)
(allow file-write* (subpath (param "WORK")) (subpath (param "SCRATCH")) (subpath "/dev"))
(deny file-read* (subpath (param "HOME"))`)
	for _, dir := range deniedOnDarwin {
		fmt.Fprintf(&p, " (subpath %q)", dir)
	}
	p.WriteString(`)
(allow file-read* (subpath (param "WORK")) (subpath (param "SCRATCH")) (subpath (param "SHIMS")))
(allow file-read-metadata)
`)
	for i, dir := range hidden {
		name := fmt.Sprintf("HIDDEN%d", i)
		args = append(args, "-D", name+"="+dir)
		fmt.Fprintf(&p, "(deny file-read* file-write* (subpath (param %q)))\n", name)
	}
	if !network {
		p.WriteString("(deny network*)\n")
	}
	return append(args, "-p", p.String())
}

// darwinEnv overrides, ahead of the command, the variables that point at
// directories the profile denies. Programs such as git fail on "operation not
// permitted" where they would ignore a missing file, so HOME moves to scratch
// unless the work directory contains the real one. The shims come first on
// PATH.
func darwinEnv(scratch, shims, path string, homeVisible bool) []string {
	env := []string{"/usr/bin/env", "TMPDIR=" + scratch, "PATH=" + shims + ":" + path}
	if !homeVisible {
		env = append(env, "HOME="+scratch)
	}
	return env
}
