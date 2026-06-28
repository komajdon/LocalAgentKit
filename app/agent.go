package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"agent-gui/domain"
)

// pendingCall is one tool invocation parsed from an LLM response. Entries with
// execute=true are run concurrently; the rest carry a pre-filled result.
type pendingCall struct {
	tc      *domain.ToolCall
	tool    domain.Tool
	result  string
	execute bool
}

// PermissionRequest is sent to the UI before any tool execution.
type PermissionRequest struct {
	Tool        string            `json:"tool"`
	Description string            `json:"description"`
	Args        map[string]string `json:"args"`
}

// ContextUsage reports token usage for the current conversation.
// Used/Limit drive the context bar; Total is the cumulative billed tokens.
type ContextUsage struct {
	Used      int  `json:"used"`
	Limit     int  `json:"limit"`
	Total     int  `json:"total"`     // cumulative prompt+completion tokens this conversation
	Estimated bool `json:"estimated"` // true when Used is a character heuristic, not real usage
}

// ConversationalAgent drives one conversation: it maintains history,
// calls the LLM, and executes tools until no more tool calls remain.
type ConversationalAgent struct {
	model        string
	provider     domain.LLMProvider
	registry     *domain.ToolRegistry
	conv         *domain.Conversation
	contextLimit int // max tokens (0 = no limit)

	// Real token usage reported by the provider.
	lastPromptTokens      int  // prompt tokens of the most recent call (≈ current context size)
	lastCompletionTokens  int  // completion tokens of the most recent call
	totalPromptTokens     int  // cumulative across the conversation (for cost)
	totalCompletionTokens int  // cumulative across the conversation
	hasRealUsage          bool // true once any provider call reported usage

	OnChunk             func(string)
	OnToolCall          func(name string, args map[string]string)
	OnToolResult        func(name, result string)
	OnPermissionRequest func(req PermissionRequest) bool
	OnContextUsage      func(usage ContextUsage)
}

func NewConversationalAgent(
	model string,
	provider domain.LLMProvider,
	registry *domain.ToolRegistry,
	systemPrompt string,
	contextLimit int,
) *ConversationalAgent {
	prompt := buildSystemPrompt(registry)
	if strings.TrimSpace(systemPrompt) != "" {
		prompt = strings.TrimSpace(systemPrompt) + "\n\n" + prompt
	}
	return &ConversationalAgent{
		model:        model,
		provider:     provider,
		registry:     registry,
		contextLimit: contextLimit,
		conv:         &domain.Conversation{SystemPrompt: prompt},
	}
}

func (a *ConversationalAgent) Reset() { a.conv.Reset() }

func (a *ConversationalAgent) SetModel(model string) { a.model = model }

func (a *ConversationalAgent) ListModels() ([]string, error) { return a.provider.ListModels() }

// InjectMessage adds a message directly to conversation history (used for replay).
func (a *ConversationalAgent) InjectMessage(role domain.Role, content string) {
	a.conv.Add(domain.Message{Role: role, Content: content})
}

// Messages returns the current conversation history (excluding the system prompt).
func (a *ConversationalAgent) Messages() []domain.Message { return a.conv.History() }

// TokenCount returns an approximate token count for all messages including the system prompt.
// Uses the rough heuristic of 1 token ≈ 4 characters.
func (a *ConversationalAgent) TokenCount() int {
	total := len(a.conv.SystemPrompt)
	for _, m := range a.conv.Full() {
		total += len(m.Content)
	}
	return total / 4
}

const maxToolIterations = 50

// Chat runs one user turn, including the tool-use loop.
func (a *ConversationalAgent) Chat(ctx context.Context, userInput string) error {
	a.conv.Add(domain.Message{Role: domain.RoleUser, Content: userInput})
	a.emitContextUsage()

	for iteration := 0; iteration < maxToolIterations; iteration++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Trim history if context limit is set.
		if a.contextLimit > 0 {
			a.trimToContextLimit()
		}

		response, usage, err := a.provider.ChatStream(ctx, a.model, a.conv.Full(), func(chunk string) {
			if a.OnChunk != nil {
				a.OnChunk(chunk)
			}
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("LLM error: %w", err)
		}
		a.recordUsage(usage)

		a.conv.Add(domain.Message{Role: domain.RoleAssistant, Content: response})
		a.emitContextUsage()

		// Collect every tool call in this response. Permission prompts happen
		// here, sequentially, so the UI only ever shows one dialog at a time.
		var pending []*pendingCall
		for _, line := range strings.Split(response, "\n") {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			tc, ok := domain.ParseToolCall(line)
			if !ok {
				continue
			}

			tool, found := a.registry.Find(tc.Tool)
			if !found {
				pending = append(pending, &pendingCall{tc: tc, result: "ERROR: unknown tool '" + tc.Tool + "'"})
				continue
			}

			if a.OnPermissionRequest != nil {
				allowed := a.OnPermissionRequest(PermissionRequest{
					Tool:        tc.Tool,
					Description: tool.Description(),
					Args:        tc.Args,
				})
				if !allowed {
					pending = append(pending, &pendingCall{tc: tc, result: "PERMISSION DENIED by user"})
					continue
				}
			}

			if a.OnToolCall != nil {
				a.OnToolCall(tc.Tool, tc.Args)
			}
			pending = append(pending, &pendingCall{tc: tc, tool: tool, execute: true})
		}

		if len(pending) == 0 {
			return nil
		}

		// Execute all approved tool calls concurrently, then collect their
		// results. Pre-filled entries (unknown tool / denied) are left as-is.
		var wg sync.WaitGroup
		for _, p := range pending {
			if !p.execute {
				continue
			}
			wg.Add(1)
			go func(p *pendingCall) {
				defer wg.Done()
				p.result = p.tool.Execute(p.tc.Args)
			}(p)
		}
		wg.Wait()

		// Emit results and append to history in the original call order so the
		// conversation stays deterministic regardless of completion order.
		for _, p := range pending {
			if a.OnToolResult != nil {
				a.OnToolResult(p.tc.Tool, p.result)
			}
			a.conv.Add(domain.Message{Role: domain.RoleUser, Content: "TOOL_RESULT for " + p.tc.Tool + ":\n" + p.result})
		}
	}
	return fmt.Errorf("agent exceeded maximum tool iterations (%d) — stopping to prevent a runaway loop", maxToolIterations)
}

