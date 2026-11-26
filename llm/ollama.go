package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"agent-gui/domain"
	"agent-gui/internal/redact"
)

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error"`
}

// OllamaProvider talks to Ollama's native /api/chat endpoint.
type OllamaProvider struct {
	host   string
	client *http.Client
}

func NewOllamaProvider(host string) *OllamaProvider {
	return &OllamaProvider{
		host:   strings.TrimRight(host, "/"),
		client: newHTTPClient(),
	}
}

func (p *OllamaProvider) ChatStream(ctx context.Context, model string, messages []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	var (
		result string
		usage  *domain.Usage
	)
	emitted := false
	emit := func(chunk string) {
		emitted = true
		onChunk(chunk)
	}
	err := withRetry(ctx, func() bool { return emitted }, func() error {
		var e error
		result, usage, e = p.chatStream(ctx, model, messages, emit)
		return e
	})
	// A base URL may carry a password.
	return result, usage, redact.Error(err)
}

func (p *OllamaProvider) chatStream(ctx context.Context, model string, messages []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	msgs := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		msgs[i] = ollamaMessage{Role: string(m.Role), Content: m.Content}
	}

	body, err := json.Marshal(ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
	})
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil, newHTTPError("ollama", resp)
	}

	var full strings.Builder
	var usage *domain.Usage
	finished := false
	stream := newIdleReader(resp.Body)
	defer stream.stop()
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // 8 MB max token
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var cr ollamaChatResponse
		if err := json.Unmarshal(line, &cr); err != nil {
			continue
		}
		if cr.Error != "" {
			return full.String(), usage, interrupted(ctx, fmt.Errorf("ollama: %s", cr.Error))
		}
		if cr.Message.Content != "" {
			full.WriteString(cr.Message.Content)
			onChunk(cr.Message.Content)
		}
		if cr.Done {
			// The final message carries token counts for the whole exchange.
			if cr.PromptEvalCount > 0 || cr.EvalCount > 0 {
				usage = &domain.Usage{
					PromptTokens:     cr.PromptEvalCount,
					CompletionTokens: cr.EvalCount,
					TotalTokens:      cr.PromptEvalCount + cr.EvalCount,
				}
			}
			finished = true
			break
		}
	}
	if !finished {
		return full.String(), usage, interrupted(ctx, scanner.Err())
	}
	return full.String(), usage, nil
}

func (p *OllamaProvider) ListModels() ([]string, error) {
	names, err := p.listModels()
	return names, redact.Error(err)
}

func (p *OllamaProvider) listModels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.host+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}
	return names, nil
}
