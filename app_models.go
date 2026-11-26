package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/store"
	"agent-gui/llm"
)

const (
	modelBucket  = "model_settings"
	modelKey     = "settings"
	modelListTTL = time.Hour
	describeWait = 10 * time.Second
)

// ModelOverride replaces what is detected about one model.
type ModelOverride struct {
	ContextLimit int    `json:"context_limit"` // 0: automatic
	ToolCalling  string `json:"tool_calling"`  // "" automatic, "native", "text"
}

// ModelSettings holds provider timeouts and per-model overrides, keyed by
// "provider/model".
type ModelSettings struct {
	TimeoutSeconds int                      `json:"timeout_seconds"` // 0: defaults
	Overrides      map[string]ModelOverride `json:"overrides"`
}

// modelCache remembers discovery results for a while.
type modelCache struct {
	mu    sync.Mutex
	lists map[string]cachedList
	caps  map[string]domain.Capabilities
}

type cachedList struct {
	models []string
	at     time.Time
}

// cacheKey changes with anything that changes what the service would answer.
func cacheKey(cfg configSnapshot) string {
	return strings.Join([]string{string(cfg.Provider), cfg.BaseURL, cfg.APIKey}, "\x00")
}

func overrideKey(provider config.ProviderType, model string) string {
	return string(provider) + "/" + model
}

// GetModelSettings returns timeouts and overrides.
func (a *App) GetModelSettings() ModelSettings {
	s := ModelSettings{}
	if store.Has(modelBucket, modelKey) {
		if err := store.Get(modelBucket, modelKey, &s); err != nil {
			a.logf("could not read model settings: %v", err)
		}
	}
	if s.Overrides == nil {
		s.Overrides = map[string]ModelOverride{}
	}
	return s
}

// SaveModelSettings stores timeouts and overrides. It returns an error message
// or "".
func (a *App) SaveModelSettings(s ModelSettings) string {
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 3600 {
		return "the timeout must be between 0 and 3600 seconds"
	}
	for key, o := range s.Overrides {
		if o.ContextLimit < 0 || (o.ToolCalling != "" && o.ToolCalling != "native" && o.ToolCalling != "text") {
			return "invalid override for " + key
		}
		if o == (ModelOverride{}) {
			delete(s.Overrides, key)
		}
	}
	return errString(store.Put(modelBucket, modelKey, s))
}

// provider builds the configured provider with the user's timeout.
func (a *App) provider(cfg configSnapshot) domain.LLMProvider {
	p := a.providerFor(cfg)
	if t, ok := p.(interface{ WithTimeout(llm.Options) }); ok {
		if secs := a.GetModelSettings().TimeoutSeconds; secs > 0 {
			t.WithTimeout(llm.Options{Timeout: time.Duration(secs) * time.Second})
		}
	}
	return p
}

// capabilities asks the provider about a model, caching successful answers.
func (a *App) capabilities(ctx context.Context, cfg configSnapshot, model string) domain.Capabilities {
	key := cacheKey(cfg) + "\x00" + model
	a.models.mu.Lock()
	c, ok := a.models.caps[key]
	a.models.mu.Unlock()
	if ok {
		return c
	}
	d, can := a.provider(cfg).(domain.ModelDescriber)
	if !can {
		return domain.Capabilities{}
	}
	ctx, cancel := context.WithTimeout(ctx, describeWait)
	defer cancel()
	c, err := d.Describe(ctx, model)
	if err == nil {
		a.models.mu.Lock()
		if a.models.caps == nil {
			a.models.caps = map[string]domain.Capabilities{}
		}
		a.models.caps[key] = c
		a.models.mu.Unlock()
	}
	return c
}

// ModelProfile is how one model will be used.
type ModelProfile struct {
	Model        string              `json:"model"`
	Detected     domain.Capabilities `json:"detected"`
	ContextLimit int                 `json:"context_limit"`
	ToolCalling  string              `json:"tool_calling"` // "auto", "native" or "text"
}

// effective combines the settings, what was detected and the override. The
// detected context window only lowers the configured limit: a large window is
// not a request to send more.
func effective(cfg configSnapshot, model string, detected domain.Capabilities, o ModelOverride) ModelProfile {
	p := ModelProfile{Model: model, Detected: detected, ContextLimit: cfg.ContextLimit, ToolCalling: cfg.ToolCalling}
	if detected.ContextWindow > 0 && detected.ContextWindow < p.ContextLimit {
		p.ContextLimit = detected.ContextWindow
	}
	if o.ContextLimit > 0 {
		p.ContextLimit = o.ContextLimit
	}
	if p.ToolCalling != "text" && detected.NativeTools != nil && !*detected.NativeTools {
		p.ToolCalling = "text"
	}
	if o.ToolCalling != "" {
		p.ToolCalling = o.ToolCalling
	}
	return p
}

func (a *App) modelProfile(ctx context.Context, cfg configSnapshot, model string) ModelProfile {
	o := a.GetModelSettings().Overrides[overrideKey(cfg.Provider, model)]
	return effective(cfg, model, a.capabilities(ctx, cfg, model), o)
}

// DescribeModel reports how the model would be used with the saved settings.
func (a *App) DescribeModel(model string) ModelProfile {
	return a.modelProfile(a.ctx, a.config(), model)
}

type ModelList struct {
	Models []string `json:"models"`
	Error  string   `json:"error,omitempty"`
}

func (a *App) listModels(refresh bool) ModelList {
	cfg := a.config()
	key := cacheKey(cfg)
	a.models.mu.Lock()
	cached, ok := a.models.lists[key]
	if refresh {
		a.models.caps = nil
	}
	a.models.mu.Unlock()
	if ok && !refresh && time.Since(cached.at) < modelListTTL {
		return ModelList{Models: cached.models}
	}
	models, err := a.provider(cfg).ListModels()
	if err != nil {
		// A stale list is more useful than none, but say what happened.
		return ModelList{Models: cached.models, Error: err.Error()}
	}
	a.models.mu.Lock()
	if a.models.lists == nil {
		a.models.lists = map[string]cachedList{}
	}
	a.models.lists[key] = cachedList{models, time.Now()}
	a.models.mu.Unlock()
	return ModelList{Models: models}
}

// ListModels returns the provider's models, cached for an hour.
func (a *App) ListModels() ModelList { return a.listModels(false) }

// RefreshModels discards cached discovery results and asks again.
func (a *App) RefreshModels() ModelList { return a.listModels(true) }

// ConnectionResult reports a connection test.
type ConnectionResult struct {
	OK     bool   `json:"ok"`
	Models int    `json:"models"`
	Millis int64  `json:"millis"`
	Error  string `json:"error"`
}

// TestConnection asks the configured service for its models, bypassing caches
// and built-in lists, so a bad key or address shows as a failure.
func (a *App) TestConnection() ConnectionResult {
	p := a.provider(a.config())
	if free, ok := p.(*llm.FreeProvider); ok {
		p = free.OpenAIProvider
	}
	start := time.Now()
	models, err := p.ListModels()
	r := ConnectionResult{Millis: time.Since(start).Milliseconds()}
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.OK, r.Models = true, len(models)
	return r
}
