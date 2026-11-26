# Changelog

## Unreleased

See `docs/UPGRADING.md` for behaviour changes and downgrade steps.

### Fixed — data loss
- Trimming to the context limit permanently deleted old messages.
- A damaged or unreadable key file was overwritten with a new key.
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
- Live model discovery.
- MCP server status and restart.
- Go and frontend tests, and CI quality gates.
