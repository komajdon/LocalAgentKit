# Changelog

## Unreleased

See `docs/UPGRADING.md` for behaviour changes and downgrade steps.

### Fixed — data loss
- Trimming to the context limit permanently deleted old messages.
- A damaged or unreadable key file was overwritten with a new key.
- ChatGPT and Claude export files imported nothing.
- Restoring a backup overwrote the open database.
- Stopped, failed and interrupted turns were not saved.

### Fixed
- Streaming retries duplicated text; cut-off replies looked complete.
- Switching conversations during a run mixed their output.
- MCP server processes accumulated until the app quit.
- A second permission response could deadlock the app.
- Tool calls with non-string arguments were silently ignored.
- The SerpAPI key appeared in error messages.
- Claude-style imports lost their names; titles could be cut mid-character.

### Security
- Workspace confinement resistant to symlinks.
- Shell sandbox (Linux, macOS).
- Scoped, revocable approvals; extra prompts after external content.
- Web view permissions, navigation and Content Security Policy.
- Keychain option; encrypted backups; no key in automatic backups.
- Go, Wails and frontend tooling upgrades.

### Added
- Native function calling for Ollama and OpenAI-compatible providers.
- Cancellation of running tools.
- Live model discovery, capability detection, per-model overrides, connection test.
- Agent profiles and named workspaces with ignore patterns.
- File change review: diffs, conflict detection, undo, git status.
- Usage records, prices and limits (replacing the budget widget).
- Tool timeline, structured log, diagnostics export.
- Context summarisation strategies.
- Encryption key rotation; MCP health checks.
- Conversation index with versioned migration.
- MCP server status and restart.
- Go and frontend tests, ESLint, and CI quality gates.

### Changed
- Redesigned interface: tool calls fold into rows, activity and file changes
  are side panels, settings are in sections, the approval dialog leads with the
  change, and Escape denies it. New light and dark themes.
- `app.go` and `App.tsx` split into smaller files.
- React 19, Vite 6, TypeScript 6.
