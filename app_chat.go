package main

// Chat runs.
//
// Policy: one run at a time. Other conversations can be opened, renamed or
// deleted while it runs. Each run builds its own agent from the stored history
// and writes only to its own conversation, so nothing is shared with whatever
// the user is looking at. Every chat event names its conversation and run.
//
// Locks: convMu serialises read-modify-write of stored conversations and is
// taken before mu. mu guards App fields and is never held during network,
// tool or keychain operations.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/history"
	"agent-gui/internal/store"
	"agent-gui/tools"
)

// shutdownWait is how long quitting waits for a run to stop and save.
const shutdownWait = 5 * time.Second

// ChatResponse reports whether a run could be started.
type ChatResponse struct {
	Error string `json:"error,omitempty"`
}

// ChatEvent wraps every chat event. Seq increases within a run, so a view
// loaded mid-run can tell which events its snapshot already includes.
type ChatEvent struct {
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	Seq            uint64 `json:"seq"`
	Data           any    `json:"data"`
}

// RunState identifies the active run, if any.
type RunState struct {
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
}

type run struct {
	id     string
	convID string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	seq       uint64
	base      []displayItem // transcript up to and including this turn's input
	turn      []displayItem // completed items of this turn
	streaming strings.Builder
	usage     *appLayer.ContextUsage
	discard   bool // the conversation was deleted
}

// transcript returns the items so far and the last event they include. The
// reply still being streamed is marked as such.
func (r *run) transcript() ([]displayItem, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := append(append([]displayItem{}, r.base...), r.turn...)
	if r.streaming.Len() > 0 {
		items = append(items, msgItem("assistant", r.streaming.String(), true))
	}
	return items, r.seq
}

func msgItem(role, text string, streaming bool) displayItem {
	return displayItem{Type: "msg", Data: map[string]any{"role": role, "text": text, "streaming": streaming}}
}

// emitRun updates the run's transcript with record and sends the event, with a
// sequence number taken under the same lock.
func (a *App) emitRun(r *run, name string, data any, record func()) {
	r.mu.Lock()
	if record != nil {
		record()
	}
	r.seq++
	ev := ChatEvent{ConversationID: r.convID, RunID: r.id, Seq: r.seq, Data: data}
	r.mu.Unlock()
	a.emit(name, ev)
}

// visibleReply is what the transcript shows for an assistant message: no tool
// call lines, and no interruption marker, which has its own status row. A
// message with nothing visible gets no transcript item.
func visibleReply(content string) string {
	return domain.StripToolCalls(strings.TrimSuffix(content, appLayer.InterruptedNote))
}

// modelFor is the conversation's model override, or the configured one.
func modelFor(cfg configSnapshot, conv *history.Conversation) string {
	if conv.Model != "" {
		return conv.Model
	}
	return cfg.Model
}

// agentFor builds an agent for one conversation as the profile describes.
func (a *App) agentFor(cfg configSnapshot, profile ModelProfile, conv *history.Conversation, registry *domain.ToolRegistry) *appLayer.ConversationalAgent {
	agent := appLayer.NewConversationalAgent(profile.Model, a.provider(cfg), registry, cfg.SystemPrompt, profile.ContextLimit)
	for _, m := range conv.Messages {
		agent.Inject(m.Message())
	}
	agent.Strategy = appLayer.Strategy(cfg.ContextStrategy)
	if s := conv.Summary; s != nil {
		agent.Summary = &appLayer.Summary{Text: s.Text, Covered: s.Covered}
	}
	if profile.ToolCalling == "text" {
		agent.UseTextTools()
	}
	agent.SetUsage(conv.TokenUsage.PromptTokens, conv.TokenUsage.CompletionTokens)
	return agent
}

// registryFor builds the tools for a work directory. Without a pool there are
// no MCP tools and no server is started.
func (a *App) registryFor(ctx context.Context, cfg configSnapshot, workDir string, pool *tools.MCPPool) *domain.ToolRegistry {
	return tools.NewRegistry(tools.Options{
		WorkDir:    workDir,
		MCPServers: cfg.MCPServers,
		MCP:        pool,
		Context:    ctx,
		Search:     tools.SearchConfig{Provider: cfg.SearchProvider, APIKey: cfg.SearchAPIKey},
		Sandbox:    sandboxOptions(cfg),
		Protected:  []string{store.DataDir()},
	})
}

