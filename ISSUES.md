# Issues

Track unresolved bugs, design concerns, and technical debt discovered while working on other tasks.

This file is not a replacement for GitHub issues. Remove an item when it has been resolved, or move it to `Resolved` when keeping a short record is useful.

## Open

<!-- Add items here as they are discovered. Include the relevant file or area when possible. -->

- **Workflow import of a filtered array retry silently accepts stale failed tasks** (`cmd/rotari/workflow_reconcile.go`, `reconcileCommandLeaves` / `sourceLeafForCommand`; test `TestWorkflowExportImportOfFilteredArrayRetryReusesRetriedTask`): an all-success array run whose task 1 was carried from an earlier run exports its job-level `attempt_id` from that earlier run. Import anchors the whole array on that earlier run and reads the other tasks' results from it, so a task that was re-executed successfully in the exported run resolves to its earlier failed attempt. Because the aggregate status is `success`, that task becomes `TaskAccepted` (`success (accepted)`) linked to the failed attempt instead of reusing the successful retry.

## Resolved

<!-- Keep only short records of resolved items when they may help prevent recurrence. -->

- **SSH metadata read errors are reported as "job is not running"** (`internal/executor/ssh.go`, `readSSHMetadata`): fixed to treat only missing metadata as "job is not running" while preserving distinct errors for malformed or unreadable state.
- **`state.WriteJSON` ignores the configured shared/private file modes** (`internal/state/store.go`, `cmd/rotari/state_store.go`): fixed to use the configured `DirectoryMode()` / `FileMode()` instead of hard-coded private defaults.
- **Production logging depends directly on the Go test runtime** (`cmd/rotari/color.go`, `jobLogf`): fixed by removing the `testing.Testing()` dependency and exposing an explicit package-level log hook that tests can override without affecting production behavior.
