package domain

import (
	"strings"
	"testing"
)

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name, text string
		tools      []string // "!" marks a malformed call
		visible    string
	}{
		{"plain", `TOOL_CALL: {"tool":"read_file","args":{"path":"a"}}`, []string{"read_file"}, ""},
		{"leading text", `sure: TOOL_CALL: {"tool":"get_time","args":{}}`, []string{"get_time"}, "sure:"},
		{"multiline", "Reading.\nTOOL_CALL: {\n \"tool\": \"a\",\n \"args\": {\"c\": \"x\"}\n}\nThen.", []string{"a"}, "Reading.\n\nThen."},
		{"two", "TOOL_CALL: {\"tool\":\"a\"}\nTOOL_CALL: {\"tool\":\"b\"}", []string{"a", "b"}, ""},
		{"no prefix", `{"tool":"get_time"}`, nil, `{"tool":"get_time"}`},
		{"truncated", `TOOL_CALL: {"tool":`, []string{"!"}, ""},
		{"no tool name", `TOOL_CALL: {"args":{}}`, []string{"!"}, ""},
		{"bad then good", "TOOL_CALL: {oops}\nTOOL_CALL: {\"tool\":\"b\"}", []string{"!", "b"}, ""},
		{"mentioned in prose", "Write TOOL_CALL: followed by JSON.", nil, "Write TOOL_CALL: followed by JSON."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, p := range ParseToolCalls(tt.text) {
				if p.Call.Malformed != "" {
					got = append(got, "!")
				} else {
					got = append(got, p.Call.Tool)
				}
			}
			if strings.Join(got, ",") != strings.Join(tt.tools, ",") {
				t.Fatalf("calls = %v, want %v", got, tt.tools)
			}
			if v := StripToolCalls(tt.text); v != tt.visible {
				t.Fatalf("visible = %q, want %q", v, tt.visible)
			}
		})
	}
}

func TestTypedArgumentsSurviveParsing(t *testing.T) {
	calls := ParseToolCalls(`TOOL_CALL: {"tool":"t","args":{"s":"x","n":9007199254740993,"f":1.5,"b":true,"a":[1,2],"o":{"k":"<v>"},"z":null}}`)
	if len(calls) != 1 || calls[0].Call.Malformed != "" {
		t.Fatal("not parsed")
	}
	tc := calls[0].Call
	for key, want := range map[string]string{
		"s": "x", "n": "9007199254740993", "f": "1.5", "b": "true",
		"a": "[1,2]", "o": `{"k":"<v>"}`, "z": "", "missing": "",
	} {
		if got := tc.Args.String(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
