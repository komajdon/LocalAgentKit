package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/testutil"
)

// native is a scripted provider with tool calling.
type native struct {
	testutil.FakeProvider
	replies     []domain.Reply
	unsupported bool
	sent        [][]domain.Message
}

func (n *native) ChatTools(_ context.Context, _ string, msgs []domain.Message, specs []domain.ToolSpec, _ func(string)) (domain.Reply, *domain.Usage, error) {
	if n.unsupported {
		return domain.Reply{}, nil, domain.ErrToolsUnsupported
	}
	n.sent = append(n.sent, msgs)
	if len(n.replies) == 0 || len(specs) == 0 {
		return domain.Reply{Content: "final"}, nil, nil
	}
	r := n.replies[0]
	n.replies = n.replies[1:]
	return r, nil, nil
}

type typed struct{ testutil.FakeTool }

func (t *typed) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`)
}

func TestNativeCallsGetPairedResults(t *testing.T) {
	var got domain.Args
	tool := &typed{testutil.FakeTool{ToolName: "count", Run: func(a domain.Args) string { got = a; return "done" }}}
	p := &native{replies: []domain.Reply{{ToolCalls: []domain.ToolCall{
		{ID: "1", Tool: "count", Args: domain.Args{"n": "7"}},
		{ID: "2", Tool: "count", Args: domain.Args{}},
		{ID: "3", Tool: "nope"},
		{ID: "4", Tool: "count", Malformed: "bad json"},
		{ID: "5", Tool: "count", Args: domain.Args{"n": "x"}},
	}}}}
	a := newTestAgent(p, tool)
	if strings.Contains(a.conv.SystemPrompt, "TOOL_CALL") {
		t.Fatal("native mode must not teach the text protocol")
	}
	if err := a.Chat(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if tool.Calls() != 1 || got["n"] != json.Number("7") {
		t.Fatalf("calls=%d args=%#v", tool.Calls(), got)
	}
	msgs := a.Messages()
	if len(msgs[1].ToolCalls) != 5 {
		t.Fatalf("assistant message: %+v", msgs[1])
	}
	for i, want := range []string{"done", `missing required argument "n"`, "unknown tool", "malformed", `must be integer`} {
		m := msgs[2+i]
		if m.Role != domain.RoleTool || m.ToolCallID != msgs[1].ToolCalls[i].ID || !strings.Contains(m.Content, want) {
			t.Errorf("result %d: %+v lacks %q", i, m, want)
		}
	}
	if msgs[len(msgs)-1].Content != "final" {
		t.Fatalf("final = %+v", msgs[len(msgs)-1])
	}
}

func TestFallsBackToTextAndRewritesNativeHistory(t *testing.T) {
	p := &native{unsupported: true}
	p.Replies = []testutil.Reply{reply(call("echo")), reply("done")}
	tool := &testutil.FakeTool{ToolName: "echo"}
	a := newTestAgent(p, tool)
	// History written by a model that did support tools.
	a.Inject(
		domain.Message{Role: domain.RoleUser, Content: "earlier"},
		domain.Message{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "1", Tool: "echo", Args: domain.Args{"k": "v"}}}},
		domain.Message{Role: domain.RoleTool, ToolCallID: "1", ToolName: "echo", Content: frameToolResult("echo", "echo", "r")},
	)
	if err := a.Chat(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if tool.Calls() != 1 {
		t.Fatalf("calls = %d", tool.Calls())
	}
	sent := p.Requests[0]
	if !strings.Contains(sent[0].Content, "TOOL_CALL:") {
		t.Fatal("text instructions missing")
	}
	if !strings.Contains(sent[2].Content, `TOOL_CALL: {"tool":"echo","args":{"k":"v"}}`) || sent[3].Role != domain.RoleUser || len(sent[2].ToolCalls) != 0 {
		t.Fatalf("history not rewritten: %+v %+v", sent[2], sent[3])
	}
}

func TestExternalContentFromNativeResult(t *testing.T) {
	if !hasExternalContent([]domain.Message{{Role: domain.RoleTool, ToolName: "search_web"}}) {
		t.Fatal("not detected")
	}
}

func TestMalformedTextCallIsReported(t *testing.T) {
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply("TOOL_CALL: {oops"), reply("sorry")}}
	a := newTestAgent(p)
	if err := a.Chat(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if got := lastContent(a, 1); !strings.Contains(got, "malformed tool call") {
		t.Fatalf("result = %q", got)
	}
}
