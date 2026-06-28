---
name: commit
description: >
  Generate git commit messages with branch name prefix and conventional commit format.
  Use whenever the user asks to write a commit message, generate a commit, describe
  changes for a commit, or says things like "write a commit for this", "commit message
  for my changes", "help me commit", or "what should my commit say". Always triggers
  when the user mentions staged changes, diffs, or wants to record work in git.
---

# Commit Message Skill

Generate well-structured git commit messages with the branch name prefix.

## Format

```
[branch-name]  type: short description

Optional body explaining what and why (not how).
```

### Rules

1. **Branch prefix is always first** — wrap the current branch name in square brackets: `[feature/login]`
2. **Conventional commit type** — use one of: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `perf`, `ci`, `build`, `revert`
3. **Short description** — imperative mood, lowercase, no period, max ~72 chars total per line
4. **No Co-authored-by lines** — never add `Co-authored-by:` trailers under any circumstances
5. **No signatures or trailers** — no `Signed-off-by`, `Reviewed-by`, or any other trailer lines
6. **Body is optional** — include only when the why/context genuinely adds value

### Commit Types

| Type | When to use |
|------|-------------|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, whitespace (no logic change) |
| `refactor` | Code restructure without behavior change |
| `test` | Adding or fixing tests |
| `chore` | Tooling, deps, config (no production code) |
| `perf` | Performance improvement |
| `ci` | CI/CD pipeline changes |
| `build` | Build system or dependency updates |
| `revert` | Reverting a prior commit |

### Branch Name Handling

- Use the branch name exactly as provided or detected
- If no branch is given, ask for it or use a placeholder like `[your-branch]`
- Common patterns: `feature/thing`, `fix/bug-name`, `hotfix/issue`, `chore/update-deps`

## Examples

**Simple feature:**
```
[feature/login]  feat: add JWT authentication middleware
```

**Bug fix with body:**
```
[fix/null-pointer]  fix: handle null user in profile loader

Previously crashed when a guest session tried to load profile data.
Now returns an empty profile object instead.
```

**Refactor:**
```
[refactor/auth]  refactor: extract token validation into separate service
```

**Docs:**
```
[docs/readme]  docs: update setup instructions for Node 20
```

## Workflow

1. **Get the branch name** — ask if not provided or inferable
2. **Understand the changes** — read the diff, file list, or user description
3. **Pick the right type** — match the primary intent of the change
4. **Write the subject line** — `[branch]  type: description`
5. **Add body if needed** — only when context genuinely helps the reader
6. **Never add Co-authored-by or any trailers**

## What NOT to do

- Missing branch prefix: `feat: add login`
- Capitalized or period at end: `[feature/login] feat: Add Login.`
- Co-authored-by trailer: never add these under any circumstances
- Vague messages: `[main]  fix: fixed stuff`
- Multiple types in one commit message