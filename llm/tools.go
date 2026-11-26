package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"

	"agent-gui/domain"
)

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

// toolNames maps tool names to ones every API accepts and back. MCP names can
// hold characters, or have lengths, that OpenAI-style APIs reject.
type toolNames struct{ wire, real map[string]string }

var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

const maxNameLen = 64

func buildTools(specs []domain.ToolSpec) ([]wireTool, toolNames) {
	names := toolNames{wire: map[string]string{}, real: map[string]string{}}
	out := make([]wireTool, 0, len(specs))
	for _, s := range specs {
		base := invalidNameChars.ReplaceAllString(s.Name, "_")
		if len(base) > maxNameLen {
			base = base[:maxNameLen]
		}
		wire := base
		for i := 2; names.real[wire] != ""; i++ {
			suffix := fmt.Sprintf("_%d", i)
			wire = base[:min(len(base), maxNameLen-len(suffix))] + suffix
		}
		names.wire[s.Name], names.real[wire] = wire, s.Name

		var t wireTool
		t.Type = "function"
		t.Function.Name, t.Function.Description = wire, s.Description
		t.Function.Parameters = s.Parameters
		out = append(out, t)
	}
	return out, names
}

func (n toolNames) toWire(name string) string {
	if w, ok := n.wire[name]; ok {
		return w
	}
	// A call to a tool that no longer exists is still sent in history.
	w := invalidNameChars.ReplaceAllString(name, "_")
	return w[:min(len(w), maxNameLen)]
}

func (n toolNames) toReal(name string) string {
	if r, ok := n.real[name]; ok {
		return r
	}
	return name
}

var callSeq atomic.Uint64

// newCallID names calls from servers that do not.
func newCallID() string { return fmt.Sprintf("call_local_%d", callSeq.Add(1)) }

// parseArguments reads a call's argument object, recording why it could not.
func parseArguments(call *domain.ToolCall, raw string) {
	call.Args = domain.Args{}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return
	}
	if err := domain.DecodeJSON([]byte(raw), &call.Args); err != nil {
		call.Args = domain.Args{}
		call.Malformed = "the arguments are not a JSON object: " + err.Error()
	}
}

func encodeArguments(args domain.Args) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(args) // came from JSON, so cannot fail
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

var rejectsTools = regexp.MustCompile(`(?i)(tool|function)`)

// toolRejection turns a "this model has no tools" reply into
// ErrToolsUnsupported.
func toolRejection(err error, sentTools bool) error {
	var httpErr *HTTPError
	if sentTools && errors.As(err, &httpErr) && rejectsTools.MatchString(httpErr.Body) {
		switch httpErr.Status {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusNotImplemented:
			return fmt.Errorf("%w: %w", domain.ErrToolsUnsupported, err)
		}
	}
	return err
}
