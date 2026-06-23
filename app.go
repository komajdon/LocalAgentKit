package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/history"
	"agent-gui/internal/recorder"
	"agent-gui/internal/store"
	"agent-gui/internal/whisper"
	"agent-gui/llm"
	"agent-gui/tools"
)

// displayItem mirrors the frontend Item type for serialising display history.
type displayItem struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// App is the Wails-bound facade. All public methods are exposed to the frontend.
type App struct {
	ctx context.Context
	cfg config.Config

	mu       sync.Mutex
	agent    *appLayer.ConversationalAgent
	cancelFn context.CancelFunc
	permCh   chan bool

	activeConv *history.Conversation

	recMu  sync.Mutex
	recSes *recorder.Session
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := store.Init(store.DataDir()); err != nil {
		// Surface the error to the user immediately rather than failing silently.
		runtime.EventsEmit(ctx, "app:fatal", "Failed to open data store: "+err.Error())
	}
	a.cfg = config.Load()
}

func (a *App) shutdown(_ context.Context) {
	tools.ShutdownMCPClients()
	_ = store.Close()
}

func (a *App) newProvider() domain.LLMProvider {
	switch a.cfg.Provider {
	case config.ProviderOpenAI:
		return llm.NewOpenAIProvider(a.cfg.BaseURL, a.cfg.APIKey)
	case config.ProviderOllama:
		return llm.NewOllamaProvider(a.cfg.BaseURL)
	default:
		if info, ok := llm.FreeProviderConfigs[string(a.cfg.Provider)]; ok {
			return llm.NewFreeProvider(info.BaseURL, a.cfg.APIKey, info.Models)
		}
		return llm.NewOllamaProvider(a.cfg.BaseURL)
	}
}

func (a *App) buildAgent(workDir string) *appLayer.ConversationalAgent {
	provider := a.newProvider()

	registry := tools.DefaultRegistryWithMCP(workDir, a.cfg.MCPServers)
	agent := appLayer.NewConversationalAgent(a.cfg.Model, provider, registry, a.cfg.SystemPrompt, a.cfg.ContextLimit)

	agent.OnChunk = func(chunk string) {
		runtime.EventsEmit(a.ctx, "chat:chunk", chunk)
	}
	agent.OnToolCall = func(name string, args map[string]string) {
		parts := make([]string, 0, len(args))
		for k, v := range args {
			parts = append(parts, k+"="+v)
		}
		runtime.EventsEmit(a.ctx, "chat:tool_call", map[string]any{
			"tool": name,
			"args": strings.Join(parts, ", "),
		})
	}
	agent.OnToolResult = func(name, result string) {
		runtime.EventsEmit(a.ctx, "chat:tool_result", map[string]any{
			"tool":   name,
			"result": result,
		})
	}
	agent.OnContextUsage = func(usage appLayer.ContextUsage) {
		runtime.EventsEmit(a.ctx, "chat:context_usage", usage)
	}
	agent.OnPermissionRequest = func(req appLayer.PermissionRequest) bool {
		a.mu.Lock()
		perm := a.cfg.ToolPermissions[req.Tool]
		a.mu.Unlock()

		switch perm {
		case config.PermAllow:
			runtime.EventsEmit(a.ctx, "chat:tool_auto_allowed", req.Tool)
			return true
		case config.PermDeny:
			runtime.EventsEmit(a.ctx, "chat:tool_auto_denied", req.Tool)
			return false
		}

		ch := make(chan bool, 1)
		a.mu.Lock()
		a.permCh = ch
		a.mu.Unlock()
		runtime.EventsEmit(a.ctx, "chat:permission_request", req)
		var result bool
		select {
		case result = <-ch:
		case <-time.After(5 * time.Minute):
			// Auto-deny if user never responds within 5 minutes.
			result = false
		}
		a.mu.Lock()
		a.permCh = nil
		a.mu.Unlock()
		return result
	}

	return agent
}

// ── Conversations ─────────────────────────────────────────────────────────────

func (a *App) ListConversations() ([]history.ConvMeta, error) {
	return history.List()
}

func (a *App) NewConversation(workDir string) (*history.Conversation, error) {
	if workDir == "" {
		workDir, _ = os.UserHomeDir()
	}
	now := time.Now()
	conv := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        fmt.Sprintf("%d", now.UnixMilli()),
			Title:     "New conversation",
			WorkDir:   workDir,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages:     []history.SavedMessage{},
		DisplayItems: json.RawMessage("[]"),
	}
	if err := history.Save(conv); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.activeConv = conv
	a.agent = a.buildAgent(workDir)
	a.mu.Unlock()
	return conv, nil
}

func (a *App) LoadConversation(id string) (*history.Conversation, error) {
	conv, err := history.Load(id)
	if err != nil {
		return nil, err
	}
	agent := a.buildAgent(conv.WorkDir)
	for _, m := range conv.Messages {
		agent.InjectMessage(domain.Role(m.Role), m.Content)
	}
	a.mu.Lock()
	a.activeConv = conv
	a.agent = agent
	a.mu.Unlock()
	return conv, nil
}

