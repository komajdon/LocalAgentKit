package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"agent-gui/domain"
)

// Options adjusts a provider's behaviour.
type Options struct {
	// Timeout bounds waiting for a reply to start and silence during it.
	// Zero keeps the defaults.
	Timeout time.Duration
}

// WithTimeout sets how long the provider waits for data.
func (p *OpenAIProvider) WithTimeout(o Options) { p.client, p.idle = clientFor(o.Timeout) }

// WithTimeout sets how long the provider waits for data.
func (p *OllamaProvider) WithTimeout(o Options) { p.client, p.idle = clientFor(o.Timeout) }

func clientFor(timeout time.Duration) (*http.Client, time.Duration) {
	c := newHTTPClient()
	if timeout > 0 {
		c.Transport.(*http.Transport).ResponseHeaderTimeout = timeout
	}
	return c, timeout
}

// Describe asks Ollama about a model. The tool and context information comes
// from the server; older versions report neither.
func (p *OllamaProvider) Describe(ctx context.Context, model string) (domain.Capabilities, error) {
	caps := domain.Capabilities{Streaming: true, UsageReporting: true, ModelDiscovery: true}
	body, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return caps, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/show", bytes.NewReader(body))
	if err != nil {
		return caps, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return caps, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return caps, newHTTPError("ollama", resp)
	}
	var show struct {
		Capabilities []string       `json:"capabilities"`
		ModelInfo    map[string]any `json:"model_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&show); err != nil {
		return caps, err
	}
	if len(show.Capabilities) > 0 {
		tools := slices.Contains(show.Capabilities, "tools")
		caps.NativeTools = &tools
	}
	// The key is prefixed with the architecture, such as llama.context_length.
	for k, v := range show.ModelInfo {
		if n, ok := v.(float64); ok && strings.HasSuffix(k, ".context_length") {
			caps.ContextWindow = int(n)
		}
	}
	return caps, nil
}

// Describe reads what the service's model list says about a model. The OpenAI
// API itself reports no capabilities; some compatible services add them.
func (p *OpenAIProvider) Describe(ctx context.Context, model string) (domain.Capabilities, error) {
	caps := domain.Capabilities{Streaming: true, UsageReporting: true, ModelDiscovery: true}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return caps, err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return caps, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return caps, newHTTPError("API", resp)
	}
	var list struct {
		Data []struct {
			ID                  string   `json:"id"`
			ContextLength       int      `json:"context_length"`
			ContextWindow       int      `json:"context_window"`
			MaxContextLength    int      `json:"max_model_len"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return caps, err
	}
	for _, m := range list.Data {
		if m.ID != model {
			continue
		}
		caps.ContextWindow = max(m.ContextLength, m.ContextWindow, m.MaxContextLength)
		if len(m.SupportedParameters) > 0 {
			tools := slices.Contains(m.SupportedParameters, "tools")
			structured := slices.Contains(m.SupportedParameters, "structured_outputs")
			caps.NativeTools, caps.StructuredOutput = &tools, &structured
		}
	}
	return caps, nil
}
