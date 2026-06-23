package config

import "agent-gui/internal/store"

const (
	bucket = "config"
	key    = "cfg"
)

type ProviderType string

const (
	ProviderOllama      ProviderType = "ollama"
	ProviderOpenAI      ProviderType = "openai"
	ProviderGroq        ProviderType = "groq"
	ProviderGemini      ProviderType = "gemini"
	ProviderMistral     ProviderType = "mistral"
	ProviderCohere      ProviderType = "cohere"
	ProviderCerebras    ProviderType = "cerebras"
	ProviderTogether    ProviderType = "together"
	ProviderOpenRouter  ProviderType = "openrouter"
	ProviderHuggingFace ProviderType = "huggingface"
	ProviderSambaNova   ProviderType = "sambanova"
	ProviderNvidia      ProviderType = "nvidia"
	// No API key required
	ProviderLLM7   ProviderType = "llm7"
	ProviderZAI    ProviderType = "zai"
	// GitHub personal access token (free with any GitHub account)
	ProviderGitHub ProviderType = "github"
)

type ToolPerm string

const (
	PermAsk   ToolPerm = "ask"
	PermAllow ToolPerm = "allow"
	PermDeny  ToolPerm = "deny"
)

// MCPServerConfig defines a single MCP server to connect to via stdio.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
}

type Config struct {
	Provider        ProviderType        `json:"provider"`
	BaseURL         string              `json:"base_url"`
	APIKey          string              `json:"api_key"`
	Model           string              `json:"model"`
	WorkDir         string              `json:"work_dir"`
	ToolPermissions map[string]ToolPerm `json:"tool_permissions"`
	WhisperModel    string              `json:"whisper_model"`
	SystemPrompt    string              `json:"system_prompt"`
	ContextLimit    int                 `json:"context_limit"`
	MCPServers      []MCPServerConfig   `json:"mcp_servers"`
}

func Default() Config {
	return Config{
		Provider:        ProviderOllama,
		BaseURL:         "http://localhost:11434",
		Model:           "",
		WhisperModel:    "base",
		ContextLimit:    8192,
		ToolPermissions: map[string]ToolPerm{},
		MCPServers:      []MCPServerConfig{},
	}
}

// Load reads the config from the encrypted store.
// Returns the default config if none has been saved yet.
func Load() Config {
	cfg := Default()
	_ = store.Get(bucket, key, &cfg)
	if cfg.ToolPermissions == nil {
		cfg.ToolPermissions = map[string]ToolPerm{}
	}
	if cfg.ContextLimit == 0 {
		cfg.ContextLimit = 8192
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = []MCPServerConfig{}
	}
	return cfg
}

// Save persists the config to the encrypted store.
func Save(cfg Config) error {
	return store.Put(bucket, key, cfg)
}
