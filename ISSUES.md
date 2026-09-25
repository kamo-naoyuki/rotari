# Issues

Track unresolved bugs, design concerns, and technical debt discovered while working on other tasks.

This file is not a replacement for GitHub issues. Remove an item when it has been resolved, or move it to `Resolved` when keeping a short record is useful.

## Open

<!-- Add items here as they are discovered. Include the relevant file or area when possible. -->

## Resolved

<!-- Keep only short records of resolved items when they may help prevent recurrence. -->

- **Workflow import of a filtered array retry silently accepted stale failed tasks** (`cmd/rotari/workflow_reconcile.go`, `listedCommandRun`): leaves without a manifest attempt were read from the run named by the job-level anchor attempt. They are now recovered from the listed source run that supplied the command, following carried attempt IDs to their original run.
- **SSH metadata read errors are reported as "job is not running"** (`internal/executor/ssh.go`, `readSSHMetadata`): fixed to treat only missing metadata as "job is not running" while preserving distinct errors for malformed or unreadable state.
- **`state.WriteJSON` ignores the configured shared/private file modes** (`internal/state/store.go`, `cmd/rotari/state_store.go`): fixed to use the configured `DirectoryMode()` / `FileMode()` instead of hard-coded private defaults.
- **Production logging depends directly on the Go test runtime** (`cmd/rotari/color.go`, `jobLogf`): fixed by removing the `testing.Testing()` dependency and exposing an explicit package-level log hook that tests can override without affecting production behavior.
