## Summary

Describe the user-visible change and why it belongs in Raid.

## Safety review

- [ ] The exact mutation target is visible before execution.
- [ ] Destructive behavior still requires explicit approval.
- [ ] Path, symlink, mount, and package identity boundaries were considered.
- [ ] Failure does not trigger a broader or more destructive fallback.
- [ ] No password, credential, session, or user document data is collected.

## Verification

- [ ] `make check`
- [ ] `make test-race`
- [ ] `make build`
- [ ] Relevant TUI and non-interactive behavior tested
