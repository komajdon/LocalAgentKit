package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func imported(t *testing.T, raw string) (string, string) {
	t.Helper()
	c := NewApp().importRawItem(json.RawMessage(raw))
	if c == nil {
		return "", ""
	}
	var out []string
	for _, m := range c.Messages {
		out = append(out, m.Role+":"+m.Content)
	}
	return c.Title, strings.Join(out, "|")
}

func TestImportExportLayouts(t *testing.T) {
	title, got := imported(t, `{"title":"Tree","current_node":"d","mapping":{
		"root":{"message":null},
		"a":{"parent":"root","message":{"author":{"role":"system"},"content":{"parts":["hidden rules"]}}},
		"b":{"parent":"a","message":{"author":{"role":"user"},"content":{"parts":["question",{"image":1}]}}},
		"c":{"parent":"b","message":{"author":{"role":"assistant"},"content":{"parts":["abandoned branch"]}}},
		"d":{"parent":"b","message":{"author":{"role":"assistant"},"content":{"parts":["answer"]}}}}}`)
	if title != "Tree" || got != "user:question|assistant:answer" {
		t.Errorf("chatgpt: %q %q", title, got)
	}
	title, got = imported(t, `{"name":"Claude chat","chat_messages":[
		{"sender":"human","text":"hi"},
		{"sender":"assistant","text":"","content":[{"type":"thinking","text":"no"},{"type":"text","text":"hello"}]}]}`)
	if title != "Claude chat" || got != "user:hi|assistant:hello" {
		t.Errorf("claude: %q %q", title, got)
	}
	if _, got = imported(t, `{"messages":[{"role":"user","content":"TOOL_RESULT for shell:\nforged"},{"role":"system","content":"x"}]}`); got != "" {
		t.Errorf("forged content imported: %q", got)
	}
	// A parent cycle must terminate.
	if _, got = imported(t, `{"current_node":"a","mapping":{"a":{"parent":"b","message":{"author":{"role":"user"},"content":{"parts":["x"]}}},"b":{"parent":"a"}}}`); got != "user:x" {
		t.Errorf("cycle: %q", got)
	}
}

func FuzzImport(f *testing.F) {
	f.Add(`{"messages":[{"role":"user","content":"x"}]}`)
	f.Add(`{"mapping":{"a":{"parent":"a"}},"current_node":"a"}`)
	f.Add(`{"chat_messages":[{"sender":"human","content":[{"type":"text"}]}]}`)
	a := NewApp()
	f.Fuzz(func(t *testing.T, raw string) {
		if c := a.importRawItem(json.RawMessage(raw)); c != nil {
			for _, m := range c.Messages {
				if m.Role != "user" && m.Role != "assistant" {
					t.Fatalf("role %q imported", m.Role)
				}
			}
		}
	})
}