func (a *App) DeleteConversation(id string) error {
	return history.Delete(id)
}

func (a *App) SearchConversations(query string) ([]history.ConvMeta, error) {
	return history.Search(query)
}

func (a *App) ExportConversation(id, format string) (string, error) {
	conv, err := history.Load(id)
	if err != nil {
		return "", err
	}
	return history.Export(conv, format), nil
}

func (a *App) SetConversationModel(model string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeConv == nil {
		return fmt.Errorf("no active conversation")
	}
	a.activeConv.Model = model
	a.activeConv.UpdatedAt = time.Now()
	a.agent.SetModel(model)
	return history.Save(a.activeConv)
}

func (a *App) GetContextUsage() appLayer.ContextUsage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.agent == nil {
		return appLayer.ContextUsage{}
	}
	limit := a.cfg.ContextLimit
	if limit == 0 {
		limit = 8192
	}
	return appLayer.ContextUsage{Used: a.agent.TokenCount(), Limit: limit}
}

func (a *App) RenameConversation(id, title string) error {
	conv, err := history.Load(id)
	if err != nil {
		return err
	}
	conv.Title = strings.TrimSpace(title)
	if conv.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	conv.UpdatedAt = time.Now()
	if err := history.Save(conv); err != nil {
		return err
	}
	a.mu.Lock()
	if a.activeConv != nil && a.activeConv.ID == id {
		a.activeConv.Title = conv.Title
	}
	a.mu.Unlock()
	return nil
}

func (a *App) UpdateConversationPath(workDir string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeConv == nil {
		return fmt.Errorf("no active conversation")
	}
	a.activeConv.WorkDir = workDir
	a.activeConv.UpdatedAt = time.Now()
	a.agent = a.buildAgent(workDir)
	return history.Save(a.activeConv)
}

// ── Chat ──────────────────────────────────────────────────────────────────────

type ChatResponse struct {
	Error string `json:"error,omitempty"`
}

func (a *App) SendMessage(text string) ChatResponse {
	a.mu.Lock()
	if a.agent == nil {
		a.mu.Unlock()
		return ChatResponse{Error: "no active conversation — create or load one first"}
	}
	if a.cancelFn != nil {
		a.mu.Unlock()
		return ChatResponse{Error: "agent is already running"}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFn = cancel
	conv := a.activeConv
	convID := ""
	if conv != nil {
		convID = conv.ID
	}
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.cancelFn = nil
			a.mu.Unlock()
		}()

		var (
			turnMu   sync.Mutex
			asstBuf  strings.Builder
			turnItems = []displayItem{
				{Type: "msg", Data: map[string]any{"role": "user", "text": text, "streaming": false}},
			}
		)

		a.mu.Lock()
		origChunk := a.agent.OnChunk
		origToolCall := a.agent.OnToolCall
		origToolResult := a.agent.OnToolResult

		a.agent.OnChunk = func(chunk string) {
			turnMu.Lock()
			asstBuf.WriteString(chunk)
			turnMu.Unlock()
			origChunk(chunk)
		}
		a.agent.OnToolCall = func(name string, args map[string]string) {
			parts := make([]string, 0, len(args))
			for k, v := range args {
				parts = append(parts, k+"="+v)
			}
			body := strings.Join(parts, ", ")
			turnMu.Lock()
			if asstBuf.Len() > 0 {
				turnItems = append(turnItems, displayItem{
					Type: "msg",
					Data: map[string]any{"role": "assistant", "text": asstBuf.String(), "streaming": false},
				})
				asstBuf.Reset()
			}
			turnItems = append(turnItems, displayItem{
				Type: "tool",
				Data: map[string]any{"kind": "call", "tool": name, "body": body},
			})
			turnMu.Unlock()
			origToolCall(name, args)
		}
		a.agent.OnToolResult = func(name, result string) {
			turnMu.Lock()
			turnItems = append(turnItems, displayItem{
				Type: "tool",
				Data: map[string]any{"kind": "result", "tool": name, "body": result},
			})
			turnMu.Unlock()
			origToolResult(name, result)
		}
		a.mu.Unlock()

		err := a.agent.Chat(ctx, text)

		a.mu.Lock()
		a.agent.OnChunk = origChunk
		a.agent.OnToolCall = origToolCall
		a.agent.OnToolResult = origToolResult
		a.mu.Unlock()

		if err != nil {
			if ctx.Err() != nil {
				runtime.EventsEmit(a.ctx, "chat:stopped", nil)
			} else {
				runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			}
			return
		}

		turnMu.Lock()
		if asstBuf.Len() > 0 {
			turnItems = append(turnItems, displayItem{
				Type: "msg",
				Data: map[string]any{"role": "assistant", "text": asstBuf.String(), "streaming": false},
			})
		}
		turnMu.Unlock()

		if conv != nil {
			a.mu.Lock()
			// Guard against the active conversation being switched while we were running.
			if a.activeConv == nil || a.activeConv.ID != convID {
				a.mu.Unlock()
				runtime.EventsEmit(a.ctx, "chat:done", nil)
				return
			}

			msgs := a.agent.Messages()
			conv.Messages = make([]history.SavedMessage, len(msgs))
			for i, m := range msgs {
				conv.Messages[i] = history.SavedMessage{Role: string(m.Role), Content: m.Content}
			}

			if conv.Title == "New conversation" {
				for _, m := range msgs {
					if m.Role == domain.RoleUser {
						t := m.Content
						if len(t) > 60 {
							t = t[:57] + "…"
						}
						conv.Title = t
						break
					}
				}
			}

			existing := parseDisplayItems(conv.DisplayItems)
			all := append(existing, turnItems...)
			if raw, err := json.Marshal(all); err == nil {
				conv.DisplayItems = json.RawMessage(raw)
			}

			conv.UpdatedAt = time.Now()
			_ = history.Save(conv)
			a.mu.Unlock()
		}

		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()

	return ChatResponse{}
}

