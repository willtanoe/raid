# Safety Design

Raid changes and removes local data, so safety constraints are part of its public behavior rather than optional implementation details.

## Operating principles

1. Preview destructive plans before execution.
2. Require explicit `--yes` approval for command-line mutations.
3. Prefer recoverable Trash operations over permanent deletion.
4. Fail closed when path identity, permissions, or Trash behavior is uncertain.
5. Keep privileged package and maintenance operations separate from user-file removal.
6. Never infer removable data from broad vendor names or wildcard package identities.

## User-file boundary

Raid resolves the current user's home directory and only permits cleanup targets below that boundary. The filesystem root and home root are always rejected.

Protected home locations include credentials, keyrings, configuration, containers, and common personal-data directories. Raid also checks every existing ancestor of a target and rejects symlink ancestors that could redirect an apparently safe path.

Raid refuses user-file removal when the process effective user ID is root. Run Raid as the regular desktop user even when a package operation needs sudo.

## Trash behavior

The default deletion mode moves an item to:

```text
~/.local/share/Trash/files
```

Raid writes the matching FreeDesktop `.trashinfo` metadata under `~/.local/share/Trash/info`. Trash directories must be real directories rather than symlinks. Name collisions are handled with unique suffixes.

Raid does not silently switch to permanent deletion when a Trash move fails or crosses a filesystem boundary. `--permanent` must be explicitly combined with `--yes`.

## Project purge

Purge does not remove every directory named `build`, `target`, or `node_modules`. A candidate must be inside a recognized project and match the relevant ecosystem marker. Examples include:

- `node_modules` with `package.json`
- `target` with `Cargo.toml`
- Python caches with `pyproject.toml` or `requirements.txt`
- Gradle artifacts with `build.gradle`

Traversal does not cross filesystem device boundaries, and mounted target directories are rejected again before mutation.

## Package operations

Raid enumerates exact installed package identities through dpkg, Snap, and Flatpak. Package names are passed directly to subprocess argument arrays and are never interpolated through a shell.

Privileged package operations use `sudo -n`. This requires a previously authorized sudo session, typically created with `sudo -v`. Raid does not collect, store, or process passwords.

## Test overrides

`RAID_HOME` and `RAID_STATE_DIR` are ignored during normal execution. They are only honored when `RAID_TEST_MODE=1`, allowing tests to use isolated temporary directories without widening production deletion boundaries.

## Known limitations

- The Trash implementation currently targets the user's home Trash and refuses cross-filesystem moves instead of selecting a per-volume Trash directory.
- Path validation narrows time-of-check/time-of-use races but does not yet use Linux `openat2` directory-file-descriptor enforcement.
- Cleanup targets are intentionally conservative and do not attempt broad orphan discovery.
- Raid currently targets Ubuntu and has not been validated as a general cross-distribution maintenance tool.

Security-sensitive changes should include regression tests and receive explicit review of every destructive branch.
