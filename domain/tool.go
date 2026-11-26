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

// ToolCall is parsed from an LLM response.
type ToolCall struct {
	Tool string `json:"tool"`
	Args Args   `json:"args"`
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

// All returns the tools sorted by name, so prompts are stable between runs.
func (r *ToolRegistry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ParseToolCall extracts a ToolCall from a line of LLM output.
func ParseToolCall(line string) (*ToolCall, bool) {
	const prefix = "TOOL_CALL:"
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return nil, false
	}
	var tc ToolCall
	if err := DecodeJSON([]byte(strings.TrimSpace(line[idx+len(prefix):])), &tc); err != nil {
		return nil, false
	}
	return &tc, true
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
