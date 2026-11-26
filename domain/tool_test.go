package domain

import "testing"

func TestParseToolCall(t *testing.T) {
	tests := []struct {
		name, line, tool string
		ok               bool
	}{
		{"plain", `TOOL_CALL: {"tool":"read_file","args":{"path":"a"}}`, "read_file", true},
		{"leading text", `sure: TOOL_CALL: {"tool":"get_time","args":{}}`, "get_time", true},
		{"no prefix", `{"tool":"get_time"}`, "", false},
		{"bad json", `TOOL_CALL: {"tool":`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, ok := ParseToolCall(tt.line)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && tc.Tool != tt.tool {
				t.Fatalf("tool = %q, want %q", tc.Tool, tt.tool)
			}
		})
	}
}
