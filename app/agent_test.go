package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/testutil"
)

func newTestAgent(p domain.LLMProvider, tools ...domain.Tool) *ConversationalAgent {
	r := domain.NewToolRegistry()
	for _, t := range tools {
		r.Register(t)
	}
	return NewConversationalAgent("m", p, r, "", 0)
}

func call(tool string) string {
	return `TOOL_CALL: {"tool":"` + tool + `","args":{}}` + "\n"
}

func reply(chunks ...string) testutil.Reply { return testutil.Reply{Chunks: chunks} }

func lastContent(a *ConversationalAgent, fromEnd int) string {
	m := a.Messages()
	return m[len(m)-1-fromEnd].Content
}

func TestChatRunsToolAndReturnsAnswer(t *testing.T) {
	tool := &testutil.FakeTool{ToolName: "echo", Run: func(map[string]string) string { return "pong" }}
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply(call("echo")), reply("done")}}
	a := newTestAgent(p, tool)

	if err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if tool.Calls() != 1 {
		t.Fatalf("tool calls = %d", tool.Calls())
	}
	if got := lastContent(a, 0); got != "done" {
		t.Fatalf("final = %q", got)
	}
	if got := lastContent(a, 1); !strings.Contains(got, "pong") {
		t.Fatalf("tool result not in history: %q", got)
	}
}

func TestChatStreamsChunksInOrder(t *testing.T) {
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply("a", "b", "c")}}
	a := newTestAgent(p)
	var got strings.Builder
	a.OnChunk = func(c string) { got.WriteString(c) }
	if err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got.String() != "abc" {
		t.Fatalf("chunks = %q", got.String())
	}
}

func TestChatDeniedToolDoesNotRun(t *testing.T) {
	tool := &testutil.FakeTool{ToolName: "danger"}
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply(call("danger"))}}
	a := newTestAgent(p, tool)
	a.OnPermissionRequest = func(PermissionRequest) bool { return false }
	if err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if tool.Calls() != 0 {
		t.Fatal("denied tool ran")
	}
	if got := lastContent(a, 1); !strings.Contains(got, "PERMISSION DENIED") {
		t.Fatalf("result = %q", got)
	}
}

func TestChatParallelResultsKeepCallOrder(t *testing.T) {
	slow := &testutil.FakeTool{ToolName: "slow", Run: func(map[string]string) string {
		time.Sleep(30 * time.Millisecond)
		return "S"
	}}
	fast := &testutil.FakeTool{ToolName: "fast", Run: func(map[string]string) string { return "F" }}
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply(call("slow") + call("fast"))}}
	a := newTestAgent(p, slow, fast)
	var order []string
	a.OnToolResult = func(name, _ string) { order = append(order, name) }
	if err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "slow,fast" {
		t.Fatalf("order = %v", order)
	}
}

func TestChatUnknownTool(t *testing.T) {
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply(call("nope"))}}
	a := newTestAgent(p)
	if err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := lastContent(a, 1); !strings.Contains(got, "unknown tool") {
		t.Fatalf("result = %q", got)
	}
}

func TestChatStopsAtIterationLimit(t *testing.T) {
	var replies []testutil.Reply
	for i := 0; i < maxToolIterations+5; i++ {
		replies = append(replies, reply(call("echo")))
	}
	a := newTestAgent(&testutil.FakeProvider{Replies: replies}, &testutil.FakeTool{ToolName: "echo"})
	if err := a.Chat(context.Background(), "hi"); err == nil {
		t.Fatal("want iteration limit error")
	}
}

func TestChatPropagatesProviderErrorAndCancel(t *testing.T) {
	boom := errors.New("boom")
	a := newTestAgent(&testutil.FakeProvider{Replies: []testutil.Reply{{Err: boom}}})
	if err := a.Chat(context.Background(), "hi"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Chat(ctx, "hi"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
