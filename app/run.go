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

		response, usage, err := a.provider.ChatStream(ctx, a.model, a.window(), func(chunk string) {
			if a.OnChunk != nil {
				a.OnChunk(chunk)
			}
		})
		if err != nil {
			// Keep what the user already saw, so history matches the screen.
			// Tool calls in a partial reply are never run.
			if response != "" {
				a.recordUsage(usage)
				a.addAssistant(response + "\n\n" + InterruptedNote)
				a.emitContextUsage()
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("LLM error: %w", err)
		}
		a.recordUsage(usage)

		a.addAssistant(response)
		a.emitContextUsage()

		pending := a.collectCalls(ctx, response)
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
			a.conv.Add(domain.Message{Role: domain.RoleUser, Content: frameToolResult(p.tc.Tool, sourceOf(p.tc.Tool, p.tc.Args), p.result)})
		}
		if repeats >= maxRepeats {
			return ErrRepeatedCall
		}
	}
	return fmt.Errorf("%w (%d)", ErrIterationLimit, maxToolIterations)
}

// collectCalls parses every tool call in a reply. Permission prompts happen
// here, one at a time, so the UI only ever shows one dialog.
func (a *ConversationalAgent) collectCalls(ctx context.Context, response string) []*pendingCall {
	var pending []*pendingCall
	for _, line := range strings.Split(response, "\n") {
		tc, ok := domain.ParseToolCall(line)
		if !ok {
			continue
		}
		p := &pendingCall{tc: tc}
		pending = append(pending, p)

		if ctx.Err() != nil {
			p.result = canceledResult
			continue
		}
		tool, found := a.registry.Find(tc.Tool)
		if !found {
			p.result = "ERROR: unknown tool '" + tc.Tool + "'"
			continue
		}
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
	calls := make([]*domain.ToolCall, len(pending))
	for i, p := range pending {
		calls[i] = p.tc
	}
	// Arguments came from JSON, so marshalling them cannot fail.
	data, _ := json.Marshal(calls)
	return string(data)
}
