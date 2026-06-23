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

func startMCPClient(cfg config.MCPServerConfig) (*mcpClient, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

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
	if _, err := c.call(p); err != nil {
		_ = cmd.Process.Kill()
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

func (c *mcpClient) call(params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan jsonRPCResponse, 1)

	c.mu.Lock()
	// Check if server has already exited before registering.
	select {
	case <-c.done:
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP server is not running")
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()

	req := jsonRPCRequest{JSONRPC: "2.0", ID: &id, Method: methodForParams(params), Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(mcpCallTimeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP call timed out after %s", mcpCallTimeout)
	}
}

// callMethod sends a method call with explicit method name and params.
func (c *mcpClient) callMethod(method string, params any) (json.RawMessage, error) {
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

	req := jsonRPCRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(mcpCallTimeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
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
	_, err = c.stdin.Write(data)
	return err
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	// Wait for readLoop to drain pending channels before proceeding.
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}

// ListTools queries the MCP server for its tool list.
func (c *mcpClient) ListTools() ([]mcpToolDef, error) {
	raw, err := c.callMethod("tools/list", nil)
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
func (c *mcpClient) CallTool(name string, args map[string]any) (string, error) {
	type callParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	raw, err := c.callMethod("tools/call", callParams{Name: name, Arguments: args})
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
	client     *mcpClient
}

func (t *mcpTool) Name() string        { return t.serverName + "__" + t.def.Name }
func (t *mcpTool) Description() string { return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.def.Description) }
func (t *mcpTool) Parameters() string  { return string(t.def.InputSchema) }

func (t *mcpTool) Execute(args map[string]string) string {
	anyArgs := make(map[string]any, len(args))
	for k, v := range args {
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			anyArgs[k] = parsed
		} else {
			anyArgs[k] = v
		}
	}
	result, err := t.client.CallTool(t.def.Name, anyArgs)
	if err != nil {
		return "MCP call failed: " + err.Error()
	}
	return result
}

// ── mcpPool holds all live MCP clients for shutdown ─────────────────────────

var (
	mcpPoolMu      sync.Mutex
	mcpActiveClients []*mcpClient
)

// ShutdownMCPClients terminates all running MCP server subprocesses. Call on app exit.
func ShutdownMCPClients() {
	mcpPoolMu.Lock()
	clients := mcpActiveClients
	mcpActiveClients = nil
	mcpPoolMu.Unlock()

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(cl *mcpClient) {
			defer wg.Done()
			cl.close()
		}(c)
	}
	wg.Wait()
}

// LoadMCPTools connects to all enabled MCP servers and returns their tools.
func LoadMCPTools(servers []config.MCPServerConfig) []domain.Tool {
	var tools []domain.Tool
	var newClients []*mcpClient

	for _, srv := range servers {
		if !srv.Enabled || srv.Command == "" {
			continue
		}
		client, err := startMCPClient(srv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "MCP: failed to start %q: %v\n", srv.Name, err)
			continue
		}
		newClients = append(newClients, client)

		defs, err := client.ListTools()
		if err != nil {
			fmt.Fprintf(os.Stderr, "MCP: failed to list tools for %q: %v\n", srv.Name, err)
			client.close()
			newClients = newClients[:len(newClients)-1]
			continue
		}
		for _, def := range defs {
			tools = append(tools, &mcpTool{
				serverName: srv.Name,
				def:        def,
				client:     client,
			})
		}
	}

	if len(newClients) > 0 {
		mcpPoolMu.Lock()
		mcpActiveClients = append(mcpActiveClients, newClients...)
		mcpPoolMu.Unlock()
	}

	return tools
}

// ProbeMCPServer starts a server temporarily, lists its tools, and shuts it down.
func ProbeMCPServer(cfg config.MCPServerConfig) ([]string, error) {
	client, err := startMCPClient(cfg)
	if err != nil {
		return nil, err
	}
	defer client.close()

	defs, err := client.ListTools()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names, nil
}

// methodForParams returns the JSON-RPC method name based on struct type.
// This is only used for the initialize call via call(); all others use callMethod().
func methodForParams(params any) string {
	// The only call that goes through call() is initialize.
	return "initialize"
}

// contextCallMethod is a context-aware version for future use.
func (c *mcpClient) contextCallMethod(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

	req := jsonRPCRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	data = append(data, '\n')

	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-time.After(mcpCallTimeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP call timed out after %s", mcpCallTimeout)
	}
}
