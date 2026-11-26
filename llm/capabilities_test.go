package llm

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDescribe(t *testing.T) {
	url := serveStatus(t, 200, `{"capabilities":["completion","tools"],"model_info":{"llama.context_length":131072}}`)
	c, err := NewOllamaProvider(url).Describe(context.Background(), "m")
	if err != nil || c.NativeTools == nil || !*c.NativeTools || c.ContextWindow != 131072 {
		t.Fatalf("ollama = %+v %v", c, err)
	}
	// An older server reports nothing: unknown, not "unsupported".
	url = serveStatus(t, 200, `{"modelfile":"x"}`)
	if c, _ = NewOllamaProvider(url).Describe(context.Background(), "m"); c.NativeTools != nil || c.ContextWindow != 0 {
		t.Fatalf("old ollama = %+v", c)
	}
	url = serveStatus(t, 200, `{"data":[{"id":"other","context_window":1},{"id":"m","context_window":8000,"supported_parameters":["temperature"]}]}`)
	c, err = NewOpenAIProvider(url, "").Describe(context.Background(), "m")
	if err != nil || c.ContextWindow != 8000 || c.NativeTools == nil || *c.NativeTools {
		t.Fatalf("openai = %+v %v", c, err)
	}
}

func TestConfiguredTimeout(t *testing.T) {
	release := make(chan struct{})
	url, _ := script(t, func(_ int32, w http.ResponseWriter) {
		_, _ = w.Write([]byte(openAIPartial))
		w.(http.Flusher).Flush()
		<-release
	})
	defer close(release)
	p := NewOpenAIProvider(url, "")
	p.WithTimeout(Options{Timeout: 100 * time.Millisecond})
	start := time.Now()
	if _, _, err := chat(p); err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout not applied: %v after %v", err, time.Since(start))
	}
}
