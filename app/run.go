package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"agent-gui/domain"
)

const (
	maxToolIterations = 50
	// maxParallelTools bounds how many tools from one reply run at once.
	maxParallelTools = 4
	// maxRepeats is how many identical rounds of calls in a row end the run.
	maxRepeats = 3
)

// toolTimeout is the longest any single tool may run. Tests shorten it.
var toolTimeout = 6 * time.Minute

// InterruptedNote ends a reply that was cut off.
const InterruptedNote = "[reply interrupted]"

// canceledResult records a call that was not run because the user stopped.
const canceledResult = "CANCELED: the user stopped the run"

// Reasons a run can end other than finishing or being canceled.
var (
	ErrIterationLimit = errors.New("agent exceeded maximum tool iterations — stopping to prevent a runaway loop")
	ErrRepeatedCall   = errors.New("agent repeated the same tool call without progress — stopping")
)

// Chat runs one user turn, including the tool-use loop. Canceling ctx stops
// the model request and any running tool; the history still records a result
// for every call the model made.
func (a *ConversationalAgent) Chat(ctx context.Context, userInput string) error {
	a.conv.Add(domain.Message{Role: domain.RoleUser, Content: userInput})
	a.emitContextUsage()

	var last string
	repeats := 0
	for range maxToolIterations {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reply, usage, err := a.request(ctx)
		if err != nil {
			// Keep what the user already saw, so history matches the screen.
			// Tool calls in a partial reply are never run.
			if reply.Content != "" {
				a.recordUsage(usage)
				a.addAssistant(reply.Content + "\n\n" + InterruptedNote)
				a.emitContextUsage()
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("LLM error: %w", err)
		}
		a.recordUsage(usage)

		a.addAssistant(reply.Content, reply.ToolCalls...)
		a.emitContextUsage()

		calls := reply.ToolCalls
		if !a.native {
			calls = nil
			for _, p := range domain.ParseToolCalls(reply.Content) {
				calls = append(calls, p.Call)
			}
		}
		pending := a.collectCalls(ctx, calls)
		if len(pending) == 0 {
			return nil
		}

		signature := signatureOf(pending)
		if signature == last {
			repeats++
		} else {
			last, repeats = signature, 1
		}
		if repeats >= maxRepeats {
			for _, p := range pending {
				p.execute, p.result = false, "ERROR: not run: this call was repeated "+fmt.Sprint(maxRepeats)+" times"
			}
		}

		a.execute(ctx, pending)

		// Emit results and append to history in the original call order so the
		// conversation stays deterministic regardless of completion order.
		for _, p := range pending {
			if a.OnToolResult != nil {
				a.OnToolResult(p.tc.Tool, p.result)
			}
			content := frameToolResult(p.tc.Tool, sourceOf(p.tc.Tool, p.tc.Args), p.result)
			if a.native {
				a.conv.Add(domain.Message{Role: domain.RoleTool, Content: content, ToolCallID: p.tc.ID, ToolName: p.tc.Tool})
			} else {
				a.conv.Add(domain.Message{Role: domain.RoleUser, Content: content})
			}
		}
		if repeats >= maxRepeats {
			return ErrRepeatedCall
		}
	}
	return fmt.Errorf("%w (%d)", ErrIterationLimit, maxToolIterations)
}

// request asks the model for its next reply, natively when possible. A model
// that rejects tools is served with the text protocol from then on.
func (a *ConversationalAgent) request(ctx context.Context) (domain.Reply, *domain.Usage, error) {
	onChunk := func(chunk string) {
		if a.OnChunk != nil {
			a.OnChunk(chunk)
		}
	}
	if p, ok := a.provider.(domain.ToolCallingProvider); ok && a.native {
		reply, usage, err := p.ChatTools(ctx, a.model, a.window(), a.toolSpecs(), onChunk)
		if !errors.Is(err, domain.ErrToolsUnsupported) {
			return reply, usage, err
		}
		a.useNativeTools(false)
	}
	msgs := a.window()
	if !a.native {
		msgs = asText(msgs)
	}
	content, usage, err := a.provider.ChatStream(ctx, a.model, msgs, onChunk)
	return domain.Reply{Content: content}, usage, err
}

func (a *ConversationalAgent) toolSpecs() []domain.ToolSpec {
	var specs []domain.ToolSpec
	for _, t := range a.registry.All() {
		specs = append(specs, domain.ToolSpec{Name: t.Name(), Description: t.Description(), Parameters: domain.SchemaOf(t)})
	}
	return specs
}

// asText rewrites native calls and results in the history into the text
// protocol, for a model that does not take them natively.
func asText(msgs []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.Role == domain.RoleTool:
			out = append(out, domain.Message{Role: domain.RoleUser, Content: m.Content})
		case len(m.ToolCalls) > 0:
			lines := []string{m.Content}
			for _, c := range m.ToolCalls {
				// Arguments came from JSON, so marshalling cannot fail.
				data, _ := json.Marshal(domain.ToolCall{Tool: c.Tool, Args: c.Args})
				lines = append(lines, "TOOL_CALL: "+string(data))
			}
			out = append(out, domain.Message{Role: m.Role, Content: strings.TrimSpace(strings.Join(lines, "\n"))})
		default:
			out = append(out, m)
		}
	}
	return out
}

