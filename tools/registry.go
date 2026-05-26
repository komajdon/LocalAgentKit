package tools

import (
	"agent-gui/domain"
	"agent-gui/internal/config"
)

// DefaultRegistry builds a ToolRegistry with all built-in tools for the given workDir.
func DefaultRegistry(workDir string) *domain.ToolRegistry {
	return DefaultRegistryWithMCP(workDir, nil)
}

// DefaultRegistryWithMCP builds the tool registry, also loading tools from enabled MCP servers.
func DefaultRegistryWithMCP(workDir string, mcpServers []config.MCPServerConfig) *domain.ToolRegistry {
	r := domain.NewToolRegistry()
	r.Register(NewReadFile(workDir))
	r.Register(NewWriteFile(workDir))
	r.Register(NewListDir(workDir))
	r.Register(NewSearchFiles(workDir))
	r.Register(NewDeleteFile(workDir))
	r.Register(NewShell(workDir))
	r.Register(NewGetTime())

	for _, t := range LoadMCPTools(mcpServers) {
		r.Register(t)
	}
	return r
}