func (a *App) StopAgent() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelFn != nil {
		a.cancelFn()
		if a.permCh != nil {
			a.permCh <- false
		}
	}
}

func (a *App) RespondPermission(allow bool) {
	a.mu.Lock()
	ch := a.permCh
	a.mu.Unlock()
	if ch != nil {
		ch <- allow
	}
}

// ── Tools ─────────────────────────────────────────────────────────────────────

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

// ── Models & config ───────────────────────────────────────────────────────────

type ModelList struct {
	Models []string `json:"models"`
	Error  string   `json:"error,omitempty"`
}

func (a *App) ListModels() ModelList {
	provider := a.newProvider()
	models, err := provider.ListModels()
	if err != nil {
		return ModelList{Error: err.Error()}
	}
	return ModelList{Models: models}
}

func (a *App) GetConfig() config.Config { return a.cfg }

func (a *App) SaveConfig(cfg config.Config) string {
	a.cfg = cfg
	if err := config.Save(cfg); err != nil {
		return err.Error()
	}
	a.mu.Lock()
	if a.activeConv != nil {
		a.agent = a.buildAgent(a.activeConv.WorkDir)
	}
	a.mu.Unlock()
	return ""
}

// ── Speech-to-text ────────────────────────────────────────────────────────────

// WhisperAvailable reports whether the whisper CLI is installed on PATH.
func (a *App) WhisperAvailable() bool {
	return whisper.Available()
}

// TranscribeAudio receives raw audio bytes (WebM/WAV), runs whisper on them,
// and returns the plain-text transcript.
// On first call the model (~74 MB for base) is downloaded automatically.
func (a *App) TranscribeAudio(audioBytes []byte) (string, error) {
	model := a.cfg.WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	// Pre-download model and emit progress if it isn't cached yet.
	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(audioBytes, model, dataDir)
}

// StartRecording begins capturing microphone audio in the background.
func (a *App) StartRecording() error {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.recSes != nil {
		return fmt.Errorf("recording already in progress")
	}
	ses, err := recorder.Start()
	if err != nil {
		return err
	}
	a.recSes = ses
	return nil
}

// StopRecording stops the microphone, runs Whisper on the captured audio,
// and returns the transcript.
func (a *App) StopRecording() (string, error) {
	a.recMu.Lock()
	ses := a.recSes
	a.recSes = nil
	a.recMu.Unlock()

	if ses == nil {
		return "", fmt.Errorf("no active recording")
	}

	wavData, err := ses.Stop()
	if err != nil {
		return "", err
	}

	model := a.cfg.WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(wavData, model, dataDir)
}

func (a *App) PickDirectory() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Working Directory",
	})
	if err != nil {
		return ""
	}
	return dir
}

// ── MCP server management ────────────────────────────────────────────────────

func (a *App) ListMCPServers() []config.MCPServerConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.MCPServers
}

func (a *App) SaveMCPServers(servers []config.MCPServerConfig) string {
	a.mu.Lock()
	a.cfg.MCPServers = servers
	cfg := a.cfg
	a.mu.Unlock()
	if err := config.Save(cfg); err != nil {
		return err.Error()
	}
	return ""
}

// ProbeMCPServer starts a server temporarily, lists its tools, and returns their names.
func (a *App) ProbeMCPServer(srv config.MCPServerConfig) ([]string, error) {
	return tools.ProbeMCPServer(srv)
}

// parseDisplayItems safely parses existing display items from raw JSON.
func parseDisplayItems(raw json.RawMessage) []displayItem {
	if len(raw) == 0 {
		return nil
	}
	var items []displayItem
	_ = json.Unmarshal(raw, &items)
	return items
}
