# Contributing to Raid

Thank you for helping improve Raid. Because Raid performs cleanup and package maintenance, safety and reviewability take priority over adding more targets quickly.

## Development setup

Requirements:

- A Linux environment with procfs and sysfs
- Go 1.24 or newer
- GNU Make

```bash
git clone https://github.com/willtanoe/raid.git
cd raid
make check
make test-race
make build
```

## Pull requests

- Keep changes focused and explain user-visible behavior.
- Add tests for bug fixes and destructive code paths.
- Preserve preview-first behavior and non-interactive JSON/text output.
- Do not add broad wildcard cleanup or vendor-wide leftover matching.
- Do not introduce password collection or run the entire program as root.
- Update documentation when flags, key bindings, output, or safety behavior changes.
- Run `make check`, `make test-race`, and `make build` before opening the pull request.

## Destructive code checklist

Changes that remove files, packages, or system state must answer all of the following:

1. Can the user review the exact target before mutation?
2. Is the target rebuildable or explicitly selected by the user?
3. Does the operation remain within the documented path or package boundary?
4. Does failure stop safely without a more destructive fallback?
5. Is there a regression test for symlinks, path boundaries, and partial failure where relevant?

## Style

- Format Go code with `gofmt`.
- Prefer standard-library functionality unless a dependency materially improves the TUI or safety model.
- Keep package-manager commands as argument arrays; never build shell command strings from user input.
- Comments should explain non-obvious safety decisions, not restate code.

## Reporting security issues

Do not open a public issue for a vulnerability involving deletion boundaries, privilege handling, or command execution. Follow [SECURITY.md](SECURITY.md) instead.
