package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"agent-gui/internal/config"
)

// TestMain lets the test binary act as a stdio MCP server, so the client can
// be tested against a real subprocess.
func TestMain(m *testing.M) {
	if os.Getenv("LAK_FAKE_MCP") == "1" {
		fakeMCPServer()
		return
	}
	os.Exit(m.Run())
}

// fakeMCP returns a server configuration running this test binary.
func fakeMCP(name, logPath string) config.MCPServerConfig {
	return config.MCPServerConfig{
		Name: name, Command: os.Args[0], Enabled: true,
		Env: map[string]string{"LAK_FAKE_MCP": "1", "LAK_MCP_LOG": logPath},
	}
}

// fakeMCPServer logs "start" and each notification, and offers the tools echo,
// hang (never answers) and die (exits).
func fakeMCPServer() {
	logf := func(s string) {
		if f, err := os.OpenFile(os.Getenv("LAK_MCP_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = fmt.Fprintln(f, s)
			_ = f.Close()
		}
	}
	logf("start")
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var req struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil {
			continue
		}
		if req.ID == nil {
			logf(req.Method)
			continue
		}
		var result any = map[string]any{}
		switch req.Method {
		case "tools/list":
			schema := map[string]any{"type": "object"}
			result = map[string]any{"tools": []map[string]any{
				{"name": "echo", "inputSchema": schema},
				{"name": "hang", "inputSchema": schema},
				{"name": "die", "inputSchema": schema},
			}}
		case "tools/call":
			switch req.Params.Name {
			case "hang":
				continue
			case "die":
				os.Exit(1)
			}
			args, _ := json.Marshal(req.Params.Arguments)
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": string(args)}}}
		}
		out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
		fmt.Println(string(out))
	}
	// Give the parent a moment to observe a clean exit.
	time.Sleep(10 * time.Millisecond)
}
