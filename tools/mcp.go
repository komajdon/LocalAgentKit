package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/redact"
	"agent-gui/internal/sandbox"
)

const mcpCallTimeout = 30 * time.Second

// mcpClient manages a long-lived stdio subprocess implementing the MCP protocol.
type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	mu      sync.Mutex
	pending map[int64]chan jsonRPCResponse
	nextID  atomic.Int64
	done    chan struct{} // closed when readLoop exits

	// writeMu serialises writes to stdin so concurrent tool calls (parallel
	// tool execution) cannot interleave bytes of different JSON-RPC frames.
	writeMu sync.Mutex
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"` // nil for notifications
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func startMCPClient(ctx context.Context, cfg config.MCPServerConfig) (*mcpClient, error) {
	// Not tied to ctx: the process outlives the call that started it.
	cmd := exec.CommandContext(context.Background(), cfg.Command, cfg.Args...)
	// Launchers such as npx start child processes that must die with them.
	sandbox.ProcessTree(cmd)

	// Only pass explicitly configured env vars — do not inherit parent environment.
	// This prevents leaking API keys or secrets from the parent process.
	env := make([]string, 0, len(cfg.Env))
	// PATH is required for the subprocess to find executables.
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	if h := os.Getenv("HOME"); h != "" {
		env = append(env, "HOME="+h)
	}
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Discard MCP subprocess stderr — it may contain connection strings, passwords,
	// or query logs from database drivers. Protocol errors are surfaced via JSON-RPC responses.
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MCP server %q: %w", cfg.Name, err)
	}

	c := &mcpClient{
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewReader(stdout),
		pending: make(map[int64]chan jsonRPCResponse),
		done:    make(chan struct{}),
	}

	go c.readLoop()

	// MCP initialize handshake
	type initParams struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
		Capabilities struct{} `json:"capabilities"`
	}
	p := initParams{ProtocolVersion: "2024-11-05"}
	p.ClientInfo.Name = "agent-gui"
	p.ClientInfo.Version = "1.0.0"
	if _, err := c.callMethod(ctx, "initialize", p); err != nil {
		c.close()
		return nil, fmt.Errorf("MCP initialize %q: %w", cfg.Name, err)
	}
	_ = c.notify("notifications/initialized", nil)

	return c, nil
}

// readLoop dispatches incoming JSON-RPC responses to waiting callers.
// When the pipe closes (process died), all pending channels receive an error sentinel.
func (c *mcpClient) readLoop() {
	defer func() {
		close(c.done)
		// Drain all pending callers with an error so they don't hang forever.
		c.mu.Lock()
		for id, ch := range c.pending {
			ch <- jsonRPCResponse{ID: id, Error: &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{Code: -32000, Message: "MCP server process exited"}}
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()

	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			fmt.Fprintf(os.Stderr, "MCP: invalid JSON from server: %v\n", err)
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

// callMethod sends a request and waits for its reply, the call timeout or ctx.
func (c *mcpClient) callMethod(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := c.nextID.Add(1)
	ch := make(chan jsonRPCResponse, 1)

	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP server is not running")
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}

	data, err := json.Marshal(jsonRPCRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		forget()
		return nil, fmt.Errorf("MCP marshal: %w", err)
	}
	c.writeMu.Lock()
	_, werr := c.stdin.Write(append(data, '\n'))
	c.writeMu.Unlock()
	if werr != nil {
		forget()
		return nil, werr
	}

	timeout := time.NewTimer(mcpCallTimeout)
	defer timeout.Stop()
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		forget()
		// Ask the server to stop working on it.
		_ = c.notify("notifications/cancelled", map[string]any{"requestId": id, "reason": "canceled by user"})
		return nil, ctx.Err()
	case <-timeout.C:
		forget()
		return nil, fmt.Errorf("MCP call timed out after %s", mcpCallTimeout)
	}
}

func (c *mcpClient) notify(method string, params any) error {
	req := jsonRPCRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	_, err = c.stdin.Write(data)
	c.writeMu.Unlock()
	return err
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	// Wait for readLoop to drain pending channels before proceeding.
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		_ = c.cmd.Cancel()
	}
	_ = c.cmd.Wait()
}

// ListTools queries the MCP server for its tool list.
func (c *mcpClient) ListTools(ctx context.Context) ([]mcpToolDef, error) {
	raw, err := c.callMethod(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []mcpToolDef `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("MCP tools/list parse: %w", err)
	}
	return result.Tools, nil
}

type mcpToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// CallTool invokes a named tool with the given arguments.
func (c *mcpClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	type callParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	raw, err := c.callMethod(ctx, "tools/call", callParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw), nil
	}
	var parts []string
	for _, item := range result.Content {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	out := strings.Join(parts, "\n")
	if result.IsError {
		return "ERROR: " + out, nil
	}
	return out, nil
}

// ── mcpTool wraps a single MCP tool as a domain.Tool ────────────────────────

type mcpTool struct {
	serverName string
	def        mcpToolDef
	entry      *mcpEntry
}

func (t *mcpTool) Name() string { return t.serverName + "__" + t.def.Name }
func (t *mcpTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.def.Description)
}
func (t *mcpTool) Parameters() string { return string(t.def.InputSchema) }

func (t *mcpTool) Execute(ctx context.Context, args domain.Args) string {
	// Reconnects if the server has exited since the tool was listed.
	client, err := t.entry.connect(ctx)
	if err != nil {
		return "MCP call failed: " + redact.String(err.Error())
	}
	result, err := client.CallTool(ctx, t.def.Name, args)
	if err != nil {
		return "MCP call failed: " + redact.String(err.Error())
	}
	return result
}

// ProbeMCPServer starts a server temporarily, lists its tools, and shuts it down.
func ProbeMCPServer(cfg config.MCPServerConfig) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
	defer cancel()
	client, err := startMCPClient(ctx, cfg)
	if err != nil {
		return nil, redact.Error(err)
	}
	defer client.close()

	defs, err := client.ListTools(ctx)
	if err != nil {
		return nil, redact.Error(err)
	}
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names, nil
}