// collectCalls validates each call and asks permission. Prompts happen here,
// one at a time, so the UI only ever shows one dialog. Every call gets an
// entry, so every call gets a result.
func (a *ConversationalAgent) collectCalls(ctx context.Context, calls []domain.ToolCall) []*pendingCall {
	var pending []*pendingCall
	for _, call := range calls {
		tc := &call
		p := &pendingCall{tc: tc}
		pending = append(pending, p)

		if ctx.Err() != nil {
			p.result = canceledResult
			continue
		}
		if tc.Malformed != "" {
			if tc.Tool == "" {
				tc.Tool = "unknown"
			}
			p.result = "ERROR: malformed tool call, not run: " + tc.Malformed
			continue
		}
		tool, found := a.registry.Find(tc.Tool)
		if !found {
			p.result = "ERROR: unknown tool '" + tc.Tool + "'"
			continue
		}
		// Validated before the prompt, so what is approved is what runs.
		args, err := domain.ValidateArgs(domain.SchemaOf(tool), tc.Args)
		if err != nil {
			p.result = "ERROR: " + err.Error()
			continue
		}
		tc.Args = args
		if a.OnPermissionRequest != nil {
			allowed := a.OnPermissionRequest(PermissionRequest{
				Tool:            tc.Tool,
				Description:     tool.Description(),
				Args:            tc.Args,
				ExternalContent: hasExternalContent(a.conv.History()),
			})
			if ctx.Err() != nil {
				p.result = canceledResult
				continue
			}
			if !allowed {
				p.result = "PERMISSION DENIED by user"
				continue
			}
		}
		if a.OnToolCall != nil {
			a.OnToolCall(tc.Tool, tc.Args)
		}
		p.tool, p.execute = tool, true
	}
	return pending
}

// execute runs the approved calls with bounded parallelism and a deadline each.
func (a *ConversationalAgent) execute(ctx context.Context, pending []*pendingCall) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, maxParallelTools)
	for _, p := range pending {
		if !p.execute {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func(p *pendingCall) {
			defer func() {
				// A panicking tool must not take the application down.
				if r := recover(); r != nil {
					p.result = fmt.Sprintf("ERROR: tool crashed: %v", r)
				}
				<-slots
				wg.Done()
			}()
			if ctx.Err() != nil {
				p.result = canceledResult
				return
			}
			toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
			defer cancel()
			p.result = p.tool.Execute(toolCtx, p.tc.Args)
		}(p)
	}
	wg.Wait()
}

func signatureOf(pending []*pendingCall) string {
	calls := make([]domain.ToolCall, len(pending))
	for i, p := range pending {
		calls[i] = *p.tc
		calls[i].ID = "" // differs every time
	}
	// Arguments came from JSON, so marshalling them cannot fail.
	data, _ := json.Marshal(calls)
	return string(data)
}
