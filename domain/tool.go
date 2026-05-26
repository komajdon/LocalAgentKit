package domain

import (
	"encoding/json"
	"strings"
)

// Tool is the interface every agent capability must implement.
type Tool interface {
	Name() string
	Description() string
	// Parameters returns a JSON schema hint string shown to the LLM.
	Parameters() string
	Execute(args map[string]string) string
}

// ToolCall is parsed from an LLM response.
type ToolCall struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
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

func (r *ToolRegistry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// ParseToolCall extracts a ToolCall from a line of LLM output.
func ParseToolCall(line string) (*ToolCall, bool) {
	const prefix = "TOOL_CALL:"
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return nil, false
	}
	jsonStr := strings.TrimSpace(line[idx+len(prefix):])
	var tc ToolCall
	if err := json.Unmarshal([]byte(jsonStr), &tc); err != nil {
		return nil, false
	}
	return &tc, true
}