// recordUsage folds one provider usage report into the running totals.
func (a *ConversationalAgent) recordUsage(u *domain.Usage) {
	if u == nil {
		return
	}
	a.hasRealUsage = true
	a.lastPromptTokens = u.PromptTokens
	a.lastCompletionTokens = u.CompletionTokens
	a.totalPromptTokens += u.PromptTokens
	a.totalCompletionTokens += u.CompletionTokens
}

// Usage returns the cumulative prompt and completion token totals.
func (a *ConversationalAgent) Usage() (prompt, completion int) {
	return a.totalPromptTokens, a.totalCompletionTokens
}

// SetUsage restores cumulative totals (used when a conversation is reloaded).
// It deliberately does not touch the "current context" estimate: after a reload
// we have no real measurement of the live context size until the next call, so
// the heuristic stays in effect (hasRealUsage remains false) while Total still
// reflects the restored cumulative usage.
func (a *ConversationalAgent) SetUsage(prompt, completion int) {
	a.totalPromptTokens = prompt
	a.totalCompletionTokens = completion
}

// CurrentUsage builds the context-usage snapshot, preferring real provider
// token counts and falling back to the character heuristic.
func (a *ConversationalAgent) CurrentUsage() ContextUsage {
	limit := a.contextLimit
	if limit == 0 {
		limit = 8192
	}
	usage := ContextUsage{Limit: limit, Total: a.totalPromptTokens + a.totalCompletionTokens}
	if a.hasRealUsage {
		// Real current context ≈ the last prompt plus the completion it produced.
		usage.Used = a.lastPromptTokens + a.lastCompletionTokens
	} else {
		usage.Used = a.TokenCount()
		usage.Estimated = true
	}
	return usage
}

func (a *ConversationalAgent) emitContextUsage() {
	if a.OnContextUsage == nil {
		return
	}
	a.OnContextUsage(a.CurrentUsage())
}

// trimToContextLimit drops the oldest non-system messages (pairs if possible)
// until the token count is under 90% of the limit.
func (a *ConversationalAgent) trimToContextLimit() {
	limit := a.contextLimit
	target := limit * 90 / 100
	history := a.conv.History()
	for a.TokenCount() > target && len(history) > 2 {
		// Remove the oldest message.
		a.conv.SetHistory(history[1:])
		history = a.conv.History()
	}
}

// buildSystemPrompt generates the tool-use instructions from the registry.
// MCP tools (name contains "__") are listed first with an explicit preference
// rule so the agent does not fall back to shell for tasks an MCP tool covers.
func buildSystemPrompt(r *domain.ToolRegistry) string {
	var mcpTools, builtinTools []domain.Tool
	for _, t := range r.All() {
		if strings.Contains(t.Name(), "__") {
			mcpTools = append(mcpTools, t)
		} else {
			builtinTools = append(builtinTools, t)
		}
	}

	var sb strings.Builder
	sb.WriteString(`You are a personal AI assistant with access to tools.
To use a tool, output a JSON block on its own line:

TOOL_CALL: {"tool": "<tool_name>", "args": {<key>: <value>, ...}}

You may issue several TOOL_CALL lines in the SAME response to run independent
tools in parallel — all of their TOOL_RESULT outputs come back together. When a
step depends on a previous tool's result, call that tool first and wait for its
TOOL_RESULT before continuing.
After gathering all information, give your final answer.

IMPORTANT TOOL PRIORITY RULE:
Always prefer MCP tools over built-in tools (especially shell) when an MCP
tool is available for the task. For example, if a mongodb MCP tool exists,
use it instead of running mongosh via shell. Only fall back to shell if no
suitable MCP tool covers the operation.
`)

	if len(mcpTools) > 0 {
		sb.WriteString("\n## MCP Tools (prefer these first)\n")
		for _, t := range mcpTools {
			sb.WriteString("\n- **" + t.Name() + "**: " + t.Description() + "\n")
			sb.WriteString("  Parameters: " + t.Parameters() + "\n")
		}
	}

	sb.WriteString("\n## Built-in Tools\n")
	for _, t := range builtinTools {
		sb.WriteString("\n- **" + t.Name() + "**: " + t.Description() + "\n")
		sb.WriteString("  Parameters: " + t.Parameters() + "\n")
	}

	return sb.String()
}
