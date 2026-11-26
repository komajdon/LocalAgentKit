// Package testutil holds deterministic fakes shared by tests.
package testutil

import (
	"context"
	"sync"

	"agent-gui/domain"
)

// Reply is one scripted response of a FakeProvider.
type Reply struct {
	Chunks []string
	Usage  *domain.Usage
	Err    error
}

// FakeProvider replays scripted replies and records every request.
type FakeProvider struct {
	mu       sync.Mutex
	Replies  []Reply
	Requests [][]domain.Message
}

// ChatStream streams the next scripted reply. When the script is exhausted it
// returns an empty reply, which ends an agent loop.
func (p *FakeProvider) ChatStream(ctx context.Context, _ string, msgs []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Requests = append(p.Requests, append([]domain.Message(nil), msgs...))
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if len(p.Replies) == 0 {
		return "", nil, nil
	}
	r := p.Replies[0]
	p.Replies = p.Replies[1:]
	var full string
	for _, c := range r.Chunks {
		full += c
		onChunk(c)
	}
	return full, r.Usage, r.Err
}

// ListModels returns a fixed list.
func (p *FakeProvider) ListModels() ([]string, error) { return []string{"fake"}, nil }

// FakeTool is a Tool whose behaviour is supplied by Run.
type FakeTool struct {
	ToolName string
	Run      func(args map[string]string) string

	mu    sync.Mutex
	calls []map[string]string
}

func (t *FakeTool) Name() string        { return t.ToolName }
func (t *FakeTool) Description() string { return "fake tool " + t.ToolName }
func (t *FakeTool) Parameters() string  { return `{}` }

// Execute records the call and delegates to Run.
func (t *FakeTool) Execute(args map[string]string) string {
	t.mu.Lock()
	t.calls = append(t.calls, args)
	t.mu.Unlock()
	if t.Run == nil {
		return "ok"
	}
	return t.Run(args)
}

// Calls reports how many times the tool ran.
func (t *FakeTool) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}
