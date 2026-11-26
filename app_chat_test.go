package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/history"
)

// gated streams "par", waits to be released (or canceled), then streams "tial".
type gated struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func newGated() *gated {
	return &gated{started: make(chan struct{}, 100), release: make(chan struct{})}
}

func (g *gated) ChatStream(ctx context.Context, _ string, _ []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	onChunk("par")
	g.started <- struct{}{}
	select {
	case <-g.release:
	case <-ctx.Done():
		return "par", nil, ctx.Err()
	}
	if g.err != nil {
		return "par", nil, g.err
	}
	onChunk("tial")
	return "partial", &domain.Usage{PromptTokens: 5, CompletionTokens: 2}, nil
}

func (g *gated) ListModels() ([]string, error) { return nil, nil }

type recorded struct {
	name string
	ev   ChatEvent
}

type sink struct {
	mu     sync.Mutex
	events []recorded
}

func (s *sink) emit(name string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, _ := data.(ChatEvent)
	s.events = append(s.events, recorded{name, ev})
}

func (s *sink) all() []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recorded(nil), s.events...)
}

func testApp(t *testing.T, p domain.LLMProvider) (*App, *sink) {
	t.Helper()
	a, s := NewApp(), &sink{}
	a.ctx = context.Background()
	a.cfg = config.Default()
	a.emit = s.emit
	a.logf = t.Logf
	a.providerFor = func(config.Config) domain.LLMProvider { return p }
	return a, s
}

var convSeq int

func newConv(t *testing.T, a *App) string {
	t.Helper()
	convSeq++
	time.Sleep(2 * time.Millisecond) // IDs are millisecond timestamps
	conv, err := a.NewConversation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return conv.ID
}

func send(t *testing.T, a *App, id, text string) *run {
	t.Helper()
	if resp := a.SendMessage(id, text); resp.Error != "" {
		t.Fatal(resp.Error)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.run
}

func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func texts(raw []byte) string {
	var out []string
	for _, it := range parseDisplayItems(raw) {
		s, _ := it.Data["text"].(string)
		out = append(out, it.Type+":"+s)
	}
	return strings.Join(out, "|")
}

func stored(t *testing.T, id string) *history.Conversation {
	t.Helper()
	c, err := history.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRunWritesOnlyToItsOwnConversation(t *testing.T) {
	g := newGated()
	a, s := testApp(t, g)
	one, two := newConv(t, a), newConv(t, a)
	r := send(t, a, one, "question one")
	wait(t, g.started)

	// Switch away, rename the running conversation, and try a second run.
	if _, err := a.LoadConversation(two); err != nil {
		t.Fatal(err)
	}
	if err := a.RenameConversation(one, "Renamed while running"); err != nil {
		t.Fatal(err)
	}
	if resp := a.SendMessage(two, "x"); resp.Error == "" {
		t.Fatal("a second concurrent run was accepted")
	}
	if _, err := a.BranchConversation(one, 0); err == nil {
		t.Fatal("branching during a run was accepted")
	}
	close(g.release)
	wait(t, r.done)

	got := stored(t, one)
	if texts(got.DisplayItems) != "msg:question one|msg:partial" || len(got.Messages) != 2 {
		t.Fatalf("saved %q, %d messages", texts(got.DisplayItems), len(got.Messages))
	}
	if got.Title != "Renamed while running" || got.TokenUsage.PromptTokens != 5 {
		t.Fatalf("title=%q usage=%+v", got.Title, got.TokenUsage)
	}
	if other := stored(t, two); len(other.Messages) != 0 || texts(other.DisplayItems) != "" {
		t.Fatal("the other conversation was modified")
	}
	var last uint64
	for _, e := range s.all() {
		if e.name == "chat:usage_budget" {
			continue
		}
		if e.ev.ConversationID != one || e.ev.RunID != r.id || e.ev.Seq <= last {
			t.Fatalf("bad event %s %+v", e.name, e.ev)
		}
		last = e.ev.Seq
	}
}

func TestLoadDuringRunReturnsLiveTranscript(t *testing.T) {
	g := newGated()
	a, s := testApp(t, g)
	id := newConv(t, a)
	r := send(t, a, id, "q")
	wait(t, g.started)

	view, err := a.LoadConversation(id)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Running || texts(view.DisplayItems) != "msg:q|msg:par" {
		t.Fatalf("running=%v items=%q", view.Running, texts(view.DisplayItems))
	}
	close(g.release)
	wait(t, r.done)
	// Exactly the chunk not in the snapshot has a later sequence number.
	var later []any
	for _, e := range s.all() {
		if e.name == "chat:chunk" && e.ev.Seq > view.Seq {
			later = append(later, e.ev.Data)
		}
	}
	if len(later) != 1 || later[0] != "tial" {
		t.Fatalf("later chunks = %v", later)
	}
}

func TestStoppedAndFailedRunsAreSaved(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		finish func(*App, *gated)
		event  string
		status string
	}{
		"stopped":     {nil, func(a *App, _ *gated) { a.StopAgent() }, "chat:stopped", "status:Stopped by user"},
		"interrupted": {domain.ErrInterrupted, func(_ *App, g *gated) { close(g.release) }, "chat:error", "error:Reply interrupted"},
		"failed":      {errors.New("boom"), func(_ *App, g *gated) { close(g.release) }, "chat:error", "error:LLM error: boom"},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGated()
			g.err = tc.err
			a, s := testApp(t, g)
			id := newConv(t, a)
			r := send(t, a, id, "q")
			wait(t, g.started)
			tc.finish(a, g)
			wait(t, r.done)

			got := stored(t, id)
			items := texts(got.DisplayItems)
			if strings.Contains(items, "[reply interrupted]") {
				t.Fatalf("marker shown in transcript: %q", items)
			}
			if !strings.Contains(items, "msg:q|msg:par") || !strings.Contains(items, tc.status) {
				t.Fatalf("items = %q", items)
			}
			if len(got.Messages) != 2 {
				t.Fatalf("messages = %+v", got.Messages)
			}
			ev := s.all()
			if ev[len(ev)-1].name != tc.event {
				t.Fatalf("last event = %s", ev[len(ev)-1].name)
			}
			if a.GetRunState().RunID != "" {
				t.Fatal("run slot not released")
			}
		})
	}
}

