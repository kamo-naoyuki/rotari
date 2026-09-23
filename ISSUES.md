# Issues

Track unresolved bugs, design concerns, and technical debt discovered while working on other tasks.

This file is not a replacement for GitHub issues. Remove an item when it has been resolved, or move it to `Resolved` when keeping a short record is useful.

## Open

<!-- Add items here as they are discovered. Include the relevant file or area when possible. -->

- **SSH metadata read errors are reported as "job is not running"** (`internal/executor/ssh.go`, `readSSHMetadata`): missing `job.json` and unrelated read failures such as permission errors currently produce the same error. This may be intentional to avoid exposing filesystem details, but it can also hide corrupted or inaccessible state and mislead users. Decide whether only `os.ErrNotExist` should map to "job is not running", with other read errors returned distinctly; align the LSF/PBS/Slurm readers if the policy changes.
- **`state.WriteJSON` ignores the configured shared/private file modes** (`internal/state/store.go`, `cmd/rotari/state_store.go`): command code computes `stateDirMode()` and `stateFileMode()`, but the exported helper always constructs `NewStore(0700, 0600)`. This may be an intentional security hardening, or it may silently break the shared-state mode by making files private. Decide whether callers should pass an explicit `state.Store`, or whether `WriteJSON` should remain deliberately private and be renamed/documented accordingly.
- **Production logging depends directly on the Go test runtime** (`cmd/rotari/color.go`, `jobLogf`): `testing.Testing()` is used to suppress executor logs during tests. This keeps test output quiet, but couples production behavior to the testing package and makes logger behavior implicit. Consider injecting a logger or an explicit silent logger in tests instead.

## Resolved

<!-- Keep only short records of resolved items when they may help prevent recurrence. -->

- None
