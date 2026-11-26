package tools

import (
	"agent-gui/domain"
	"agent-gui/internal/config"
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
	r := domain.NewToolRegistry()
	r.Register(NewReadFile(workDir, protected...))
	r.Register(NewWriteFile(workDir, protected...))
	r.Register(NewListDir(workDir, protected...))
	r.Register(NewSearchFiles(workDir, protected...))
	r.Register(NewDeleteFile(workDir, protected...))
	r.Register(NewShell(workDir, protected...))
	r.Register(NewGetTime())
	r.Register(NewReadDocument(workDir, protected...))
	r.Register(NewWebSearch(search.Provider, search.APIKey))

	for _, t := range LoadMCPTools(mcpServers) {
		r.Register(t)
	}
	return r
}