func TestDeletedConversationStaysDeleted(t *testing.T) {
	g := newGated()
	a, _ := testApp(t, g)
	id := newConv(t, a)
	r := send(t, a, id, "q")
	wait(t, g.started)
	if err := a.DeleteConversation(id); err != nil {
		t.Fatal(err)
	}
	wait(t, r.done)
	if _, err := history.Load(id); err == nil {
		t.Fatal("deleted conversation was recreated")
	}
}

func TestStopReleasesPendingPermissionPrompt(t *testing.T) {
	p := &scripted{reply: `TOOL_CALL: {"tool":"shell","args":{"command":"true"}}`}
	a, s := testApp(t, p)
	id := newConv(t, a)
	r := send(t, a, id, "q")
	deadline := time.Now().Add(5 * time.Second)
	for {
		a.mu.Lock()
		asking := a.pending != nil
		a.mu.Unlock()
		if asking {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no prompt")
		}
		time.Sleep(5 * time.Millisecond)
	}
	a.StopAgent()
	wait(t, r.done)
	names := ""
	for _, e := range s.all() {
		names += e.name + " "
	}
	if !strings.Contains(names, "chat:permission_closed") || strings.Contains(names, "chat:tool_call") {
		t.Fatal(names)
	}
}

// scripted replies with a fixed text once, then with nothing.
type scripted struct {
	mu    sync.Mutex
	reply string
}

func (p *scripted) ChatStream(_ context.Context, _ string, _ []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.reply
	p.reply = ""
	onChunk(out)
	return out, nil, nil
}
func (p *scripted) ListModels() ([]string, error) { return nil, nil }

func TestEditResendAndBranchUseVisibleMessages(t *testing.T) {
	// The first reply is only a tool call, which the transcript does not show.
	p := &scripted{reply: `TOOL_CALL: {"tool":"nope","args":{}}`}
	a, _ := testApp(t, p)
	id := newConv(t, a)
	wait(t, send(t, a, id, "first").done)
	p.reply = "answer two"
	wait(t, send(t, a, id, "second").done)

	items := parseDisplayItems(stored(t, id).DisplayItems)
	answer := -1
	for i, it := range items {
		if it.Data["text"] == "answer two" {
			answer = i
		}
	}
	branch, err := a.BranchConversation(id, answer)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(branch.Messages); got != len(stored(t, id).Messages) {
		t.Fatalf("branch kept %d messages", got)
	}

	p.reply = "edited answer"
	if resp := a.TruncateAndResend(id, 1, "second, edited"); resp.Error != "" {
		t.Fatal(resp.Error)
	}
	a.mu.Lock()
	r := a.run
	a.mu.Unlock()
	if r != nil {
		wait(t, r.done)
	}
	got := texts(stored(t, id).DisplayItems)
	if strings.Contains(got, "answer two") || !strings.Contains(got, "msg:second, edited|msg:edited answer") {
		t.Fatalf("items = %q", got)
	}
	if resp := a.TruncateAndResend(id, 9, "x"); resp.Error == "" || a.GetRunState().RunID != "" {
		t.Fatal("a bad ordinal must fail and release the run slot")
	}
}

func TestTitleIsCutBetweenCharacters(t *testing.T) {
	got := titleFrom([]domain.Message{{Role: domain.RoleUser, Content: strings.Repeat("سلام ", 40)}})
	if !strings.HasSuffix(got, "…") || strings.ContainsRune(got, '�') || len([]rune(got)) != 58 {
		t.Fatalf("title = %q", got)
	}
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	g := newGated()
	a, _ := testApp(t, g)
	id := newConv(t, a)
	r := send(t, a, id, "q")
	wait(t, g.started)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = a.LoadConversation(id)
			_ = a.SetConversationPinned(id, i%2 == 0)
			_ = a.SetConversationTags(id, []string{"t"})
			cfg := a.GetConfig()
			cfg.ToolPermissions = map[string]config.ToolPerm{"shell": config.PermDeny}
			_ = a.SaveConfig(cfg)
			_ = a.GetContextUsage()
			_ = a.GetUsageBudget()
			_ = a.GetRunState()
			_ = a.SendMessage(id, "again")
			_, _ = a.ListConversations()
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); close(g.release) }()
	wg.Wait()
	wait(t, r.done)
	if got := stored(t, id); len(got.Tags) != 1 || len(got.Messages) < 2 {
		t.Fatalf("tags=%v messages=%d", got.Tags, len(got.Messages))
	}
}

func TestNativeToolHistorySurvivesStorage(t *testing.T) {
	a, _ := testApp(t, &scripted{})
	id := newConv(t, a)
	args := domain.Args{"n": json.Number("9007199254740993")}
	err := a.updateConversation(id, func(c *history.Conversation) error {
		c.Messages = []history.SavedMessage{
			history.Saved(domain.Message{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "1", Tool: "t", Args: args}}}),
			history.Saved(domain.Message{Role: domain.RoleTool, ToolCallID: "1", ToolName: "t", Content: "r"}),
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stored(t, id).Messages
	if got[0].Message().ToolCalls[0].Args.String("n") != "9007199254740993" || got[1].Message().ToolName != "t" {
		t.Fatalf("stored = %+v", got)
	}
}
