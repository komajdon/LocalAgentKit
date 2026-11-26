package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/redact"
)

const (
	mcpConnectTimeout = time.Minute // npx may have to download the server
	maxStarts         = 3           // per restartWindow, for automatic restarts
	restartWindow     = 5 * time.Minute
)

// MCP connection states.
const (
	MCPConnected    = "connected"
	MCPDisconnected = "disconnected"
	MCPFailed       = "error"
	MCPDisabled     = "disabled"
)

// MCPStatus describes one configured server.
type MCPStatus struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Tools     []string `json:"tools"`
	LastError string   `json:"last_error"`
}

// MCPPool shares one server process per distinct configuration (name, command,
// arguments and environment) among all conversations. It is safe for
// concurrent use.
type MCPPool struct {
	mu      sync.Mutex
	entries map[string]*mcpEntry
	closed  bool
}

// NewMCPPool returns an empty pool. Servers start when first needed.
func NewMCPPool() *MCPPool { return &MCPPool{entries: map[string]*mcpEntry{}} }

type mcpEntry struct {
	cfg config.MCPServerConfig

	// connectMu serialises starting, so one configuration starts only once.
	connectMu sync.Mutex

	mu      sync.Mutex
	client  *mcpClient
	defs    []mcpToolDef
	lastErr string
	starts  []time.Time
	checked time.Time // last health check
	retired bool      // removed from the configuration or pool shut down
}

func configKey(c config.MCPServerConfig) string {
	// Marshalling strings cannot fail, and map keys are sorted.
	data, _ := json.Marshal([]any{c.Name, c.Command, c.Args, c.Env})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func usable(c config.MCPServerConfig) bool { return c.Enabled && c.Command != "" }

func (p *MCPPool) entry(c config.MCPServerConfig) (*mcpEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("MCP servers are shutting down")
	}
	key := configKey(c)
	e, ok := p.entries[key]
	if !ok {
		e = &mcpEntry{cfg: c}
		p.entries[key] = e
	}
	return e, nil
}

func (e *mcpEntry) alive() *mcpClient {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client == nil {
		return nil
	}
	select {
	case <-e.client.done:
		return nil
	default:
		return e.client
	}
}

// connect returns the running client, starting the server if needed. Automatic
// restarts are limited so a crashing server cannot loop.
func (e *mcpEntry) connect(ctx context.Context) (*mcpClient, error) {
	if c := e.alive(); c != nil {
		return c, nil
	}
	e.connectMu.Lock()
	defer e.connectMu.Unlock()
	if c := e.alive(); c != nil {
		return c, nil
	}

	e.mu.Lock()
	if e.retired {
		e.mu.Unlock()
		return nil, errors.New("this MCP server was removed or disabled")
	}
	var recent []time.Time
	for _, t := range e.starts {
		if time.Since(t) < restartWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= maxStarts {
		last := e.lastErr
		e.mu.Unlock()
		return nil, fmt.Errorf("MCP server %q failed %d times; restart it in Settings (%s)", e.cfg.Name, maxStarts, last)
	}
	e.starts = append(recent, time.Now())
	old := e.client
	e.client = nil
	e.mu.Unlock()
	if old != nil {
		old.close() // reap the dead process
	}

	ctx, cancel := context.WithTimeout(ctx, mcpConnectTimeout)
	defer cancel()
	client, err := startMCPClient(ctx, e.cfg)
	var defs []mcpToolDef
	if err == nil {
		if defs, err = client.ListTools(ctx); err != nil {
			client.close()
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		err = redact.Error(err)
		e.lastErr = err.Error()
		return nil, err
	}
	if e.retired {
		go client.close()
		return nil, errors.New("this MCP server was removed or disabled")
	}
	e.client, e.defs, e.lastErr = client, defs, ""
	return client, nil
}

func (e *mcpEntry) retire() {
	e.mu.Lock()
	e.retired = true
	client := e.client
	e.client = nil
	e.mu.Unlock()
	if client != nil {
		client.close()
	}
}

// refresh rediscovers the tools if the server announced a change.
func (e *mcpEntry) refresh(ctx context.Context, client *mcpClient) {
	if !client.stale.Swap(false) {
		return
	}
	defs, err := client.ListTools(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		client.stale.Store(true) // try again next time
		e.lastErr = redact.String(err.Error())
		return
	}
	e.defs, e.lastErr = defs, ""
}

// healthInterval is how often a running server is pinged; tests shorten it.
var (
	healthInterval = 30 * time.Second
	healthTimeout  = 5 * time.Second
)

// Check pings connected servers that have not been checked recently and stops
// those that do not answer; they restart when next used. Any reply, even
// "method not found", counts as alive.
func (p *MCPPool) Check(ctx context.Context) {
	p.mu.Lock()
	var entries []*mcpEntry
	for _, e := range p.entries {
		entries = append(entries, e)
	}
	p.mu.Unlock()
	var wg sync.WaitGroup
	for _, e := range entries {
		client := e.alive()
		e.mu.Lock()
		due := client != nil && time.Since(e.checked) >= healthInterval
		if due {
			e.checked = time.Now()
		}
		e.mu.Unlock()
		if !due {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, healthTimeout)
			defer cancel()
			_, err := client.callMethod(ctx, "ping", nil)
			var reply *rpcError
			if err == nil || errors.As(err, &reply) {
				e.refresh(ctx, client)
				return
			}
			e.mu.Lock()
			if e.client == client {
				e.client, e.lastErr = nil, "the server stopped responding"
			}
			e.mu.Unlock()
			client.close()
		}()
	}
	wg.Wait()
}

// Tools returns the tools of every enabled server, starting those not yet
// running in parallel. A server that fails is left out; see Status.
func (p *MCPPool) Tools(ctx context.Context, servers []config.MCPServerConfig) []domain.Tool {
	perServer := make([][]domain.Tool, len(servers))
	var wg sync.WaitGroup
	for i, cfg := range servers {
		if !usable(cfg) {
			continue
		}
		e, err := p.entry(cfg)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := e.connect(ctx)
			if err != nil {
				return
			}
			e.refresh(ctx, client)
			e.mu.Lock()
			defer e.mu.Unlock()
			for _, def := range e.defs {
				perServer[i] = append(perServer[i], &mcpTool{serverName: e.cfg.Name, def: def, entry: e})
			}
		}()
	}
	wg.Wait()
	var out []domain.Tool
	for _, t := range perServer {
		out = append(out, t...)
	}
	return out
}

