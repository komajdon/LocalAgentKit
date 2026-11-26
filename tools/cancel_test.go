package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/sandbox"
)

func cancelSoon() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 200*time.Millisecond)
}

func within(t *testing.T, limit time.Duration, f func() string) string {
	t.Helper()
	start := time.Now()
	out := f()
	if took := time.Since(start); took > limit {
		t.Fatalf("took %v, cancellation ignored", took)
	}
	return out
}

func TestShellStopsOnCancel(t *testing.T) {
	for _, mode := range []sandbox.Mode{sandbox.ModeOff, sandbox.ModeAuto} {
		ctx, cancel := cancelSoon()
		shell := NewShell(t.TempDir(), sandbox.Options{Mode: mode})
		out := within(t, 3*time.Second, func() string {
			return shell.Execute(ctx, domain.Args{"command": "sleep 20"})
		})
		cancel()
		if !strings.Contains(out, "canceled") && !strings.Contains(out, "timed out") {
			t.Errorf("mode %s: %q", mode, out)
		}
	}
}

func TestSearchHonoursCancel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("needle"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := NewSearchFiles(dir).Execute(ctx, domain.Args{"pattern": "needle"})
	if !strings.Contains(out, "canceled") {
		t.Fatalf("out = %q", out)
	}
}

func TestMCPCallCancelNotifiesServerAndPassesTypes(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	client, err := startMCPClient(context.Background(), fakeMCP("f", log))
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()

	got, err := client.CallTool(context.Background(), "echo", domain.Args{"s": "5", "n": 7, "l": []any{true}})
	if err != nil || got != `{"l":[true],"n":7,"s":"5"}` {
		t.Fatalf("echo = %q, %v", got, err)
	}

	ctx, cancel := cancelSoon()
	defer cancel()
	within(t, 2*time.Second, func() string {
		_, err = client.CallTool(ctx, "hang", nil)
		return ""
	})
	if err == nil {
		t.Fatal("want cancellation error")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, _ := os.ReadFile(log)
		if strings.Contains(string(data), "notifications/cancelled") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not told about cancellation: %s", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
