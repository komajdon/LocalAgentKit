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
	ProviderLLM7 ProviderType = "llm7"
	ProviderZAI  ProviderType = "zai"
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
	Notifications   bool                `json:"notifications"`
	// Web search tool configuration.
	SearchProvider string `json:"search_provider"` // duckduckgo (default, no key) | brave | serpapi
	SearchAPIKey   string `json:"search_api_key"`
	// UI theme: dark (default) | light | system (follow OS).
	Theme string `json:"theme"`
	// Shell isolation: auto (default) | strict | off.
	SandboxMode string `json:"sandbox_mode"`
	// SandboxNetwork lets isolated shell commands use the network.
	SandboxNetwork bool `json:"sandbox_network"`
	// ToolCalling: auto (native when the model supports it) | text.
	ToolCalling string `json:"tool_calling"`
	// BackupRetention is how many automatic backups are kept.
	BackupRetention int `json:"backup_retention"`
	// ContextStrategy: sliding (default) | summary | hybrid.
	ContextStrategy string `json:"context_strategy"`
	// Execution limits per run; 0 means the built-in default.
	MaxRounds          int `json:"max_rounds"`
	MaxParallelTools   int `json:"max_parallel_tools"`
	ToolTimeoutSeconds int `json:"tool_timeout_seconds"`
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
		Notifications:   true,
		SearchProvider:  "duckduckgo",
		Theme:           "dark",
		SandboxMode:     "auto",
		ToolCalling:     "auto",
		BackupRetention: DefaultBackupRetention,
		ContextStrategy: "sliding",
	}
}

// DefaultBackupRetention applies when the setting is unset or invalid.
const DefaultBackupRetention = 7

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
	if cfg.SearchProvider == "" {
		cfg.SearchProvider = "duckduckgo"
	}
	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}
	if cfg.BackupRetention < 1 {
		cfg.BackupRetention = DefaultBackupRetention
	}
	if cfg.SandboxMode == "" {
		cfg.SandboxMode = "auto"
	}
	return cfg
}

// Save persists the config to the encrypted store.
func Save(cfg Config) error {
	return store.Put(bucket, key, cfg)
}
