package app

import (
	"context"
	"strings"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/testutil"
)

func limited(p domain.LLMProvider, limit int) *ConversationalAgent {
	a := NewConversationalAgent("m", p, domain.NewToolRegistry(), "RULES", limit)
	return a
}

func TestWindowDropsWholeTurnsAndKeepsHistory(t *testing.T) {
	p := &testutil.FakeProvider{}
	base := limited(p, 0)
	limit := (len(base.conv.SystemPrompt)/4 + 600) * 4 / 3
	a := limited(p, limit)

	big := strings.Repeat("x", 1200) // ~300 tokens
	for _, q := range []string{"one", "two", "three"} {
		a.InjectMessage(domain.RoleUser, q)
		a.InjectMessage(domain.RoleAssistant, "call "+q)
		a.InjectMessage(domain.RoleUser, frameToolResult("read_file", "s", big))
		a.InjectMessage(domain.RoleAssistant, "answer "+q)
	}
	before := len(a.Messages())
	if err := a.Chat(context.Background(), "four"); err != nil {
		t.Fatal(err)
	}
	if got := len(a.Messages()); got != before+2 {
		t.Fatalf("history shortened: %d messages, want %d", got, before+2)
	}

	sent := p.Requests[0]
	if sent[0].Role != domain.RoleSystem || !strings.HasPrefix(sent[0].Content, "RULES") {
		t.Fatal("system prompt missing")
	}
	if !startsTurn(sent[1]) {
		t.Fatalf("window starts inside a turn: %q", sent[1].Content[:40])
	}
	if sent[len(sent)-1].Content != "four" || sent[1].Content == "one" {
		t.Fatalf("wrong turns kept: first=%q", sent[1].Content)
	}
}

func TestWindowShortensOldResultsOfAnOversizedTurn(t *testing.T) {
	a := limited(&testutil.FakeProvider{}, 200)
	huge := frameToolResult("read_file", "s", strings.Repeat("y", 8000))
	a.InjectMessage(domain.RoleUser, "q")
	a.InjectMessage(domain.RoleAssistant, "call")
	a.InjectMessage(domain.RoleUser, huge)
	a.InjectMessage(domain.RoleAssistant, "call again")
	a.InjectMessage(domain.RoleUser, huge)
	w := a.window()
	if len(w) != 6 || !strings.Contains(w[3].Content, "omitted") || w[5].Content != huge {
		t.Fatalf("window = %d messages, older=%.60q", len(w), w[3].Content)
	}
	if a.Messages()[2].Content != huge {
		t.Fatal("history was modified")
	}
}

func TestSystemPromptIsCountedOnce(t *testing.T) {
	a := limited(&testutil.FakeProvider{}, 0)
	if got, want := a.TokenCount(), len(a.conv.SystemPrompt)/4; got != want {
		t.Fatalf("TokenCount = %d, want %d", got, want)
	}
}
