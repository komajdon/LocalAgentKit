# Implementation report

Branch `dev/hardening`, from `main` @ `abbc7c1`. Date: 2026-11-26.
**All commits are local. Nothing was pushed, merged, tagged, released or
deployed, no remote CI was triggered, remotes are unchanged and no commit was
rewritten.**

## Executive summary

The specification's phases are implemented for Linux, with the gaps listed
below. The work fixes several ways the current release loses data.

**Release readiness: not ready.** CI has never run, Windows and macOS have not
been executed, and no live model, real OS keychain or real export file has been
used.

## Data-loss bugs in `main` (fixed on this branch)

1. Reaching the context limit permanently deleted old messages.
2. A damaged or unreadable `agent.key` was overwritten with a new key.
3. Restoring a backup overwrote the database while it was open.
4. Stopped and failed turns were never saved.

## Other verified issues

Confirmed in source or by a failing test: fragile tool parsing; duplicated text
on retry; MCP processes accumulating; conversation races; shell `dir` unchecked
and symlink escapes; key beside the data and inside backups; web view granting
everything; SerpAPI key in errors; no tests. Also found: permission deadlock;
real ChatGPT and Claude exports importing nothing; Claude names lost; titles cut
mid-character; an artificial, unenforced budget widget.

Correction to the original review: 13 built-in free providers, not 14.

## Specification coverage

| Phase | Status | Not done or limited |
|---|---|---|
| 1 Baseline, tests, CI | Done | CI never run (UNVERIFIED) |
| 4.1 Filesystem | Done | Hard links not detected |
| 4.2 Shell sandbox | Partial | No Windows or container backend; no memory limit; macOS profile UNVERIFIED |
| 4.3 Permissions | Done | – |
| 4.4 Prompt injection | Done | A mitigation, not a guarantee |
| 4.5 Keys and encryption | Done | Keychain is opt-in; real keychain UNVERIFIED; rotation needs a file key |
| 4.6 Web view | Done on Linux | Windows and macOS UNVERIFIED |
| 4.7 Credentials | Done | Redaction is pattern based |
| 5 Native tool calling | Done | Live providers UNVERIFIED |
| 6 Streaming and retry | Done | – |
| 7 Concurrency | Done | One run at a time, by design |
| 8 MCP | Done | stdio only; servers unsandboxed |
| 9 Cancellation and execution | Done | Run end reason is logged, not shown as its own UI state |
| 10 Context | Done | Token counts are calibrated estimates, not a provider tokenizer |
| 11 Storage and backups | Mostly | Message search still decrypts; sidebar paging is client side |
| 12 Usage and budgets | Done | No provider reports billed cost; earlier usage not back-filled |
| 13 Models | Done | Capability detection depends on what servers report |
| 14.1 Workspaces | Partial | No workspace indexing |
| 14.2 Profiles | Partial | Profiles cannot change provider (keys would go to another service) |
| 14.3–5 Timeline, review, observability | Done | Shell and MCP file changes are not journaled |
| 15 Backend refactor | Done | `app.go` 1336 → about 120 lines |
| 16 Frontend refactor | Mostly | `App.tsx` 1651 → about 630; no separate `ChatView` or settings/conversation hooks; disconnected state and responsive layout not addressed |
| 17 Testing | Mostly | No committed end-to-end tests |
| 18 Documentation | Done | Guides are sections of `docs/ARCHITECTURE.md`; threat model is in `SECURITY.md` |

## Work packages

The first 29 commits are listed with the work packages of the first round; the
later ones are below them.

