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

func TestTypedArgumentsSurviveParsing(t *testing.T) {
	tc, ok := ParseToolCall(`TOOL_CALL: {"tool":"t","args":{"s":"x","n":9007199254740993,"f":1.5,"b":true,"a":[1,2],"o":{"k":"<v>"},"z":null}}`)
	if !ok {
		t.Fatal("not parsed")
	}
	for key, want := range map[string]string{
		"s": "x", "n": "9007199254740993", "f": "1.5", "b": "true",
		"a": "[1,2]", "o": `{"k":"<v>"}`, "z": "", "missing": "",
	} {
		if got := tc.Args.String(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, ok := ParseToolCall(`TOOL_CALL: {"tool":"t"} {"extra":1}`); ok {
		t.Error("trailing data accepted")
	}
}
