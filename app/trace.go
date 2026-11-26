package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"agent-gui/domain"
)

// Tool call states.
const (
	ToolRunning   = "running"
	ToolSucceeded = "succeeded"
	ToolFailed    = "failed"
	ToolDenied    = "denied"
	ToolCanceled  = "canceled"
	ToolRejected  = "rejected" // malformed, unknown, invalid or repeated
)

// ToolEvent describes one tool call as it progresses. It is reported when the
// call starts running and again, with the same ID, when it is over.
type ToolEvent struct {
	ID         string
	Tool       string
	Args       domain.Args
	Status     string
	Permission string // how it was decided, when a permission check was made
	Started    time.Time
	Duration   time.Duration
	Result     string
}

// Why a run ended.
const (
	EndCompleted      = "completed"
	EndCanceled       = "canceled"
	EndInterrupted    = "interrupted"
	EndIterationLimit = "iteration_limit"
	EndRepeatedCall   = "repeated_call"
	EndError          = "error"
)

// EndReason classifies the error Chat returned.
func EndReason(err error) string {
	switch {
	case err == nil:
		return EndCompleted
	case errors.Is(err, context.Canceled):
		return EndCanceled
	case errors.Is(err, domain.ErrInterrupted):
		return EndInterrupted
	case errors.Is(err, ErrIterationLimit):
		return EndIterationLimit
	case errors.Is(err, ErrRepeatedCall):
		return EndRepeatedCall
	}
	return EndError
}

// failed reports whether a tool's result says it did not succeed. Shell output
// starts with a line naming the isolation in effect.
func failed(result string) bool {
	if strings.HasPrefix(result, "[isolation") {
		_, result, _ = strings.Cut(result, "\n")
	}
	for _, prefix := range []string{"ERROR:", "EXIT ERROR:", "MCP call failed:"} {
		if strings.HasPrefix(result, prefix) {
			return true
		}
	}
	return false
}

func (a *ConversationalAgent) toolEvent(p *pendingCall, status string) {
	if a.OnToolEvent == nil {
		return
	}
	e := ToolEvent{ID: p.id, Tool: p.tc.Tool, Args: p.tc.Args, Status: status,
		Permission: p.permission, Started: p.started}
	if status != ToolRunning {
		e.Result = p.result
		if !p.started.IsZero() {
			e.Duration = p.took
		}
	}
	a.OnToolEvent(e)
}
