package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-gui/domain"
)

func serve(t *testing.T, body string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func stream(t *testing.T, p domain.LLMProvider) (string, string, *domain.Usage) {
	t.Helper()
	var chunks strings.Builder
	out, usage, err := p.ChatStream(context.Background(), "m", nil, func(c string) { chunks.WriteString(c) })
	if err != nil {
		t.Fatal(err)
	}
	return out, chunks.String(), usage
}

func TestOpenAIStream(t *testing.T) {
	url := serve(t, `data: {"choices":[{"delta":{"content":"Hel"}}]}

data: {"choices":[{"delta":{"content":"lo"}}]}

data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}

data: [DONE]
`)
	out, chunks, usage := stream(t, NewOpenAIProvider(url, ""))
	if out != "Hello" || chunks != "Hello" || usage == nil || usage.TotalTokens != 5 {
		t.Fatalf("out=%q chunks=%q usage=%+v", out, chunks, usage)
	}
}

func TestOllamaStream(t *testing.T) {
	url := serve(t, `{"message":{"content":"Hi"},"done":false}
{"message":{"content":"!"},"done":true,"prompt_eval_count":4,"eval_count":2}
`)
	out, chunks, usage := stream(t, NewOllamaProvider(url))
	if out != "Hi!" || chunks != "Hi!" || usage == nil || usage.TotalTokens != 6 {
		t.Fatalf("out=%q chunks=%q usage=%+v", out, chunks, usage)
	}
}
