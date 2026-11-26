package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/diag"
	"agent-gui/internal/history"
	"agent-gui/internal/redact"
	"agent-gui/internal/store"
)

const (
	maxToolRecords = 500
	maxRecordText  = 4000
)

var secretName = regexp.MustCompile(`(?i)(pass|secret|token|api[_-]?key|authorization|credential|cookie)`)

// safeArgs renders arguments for the timeline with sensitive values hidden.
func safeArgs(args domain.Args) string {
	safe := domain.Args{}
	for k, v := range args {
		if secretName.MatchString(k) {
			v = "[REDACTED]"
		}
		safe[k] = v
	}
	return clip(redact.String(safe.Format()))
}

func clip(s string) string {
	if r := []rune(s); len(r) > maxRecordText {
		return string(r[:maxRecordText]) + "…"
	}
	return s
}

func toolRecord(runID string, e appLayer.ToolEvent) history.ToolRecord {
	return history.ToolRecord{
		ID: runID + "-" + e.ID, RunID: runID, Tool: e.Tool, Status: e.Status, Permission: e.Permission,
		Started: e.Started, Millis: e.Duration.Milliseconds(),
		Args: safeArgs(e.Args), Result: clip(redact.String(e.Result)),
	}
}

// upsert replaces the record with the same ID or appends it.
func upsert(records []history.ToolRecord, r history.ToolRecord) []history.ToolRecord {
	for i := range records {
		if records[i].ID == r.ID {
			records[i] = r
			return records
		}
	}
	return append(records, r)
}

// GetToolTimeline returns the tool calls of a conversation, oldest first,
// including those of a run in progress.
func (a *App) GetToolTimeline(conversationID string) ([]history.ToolRecord, error) {
	a.convMu.Lock()
	defer a.convMu.Unlock()
	conv, err := history.Load(conversationID)
	if err != nil {
		return nil, err
	}
	out := append([]history.ToolRecord{}, conv.Timeline...)
	if r := a.runningIn(conversationID); r != nil {
		r.mu.Lock()
		out = append(out, r.tools...)
		r.mu.Unlock()
	}
	return out, nil
}

func logDir() string { return filepath.Join(store.DataDir(), "logs") }

// Diagnostics is what an exported report contains. It deliberately has no
// conversation content, keys, addresses or MCP commands.
type Diagnostics struct {
	Generated string           `json:"generated"`
	Version   string           `json:"version"`
	OS        string           `json:"os"`
	Settings  map[string]any   `json:"settings"`
	Sandbox   string           `json:"sandbox"`
	MCP       []map[string]any `json:"mcp"`
	Log       []string         `json:"log"`
}

func (a *App) diagnostics() Diagnostics {
	cfg := a.config()
	d := Diagnostics{
		Generated: time.Now().UTC().Format(time.RFC3339),
		Version:   Version,
		OS:        runtime.GOOS + "/" + runtime.GOARCH,
		Settings: map[string]any{
			"provider": cfg.Provider, "model": cfg.Model, "context_limit": cfg.ContextLimit,
			"context_strategy": cfg.ContextStrategy, "tool_calling": cfg.ToolCalling,
			"sandbox_mode": cfg.SandboxMode, "sandbox_network": cfg.SandboxNetwork,
		},
		Sandbox: sandboxStatusLine(cfg),
		MCP:     []map[string]any{},
		Log:     diag.Tail(logDir(), 500),
	}
	for _, s := range a.mcp.Status(cfg.MCPServers) {
		d.MCP = append(d.MCP, map[string]any{"name": s.Name, "state": s.State, "tools": len(s.Tools), "last_error": redact.String(s.LastError)})
	}
	return d
}

// ExportDiagnostics saves a report to a file the user picks. It returns "ok",
// an error message, or "" if the dialog was cancelled.
func (a *App) ExportDiagnostics() string {
	dest, err := wails.SaveFileDialog(a.ctx, wails.SaveDialogOptions{
		Title: "Export Diagnostics", DefaultFilename: "ai-agent-diagnostics.json",
	})
	if err != nil || dest == "" {
		return ""
	}
	data, err := json.MarshalIndent(a.diagnostics(), "", "  ")
	if err == nil {
		err = os.WriteFile(dest, data, 0o600)
	}
	if err != nil {
		return "Export failed: " + err.Error()
	}
	return "ok"
}