| Work Package | Status | Commit Hash | Tests | Remaining Issues |
|---|---|---|---|---|
| Plan, baseline, build, format, lint | Done | `7dac9c6` `a655892` `3152778` `ae67d6b` | baseline | – |
| Baseline tests, fuzz | Done | `62df1a6` `99f9e50` | unit, fuzz | – |
| Toolchain and CI | Local only | `179bfa1` `ed1c69e` `c739673` | local equivalents | CI UNVERIFIED |
| Filesystem boundaries | Done | `6126d37` | symlink, fuzz | hard links |
| Shell sandbox | Partial | `46c9399` | Linux integration | Windows, macOS |
| Permissions, injection, redaction | Done | `422bc26` `43921c8` `608acf8` | policy, leak | – |
| Web view | Linux | `2228916` | running app | other OSes |
| Keys, backups, rotation | Done | `8f367b9` `419b509` `b523a8e` | tamper, crash recovery | real keychain |
| Streaming and retry | Done | `7d0f565` | fault injection | – |
| Typed args, cancellation | Done | `0262097` | cancel, limits | – |
| Context | Done | `92704f0` `cc6b576` | window, summary | estimates |
| Concurrency | Done | `20080e0` | race, routing | – |
| MCP | Done | `dce421b` `2ad883e` | real subprocess | stdio only |
| Native tools, models | Done | `4d06b4d` `4020b00` `71d8e02` | fake servers | live UNVERIFIED |
| Imports | Done | `f9077f0` `5df31f0` | unit, fuzz | real files UNVERIFIED |
| Storage index and migration | Done | `74e644a` | migration, benchmark | search |
| Usage and limits | Done | `72b38fc` | limits, dashboard | – |
| Timeline, logs, limits | Done | `052668c` | redaction | – |
| File review and undo | Done | `c8dd92a` `acb9698` | `git apply`, conflicts | shell changes |
| Profiles and workspaces | Partial | `66e376c` | tighten-only | indexing |
| Backend split | Done | `daa38e7` | existing | – |
| Frontend split, lint, upgrades | Mostly | `a8ce997` `49ccfa8` `426a634` `d2f5caa` `316b73a`  | Vitest, ESLint | see phase 16 |
| Documentation | Done | `f28ce11` `d4ed1a0` | – | – |

## Local test and build results (Linux amd64, Go 1.26.11, Node 20)

| Check | Result |
|---|---|
| `go test ./...` | 182 passed, 0 skipped, 0 failed |
| `go test -race -count=5 ./...` | no failures or races |
| `go vet` (default, desktop, dev tags) | pass |
| `golangci-lint` (default and desktop tags) | 0 issues |
| `govulncheck` | 0 affecting the code (17 in required modules, not called) |
| Fuzzing, 20 s each | parser, path confinement, import: no failure |
| ESLint, `tsc` | pass |
| Vitest | 38 passed |
| `npm audit` | 0 |
| `wails build` | pass (12.4 MB binary) |
| Windows / macOS | cross-compile `vet` only |

These were run before this report; the only later changes are documents.

Checked by driving the running Linux app (temporary probes, not committed): web
view restrictions, switching conversations mid-run, stop and reload, native tool
calls with approval and diff, every settings tab, the dialogs, timeline, file
changes and undo. Not clicked through: backup, restore, key and rotation
controls, MCP restart, model settings buttons, voice input.

Upgrade check with data written by `main`: conversation, tags, usage, settings,
API key and permissions survive, and a pre-migration backup is made. A backup
archive from `main` passed restore validation.

## Performance

| Measurement | Before | After |
|---|---|---|
| List 500 conversations of 40 KB | 153 ms, 41.9 MB allocated | 3.6 ms, 1.4 MB |
| Frontend bundle | 187 kB | 312 kB (React 19 and new screens) |

Nothing else was measured.

## UNVERIFIED

- Remote CI, including the new Windows and macOS test jobs. Some tests assume
  Unix behaviour and may fail there.
- Windows and macOS at runtime; the macOS sandbox profile.
- Live Ollama, OpenAI-compatible or free-tier services.
- Real system keychain.
- Real ChatGPT and Claude export files.
- Voice recording and transcription after the refactor.
- Installers and packages.

## Remaining security and release risks

- No shell isolation on Windows; MCP servers run unsandboxed everywhere.
- With the default key file, the data folder can be read by anyone who copies it.
- Backups from releases before this branch contain the key.
- The change journal keeps previous file contents, which may include secrets.
- The sandbox default will break workflows needing network or outside files.
- A large amount of change in one branch has had no human review.

## Data and compatibility

Schema 2 adds a conversation index, with an automatic backup before migrating.
Breaking changes and rollback steps: `docs/UPGRADING.md`. Move a keychain key
back to a file before downgrading.

