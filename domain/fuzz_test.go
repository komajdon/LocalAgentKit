package domain

import "testing"

func FuzzParseToolCalls(f *testing.F) {
	for _, s := range []string{
		`TOOL_CALL: {"tool":"a","args":{}}`, "TOOL_CALL: {", "TOOL_CALL:", "x TOOL_CALL: []\nTOOL_CALL: null",
		"TOOL_CALL: TOOL_CALL: {\"tool\":\"a\"}", "  TOOL_CALL: {\n\"tool\":\"é\"\n} trailing", `TOOL_CALL: "s"`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		last := 0
		for _, p := range ParseToolCalls(text) {
			if p.Start < last || p.End < p.Start || p.End > len(text) {
				t.Fatalf("bad range %d-%d after %d in %q", p.Start, p.End, last, text)
			}
			if p.Call.Malformed == "" && p.Call.Tool == "" {
				t.Fatalf("call without a tool in %q", text)
			}
			last = p.End
		}
		StripToolCalls(text)
	})
}
