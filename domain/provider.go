package domain

import (
	"context"
	"errors"
)

// ErrInterrupted reports a reply that stopped before the provider finished it.
// ChatStream returns the text received so far along with it.
var ErrInterrupted = errors.New("the model's reply was interrupted")

// Usage reports token counts returned by the provider for one completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// CachedTokens is the part of PromptTokens served from a cache.
	CachedTokens int `json:"cached_tokens"`
}

// LLMProvider streams tokens from a language model.
// ChatStream returns the full assistant text and, when the provider reports it,
// token usage for the call (nil when the provider does not return usage).
type LLMProvider interface {
	ChatStream(ctx context.Context, model string, messages []Message, onChunk func(string)) (string, *Usage, error)
	ListModels() ([]string, error)
}

// ToolSpec describes a tool to a provider with native tool calling.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  []byte // JSON schema
}

// Reply is a model response that may request tool calls.
type Reply struct {
	Content   string
	ToolCalls []ToolCall
}

// ToolCallingProvider is a provider with native tool calling. It returns
// ErrToolsUnsupported when the server or model rejects the tools, before any
// output, so the caller can fall back to the text protocol.
type ToolCallingProvider interface {
	LLMProvider
	ChatTools(ctx context.Context, model string, messages []Message, tools []ToolSpec, onChunk func(string)) (Reply, *Usage, error)
}

// ErrToolsUnsupported reports that native tool calling is not available.
var ErrToolsUnsupported = errors.New("this model does not support native tool calling")
