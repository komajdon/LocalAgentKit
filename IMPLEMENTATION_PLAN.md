# LocalAgentKit — Implementation Plan

Branch: `dev/hardening` (from `main` @ `abbc7c1`). Nothing is pushed or merged.

## Baseline (2026-11-26, before any change)

Environment: Linux amd64, Go 1.26.6, Node 20.20.2, npm 10.8.2, golangci-lint 2.13.1.
Not available: `webkit2gtk-4.1` / GTK dev headers, `wails` CLI, Docker access.

| Check | Result |
|---|---|
| `npm ci && npm run build` | passes |
| `npm audit` | 8 findings (3 moderate, 5 high), build tooling |
| `go build ./...` | **fails**: `pkg-config` cannot find `webkit2gtk-4.1` |
| `go vet` (all packages except `main`) | passes |
| `GOOS=windows CGO_ENABLED=0 go vet .` | passes (type check of `main`) |
| `go test` | no test files in any package |
| `gofmt -l` | 9 files unformatted |
| `golangci-lint run` (non-`main`) | 16 issues: 13 errcheck, 2 staticcheck, 1 unused |
| `govulncheck` (non-`main`) | 11 reachable stdlib vulnerabilities in go1.26.6 |
| Frontend tests / lint | none configured |

`go.mod` declares Go 1.22; CI builds with 1.24.

## Work packages

Status: `todo`, `done`, `partial`, `blocked`. Priority P0 = security/data integrity.

| ID | Package | Pri | Depends | Acceptance criteria | Status |
|----|---------|-----|---------|---------------------|--------|
| WP01 | Plan and baseline | P0 | – | This file committed | done |
| WP02 | Headless build and test on Linux; gofmt | P0 | – | `go build ./...` and `go vet ./...` pass without GTK headers | done `a655892` `3152778` |
| WP03 | Baseline tests and fakes | P0 | WP02 | Tests for parser, paths, cut helpers, versions, import, agent loop, permissions, streaming | done `62df1a6` (+`f9077f0`) |
| WP04 | CI quality gates | P0 | WP03 | CI runs format, lint, vet, test, race, frontend build | done `ae67d6b` `179bfa1` `ed1c69e`; CI not run (nothing pushed) |
| WP05 | Filesystem boundaries (4.1) | P0 | WP03 | Symlink and traversal escapes fail; regression tests | done `6126d37` |
| WP06 | Shell sandbox (4.2) | P0 | WP03 | Isolation backends, scrubbed env, strict mode fails closed, level visible | done `46c9399`; macOS profile not run; no Windows backend; no container backend |
| WP07 | Permission policy (4.3) | P0 | WP03 | Central policy; session/workspace grants; revocation; sensitive tools ask | done `422bc26` |
| WP08 | Untrusted tool output (4.4) | P0 | WP07 | Delimited, size-capped, provenance; cannot change policy | done `43921c8` |
| WP09 | Keychain, migration, encrypted backups (4.5) | P0 | WP03 | Old data still readable; no raw key in backups | done `8f367b9` `419b509`; keychain opt-in; no key rotation; real OS keychain not exercised |
| WP10 | WebView hardening (4.6) | P0 | – | Deny by default; CSP; mic only on request | done `2228916`; verified on Linux only |
| WP11 | Credential redaction (4.7) | P0 | – | Keys absent from errors and URLs where possible | done `608acf8` |
| WP12 | Typed args and context in tools (5, 9) | P1 | WP03 | JSON-typed args; Stop cancels shell, MCP, search, documents | todo |
| WP13 | Native tool calling (5) | P1 | WP12 | OpenAI and Ollama native calls; multiline text fallback | todo |
| WP14 | Streaming and retry (6) | P1 | WP13 | No duplicate output; backoff, jitter, Retry-After | todo |
| WP15 | Conversation concurrency (7) | P0 | WP12 | Events carry conversation and run IDs; race tests pass | todo |
| WP16 | MCP lifecycle (8) | P1 | WP12 | One shared process per server config; status and restart | todo |
| WP17 | Context management (10) | P1 | WP13 | Whole turns trimmed; system prompt counted once | todo |
| WP18 | Storage and backups (11) | P1 | WP15 | Metadata bucket, migration, consistent snapshots | todo |
| WP19 | Usage and budgets (12) | P2 | WP18 | Real counts; no invented prices; limits | todo |
| WP20 | Model discovery (13) | P2 | – | `/models` with static fallback and cache | todo |
| WP21 | Workspaces and profiles (14.1–2) | P2 | WP07 | | todo |
| WP22 | Timeline, diff review, observability (14.3–5) | P2 | WP13 | | todo |
| WP23 | Backend refactor (15) | P2 | WP15 | `app.go` split by responsibility | todo |
| WP24 | Frontend refactor and tests (16, 17) | P2 | WP15 | `App.tsx` split; component tests | todo |
| WP25 | Dependency upgrades (16) | P2 | WP24 | Audit clean; separate commits | todo |
| WP26 | Documentation and final report (18–20) | P1 | all | Docs match behaviour; `IMPLEMENTATION_REPORT.md` | todo |

## Decisions

- New dependencies only where the task requires them (CONTRIBUTING asks for discussion): `github.com/zalando/go-keyring`; `golang.org/x/crypto` is now used directly.
- Go 1.26.11 and Wails 2.13.0, for security fixes and `os.Root`.
- Shell sandbox defaults to `auto` with network off.
- Keychain storage is opt-in, because the data folder is otherwise portable.
- After web, document or MCP content enters a conversation, "Always Allow" no longer covers sensitive tools.

## Local verification setup

- `/tmp` is a small shared tmpfs: set `GOTMPDIR=$HOME/.cache/lak-gotmp`.
- A real Linux desktop build works against GTK/WebKit headers unpacked (without root) in `~/.cache/lak-sysroot`; source `~/.cache/lak-sysroot/env.sh`, then `go build -tags desktop,production,webkit2_41`. This replaces the original GTK blocker.
- Headless run: `broadwayd :7` plus `GDK_BACKEND=broadway`, with an isolated `HOME`.

## Known limits

- macOS and Windows are type-checked by cross-compilation only.
- The GitNexus index is from `abbc7c1`; it does not know the new files.

## Next action

WP14 (streaming and retry), then WP12, WP15, WP16, WP13.