// updateConversation applies change to the stored conversation atomically.
func (a *App) updateConversation(id string, change func(*history.Conversation) error) error {
	a.convMu.Lock()
	defer a.convMu.Unlock()
	conv, err := history.Load(id)
	if err != nil {
		return err
	}
	if err := change(conv); err != nil {
		return err
	}
	return history.Save(conv)
}

// startRun reserves the single run slot, applies prepare to the stored
// conversation, records the user's message and starts the agent.
func (a *App) startRun(convID, text string, prepare func(*history.Conversation) error) ChatResponse {
	a.mu.Lock()
	if a.run != nil {
		a.mu.Unlock()
		return ChatResponse{Error: "the agent is already running — stop it first"}
	}
	a.runSeq++
	ctx, cancel := context.WithCancel(a.ctx)
	r := &run{id: fmt.Sprintf("run-%d", a.runSeq), convID: convID, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	a.run = r
	a.mu.Unlock()

	var prior history.Conversation
	err := a.updateConversation(convID, func(conv *history.Conversation) error {
		if prepare != nil {
			if err := prepare(conv); err != nil {
				return err
			}
		}
		prior = *conv
		prior.Messages = append([]history.SavedMessage(nil), conv.Messages...)

		// Save the input now, so it survives a crash or a failed run.
		base := append(parseDisplayItems(conv.DisplayItems), msgItem("user", text, false))
		conv.Messages = append(conv.Messages, history.SavedMessage{Role: string(domain.RoleUser), Content: text})
		raw, err := json.Marshal(base)
		if err != nil {
			return err
		}
		conv.DisplayItems = raw
		conv.UpdatedAt = time.Now()
		r.mu.Lock()
		r.base = base
		r.mu.Unlock()
		return nil
	})
	if err != nil {
		a.mu.Lock()
		a.run = nil
		a.mu.Unlock()
		cancel()
		close(r.done)
		return ChatResponse{Error: err.Error()}
	}

	a.emitRun(r, "chat:started", map[string]any{"text": text}, nil)
	go a.runAgent(r, &prior, text)
	return ChatResponse{}
}

func (a *App) runAgent(r *run, prior *history.Conversation, text string) {
	defer close(r.done)
	defer r.cancel()

	cfg := a.config()
	profile := a.modelProfile(r.ctx, cfg, modelFor(cfg, prior))
	agent := a.agentFor(cfg, profile, prior, a.registryFor(r.ctx, cfg, prior.WorkDir, a.mcp))
	agent.OnChunk = func(chunk string) {
		a.emitRun(r, "chat:chunk", chunk, func() { r.streaming.WriteString(chunk) })
	}
	agent.OnAssistantMessage = func(content string) {
		visible := visibleReply(content)
		// Replaces the streamed text, which may include tool call lines.
		a.emitRun(r, "chat:assistant", visible, func() {
			r.streaming.Reset()
			if visible != "" {
				r.turn = append(r.turn, msgItem("assistant", visible, false))
			}
		})
	}
	agent.OnToolCall = func(name string, args domain.Args) {
		body := args.Format()
		a.emitRun(r, "chat:tool_call", map[string]any{"tool": name, "args": body}, func() {
			r.turn = append(r.turn, displayItem{Type: "tool", Data: map[string]any{"kind": "call", "tool": name, "body": body}})
		})
	}
	agent.OnToolResult = func(name, result string) {
		a.emitRun(r, "chat:tool_result", map[string]any{"tool": name, "result": result}, func() {
			r.turn = append(r.turn, displayItem{Type: "tool", Data: map[string]any{"kind": "result", "tool": name, "body": result}})
		})
	}
	agent.OnContextUsage = func(usage appLayer.ContextUsage) {
		a.emitRun(r, "chat:context_usage", usage, func() { r.usage = &usage })
	}
	agent.OnPermissionRequest = func(req appLayer.PermissionRequest) bool {
		return a.authorize(r.ctx, r.convID, prior.WorkDir, req)
	}

	warn := func(text string) {
		a.emitRun(r, "chat:warning", text, func() {
			r.turn = append(r.turn, displayItem{Type: "status", Data: map[string]any{"text": "Warning: " + text}})
		})
	}
	m := a.newMeter(string(cfg.Provider), profile.Model, r.convID, warn)
	agent.BeforeRequest, agent.OnRequest, agent.OnWarning = m.before, m.record, warn

	a.emitRun(r, "chat:thinking", nil, nil)
	err := agent.Chat(r.ctx, text)
	a.finishRun(r, agent, err)
}

// finishRun saves the outcome, frees the run slot and then reports it.
func (a *App) finishRun(r *run, agent *appLayer.ConversationalAgent, runErr error) {
	event, status := "chat:done", displayItem{}
	var data any
	switch {
	case runErr == nil:
	case r.ctx.Err() != nil:
		event, status = "chat:stopped", displayItem{Type: "status", Data: map[string]any{"text": "Stopped by user"}}
	default:
		msg := runErr.Error()
		if errors.Is(runErr, domain.ErrInterrupted) {
			msg = "Reply interrupted — the text above is incomplete. " + msg
		}
		event, data, status = "chat:error", msg, displayItem{Type: "error", Data: map[string]any{"text": msg}}
	}

	a.convMu.Lock()
	r.mu.Lock()
	discard := r.discard
	items := append(append([]displayItem{}, r.base...), r.turn...)
	if r.streaming.Len() > 0 {
		items = append(items, msgItem("assistant", r.streaming.String(), false))
	}
	if status.Type != "" {
		items = append(items, status)
	}
	r.mu.Unlock()

	// Reload so renames, pins and tags made during the run are kept. A deleted
	// conversation is not brought back.
	if conv, err := history.Load(r.convID); err == nil && !discard {
		msgs := agent.Messages()
		conv.Messages = make([]history.SavedMessage, len(msgs))
		for i, m := range msgs {
			conv.Messages[i] = history.Saved(m)
		}
		if conv.Title == "New conversation" {
			conv.Title = titleFrom(msgs)
		}
		if raw, err := json.Marshal(items); err == nil {
			conv.DisplayItems = raw
		}
		prompt, completion := agent.Usage()
		conv.TokenUsage = history.TokenUsage{PromptTokens: prompt, CompletionTokens: completion}
		conv.Summary = nil
		if s := agent.Summary; s != nil {
			conv.Summary = &history.Summary{Text: s.Text, Covered: s.Covered}
		}
		conv.UpdatedAt = time.Now()
		if err := history.Save(conv); err != nil {
			a.logf("could not save conversation: %v", err)
		}
	}
	a.mu.Lock()
	a.run = nil
	a.mu.Unlock()
	a.convMu.Unlock()

	a.emitRun(r, event, data, nil)
}

// titleFrom shortens the first user message, cutting between characters.
func titleFrom(msgs []domain.Message) string {
	for _, m := range msgs {
		if m.Role != domain.RoleUser {
			continue
		}
		t := []rune(strings.Join(strings.Fields(m.Content), " "))
		if len(t) > 60 {
			return string(t[:57]) + "…"
		}
		if len(t) > 0 {
			return string(t)
		}
	}
	return "New conversation"
}

// SendMessage starts a run in the given conversation.
func (a *App) SendMessage(conversationID, text string) ChatResponse {
	return a.startRun(conversationID, text, nil)
}

// TruncateAndResend drops everything from the userOrdinal-th user message
// (0-based) onward and sends newText instead: "edit and resend".
func (a *App) TruncateAndResend(conversationID string, userOrdinal int, newText string) ChatResponse {
	return a.startRun(conversationID, newText, func(conv *history.Conversation) error {
		cut := userMessageIndex(conv.Messages, userOrdinal)
		if cut < 0 {
			return errors.New("message to edit was not found")
		}
		conv.Messages = conv.Messages[:cut]
		if conv.Summary != nil && conv.Summary.Covered > cut {
			conv.Summary = nil // it describes messages being replaced
		}
		conv.DisplayItems = truncateDisplayItems(conv.DisplayItems, userOrdinal)
		return nil
	})
}

// GetRunState reports the active run, so a reloaded UI can pick it up.
func (a *App) GetRunState() RunState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.run == nil {
		return RunState{}
	}
	return RunState{ConversationID: a.run.convID, RunID: a.run.id}
}

// StopAgent cancels the active run.
func (a *App) StopAgent() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.run != nil {
		a.run.cancel()
		a.replyLocked("")
	}
}

// runningIn returns the active run if it belongs to the conversation.
func (a *App) runningIn(conversationID string) *run {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.run != nil && a.run.convID == conversationID {
		return a.run
	}
	return nil
}
