// Package llm provides LLMProvider implementations.
// OpenAIProvider uses the OpenAI-compatible REST API, which is also
// exposed by Ollama at /v1/chat/completions and by many other local/cloud services.
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

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openAIToolCall is a call in a request, or a fragment of one in a stream.
type openAIToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatRequest struct {
	Model         string               `json:"model"`
	Messages      []openAIMessage      `json:"messages"`
	Stream        bool                 `json:"stream"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
	Tools         []wireTool           `json:"tools,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type openAIChoice struct {
	Delta struct {
		Content   string           `json:"content"`
		ToolCalls []openAIToolCall `json:"tool_calls"`
	} `json:"delta"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	FinishReason *string `json:"finish_reason"`
}

type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// OpenAIProvider talks to any OpenAI-compatible endpoint.
// Works with: OpenAI, Ollama (/v1), LM Studio, LocalAI, vLLM, etc.
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  newHTTPClient(),
	}
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, model string, messages []domain.Message, onChunk func(string)) (string, *domain.Usage, error) {
	reply, usage, err := p.ChatTools(ctx, model, messages, nil, onChunk)
	return reply.Content, usage, err
}

// ChatTools streams a reply, offering the tools for native calling.
func (p *OpenAIProvider) ChatTools(ctx context.Context, model string, messages []domain.Message, specs []domain.ToolSpec, onChunk func(string)) (domain.Reply, *domain.Usage, error) {
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
	// Provider replies and URLs can echo credentials.
	return reply, usage, redact.Error(toolRejection(err, len(specs) > 0), p.apiKey)
}

func (p *OpenAIProvider) chatStream(ctx context.Context, model string, messages []domain.Message, specs []domain.ToolSpec, onChunk func(string)) (domain.Reply, *domain.Usage, error) {
	tools, names := buildTools(specs)
	msgs := make([]openAIMessage, len(messages))
	for i, m := range messages {
		msgs[i] = openAIMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			tc := openAIToolCall{ID: c.ID, Type: "function"}
			tc.Function.Name = names.toWire(c.Tool)
			tc.Function.Arguments = string(encodeArguments(c.Args))
			msgs[i].ToolCalls = append(msgs[i].ToolCalls, tc)
		}
	}

	body, err := json.Marshal(openAIChatRequest{
		Model:         model,
		Messages:      msgs,
		Stream:        true,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
		Tools:         tools,
	})
	if err != nil {
		return domain.Reply{}, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.Reply{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return domain.Reply{}, nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.Reply{}, nil, newHTTPError("API", resp)
	}

	var full strings.Builder
	var usage *domain.Usage
	// Call fragments are assembled by index; the arguments arrive in pieces.
	var calls []*openAIToolCall
	byIndex := map[int]*openAIToolCall{}
	collect := func(fragment openAIToolCall) {
		index := len(calls)
		if fragment.Index != nil {
			index = *fragment.Index
		} else if fragment.ID == "" && len(calls) > 0 {
			index = len(calls) - 1
		}
		call, ok := byIndex[index]
		if !ok {
			call = &openAIToolCall{}
			byIndex[index] = call
			calls = append(calls, call)
		}
		if fragment.ID != "" {
			call.ID = fragment.ID
		}
		if fragment.Function.Name != "" {
			call.Function.Name = fragment.Function.Name
		}
		call.Function.Arguments += fragment.Function.Arguments
	}
	finished := false
	stream := newIdleReader(resp.Body)
	defer stream.stop()
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // 8 MB max token
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "data: ")
		if line == "[DONE]" {
			finished = true
			break
		}
		if line == "" {
			continue
		}
		var chunk openAIChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		// The final usage chunk carries an empty choices array.
		if chunk.Usage != nil {
			usage = &domain.Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
				CachedTokens:     chunk.Usage.PromptDetails.CachedTokens,
			}
		}
		if chunk.Error != nil {
			return domain.Reply{Content: full.String()}, usage, interrupted(ctx, fmt.Errorf("provider error: %s", chunk.Error.Message))
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		// Some servers send a finish reason but no [DONE].
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason != "" {
			finished = true
		}
		for _, fragment := range chunk.Choices[0].Delta.ToolCalls {
			collect(fragment)
		}
		text := chunk.Choices[0].Delta.Content
		if text != "" {
			full.WriteString(text)
			onChunk(text)
		}
	}
	if !finished {
		return domain.Reply{Content: full.String()}, usage, interrupted(ctx, scanner.Err())
	}
	reply := domain.Reply{Content: full.String()}
	for _, c := range calls {
		call := domain.ToolCall{ID: c.ID, Tool: names.toReal(c.Function.Name)}
		if call.ID == "" {
			call.ID = newCallID()
		}
		parseArguments(&call, c.Function.Arguments)
		reply.ToolCalls = append(reply.ToolCalls, call)
	}
	return reply, usage, nil
}

func (p *OpenAIProvider) ListModels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	names, err := p.listModels(ctx)
	return names, redact.Error(err, p.apiKey)
}

func (p *OpenAIProvider) listModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make([]string, len(result.Data))
	for i, m := range result.Data {
		names[i] = m.ID
	}
	return names, nil
}
