//go:build linux || darwin

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func output(t *testing.T, o Options, command string) (string, error) {
	t.Helper()
	cmd, _, err := Command(context.Background(), o, o.WorkDir, command)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestEnvironmentIsScrubbed(t *testing.T) {
	t.Setenv("LAK_SECRET_TOKEN", "hunter2")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("LC_ALL", "C")
	out, err := output(t, Options{Mode: ModeOff, WorkDir: t.TempDir()}, "env")
	if err != nil {
		t.Fatal(err, out)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "sk-test") || !strings.Contains(out, "LC_ALL=C") {
		t.Fatalf("environment not scrubbed:\n%s", out)
	}
}

func TestCancelKillsChildProcesses(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd, _, err := Command(ctx, Options{Mode: ModeOff, WorkDir: dir}, dir, "(sleep 1.5; touch survived) & wait")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := cmd.Run(); err == nil {
		t.Fatal("want error from canceled command")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel waited for the child")
	}
	time.Sleep(1600 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "survived")); err == nil {
		t.Fatal("child process survived cancellation")
	}
}

func TestUnavailableBackend(t *testing.T) {
	orig := findBackend
	findBackend = func() (*backend, error) { return nil, errors.New("simulated") }
	defer func() { findBackend = orig }()
	dir := t.TempDir()

	// Strict, and any unrecognised mode, must fail closed.
	for _, mode := range []Mode{ModeStrict, "", "typo"} {
		cmd, _, err := Command(context.Background(), Options{Mode: mode, WorkDir: dir}, dir, "true")
		if !errors.Is(err, ErrUnavailable) || cmd != nil {
			t.Errorf("mode %q: cmd=%v err=%v", mode, cmd, err)
		}
	}
	// Auto runs, but says it is not isolated.
	_, status, err := Command(context.Background(), Options{Mode: ModeAuto, WorkDir: dir}, dir, "true")
	if err != nil || status.Isolated || !strings.Contains(status.String(), "isolation: none") {
		t.Errorf("auto: status=%v err=%v", status, err)
	}
}

func TestIsolation(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	hidden := filepath.Join(work, "appdata")
	outside := filepath.Join(base, "outside.txt")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{outside: "SECRET-OUTSIDE", filepath.Join(hidden, "key"): "SECRET-KEY"} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	o := Options{Mode: ModeStrict, WorkDir: work, Hidden: []string{hidden}}
	if s := Probe(o); !s.Isolated {
		t.Skip("no isolation backend:", s.Detail)
	}

	out, _ := output(t, o, "cat "+outside+" appdata/key; echo inside > made.txt; echo done")
	if strings.Contains(out, "SECRET") || !strings.Contains(out, "done") {
		t.Fatalf("boundary breached or command failed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(work, "made.txt")); err != nil {
		t.Fatal("work directory should be writable:", err)
	}
	// The temp base lives under the sandbox's private /tmp, so a write there may
	// "succeed" inside; what matters is that the host file is untouched.
	_, _ = output(t, o, "echo x > "+outside)
	if out, err := output(t, o, "echo x > /lak-probe"); err == nil && runtime.GOOS == "linux" {
		t.Fatalf("root filesystem should be read-only: %s", out)
	}
	if body, _ := os.ReadFile(outside); string(body) != "SECRET-OUTSIDE" {
		t.Fatal("outside file changed")
	}
	if out, err := output(t, o, "exec 3<>/dev/tcp/1.1.1.1/53"); err == nil {
		t.Fatalf("network should be blocked: %s", out)
	}
}

func TestOutputIsBounded(t *testing.T) {
	o := &Output{Max: 5}
	n, err := o.Write([]byte("123456789"))
	if n != 9 || err != nil || !strings.HasPrefix(o.String(), "12345\n[") {
		t.Fatalf("n=%d err=%v s=%q", n, err, o.String())
	}
}
