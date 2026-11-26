package tools

import (
	"context"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/sandbox"
)

// DefaultRegistry builds a ToolRegistry with the built-in tools for workDir.
func DefaultRegistry(workDir string) *domain.ToolRegistry {
	return NewRegistry(Options{WorkDir: workDir, Sandbox: sandbox.Options{Mode: sandbox.ModeAuto}})
}

// SearchConfig configures the built-in web search tool.
type SearchConfig struct {
	Provider string // duckduckgo (default) | brave | serpapi
	APIKey   string
}

// Options configures NewRegistry.
type Options struct {
	WorkDir    string
	MCPServers []config.MCPServerConfig
	Search     SearchConfig
	Sandbox    sandbox.Options // isolation for the shell tool
	Protected  []string        // directories no built-in tool may touch
	// Ignore holds workspace patterns the file tools must not list or open.
	Ignore []string
	// Journal records the file tools' changes.
	Journal Journal
	// MCP provides the servers' tools; without it MCPServers is ignored.
	MCP *MCPPool
	// Context bounds starting MCP servers.
	Context context.Context
}

// NewRegistry builds a registry of the built-in and MCP tools.
func NewRegistry(o Options) *domain.ToolRegistry {
	r := domain.NewToolRegistry()
	files := workspace{dir: o.WorkDir, protected: o.Protected, ignore: o.Ignore}
	r.Register(&readFileTool{files})
	r.Register(&writeFileTool{ws: files, journal: o.Journal})
	r.Register(&listDirTool{files})
	r.Register(&searchFilesTool{files})
	r.Register(&deleteFileTool{ws: files, journal: o.Journal})
	r.Register(NewShell(o.WorkDir, o.Sandbox, o.Protected...))
	r.Register(NewGetTime())
	r.Register(&readDocumentTool{files})
	r.Register(NewWebSearch(o.Search.Provider, o.Search.APIKey))

	if o.MCP != nil {
		ctx := o.Context
		if ctx == nil {
			ctx = context.Background()
		}
		for _, t := range o.MCP.Tools(ctx, o.MCPServers) {
			r.Register(t)
		}
	}
	return r
}
