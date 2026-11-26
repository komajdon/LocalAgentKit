package tools

import (
	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/sandbox"
)

// DefaultRegistry builds a ToolRegistry with all built-in tools for the given workDir.
func DefaultRegistry(workDir string) *domain.ToolRegistry {
	return DefaultRegistryWithMCP(workDir, nil)
}

// SearchConfig configures the built-in web search tool.
type SearchConfig struct {
	Provider string // duckduckgo (default) | brave | serpapi
	APIKey   string
}

// DefaultRegistryWithMCP builds the tool registry, also loading tools from enabled MCP servers.
func DefaultRegistryWithMCP(workDir string, mcpServers []config.MCPServerConfig) *domain.ToolRegistry {
	return DefaultRegistryFull(workDir, mcpServers, SearchConfig{})
}

// DefaultRegistryFull builds the full tool registry including web-search
// configuration. The built-in tools refuse to touch the protected directories.
func DefaultRegistryFull(workDir string, mcpServers []config.MCPServerConfig, search SearchConfig, protected ...string) *domain.ToolRegistry {
	return NewRegistry(Options{
		WorkDir:    workDir,
		MCPServers: mcpServers,
		Search:     search,
		Sandbox:    sandbox.Options{Mode: sandbox.ModeAuto},
		Protected:  protected,
	})
}

// Options configures NewRegistry.
type Options struct {
	WorkDir    string
	MCPServers []config.MCPServerConfig
	Search     SearchConfig
	Sandbox    sandbox.Options // isolation for the shell tool
	Protected  []string        // directories no built-in tool may touch
}

// NewRegistry builds a registry of the built-in and MCP tools.
func NewRegistry(o Options) *domain.ToolRegistry {
	r := domain.NewToolRegistry()
	r.Register(NewReadFile(o.WorkDir, o.Protected...))
	r.Register(NewWriteFile(o.WorkDir, o.Protected...))
	r.Register(NewListDir(o.WorkDir, o.Protected...))
	r.Register(NewSearchFiles(o.WorkDir, o.Protected...))
	r.Register(NewDeleteFile(o.WorkDir, o.Protected...))
	r.Register(NewShell(o.WorkDir, o.Sandbox, o.Protected...))
	r.Register(NewGetTime())
	r.Register(NewReadDocument(o.WorkDir, o.Protected...))
	r.Register(NewWebSearch(o.Search.Provider, o.Search.APIKey))

	for _, t := range LoadMCPTools(o.MCPServers) {
		r.Register(t)
	}
	return r
}
