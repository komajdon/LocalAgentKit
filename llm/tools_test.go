package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-gui/domain"
)

var specs = []domain.ToolSpec{
	{Name: "read_file", Parameters: []byte(`{"type":"object"}`)},
	{Name: "my.db__query rows", Parameters: []byte(`{"type":"object"}`)},
}

var history = []domain.Message{
	{Role: domain.RoleUser, Content: "q"},
	{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "c1", Tool: "my.db__query rows", Args: domain.Args{"n": json.Number("9007199254740993")}}}},
	{Role: domain.RoleTool, Content: "result", ToolCallID: "c1", ToolName: "my.db__query rows"},
}

// capture serves body and records the request.
func capture(t *testing.T, body string) (string, *string) {
	t.Helper()
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got = string(data)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s.URL, &got
}

func chatTools(t *testing.T, p domain.ToolCallingProvider) domain.Reply {
	t.Helper()
	reply, _, err := p.ChatTools(context.Background(), "m", history, specs, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestOpenAINativeToolCalls(t *testing.T) {
	frag := func(s string) string { return `data: {"choices":[{"delta":{"tool_calls":[` + s + `]}}]}` + "\n\n" }
	url, req := capture(t, frag(`{"index":0,"id":"a","function":{"name":"read_file","arguments":"{\"pa"}}`)+
		frag(`{"index":1,"function":{"name":"my_db__query_rows","arguments":""}}`)+
		frag(`{"index":0,"function":{"arguments":"th\":\"x\"}"}}`)+
		frag(`{"index":1,"function":{"arguments":"{broken"}}`)+
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n")

	reply := chatTools(t, NewOpenAIProvider(url, ""))
	if len(reply.ToolCalls) != 2 {
		t.Fatalf("calls = %+v", reply.ToolCalls)
	}
	first, second := reply.ToolCalls[0], reply.ToolCalls[1]
	if first.ID != "a" || first.Tool != "read_file" || first.Args.String("path") != "x" {
		t.Fatalf("first = %+v", first)
	}
	if second.ID == "" || second.Tool != "my.db__query rows" || second.Malformed == "" {
		t.Fatalf("second = %+v", second)
	}
	for _, want := range []string{
		`"name":"my_db__query_rows"`, `"role":"tool"`, `"tool_call_id":"c1"`,
		`"arguments":"{\"n\":9007199254740993}"`, `"type":"function"`,
	} {
		if !strings.Contains(*req, want) {
			t.Errorf("request lacks %s: %s", want, *req)
		}
	}
}

func TestOllamaNativeToolCalls(t *testing.T) {
	url, req := capture(t, `{"message":{"content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"x","n":5}}}]},"done":false}
{"message":{"content":""},"done":true}
`)
	reply := chatTools(t, NewOllamaProvider(url))
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].ID == "" || reply.ToolCalls[0].Args.String("n") != "5" {
		t.Fatalf("calls = %+v", reply.ToolCalls)
	}
	for _, want := range []string{`"tool_name":"my_db__query_rows"`, `"arguments":{"n":9007199254740993}`, `"tools":[`} {
		if !strings.Contains(*req, want) {
			t.Errorf("request lacks %s: %s", want, *req)
		}
	}
}

func TestToolRejectionIsRecognised(t *testing.T) {
	for body, unsupported := range map[string]bool{
		`{"error":"registry.ollama.ai/library/x does not support tools"}`: true,
		`{"error":"context length exceeded"}`:                             false,
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, body, http.StatusBadRequest)
		}))
		_, _, err := NewOllamaProvider(s.URL).ChatTools(context.Background(), "m", nil, specs, func(string) {})
		if errors.Is(err, domain.ErrToolsUnsupported) != unsupported {
			t.Errorf("%s: err = %v", body, err)
		}
		// Without tools in the request it is an ordinary error.
		_, _, err = NewOllamaProvider(s.URL).ChatStream(context.Background(), "m", nil, func(string) {})
		if errors.Is(err, domain.ErrToolsUnsupported) {
			t.Errorf("%s without tools: %v", body, err)
		}
		s.Close()
	}
}

func TestToolNamesStayUniqueAndValid(t *testing.T) {
	long := strings.Repeat("x", 80)
	_, names := buildTools([]domain.ToolSpec{{Name: "a.b"}, {Name: "a b"}, {Name: long}, {Name: long + "y"}})
	seen := map[string]bool{}
	for real, wire := range names.wire {
		if seen[wire] || len(wire) > maxNameLen || invalidNameChars.MatchString(wire) || names.toReal(wire) != real {
			t.Errorf("bad mapping %q -> %q", real, wire)
		}
		seen[wire] = true
	}
}
