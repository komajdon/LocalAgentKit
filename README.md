# Local Agent Kit

A local-first desktop AI agent with tool use, voice input, MCP support, and persistent conversation history. Runs entirely on your machine — no cloud required.

![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)
![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)

---

## Features

| | |
|---|---|
| **Local-first** | Works with [Ollama](https://ollama.com) by default; also supports any OpenAI-compatible endpoint (OpenAI, LM Studio, LocalAI, vLLM) |
| **Tool use** | Read/write files, list directories, search content, run shell commands, get current time |
| **MCP support** | Connect to external services via [Model Context Protocol](https://modelcontextprotocol.io) stdio servers — PostgreSQL, MongoDB, Gmail, Google Drive, and more |
| **Voice input** | Built-in mic recording → [whisper.cpp](https://github.com/ggerganov/whisper.cpp) transcription, bundled in the release binary |
| **Permission system** | Per-tool allow / ask / deny; runtime approval dialog with 5-minute auto-deny |
| **Encrypted storage** | Conversations and config stored in an AES-256-GCM encrypted local database (bbolt) |
| **Conversation history** | Full history with search, rename, and export (Markdown / plain text) |
| **Per-conversation model** | Override the default LLM per conversation |
| **Context management** | Live token counter; auto-trims oldest messages when approaching the limit |
| **System prompt editor** | Customise the agent's behaviour from Settings |
| **Retry on errors** | Automatic retry with back-off on transient network failures |

---

## Download

Pre-built binaries are on the [Releases](../../releases) page.

| Platform | File |
|---|---|
| Linux (x86\_64) | `ai-agent-linux-amd64-full.tar.gz` |
| macOS Intel | `ai-agent-darwin-amd64-full.tar.gz` |
| macOS Apple Silicon | `ai-agent-darwin-arm64-full.tar.gz` |
| Windows (x86\_64) | `ai-agent-windows-amd64-full.zip` |

**Full** archives include the whisper binary and base voice model (~74 MB extra).  
**Lite** archives include only the app binary — voice model is downloaded on first use.

### Runtime dependencies

| | Linux | macOS | Windows |
|---|---|---|---|
| Ollama or OpenAI key | ✓ | ✓ | ✓ |
| WebKit2GTK 4.1 | required | built-in | built-in (WebView2) |
| ffmpeg | required for STT | required for STT | required for STT |

**Linux (Debian/Ubuntu):**
```bash
sudo apt install libwebkit2gtk-4.1-dev ffmpeg
```

---

## Getting Started

### 1. Install a local model backend

```bash
# Install Ollama
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3
```

Or point the app at any OpenAI-compatible endpoint in Settings.

### 2. Run the app

```bash
# Linux
./ai-agent-linux-amd64

# macOS — right-click → Open on first launch (Gatekeeper)
open ai-agent-darwin-arm64.app

# Windows
ai-agent-windows-amd64.exe
```

### 3. Configure

Open **Settings** (gear icon) and set:
- **Provider** — Ollama or OpenAI-compatible
- **Base URL** — e.g. `http://localhost:11434` for Ollama
- **Model** — pick from the live dropdown
- **Work directory** — the folder the agent can read and write files in
- **Tool permissions** — allow, deny, or ask per tool

---

## MCP Servers

Go to **Settings → MCP Servers** to connect Local Agent Kit to external services.

| Field | Example |
|---|---|
| Name | `postgres` |
| Command | `npx` |
| Args | `-y @modelcontextprotocol/server-postgres postgresql://localhost/mydb` |
| Env | `PGPASSWORD=secret` |

Click **Test** to probe the server and list its tools. Once enabled, those tools are available to the agent automatically — prefixed as `<servername>__<toolname>`.

**Popular MCP servers:**

| Service | Package |
|---|---|
| PostgreSQL | `@modelcontextprotocol/server-postgres` |
| MongoDB | `@modelcontextprotocol/server-mongodb` |
| Gmail / Google Drive | `@modelcontextprotocol/server-gdrive` |
| SQLite | `@modelcontextprotocol/server-sqlite` |
| Filesystem (extended) | `@modelcontextprotocol/server-filesystem` |

---

## Built-in Tools

| Tool | Description |
|---|---|
| `read_file` | Read a file in the work directory (up to 2 MB) |
| `write_file` | Write or overwrite a file |
| `list_dir` | List directory contents |
| `search_files` | Search file contents by pattern (up to 200 hits) |
| `delete_file` | Delete a file |
| `shell` | Run a shell command (5-minute timeout) |
| `get_time` | Get the current date and time |

---

## Configuration

All settings are in **⚙ Settings**. Everything is saved in an encrypted local store — no plain-text secrets on disk.

| Setting | Default | Description |
|---|---|---|
| Provider | `ollama` | `ollama` or `openai` |
| Base URL | `http://localhost:11434` | LLM server endpoint |
| API Key | _(empty)_ | Required for OpenAI; ignored for Ollama |
| Model | _(auto)_ | Fetched live from the provider |
| Work Dir | _(home)_ | Directory the agent can access |
| Whisper Model | `base` | `tiny` / `base` / `small` / `medium` / `large` |
| Context Limit | `8192` | Max tokens before history is trimmed |
| System Prompt | _(empty)_ | Global instruction prepended to every conversation |

### Data location

```
<install-dir>/
├── ai-agent            ← app binary
├── whisper             ← whisper.cpp binary (full package)
└── data/
    ├── agent.db        ← AES-256-GCM encrypted database
    ├── agent.key       ← 32-byte encryption key  ← back this up!
    └── models/
        └── ggml-base.bin
```

> **Backup:** copy the entire `data/` directory including `agent.key`. Without the key the database cannot be decrypted.

---

## Building from Source

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- [cmake](https://cmake.org) (for whisper.cpp)
- **Linux only:** `sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev ffmpeg cmake`
- **macOS only:** `brew install cmake ffmpeg`

### Development mode

```bash
git clone https://github.com/komajdon/ai_local_assistant.git
cd ai_local_assistant
cd frontend && npm install && cd ..
wails dev
```

### Production build

```bash
wails build
# → build/bin/ai-agent
```

---

## Architecture

```
├── app/            # Conversational agent loop (tool dispatch, retry, token counting)
├── domain/         # Core interfaces: Message, Tool, LLMProvider
├── llm/            # Ollama + OpenAI-compatible providers with retry
├── tools/          # Built-in tools + MCP stdio client
├── internal/
│   ├── config/     # Encrypted config persistence
│   ├── history/    # Conversation storage (bbolt, AES-256-GCM)
│   ├── recorder/   # Microphone capture
│   ├── store/      # Encrypted key-value store
│   └── whisper/    # whisper.cpp subprocess wrapper
└── frontend/       # React + TypeScript UI (Vite)
```

---

## CI / Releases

Every push to `main` builds all four platform binaries via GitHub Actions. Pushing a `v*` tag creates a GitHub Release with all archives attached:

```bash
git tag v1.0.0
git push origin v1.0.0
```

---

## Contributing

1. Fork and create a feature branch
2. Run `wails dev` to start the dev server with hot-reload
3. Submit a pull request — keep commits focused

**Code style:** `gofmt` for Go, strict TypeScript with functional components only.

---

## License

MIT
