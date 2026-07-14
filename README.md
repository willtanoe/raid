# Raid

[![CI](https://github.com/willtanoe/raid/actions/workflows/ci.yml/badge.svg)](https://github.com/willtanoe/raid/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](go.mod)

Raid is a safety-first terminal maintenance toolkit for Ubuntu. It combines an interactive TUI with scriptable text and JSON output, using native Linux facilities such as APT/dpkg, Snap, Flatpak, systemd, journald, procfs, sysfs, and the FreeDesktop Trash specification.

> [!IMPORTANT]
> Raid is experimental software. Always review the displayed plan before running a command with `--yes`.

## Highlights

- Full-screen menu when `raid` is started in an interactive terminal.
- Live system dashboard for CPU, memory, disk, network, uptime, and thermal data.
- Navigable disk analyzer with safe Trash-based removal.
- Conservative cleanup for known rebuildable caches.
- Exact package lookup across APT, Snap, and Flatpak.
- Project artifact discovery tied to ecosystem-specific project markers.
- Preview-first destructive commands and operation history.
- Stable text and JSON modes for shell scripts and automation.

## Installation

### Build from source

Raid currently requires Go 1.24 or newer when building from source.

```bash
git clone https://github.com/willtanoe/raid.git
cd raid
make build
./bin/raid
```

Install the binary system-wide:

```bash
sudo make install
raid
```

Uninstall the binary:

```bash
sudo make uninstall
```

## Usage

Run Raid without arguments to open the main TUI:

```bash
raid
```

Available commands:

| Command | Purpose |
| --- | --- |
| `raid clean` | Preview known rebuildable user and developer caches |
| `raid uninstall <package>` | Find an exact APT, Snap, or Flatpak package |
| `raid optimize` | Preview bounded systemd, font, journal, and APT maintenance |
| `raid analyze [path]` | Explore disk usage interactively or produce a report |
| `raid status` | Open the live system status dashboard |
| `raid purge [path]` | Find rebuildable project artifacts |
| `raid installer` | Find installer files in Downloads that are older than seven days |
| `raid history` | Read the local operation audit log |
| `raid completion <shell>` | Generate Bash, Zsh, or Fish completion |
| `raid fingerprint [status\|enroll]` | Inspect or enroll fingerprints through fprintd |

Examples:

```bash
raid status
raid analyze "$HOME"
raid clean --dry-run
raid purge "$HOME/Projects/my-app" --dry-run
raid uninstall firefox
raid installer
raid history
```

### TUI controls

Main menu:

| Key | Action |
| --- | --- |
| `Up` / `Down`, `j` / `k` | Move selection |
| `Enter` | Open the selected feature |
| `q`, `Esc` | Exit Raid |

Disk analyzer:

| Key | Action |
| --- | --- |
| `Up` / `Down`, `j` / `k` | Move selection |
| `Enter` | Open a directory |
| `Backspace`, `Left`, `h` | Open the parent directory |
| `d` | Request Trash removal for the selected entry |
| `r` | Rescan the directory |
| `q`, `Esc` | Return to the main menu |

Status dashboard:

| Key | Action |
| --- | --- |
| `r` | Refresh immediately |
| `q`, `Esc` | Return to the main menu |

When `status` or `analyze` is launched directly rather than through the main menu, `q` and `Esc` return to the shell.

### Scriptable output

Raid automatically avoids the TUI when standard output is not a terminal. Output modes can also be selected explicitly:

```bash
raid status --text
raid status --json
raid analyze /var/log --text
raid analyze "$HOME" --json
```

## Destructive operations

File cleanup commands only display a plan by default:

```bash
raid clean
raid clean --dry-run
```

Use `--yes` after reviewing the exact targets:

```bash
raid clean --yes
```

Files are moved to FreeDesktop Trash by default. Permanent removal additionally requires `--permanent`:

```bash
raid clean --yes --permanent
```

## Sudo model

Do **not** run the entire application with `sudo`. Raid intentionally rejects user-file removal when its process is running as root.

APT, Snap, journal, and other privileged operations use non-interactive `sudo -n`. Start or refresh your sudo session first, then run Raid as your regular user:

```bash
sudo -v
raid uninstall package-name --yes
```

If no cached sudo session exists, the privileged action fails instead of opening an unexpected password prompt in the middle of a cleanup operation.

## Safety model

- Destructive commands preview their plan unless `--yes` is provided.
- User-file operations are restricted to the current user's home directory.
- Filesystem root, home root, credentials, keyrings, configuration, and common personal-data directories are protected.
- Symlink ancestors and mounted directory targets are rejected.
- Trash failure never falls back to permanent deletion.
- Purge targets require both a recognized artifact name and a matching project marker.
- Package removal uses exact installed identities and does not guess vendor-wide leftovers.
- Partial failures produce a non-zero exit status.
- Operations are logged to `~/.local/state/raid/operations.tsv`.

See [docs/SAFETY.md](docs/SAFETY.md) for the detailed safety contract and known limitations.

## Development

```bash
make check
make test-race
make build
```

The source tree follows the standard Go command layout:

```text
cmd/raid/       CLI entry point
internal/raid/  command logic, safety layer, and TUI models
docs/           design and safety documentation
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change.

## Inspiration

Raid is inspired by [Mole](https://github.com/tw93/mole), a macOS terminal cleanup and optimization tool created by [tw93](https://github.com/tw93). Raid is an independent Linux implementation and is not affiliated with or endorsed by the Mole project.

## License

Raid is available under the [MIT License](LICENSE).
