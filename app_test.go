package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"agent-gui/internal/history"
	"agent-gui/internal/store"
)

// TestMain points the process-wide store at a temporary directory so tests
// never touch real user data.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lak-test-*")
	if err != nil {
		panic(err)
	}
	if err := store.Init(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = store.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func msgs(pairs ...string) []history.SavedMessage {
	var out []history.SavedMessage
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, history.SavedMessage{Role: pairs[i], Content: pairs[i+1]})
	}
	return out
}

func TestVisibleMessageCut(t *testing.T) {
	m := msgs("user", "q1", "assistant", "call", "user", "TOOL_RESULT for x", "assistant", "a1", "user", "q2")
	tests := []struct {
		ordinal int
		include bool
		want    int
	}{
		{0, false, 0}, {0, true, 1}, {2, true, 4}, {3, false, 4}, {9, true, -1}, {-1, true, -1},
	}
	for _, tt := range tests {
		if got := visibleMessageCut(m, tt.ordinal, tt.include); got != tt.want {
			t.Errorf("visibleMessageCut(%d,%v) = %d, want %d", tt.ordinal, tt.include, got, tt.want)
		}
	}
}

func TestUserMessageIndexSkipsToolResults(t *testing.T) {
	m := msgs("user", "q1", "user", "TOOL_RESULT for x", "user", "q2")
	if got := userMessageIndex(m, 1); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := userMessageIndex(m, 2); got != -1 {
		t.Fatalf("got %d", got)
	}
}

func TestTruncateDisplayItems(t *testing.T) {
	raw := json.RawMessage(`[{"type":"msg","data":{"role":"user"}},{"type":"tool","data":{}},{"type":"msg","data":{"role":"user"}}]`)
	for ordinal, want := range map[int]int{0: 0, 1: 2, 5: 3} {
		if got := len(parseDisplayItems(truncateDisplayItems(raw, ordinal))); got != want {
			t.Errorf("ordinal %d: %d items, want %d", ordinal, got, want)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.3", "v1.2.2", true},
		{"v2.0.0", "1.9.9", true},
		{"v1.10.0", "v1.9.0", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.0", "v1.3.0", false},
		{"v1.2", "v1.2.0", false},
	}
	for _, tt := range tests {
		if got := isNewerVersion(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewerVersion(%q,%q) = %v", tt.latest, tt.current, got)
		}
	}
}

func titles(t *testing.T) map[string]int {
	t.Helper()
	list, err := history.List()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, c := range list {
		out[c.Title]++
	}
	return out
}

func TestImportFromJSON(t *testing.T) {
	tests := []struct {
		name, json string
		count      int
		title      string
	}{
		{"own", `{"id":"x","title":"Own one","messages":[{"role":"user","content":"hi"}]}`, 1, "Own one"},
		{"chatgpt", `[{"title":"GPT one","messages":[{"role":"user","content":"hi"}]}]`, 1, "GPT one"},
		{"claude", `[{"name":"Claude one","messages":[{"role":"user","content":"hi"}]}]`, 1, "Claude one"},
		{"invalid", `{"nope":1}`, 0, ""},
		{"garbage", `not json`, 0, ""},
	}
	a := NewApp()
	for i, tt := range tests {
		t.Run(fmt.Sprint(i, tt.name), func(t *testing.T) {
			if got := a.importFromJSON([]byte(tt.json)); got != tt.count {
				t.Fatalf("count = %d, want %d", got, tt.count)
			}
			if tt.title != "" && titles(t)[tt.title] == 0 {
				t.Fatalf("missing title %q in %v", tt.title, titles(t))
			}
		})
	}
}
