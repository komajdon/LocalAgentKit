# Security model

## What is being protected, and from whom

The agent runs on your computer with your account. The main threats are:

1. **A model doing something you did not intend**, by mistake or because text it
   read told it to (prompt injection).
2. **Content from outside** (web pages, documents, MCP servers, model output)
   reaching the app's own interface or the Go bindings.
3. **Someone obtaining your data files or backups.**

Out of scope: malware already running as you, a compromised OS, a malicious MCP
server you chose to install, and what a cloud model provider does with what you
send it.

## Threat model

| Threat | Main controls | What remains |
|---|---|---|
| Injected instructions in web pages, documents or MCP output | Approval checks in code; blanket "allow" suspended for sensitive tools after external content; output framed as data | You can still approve a malicious request. Read-only tools can send file contents to the model |
| The model damages files | Diff before approval, conflict detection, journal with undo | Shell and MCP changes are not journaled |
| The model reaches outside the project | `os.Root` confinement, ignore patterns, shell sandbox | No sandbox on Windows; MCP servers unsandboxed; shell ignores ignore patterns |
| Remote content runs in the app's window | No raw HTML, CSP, navigation and permission blocking | Windows and macOS web view behaviour untested |
| Stolen data folder or backup | Encryption, keychain option, passphrase backups | The default key file sits beside the database |
| Runaway cost | Round limit, repeat detection, token and cost limits | A request in flight can overshoot; cost limits depend on your prices |

## Permissions

Every tool call is checked. Order: a global *deny*, a global *allow*, a stored
approval, otherwise you are asked.

- Approvals are scoped to one work directory and last once, until the app quits,
  or until revoked.
- For anything other than the read-only tools, an approval covers only the exact
  same arguments.
- Once a conversation contains web search results, document text or MCP output,
  *Always Allow* no longer covers sensitive tools.
- Read-only tools can still send file contents to your model provider.
- Profiles and workspaces can only make a permission stricter or remove a tool.

## Workspace ignore patterns

File and document tools cannot list or open matching paths, including through
links. **Shell commands and MCP servers are not bound by them.** Git status in
the review screen can show the names of ignored files.

## File changes

Writes and deletes made by the built-in tools are journaled in the encrypted
database with the previous content (up to 1 MiB per file, 200 per
conversation). That content can include secrets from your files. Deleting the
conversation removes it. Undo refuses if the file changed since.

The system prompt tells the model to treat tool output as data. That is a
mitigation, not a boundary; the permission check is the boundary.

## File access

File tools are confined to the work directory with `os.Root`, which resists
`..` traversal and symlink swaps. The app's own data directory is refused even
if it lies inside the work directory. Hard links are not detected. Symlinks
with absolute targets are refused.

**A work directory is not a sandbox for the shell tool.** That is the job of the
shell sandbox below.

## Shell sandbox

| Platform | Backend | Files readable | Files writable | Network |
|---|---|---|---|---|
| Linux | bubblewrap | system dirs (`/usr`, `/etc`, `/opt`, …) and the work directory | work directory, private `/tmp` | blocked unless enabled |
| macOS | sandbox-exec | system paths and the work directory; not other home directories, volumes or temp dirs | work directory and a private temp dir | blocked unless enabled |
| Windows | **none** | everything you can | everything you can | allowed |

- *Automatic* runs unisolated when no backend works, and each result says so.
  *Strict* refuses instead.
- All commands get a minimal environment (no API keys from the parent), output
  and time limits, and their process tree is killed on Stop. CPU-time and
  file-size limits apply on Linux and macOS. There is no memory limit.
- If the work directory is your home directory, the sandbox masks common
  credential locations (`~/.ssh`, `~/.aws`, …) on Linux, but not every secret in
  your home. Prefer a project directory.
- `/etc` is readable inside the Linux sandbox.
- On macOS and Linux, commands get a private `HOME` unless the work directory
  contains your real one.
- On macOS, `mktemp` is wrapped to use the private temporary directory. Programs
  that ask the system for the shared one by other means are refused.
- **The current macOS profile has not been run on a Mac.**
- Container-based execution is not implemented.

**MCP servers are not sandboxed.** They run as you. They do not inherit the
app's environment beyond `PATH` and `HOME`.

## Encryption

- Conversations, settings, API keys and approvals are stored in a bbolt database
  with each value sealed by AES-256-GCM. Bucket and record names are not
  encrypted.
- By default the key is `data/agent.key`, beside the database. This protects the
  database only from someone who gets it **without** the key file.
- Moving the key to the OS keychain removes the key file. The data folder then
  only opens under that user account. Not exercised against a real keychain in
  testing.
- Exported backups are encrypted with Argon2id + AES-256-GCM under your
  passphrase. Automatic backups contain no key.
- Backups made by earlier versions, and `pre-restore-*` folders, contain the key.
- Key rotation re-encrypts everything (file key only). Automatic backups made
  before a rotation can no longer be restored.
- Upgrading copies the database to `data/backups/pre-migration-*.db`.

## Interface

- Model output is rendered without raw HTML; only `http`, `https` and `mailto`
  links are kept, and they open in your system browser.
- Production builds carry a Content Security Policy that allows only the app's
  own scripts.
- On Linux the web view denies all permission requests except notifications and
  refuses to navigate away from the app. WebView2 (Windows) and WKWebView
  (macOS) use their defaults, and were not tested.

## Credentials in messages

API keys are removed from provider, search and MCP error messages. Redaction is
pattern-based and cannot recognise every secret. A command you approve can still
print a secret into the conversation, which is then sent to the model.

## Other limits

- The Whisper model download is not checksum-verified.
- Message search decrypts conversations whose title and tags do not match.
- `data/logs/app.log` is plain text. It holds names, timings, counts and error
  categories, not prompts, arguments or results; retry errors may quote a
  provider's message, after redaction.
- There is no telemetry. The app contacts your model provider, your search
  provider, GitHub (update check) and Hugging Face (Whisper model).

## Reporting

Please report security problems privately to the maintainers rather than in a
public issue.
