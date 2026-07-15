# Security Policy

## Supported versions

Raid is currently experimental. Security fixes are applied to the latest version on the default branch.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting feature for this repository. Do not include sensitive local paths, credentials, or private logs in a public issue.

Useful reports include:

- the Raid version from `raid version`;
- the Linux distribution, version, and kernel;
- the exact command and flags used;
- whether the operation was a preview, Trash move, permanent removal, or privileged package action;
- a minimal reproduction using temporary files when possible.

Reports involving path-boundary bypasses, symlink handling, unintended package removal, command injection, or privilege escalation are treated as high priority.
