//go:build !windows

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The shim is run over a stand-in that behaves like macOS mktemp: without an
// explicit location it tries a directory that the sandbox denies.
func TestMktempShimHonoursTMPDIR(t *testing.T) {
	realMktemp, err := exec.LookPath("mktemp")
	if err != nil {
		t.Skip("mktemp not installed")
	}
	dir := t.TempDir()
	tmp, other := filepath.Join(dir, "tmp"), filepath.Join(dir, "other")
	for _, d := range []string{tmp, other} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	macLike := write("real", "#!/bin/sh\nfor a; do case $a in */*) exec "+realMktemp+
		" \"$@\";; esac; done\necho 'mkstemp failed: Operation not permitted' >&2; exit 1\n")
	shim := write("shim", mktempShim(macLike))

	run := func(args ...string) (string, error) {
		cmd := exec.Command(shim, args...)
		cmd.Env = []string{"TMPDIR=" + tmp, "PATH=/usr/bin:/bin"}
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for _, tc := range []struct {
		args   []string
		prefix string
		isDir  bool
		exists bool
	}{
		{nil, "tmp.", false, true},
		{[]string{"-d"}, "tmp.", true, true},
		{[]string{"-q", "-d"}, "tmp.", true, true},
		{[]string{"-t", "build"}, "build.", false, true},
		{[]string{"-dt", "build"}, "build.", true, true},
		{[]string{"-u"}, "tmp.", false, false},
	} {
		out, err := run(tc.args...)
		if err != nil || filepath.Dir(out) != tmp || !strings.HasPrefix(filepath.Base(out), tc.prefix) {
			t.Errorf("mktemp %v = %q, %v", tc.args, out, err)
			continue
		}
		info, statErr := os.Stat(out)
		if (statErr == nil) != tc.exists || (tc.exists && info.IsDir() != tc.isDir) {
			t.Errorf("mktemp %v: exists=%v dir=%v", tc.args, statErr == nil, tc.isDir)
		}
	}
	// An explicit location is the caller's choice and is passed through.
	out, err := run(filepath.Join(other, "mine.XXXXXX"))
	if err != nil || filepath.Dir(out) != other {
		t.Errorf("explicit template = %q, %v", out, err)
	}
}
