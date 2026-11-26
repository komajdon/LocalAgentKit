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

// mktempShim is a mktemp that honours TMPDIR. The macOS one creates files in
// the user's shared temporary directory whatever TMPDIR says, which the profile
// denies. Calls that name a location are passed through unchanged. Programs
// that ask the system for that directory by other means are still refused.
func mktempShim(real string) string {
	return `#!/bin/sh
real=` + real + `
flags=
prefix=tmp
want_prefix=
for a in "$@"; do
	if [ -n "$want_prefix" ]; then prefix=$a; want_prefix=; continue; fi
	case $a in
	-d|-q|-u|-dq|-qd|-du|-ud|-qu|-uq) flags="$flags $a" ;;
	-t) want_prefix=1 ;;
	-dt) flags="$flags -d"; want_prefix=1 ;;
	-qt) flags="$flags -q"; want_prefix=1 ;;
	*) exec "$real" "$@" ;;
	esac
done
if [ -n "$want_prefix" ]; then exec "$real" "$@"; fi
exec "$real" $flags "${TMPDIR:-/tmp}/$prefix.XXXXXXXXXX"
`
}