// Sync stops servers whose configuration is no longer present and enabled.
func (p *MCPPool) Sync(servers []config.MCPServerConfig) {
	keep := map[string]bool{}
	for _, c := range servers {
		if usable(c) {
			keep[configKey(c)] = true
		}
	}
	p.mu.Lock()
	var gone []*mcpEntry
	for key, e := range p.entries {
		if !keep[key] {
			gone = append(gone, e)
			delete(p.entries, key)
		}
	}
	p.mu.Unlock()
	retireAll(gone)
}

// Restart stops the server, forgets its failures and starts it again,
// rediscovering its tools.
func (p *MCPPool) Restart(ctx context.Context, c config.MCPServerConfig) error {
	if !usable(c) {
		return errors.New("the server is disabled")
	}
	p.mu.Lock()
	e := p.entries[configKey(c)]
	p.mu.Unlock()
	if e != nil {
		e.connectMu.Lock()
		e.mu.Lock()
		client := e.client
		e.client, e.starts, e.lastErr = nil, nil, ""
		e.mu.Unlock()
		e.connectMu.Unlock()
		if client != nil {
			client.close()
		}
	}
	e, err := p.entry(c)
	if err != nil {
		return err
	}
	_, err = e.connect(ctx)
	return err
}

// Status reports each configured server without starting any.
func (p *MCPPool) Status(servers []config.MCPServerConfig) []MCPStatus {
	out := make([]MCPStatus, 0, len(servers))
	for _, c := range servers {
		s := MCPStatus{Name: c.Name, State: MCPDisconnected, Tools: []string{}}
		p.mu.Lock()
		e := p.entries[configKey(c)]
		p.mu.Unlock()
		switch {
		case !usable(c):
			s.State = MCPDisabled
		case e != nil:
			connected := e.alive() != nil
			e.mu.Lock()
			s.LastError = e.lastErr
			switch {
			case connected:
				s.State = MCPConnected
				for _, d := range e.defs {
					s.Tools = append(s.Tools, d.Name)
				}
			case e.lastErr != "":
				s.State = MCPFailed
			case e.client != nil:
				s.State, s.LastError = MCPFailed, "the server process exited"
			}
			e.mu.Unlock()
		}
		out = append(out, s)
	}
	return out
}

// Close stops every server. The pool cannot be used afterwards.
func (p *MCPPool) Close() {
	p.mu.Lock()
	p.closed = true
	var all []*mcpEntry
	for _, e := range p.entries {
		all = append(all, e)
	}
	p.entries = map[string]*mcpEntry{}
	p.mu.Unlock()
	retireAll(all)
}

func retireAll(entries []*mcpEntry) {
	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.retire()
		}()
	}
	wg.Wait()
}