New runtime dependency: `github.com/zalando/go-keyring`; `golang.org/x/crypto`
is now direct. Development only: Vitest, jsdom, Testing Library, ESLint and its
plugins. Removed: unused `lucide-react`. CONTRIBUTING asks for new
dependencies to be discussed.

## Process notes

`CLAUDE.md` asks for impact analysis before each symbol edit and
`detect_changes` before each commit. Impact analysis was run for the main
entry points of most packages, not every symbol, and `detect_changes` only for
some early commits. The index is current as of this report's parent commit.

## Suggested next steps

1. Review the branch; authorize a push when ready and fix what CI finds.
2. Try a real model with tools, and the backup, restore and key controls.
3. Decide: keychain by default; sandbox default; the new dependencies.

## Local commits

- `7dac9c6` docs: add implementation plan and recorded baseline
- `a655892` build: allow plain go build and test on Linux without GTK headers
- `3152778` style: apply gofmt to all Go files
- `62df1a6` test: establish baseline coverage and reusable fakes
- `f9077f0` fix(import): keep Claude-style conversation names
- `ae67d6b` fix: resolve linter findings, including ignored write errors
- `179bfa1` build(deps): require Go 1.26.11 and upgrade vulnerable modules
- `ed1c69e` ci: add format, vet, lint, test, race and vulnerability gates
- `6126d37` fix(security): enforce workspace filesystem boundaries
- `46c9399` feat(security): run shell commands in an isolated sandbox
- `422bc26` feat(security): add scoped, revocable tool approvals
- `43921c8` feat(security): treat tool output as untrusted data
- `608acf8` fix(security): keep API keys out of error messages and logs
- `2228916` fix(security): lock down the embedded web view
- `8f367b9` fix(storage): never overwrite an existing encryption key
- `419b509` feat(security): keychain key storage and encrypted backups
- `261a139` docs: record progress through the security phase
- `7d0f565` fix(llm): prevent duplicate streaming output on retry
- `0262097` feat(agent): typed tool arguments and cancellation of running tools
- `92704f0` fix(agent): trimming the context no longer deletes saved history
- `20080e0` fix(chat): isolate concurrent conversation execution
- `dce421b` fix(mcp): share and manage MCP server processes
- `4d06b4d` feat(llm): implement native function calling
- `4020b00` feat(llm): discover models from the provider
- `49ccfa8` build(deps): upgrade Vite, TypeScript and the React plugin
- `426a634` test(frontend): configure Vitest and cover untrusted output handling
- `99f9e50` test(domain): fuzz the text-protocol tool call parser
- `f28ce11` docs: describe architecture, security guarantees and upgrade path
- `49ee9ca` docs: add implementation report and final plan status
- `2a04c3b` docs: record local-only git restrictions in the plan
- `74e644a` refactor(storage): index conversation metadata with a versioned migration
- `72b38fc` feat(usage): record real usage and enforce limits
- `71d8e02` feat(llm): model capabilities, overrides and connection testing
- `cc6b576` feat(context): add summarisation strategies and calibrated token counts
- `052668c` feat(observability): tool timeline, run logs, limits and diagnostics
- `c8dd92a` feat(files): review, approve and undo the agent's file changes
- `66e376c` feat(workspace): add agent profiles and named workspaces
- `daa38e7` refactor(app): split app.go by responsibility
- `5df31f0` fix(import): read real ChatGPT and Claude export layouts
- `2ad883e` feat(mcp): health checks and tool list refresh
- `b523a8e` feat(security): rotate the database encryption key
- `a8ce997` refactor(ui): split App.tsx into components and hooks
- `acb9698` fix(diff): no newline note for created or emptied files
- `316b73a` build(deps): upgrade to React 19 and drop unused lucide-react
- `d2f5caa` build(ui): add ESLint and fix accessibility findings
- `d4ed1a0` docs: cover profiles, usage, review, storage and the threat model
- `c739673` build: keep Go tooling out of node_modules
- `03746b6` test(ui): cover the permission dialog
- (this report and the final plan status)
