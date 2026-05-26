package domain

import "context"

// LLMProvider streams tokens from a language model.
type LLMProvider interface {
	ChatStream(ctx context.Context, model string, messages []Message, onChunk func(string)) (string, error)
	ListModels() ([]string, error)
}
