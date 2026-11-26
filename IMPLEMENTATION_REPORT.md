# Implementation report

Branch `dev/hardening`, from `main` @ `abbc7c1`. Date: 2026-11-26.
Nothing has been pushed or merged.

## Executive summary

The security, data-integrity, concurrency and reliability work is done and
tested on Linux. Much of the feature and refactoring work is **not** done:
usage and budgets, workspaces, profiles, tool dashboard, file-diff review,
observability, storage indexing and the frontend refactor.

**Release readiness: not ready.** CI has never run on this branch, macOS and
Windows have not been executed, and no live model has been used.

## Data-loss bugs found during the work

These were not in the original review.

1. Reaching the context limit (default 8192) permanently deleted old messages.
2. A damaged or unreadable `agent.key` was overwritten with a new key.
3. Restoring a backup overwrote the database while it was open.
4. Stopped and failed turns were never saved.

All are fixed. Releases from before this branch still have them.

## Verified original issues

Each was confirmed in source or by a failing test before the fix: fragile tool
parsing; duplicated output on retry; MCP process accumulation; conversation
races; shell `dir` unchecked and symlink escapes; key stored with the data and
in backups; web view granting every permission; SerpAPI key in errors (proved
by test); trimming and token-count errors; no tests.

Also found: a deadlock on a double permission response, Claude-style import
names being lost, titles cut mid-character.

Correction to the review: there are 13 built-in free providers, not 14.

## Work packages

| Work Package | Status | Commit Hash | Tests | Remaining Issues |
|---|---|---|---|---|
| Plan and baseline | Done | `7dac9c6` `261a139` | baseline recorded | – |
| Headless build, format, lint | Done | `a655892` `3152778` `ae67d6b` | vet, lint | – |
| Baseline tests and fakes | Done | `62df1a6` `99f9e50` | unit, fuzz | – |
| CI gates and upgrades | Partial | `179bfa1` `ed1c69e` | local equivalents | CI never run |
| Import names | Done | `f9077f0` | unit | Real ChatGPT/Claude export layouts probably unsupported |
| Filesystem boundaries (4.1) | Done | `6126d37` | symlink, fuzz | Hard links |
| Shell sandbox (4.2) | Partial | `46c9399` | Linux integration | No Windows or container backend; macOS unrun; no memory limit |
| Permissions (4.3) | Done | `422bc26` | policy, deadlock | – |
| Prompt injection (4.4) | Done | `43921c8` | malicious content | Mitigation, not a guarantee |
| Redaction (4.7) | Done | `608acf8` | leak tests | Pattern based |
| Web view (4.6) | Done (Linux) | `2228916` | probed in running app | Windows/macOS defaults unchecked |
| Keys and backups (4.5, 11) | Partial | `8f367b9` `419b509` | migration, restore, tamper | Keychain opt-in; no rotation; real keychain unused |
| Streaming and retry (6) | Done | `7d0f565` | fault injection | – |
| Typed args, cancellation (9) | Partial | `0262097` | cancel, limits | No configurable budgets |
| Context (10) | Partial | `92704f0` | window | Estimate only; no summarisation or strategies |
| Concurrency (7) | Done | `20080e0` | race, event routing | One run at a time by design |
| MCP lifecycle (8) | Done | `dce421b` | real subprocess | No health ping or tool-change notifications |
| Native tool calling (5) | Partial | `4d06b4d` | fake servers, app run | No capability abstraction; no live model |
| Model discovery (13) | Partial | `4020b00` | fallback | No cache, overrides or connection test |
| Frontend tooling (16) | Partial | `49ccfa8` `426a634` | 17 Vitest | React still 18 |
| Documentation (18) | Done | `f28ce11` | – | Threat model is within SECURITY.md |
| Storage metadata, pagination, migrations (11) | **Not done** | – | – | Listing decrypts everything |
| Usage and budgets (12) | **Not done** | – | – | The artificial budget widget is still there |
| Workspaces, profiles (14.1–2) | **Not done** | – | – | – |
| Timeline, diff review, observability (14.3–5) | **Not done** | – | – | – |
| Backend refactor (15) | Partial | in `20080e0` `419b509` | – | `app.go` 1336 → 658 lines |
| Frontend refactor (16) | **Not done** | – | – | `App.tsx` grew 1651 → 1900 lines |

## Architectural changes

- Each run owns its agent; events carry conversation, run and sequence IDs.
- New packages: `permission`, `sandbox`, `backup`, `redact`.
- Shared MCP pool; `Tool.Execute` takes a context and JSON-typed arguments.
- Native tool calls and tool-role messages, with the text protocol as fallback.

See `docs/ARCHITECTURE.md` and `SECURITY.md`.

## Data and compatibility

No migration step. Stored messages and settings gain optional fields.

Checked with data written by the code on `main`: settings, API key, tool
permissions and a conversation open correctly, and an old backup archive passes
restore validation.

Breaking changes and downgrade steps are in `docs/UPGRADING.md`. The important
one: move the key back to a file before downgrading.

New dependencies (CONTRIBUTING asks for discussion): `github.com/zalando/go-keyring`;
`golang.org/x/crypto` now direct; development only: `vitest`, `jsdom`,
`@testing-library/react`, `@testing-library/dom`.

## Test results (Linux amd64, Go 1.26.11)

| Check | Result |
|---|---|
| `go test ./...` | 134 passed, 0 skipped, 0 failed |
| `go test -race -count=5 ./...` | pass |
| `go vet` (default and desktop tags) | pass |
| `golangci-lint` (default and desktop tags) | 0 issues |
| `govulncheck` | no called vulnerabilities (17 in required modules are not called) |
| Fuzzing | path confinement 1.3M and parser 3.1M executions, no failure |
| Frontend tests, type check, build | 17 passed; pass; pass |
| Frontend lint | **no linter configured** |
| `npm audit` | 0 |
| `wails build` | pass |
| Windows, macOS | cross-compile vet only |
| CI | **not run** |

The race and Wails results were taken before the final documentation commit.

Checked by driving the running Linux app headless (temporary probes, not
committed): web view restrictions, streaming while switching conversations,
stop and reload, a native tool call with its approval dialog. MCP status panel,
backup and key controls, and remembered-approval list were **not** clicked
through.

No performance measurements were taken.

## Remaining risks

- The Windows and macOS CI jobs added here may fail on first run: several tests
  use `bash`, symlinks or open-file removal and have only run on Linux.
- The macOS sandbox profile is unproven.
- Native tool calling follows the documented wire formats but has only met fake
  servers.
- The sandbox default will break workflows that relied on network access or
  files outside the work directory.
- The GTK build used headers unpacked into `~/.cache/lak-sysroot`, not a system
  installation.
- The GitNexus index predates the branch; re-run `analyze` before relying on it.
  Impact checks were run for most, not every, edited symbol.

## Suggested next steps

1. Push the branch and fix what CI finds.
2. Try a real Ollama model with tools, and one OpenAI-compatible service.
3. Decide on keychain opt-in versus default.
4. Storage metadata bucket, then real usage accounting.
5. Split `App.tsx` before adding the diff-review and timeline views.

## Commits

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
- (this report and the final plan update)
