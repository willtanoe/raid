# Raid

[![CI](https://github.com/willtanoe/raid/actions/workflows/ci.yml/badge.svg)](https://github.com/willtanoe/raid/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](go.mod)

> **A safety-first terminal toolkit for Linux maintenance and shell workflow.**

Raid combines an interactive TUI with scriptable text and JSON output. Built on native Linux facilities — procfs, sysfs, systemd, journald, APT/dpkg, DNF, Pacman, Snap, Flatpak, Docker, and the FreeDesktop Trash specification.

> [!IMPORTANT]
> Raid is experimental software. Always review the displayed plan before running a command with `--yes`.

## Highlights

- **Full TUI menu** — interactive terminal dashboard for all features
- **Live status** — CPU, memory, disk, network, uptime, battery, GPU, and thermal telemetry
- **Disk analyzer** — navigable tree with Trash-based removal
- **Cache cleanup** — predictable rebuildable caches: Go, Rust, Node, Python, Gradle, fonts, thumbnails, Mesa shaders, plus user-configurable extras
- **Cross-distro packages** — exact lookup and removal across APT, DNF, Pacman, Snap, and Flatpak
- **System updates** — unified `update` command across all detected package managers
- **Project artifact purge** — `node_modules`, `target`, `build`, `.next`, `__pycache__`, and more with ecosystem-marker validation
- **Installer cleanup** — stale `.deb`, `.appimage`, `.tar.*`, `.rpm`, `.iso`, `.zip`, `.dmg` older than 7 days
- **Docker maintenance** — prune stopped containers, unused images, volumes, and build cache
- **File search** — find by size, age, or glob pattern with JSON export and Trash removal
- **Shell history converter** — convert between zsh, fish, and bash history formats
- **Preview-first safety** — every destructive command shows an exact plan; mutations require `--yes`
- **Operation audit** — all actions logged to `~/.local/state/raid/operations.tsv` with filtering

## Installation

### Download binary (no Go required)

Download the latest pre-built binary from the [releases page](https://github.com/willtanoe/raid/releases/latest):

```bash
# x86_64 / amd64
curl -L https://github.com/willtanoe/raid/releases/latest/download/raid -o raid
chmod +x raid
sudo mv raid /usr/local/bin/
raid

# ARM64 (Raspberry Pi, Apple Silicon Linux VM, etc.)
curl -L https://github.com/willtanoe/raid/releases/latest/download/raid-linux-arm64 -o raid
chmod +x raid
sudo mv raid /usr/local/bin/
raid
```

Or download a specific version from the [releases page](https://github.com/willtanoe/raid/releases).

### Build from source

Go 1.24 or newer required.

```bash
git clone https://github.com/willtanoe/raid.git
cd raid
make build
./bin/raid
```

System-wide install:

```bash
sudo make install
raid
```

Uninstall:

```bash
sudo make uninstall
```

## Usage

Run without arguments to open the full TUI menu:

```bash
raid
```

| Command | Purpose |
|---|---|
| `raid status` | Live system dashboard (CPU, memory, disk, network, thermal, battery, GPU) |
| `raid analyze [path]` | Interactive disk analyzer or JSON/text report |
| `raid clean` | Preview and remove rebuildable user and developer caches |
| `raid uninstall <pkg>` | Exact package lookup across APT, DNF, Pacman, Snap, Flatpak |
| `raid update` | Unified system updates across all detected package managers |
| `raid optimize` | systemd, font, journal, and package-cache maintenance (cross-distro) |
| `raid purge [path]` | Find and remove rebuildable project artifacts |
| `raid installer` | Find stale installer files in `~/Downloads` |
| `raid docker` | Prune unused Docker containers, images, volumes, and build cache |
| `raid search` | Find files by `--min-size`, `--older-than`, `--pattern` with `--json` |
| `raid convert <src>` | Convert shell history between zsh, fish, and bash formats |
| `raid history` | Filterable operation audit log (`--command`, `--since`, `--json`) |
| `raid completion <sh>` | Generate shell completions for Bash, Zsh, or Fish |
| `raid fingerprint [status\|enroll]` | Inspect or enroll fingerprints via fprintd |

### Common flags

| Flag | Effect |
|---|---|
| `--dry-run` | Force preview (default for destructive commands) |
| `--yes`, `-y` | Execute the displayed plan |
| `--permanent` | Bypass Trash for permanent removal |
| `--json` | Machine-readable JSON output |
| `--text` | Force text output when stdout is redirected |

### Examples

```bash
raid status
raid status --json

raid analyze "$HOME"
raid analyze /var/log --text

raid clean --dry-run
raid clean --yes

raid purge "$HOME/Projects/my-app" --dry-run

raid uninstall firefox
raid update --yes

raid search --min-size 100M --older-than 30d
raid search --pattern '*.log' --json

raid convert zsh-history        # zsh -> fish (default)
raid convert fish-history --to bash --yes

raid docker --yes
raid history --command clean --json
```

## TUI controls

### Main menu

| Key | Action |
|---|---|
| `Up` / `Down`, `j` / `k` | Move selection |
| `Enter` | Open selected feature |
| `q`, `Esc` | Quit |

### Disk analyzer

| Key | Action |
|---|---|
| `Enter`, `Right`, `l` | Enter directory |
| `Backspace`, `Left`, `h` | Go to parent |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `d` | Move selected entry to Trash |
| `r` | Rescan directory |
| `q`, `Esc` | Return to menu |

### Status dashboard

| Key | Action |
|---|---|
| `r` | Refresh immediately |
| `q`, `Esc` | Return to menu |

When `status` or `analyze` is launched directly from the command line, `q` and `Esc` return to the shell.

## Scriptable output

JSON and text modes are available for every command that produces structured output. Raid automatically disables the TUI when stdout is not a terminal:

```bash
raid status --json
raid analyze "$HOME" --json
raid search --min-size 1G --json
raid history --json
raid convert zsh-history --json
```

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

Do **not** run Raid with `sudo` — it intentionally rejects user-file removal as root. Privileged operations use non-interactive `sudo -n`:

```bash
sudo -v
raid uninstall package-name --yes
```

See [docs/SAFETY.md](docs/SAFETY.md) for the detailed safety contract and known limitations.

## User configuration

Place additional cache directories for `raid clean` in `~/.config/raid/config`:

```
# Optional: extra directories to include in raid clean
additional-cache-dir=/home/user/.custom/cache
additional-cache-dir=/home/user/project/.build-cache
```

Lines starting with `#` are ignored.

## Development

```bash
make check        # gofmt + go vet + go test
make test-race    # tests with race detector
make build        # compile to bin/raid
```

```
cmd/raid/          CLI entry point
internal/raid/     command logic, safety layer, TUI models
docs/              design and safety documentation
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change.

## Inspiration

Raid is inspired by [Mole](https://github.com/tw93/mole), a macOS terminal cleanup and optimization tool created by [tw93](https://github.com/tw93). Raid is an independent Linux implementation and is not affiliated with or endorsed by the Mole project.

## License

Raid is available under the [MIT License](LICENSE).
