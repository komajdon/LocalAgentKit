package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Args are the JSON-typed arguments of a tool call. Numbers are json.Number so
// that large integers survive unchanged.
type Args map[string]any

// String returns the argument as text: strings as they are, other values as
// JSON, and "" when absent or null.
func (a Args) String(key string) string {
	switch v := a[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return encode(v)
	}
}

// Int returns the argument as an integer, accepting numeric strings.
func (a Args) Int(key string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(a.String(key)))
	return n, err == nil
}

// Format renders the arguments for display, in a stable order.
func (a Args) Format() string {
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + a.String(k)
	}
	return strings.Join(parts, ", ")
}

func encode(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// Tool is the interface every agent capability must implement.
type Tool interface {
	Name() string
	Description() string
	// Parameters returns a JSON schema hint string shown to the LLM.
	Parameters() string
	// Execute must return promptly once ctx is done.
	Execute(ctx context.Context, args Args) string
}

// ToolCall is a request from the model to run a tool.
type ToolCall struct {
	ID   string `json:"id,omitempty"`
	Tool string `json:"tool"`
	Args Args   `json:"args"`
	// Malformed explains why the call could not be read. It is not run.
	Malformed string `json:"malformed,omitempty"`
}

// ToolRegistry maps tool names to implementations.
type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *ToolRegistry) Find(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Keep removes the tools for which keep returns false.
func (r *ToolRegistry) Keep(keep func(name string) bool) {
	for name := range r.tools {
		if !keep(name) {
			delete(r.tools, name)
		}
	}
}

// All returns the tools sorted by name, so prompts are stable between runs.
func (r *ToolRegistry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

const callPrefix = "TOOL_CALL:"

// ParsedCall is a tool call found in reply text, with the byte range it used.
type ParsedCall struct {
	Call       ToolCall
	Start, End int
}

// ParseToolCalls finds the text-protocol calls in a reply. The JSON may span
// several lines. A line that starts with the prefix but does not hold a valid
// call yields a Malformed entry, so it is reported instead of ignored.
func ParseToolCalls(text string) []ParsedCall {
	var out []ParsedCall
	for pos := 0; pos < len(text); {
		i := strings.Index(text[pos:], callPrefix)
		if i < 0 {
			break
		}
		start := pos + i
		body := start + len(callPrefix)
		lineStart := strings.LastIndexByte(text[:start], '\n') + 1
		atLineStart := strings.TrimSpace(text[lineStart:start]) == ""

		var tc ToolCall
		dec := json.NewDecoder(strings.NewReader(text[body:]))
		dec.UseNumber()
		err := dec.Decode(&tc)
		if err == nil && tc.Tool == "" {
			err = fmt.Errorf(`the "tool" field is missing`)
		}
		end := body + int(dec.InputOffset())
		if err != nil {
			end = len(text)
			if nl := strings.IndexByte(text[body:], '\n'); nl >= 0 {
				end = body + nl
			}
			// Prose that merely mentions the prefix is not a call.
			if atLineStart {
				out = append(out, ParsedCall{ToolCall{Malformed: err.Error()}, start, end})
			}
			pos = end
			continue
		}
		tc.ID, tc.Malformed = "", ""
		if atLineStart {
			start = lineStart
		}
		out = append(out, ParsedCall{tc, start, end})
		pos = end
	}
	return out
}

// StripToolCalls removes the calls from reply text.
func StripToolCalls(text string) string {
	var b strings.Builder
	last := 0
	for _, p := range ParseToolCalls(text) {
		b.WriteString(text[last:p.Start])
		last = p.End
	}
	b.WriteString(text[last:])
	return strings.TrimSpace(b.String())
}

// DecodeJSON unmarshals exactly one JSON value, keeping numbers exact.
func DecodeJSON(data []byte, v any) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// UnmarshalJSON keeps numbers exact wherever arguments are read, including
// from stored history.
func (a *Args) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return err
	}
	*a = m
	return nil
}

// Plan is a change a tool has worked out but not yet made.
type Plan struct {
	// Preview shows what would change, for the user to approve.
	Preview string
	// Apply makes the change. It refuses if what the preview was based on has
	// changed since.
	Apply func(ctx context.Context) string
}

// Planner is a tool whose effect can be previewed.
type Planner interface {
	Plan(ctx context.Context, args Args) (Plan, error)
}
