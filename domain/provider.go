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
}

// LLMProvider streams tokens from a language model.
// ChatStream returns the full assistant text and, when the provider reports it,
// token usage for the call (nil when the provider does not return usage).
type LLMProvider interface {
	ChatStream(ctx context.Context, model string, messages []Message, onChunk func(string)) (string, *Usage, error)
	ListModels() ([]string, error)
}
