# Architecture

```
main.go            Wails entry point
app*.go            Bindings exposed to the UI
  app_chat.go        runs: one at a time, events, saving
  app_conversations.go, app_config.go, app_security.go, app_backup.go
app/               Agent loop (run.go), context window (window.go),
                   untrusted-output framing (untrusted.go)
domain/            Messages, Tool and provider interfaces, parser, schemas
llm/               Ollama and OpenAI-compatible providers, retry, native tools
tools/             Built-in tools, workspace confinement, MCP client and pool
internal/
  permission/      Approval policy
  sandbox/         Shell isolation
  store/           Encrypted database, key location, restore
  backup/          Archive formats
  redact/          Credential removal
  config/, history/, recorder/, whisper/
frontend/          React + TypeScript (Vite)
```

Dependencies point inwards: `app` knows only `domain`; the root package wires
implementations together.

## A run

1. `SendMessage(conversationID, text)` reserves the single run slot and saves
   the user's message.
2. A goroutine builds an agent from the stored history: provider, tool registry,
   settings snapshot.
3. The agent requests a reply, validates and authorizes each tool call, runs the
   approved ones (at most four at once) and repeats until there are no calls.
4. The result is saved whatever the outcome, then `chat:done`, `chat:stopped`
   or `chat:error` is emitted.

Every `chat:*` event is `{conversation_id, run_id, seq, data}`.
`LoadConversation` returns the live transcript and the last `seq` it includes.

Run limits: 50 model rounds, 6 minutes per tool, and the same calls three times
in a row end the run.

## Locking

`convMu` serialises read-modify-write of stored conversations and is taken
before `mu`. `mu` guards `App` fields and is never held during network, tool or
keychain operations.

## Adding a tool

Implement `domain.Tool` and, for built-ins, `Schema()` (see `tools/schema.go`);
register it in `tools.NewRegistry`.

- `Execute` must return promptly when its context is done.
- Return failures as text starting with `ERROR:`.
- Confine file access with the `workspace` type.
- A new tool is *sensitive* unless added to `readOnly` in
  `internal/permission`. If it returns text written by others, add it to
  `isExternalSource` in `app/untrusted.go`.
- Add a case to `permission.Describe` for the approval dialog.

## Adding a provider

OpenAI-compatible services need only an entry in `llm/free_providers.go` and a
constant in `internal/config`. Otherwise implement `domain.LLMProvider`, and
`domain.ToolCallingProvider` for native tool calls.

- Return `domain.ErrInterrupted` (wrapped) with the partial text if a stream
  ends early.
- Do not retry after calling `onChunk`; `withRetry` handles this.
- Return `domain.ErrToolsUnsupported` when tools are rejected.

| Provider | Native tools | Model list |
|---|---|---|
| Ollama | yes, with fallback | `/api/tags` |
| OpenAI-compatible | yes, with fallback | `/models` |
| Built-in free tiers | yes, with fallback | `/models`, else built-in list |

These were tested against fake servers, not live services.

## MCP

Add stdio servers in Settings. One process runs per distinct configuration
(name, command, arguments, environment), shared by all conversations. It starts
when a run first needs it, is restarted automatically at most three times in
five minutes, and is stopped when its configuration changes. Tools are named
`<server>__<tool>`. Only stdio transport and tools are supported.
