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
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []wireTool      `json:"tools,omitempty"`
}

type ollamaChatResponse struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []ollamaToolCall `json:"tool_calls"`
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
	reply, usage, err := p.ChatTools(ctx, model, messages, nil, onChunk)
	return reply.Content, usage, err
}

// ChatTools streams a reply, offering the tools for native calling.
func (p *OllamaProvider) ChatTools(ctx context.Context, model string, messages []domain.Message, specs []domain.ToolSpec, onChunk func(string)) (domain.Reply, *domain.Usage, error) {
	var (
		reply domain.Reply
		usage *domain.Usage
	)
	emitted := false
	emit := func(chunk string) {
		emitted = true
		onChunk(chunk)
	}
	err := withRetry(ctx, func() bool { return emitted }, func() error {
		var e error
		reply, usage, e = p.chatStream(ctx, model, messages, specs, emit)
		return e
	})
	// A base URL may carry a password.
	return reply, usage, redact.Error(toolRejection(err, len(specs) > 0))
}

func (p *OllamaProvider) chatStream(ctx context.Context, model string, messages []domain.Message, specs []domain.ToolSpec, onChunk func(string)) (domain.Reply, *domain.Usage, error) {
	tools, names := buildTools(specs)
	msgs := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		msgs[i] = ollamaMessage{Role: string(m.Role), Content: m.Content}
		if m.Role == domain.RoleTool {
			msgs[i].ToolName = names.toWire(m.ToolName)
		}
		for _, c := range m.ToolCalls {
			var tc ollamaToolCall
			tc.Function.Name = names.toWire(c.Tool)
			tc.Function.Arguments = encodeArguments(c.Args)
			msgs[i].ToolCalls = append(msgs[i].ToolCalls, tc)
		}
	}

	body, err := json.Marshal(ollamaChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		Tools:    tools,
	})
	if err != nil {
		return domain.Reply{}, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return domain.Reply{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return domain.Reply{}, nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.Reply{}, nil, newHTTPError("ollama", resp)
	}

	var full strings.Builder
	var usage *domain.Usage
	var calls []domain.ToolCall
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
			return domain.Reply{Content: full.String()}, usage, interrupted(ctx, fmt.Errorf("ollama: %s", cr.Error))
		}
		for _, c := range cr.Message.ToolCalls {
			call := domain.ToolCall{ID: newCallID(), Tool: names.toReal(c.Function.Name)}
			parseArguments(&call, string(c.Function.Arguments))
			calls = append(calls, call)
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
		return domain.Reply{Content: full.String()}, usage, interrupted(ctx, scanner.Err())
	}
	return domain.Reply{Content: full.String(), ToolCalls: calls}, usage, nil
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
