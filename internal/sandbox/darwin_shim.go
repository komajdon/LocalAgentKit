//go:build !windows

package sandbox

// Built on other Unix systems too, so that the script can be tested there.

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
