package main

import (
	"maps"
	"slices"
	"strings"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/sandbox"
	"agent-gui/llm"
	"agent-gui/tools"
)

// configSnapshot is a private copy of the settings, safe to use without locks.
type configSnapshot = config.Config

// config returns a copy of the current settings.
func (a *App) config() configSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.cfg
	c.ToolPermissions = maps.Clone(c.ToolPermissions)
	c.MCPServers = slices.Clone(c.MCPServers)
	return c
}

func newProvider(cfg configSnapshot) domain.LLMProvider {
	switch cfg.Provider {
	case config.ProviderOpenAI:
		return llm.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
	case config.ProviderOllama:
		return llm.NewOllamaProvider(cfg.BaseURL)
	default:
		if info, ok := llm.FreeProviderConfigs[string(cfg.Provider)]; ok {
			return llm.NewFreeProvider(info.BaseURL, cfg.APIKey, info.Models)
		}
		return llm.NewOllamaProvider(cfg.BaseURL)
	}
}

func sandboxOptions(cfg configSnapshot) sandbox.Options {
	return sandbox.Options{Mode: sandbox.Mode(cfg.SandboxMode), AllowNetwork: cfg.SandboxNetwork}
}

func sandboxStatusLine(cfg configSnapshot) string {
	return sandbox.Probe(sandboxOptions(cfg)).String()
}

// GetSandboxStatus reports the isolation shell commands currently get.
func (a *App) GetSandboxStatus() sandbox.Status {
	return sandbox.Probe(sandboxOptions(a.config()))
}

func mask(key string) string {
	if len(key) > 4 {
		return "***" + key[len(key)-4:]
	}
	return key
}

func (a *App) GetConfig() config.Config {
	safe := a.config()
	safe.APIKey = mask(safe.APIKey)
	safe.SearchAPIKey = mask(safe.SearchAPIKey)
	return safe
}

// SaveConfig stores the settings. A run in progress keeps the settings it
// started with, except tool permissions, which are read for every call.
func (a *App) SaveConfig(cfg config.Config) string {
	a.mu.Lock()
	// A masked key means "unchanged".
	if strings.HasPrefix(cfg.APIKey, "***") {
		cfg.APIKey = a.cfg.APIKey
	}
	if strings.HasPrefix(cfg.SearchAPIKey, "***") {
		cfg.SearchAPIKey = a.cfg.SearchAPIKey
	}
	a.cfg = cfg
	a.mu.Unlock()
	a.mcp.Sync(cfg.MCPServers)
	return errString(config.Save(cfg))
}

// ── MCP server management ────────────────────────────────────────────────────

func (a *App) ListMCPServers() []config.MCPServerConfig {
	return a.config().MCPServers
}

func (a *App) SaveMCPServers(servers []config.MCPServerConfig) string {
	a.mu.Lock()
	a.cfg.MCPServers = servers
	cfg := a.cfg
	a.mu.Unlock()
	a.mcp.Sync(servers) // stop servers that changed or were disabled
	return errString(config.Save(cfg))
}

// GetMCPStatus reports the state of each configured server.
func (a *App) GetMCPStatus() []tools.MCPStatus {
	a.mcp.Check(a.ctx)
	return a.mcp.Status(a.config().MCPServers)
}

// RestartMCPServer (re)starts the named server and rediscovers its tools. It
// returns an error message or "".
func (a *App) RestartMCPServer(name string) string {
	for _, srv := range a.config().MCPServers {
		if srv.Name == name {
			return errString(a.mcp.Restart(a.ctx, srv))
		}
	}
	return "no such server"
}

// ProbeMCPServer starts a server temporarily, lists its tools, and returns their names.
func (a *App) ProbeMCPServer(srv config.MCPServerConfig) ([]string, error) {
	return tools.ProbeMCPServer(srv)
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a *App) ListTools() []ToolInfo {
	reg := tools.DefaultRegistry("/")
	all := reg.All()
	out := make([]ToolInfo, len(all))
	for i, t := range all {
		out[i] = ToolInfo{Name: t.Name(), Description: t.Description()}
	}
	return out
}
