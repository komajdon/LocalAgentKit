package llm

// FreeProviderInfo holds the hardcoded base URL and available model list for a
// free-tier cloud LLM provider. All listed providers expose an
// OpenAI-compatible /v1/chat/completions endpoint.
type FreeProviderInfo struct {
	BaseURL    string
	Models     []string
	NoAPIKey   bool // true = works without an API key
}

// FreeProviderConfigs maps ProviderType string values to their API base URL
// and a curated list of freely available models.
// Sources: github.com/cheahjs/free-llm-api-resources (updated June 2026)
//          github.com/mnfst/awesome-free-llm-apis
var FreeProviderConfigs = map[string]FreeProviderInfo{
	"groq": {
		BaseURL: "https://api.groq.com/openai/v1",
		Models: []string{
			"llama-3.3-70b-versatile",
			"llama-3.1-8b-instant",
			"llama-4-scout-17b-16e-instruct",
			"openai/gpt-oss-120b",
			"openai/gpt-oss-20b",
			"qwen/qwen3-32b",
			"qwen/qwen3.6-27b",
			"mixtral-8x7b-32768",
			"gemma2-9b-it",
		},
	},
	"gemini": {
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		Models: []string{
			"gemini-3.5-flash",
			"gemini-2.5-flash",
			"gemini-2.5-flash-lite",
			"gemini-3.1-flash-lite",
			"gemini-2.5-pro",
			"gemma-3-27b-it",
			"gemma-3-12b-it",
			"gemma-3-4b-it",
		},
	},
	"mistral": {
		BaseURL: "https://api.mistral.ai/v1",
		Models: []string{
			"mistral-medium-3.5",
			"mistral-small-latest",
			"mistral-large-latest",
			"open-mistral-nemo",
			"open-mistral-7b",
			"open-mixtral-8x7b",
			"codestral-latest",
		},
	},
	"cohere": {
		BaseURL: "https://api.cohere.com/compatibility/v1",
		Models: []string{
			"command-a-plus-05-2026",
			"command-a-03-2025",
			"command-r-plus-08-2024",
			"command-r-08-2024",
			"command-r7b-12-2024",
			"c4ai-aya-expanse-32b",
		},
	},
	"cerebras": {
		BaseURL: "https://api.cerebras.ai/v1",
		Models: []string{
			"gpt-oss-120b",
			"llama-3.3-70b",
			"llama3.1-70b",
			"llama3.1-8b",
		},
	},
	"together": {
		BaseURL: "https://api.together.xyz/v1",
		Models: []string{
			"meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo",
			"meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			"meta-llama/Llama-4-Scout-17B-16E-Instruct",
			"mistralai/Mixtral-8x7B-Instruct-v0.1",
			"google/gemma-2-9b-it",
			"Qwen/Qwen2.5-72B-Instruct-Turbo",
			"deepseek-ai/DeepSeek-V3",
		},
	},
	"openrouter": {
		BaseURL: "https://openrouter.ai/api/v1",
		Models: []string{
			"meta-llama/llama-3.3-70b-instruct:free",
			"meta-llama/llama-3.2-3b-instruct:free",
			"qwen/qwen3-coder:free",
			"openai/gpt-oss-120b:free",
			"openai/gpt-oss-20b:free",
			"nvidia/nemotron-3-ultra-550b-a55b:free",
			"nvidia/nemotron-3-super-120b-a12b:free",
			"google/gemma-4-31b-it:free",
			"google/gemma-4-26b-a4b-it:free",
			"nousresearch/hermes-3-llama-3.1-405b:free",
			"mistralai/mistral-7b-instruct:free",
		},
	},
	"huggingface": {
		BaseURL: "https://router.huggingface.co/v1",
		Models: []string{
			"meta-llama/Meta-Llama-3-8B-Instruct",
			"meta-llama/Llama-3.2-3B-Instruct",
			"mistralai/Mistral-7B-Instruct-v0.3",
			"microsoft/Phi-3.5-mini-instruct",
			"Qwen/Qwen2.5-7B-Instruct",
			"HuggingFaceH4/zephyr-7b-beta",
		},
	},
	"sambanova": {
		BaseURL: "https://api.sambanova.ai/v1",
		Models: []string{
			"DeepSeek-V3-1",
			"Meta-Llama-3.3-70B-Instruct",
			"Meta-Llama-3.1-70B-Instruct",
			"MiniMax-M2.7",
			"Qwen2.5-72B-Instruct",
			"Qwen2.5-Coder-32B-Instruct",
		},
	},
	"nvidia": {
		BaseURL: "https://integrate.api.nvidia.com/v1",
		Models: []string{
			"meta/llama-3.3-70b-instruct",
			"meta/llama-3.1-70b-instruct",
			"meta/llama-4-scout-17b-16e-instruct",
			"deepseek-ai/deepseek-r1",
			"nvidia/nemotron-4-340b-instruct",
			"mistralai/mistral-7b-instruct-v0.3",
			"google/gemma-3-27b-it",
			"qwen/qwen2.5-72b-instruct",
		},
	},

	// ── No API key required ───────────────────────────────────────────────────

	"llm7": {
		BaseURL:  "https://api.llm7.io/v1",
		NoAPIKey: true,
		Models: []string{
			"deepseek-r1",
			"deepseek-v3",
			"deepseek-r1-distill-llama-70b",
			"gemini-2.5-flash-lite",
			"gpt-4o-mini",
			"llama-3.3-70b-instruct",
			"qwen3-235b-a22b",
			"mistral-small-3.1",
		},
	},
	"zai": {
		BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
		NoAPIKey: true,
		Models: []string{
			"glm-4.7-flash",
			"glm-4.6v-flash",
			"glm-4-flash",
			"glm-z1-flash",
		},
	},

	// ── Free with free-tier account token ────────────────────────────────────

	"github": {
		BaseURL: "https://models.github.ai/inference",
		Models: []string{
			"openai/gpt-4.1",
			"openai/gpt-4.1-mini",
			"openai/gpt-4.1-nano",
			"openai/o4-mini",
			"meta/Llama-4-Scout-17B-16E-Instruct",
			"meta/Llama-4-Maverick-17B-128E-Instruct",
			"deepseek/DeepSeek-R1-0528",
			"deepseek/DeepSeek-V3-0324",
			"microsoft/Phi-4-reasoning-plus",
			"mistral-ai/Mistral-Large-2411",
			"xai/grok-3",
			"xai/grok-3-mini",
		},
	},
}

// FreeProvider wraps OpenAIProvider with a hardcoded model list so users
// immediately see available models without requiring a live API call.
type FreeProvider struct {
	*OpenAIProvider
	models []string
}

func NewFreeProvider(baseURL, apiKey string, models []string) *FreeProvider {
	return &FreeProvider{
		OpenAIProvider: NewOpenAIProvider(baseURL, apiKey),
		models:         models,
	}
}

func (p *FreeProvider) ListModels() ([]string, error) {
	return p.models, nil
}
