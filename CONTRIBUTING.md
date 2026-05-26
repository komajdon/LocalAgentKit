# Contributing

Thank you for your interest in contributing!

## Getting started

```bash
git clone https://github.com/your-username/agent-gui
cd agent-gui
make dev          # starts the dev server with hot-reload
```

## Reporting bugs

Open an issue with:
- OS and version
- Steps to reproduce
- Expected vs. actual behaviour
- Logs (run with `WAILS_DEBUG=1 make dev` to get verbose output)

## Pull requests

- Keep PRs focused — one feature or fix per PR.
- Run `go build ./...` and `cd frontend && tsc --noEmit` before submitting.
- No new dependencies without discussion.

## Project structure

See the Architecture section in [README.md](README.md).
