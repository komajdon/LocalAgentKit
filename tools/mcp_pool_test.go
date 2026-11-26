package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/config"
)

func starts(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Count(string(data), "start\n")
}

func find(t *testing.T, list []domain.Tool, name string) domain.Tool {
	t.Helper()
	for _, tool := range list {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("no tool %s", name)
	return nil
}

func call(tool domain.Tool) string {
	return tool.Execute(context.Background(), domain.Args{"x": 1})
}

func newPool(t *testing.T) *MCPPool {
	t.Helper()
	p := NewMCPPool()
	t.Cleanup(p.Close)
	return p
}

func TestPoolStartsOneProcessPerConfiguration(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	p := newPool(t)
	servers := []config.MCPServerConfig{fakeMCP("a", log)}
	var wg sync.WaitGroup
	for range 20 { // twenty conversations at once
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := len(p.Tools(context.Background(), servers)); got != 3 {
				t.Errorf("tools = %d", got)
			}
		}()
	}
	wg.Wait()
	if n := starts(t, log); n != 1 {
		t.Fatalf("started %d processes", n)
	}
	st := p.Status(servers)
	if st[0].State != MCPConnected || len(st[0].Tools) != 3 {
		t.Fatalf("status = %+v", st[0])
	}
}

func TestPoolReconnectsAfterExitButNotForever(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	p := newPool(t)
	servers := []config.MCPServerConfig{fakeMCP("a", log)}
	list := p.Tools(context.Background(), servers)
	echo, die := find(t, list, "a__echo"), find(t, list, "a__die")

	for i := 1; i < maxStarts; i++ {
		if out := call(die); !strings.Contains(out, "failed") {
			t.Fatalf("die: %q", out)
		}
		if out := call(echo); out != `{"x":1}` {
			t.Fatalf("after exit %d: %q", i, out)
		}
	}
	call(die)
	if out := call(echo); !strings.Contains(out, "restart it in Settings") {
		t.Fatalf("restart loop not bounded: %q", out)
	}
	if n := starts(t, log); n != maxStarts {
		t.Fatalf("started %d times", n)
	}
	if st := p.Status(servers); st[0].State != MCPFailed || st[0].LastError == "" {
		t.Fatalf("status = %+v", st[0])
	}
	if err := p.Restart(context.Background(), servers[0]); err != nil {
		t.Fatal(err)
	}
	if out := call(echo); out != `{"x":1}` {
		t.Fatalf("after manual restart: %q", out)
	}
}

func TestPoolFollowsConfigurationChanges(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	p := newPool(t)
	first := fakeMCP("a", log)
	old := find(t, p.Tools(context.Background(), []config.MCPServerConfig{first}), "a__echo")

	changed := fakeMCP("a", log)
	changed.Env["EXTRA"] = "1"
	p.Sync([]config.MCPServerConfig{changed})
	if out := call(old); !strings.Contains(out, "removed or disabled") {
		t.Fatalf("old tool still works: %q", out)
	}
	next := p.Tools(context.Background(), []config.MCPServerConfig{changed})
	if out := call(find(t, next, "a__echo")); out != `{"x":1}` || starts(t, log) != 2 {
		t.Fatalf("out=%q starts=%d", out, starts(t, log))
	}

	changed.Enabled = false
	p.Sync([]config.MCPServerConfig{changed})
	if got := p.Tools(context.Background(), []config.MCPServerConfig{changed}); len(got) != 0 {
		t.Fatal("disabled server still offers tools")
	}
	if st := p.Status([]config.MCPServerConfig{changed}); st[0].State != MCPDisabled {
		t.Fatalf("status = %+v", st[0])
	}
}

func TestPoolReportsBrokenServerAndClose(t *testing.T) {
	p := NewMCPPool()
	broken := []config.MCPServerConfig{{Name: "b", Command: "/no/such/binary", Enabled: true}}
	if got := p.Tools(context.Background(), broken); len(got) != 0 {
		t.Fatal("tools from a broken server")
	}
	if st := p.Status(broken); st[0].State != MCPFailed || st[0].LastError == "" {
		t.Fatalf("status = %+v", st[0])
	}
	p.Close()
	log := filepath.Join(t.TempDir(), "log")
	if got := p.Tools(context.Background(), []config.MCPServerConfig{fakeMCP("a", log)}); len(got) != 0 || starts(t, log) != 0 {
		t.Fatal("closed pool started a server")
	}
}

func TestPoolRefreshesToolsAndDetectsFrozenServers(t *testing.T) {
	interval, timeout := healthInterval, healthTimeout
	healthInterval, healthTimeout = 0, 300*time.Millisecond
	defer func() { healthInterval, healthTimeout = interval, timeout }()

	log := filepath.Join(t.TempDir(), "log")
	p := newPool(t)
	servers := []config.MCPServerConfig{fakeMCP("a", log)}
	e, err := p.entry(servers[0])
	if err != nil {
		t.Fatal(err)
	}
	client, err := e.connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CallTool(context.Background(), "grow", nil); err != nil {
		t.Fatal(err)
	}
	p.Check(context.Background())
	if got := p.Status(servers)[0]; got.State != MCPConnected || len(got.Tools) != 4 {
		t.Fatalf("after list_changed: %+v", got)
	}

	if _, err := client.CallTool(context.Background(), "freeze", nil); err != nil {
		t.Fatal(err)
	}
	p.Check(context.Background())
	if got := p.Status(servers)[0]; got.State != MCPFailed || !strings.Contains(got.LastError, "stopped responding") {
		t.Fatalf("frozen server: %+v", got)
	}
}
