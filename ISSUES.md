# Issues

Track unresolved bugs, design concerns, and technical debt discovered while working on other tasks.

This file is not a replacement for GitHub issues. Remove an item when it has been resolved, or move it to `Resolved` when keeping a short record is useful.

## Open

<!-- Add items here as they are discovered. Include the relevant file or area when possible. -->

## Resolved

<!-- Keep only short records of resolved items when they may help prevent recurrence. -->

- **`show` job hid manual acceptance** (`cmd/rotari/show.go`, `showJobAttempt`): an accepted job printed only the ordinary carry note and the source attempt's failing exit code, while `show` run and the Web table showed `success (accepted)`. It now prints the accepted status and follows `Origin.AttemptID`.
- **Workflow import ignored local results of non-latest attempts** (`cmd/rotari/workflow_reconcile.go`, `resolveAttempt`): only wrapper `status.json` was read, so a local-executor attempt that was not the summary's latest failed with "has no completed result". It now reads the local `status` result first, matching run planning.
- **Workflow import rejected unchanged matrix runs** (`cmd/rotari/workflow_reconcile.go`, `reconcileCommandLeaves`): every matrix member was resolved against the group's job-level `attempt_id`, so any group with two or more successful combinations failed with "attempt belongs to job". Matrix members now use their own instance attempt or recover from the listed source run.
- **Workflow import of a filtered array retry silently accepted stale failed tasks** (`cmd/rotari/workflow_reconcile.go`, `listedCommandRun`): leaves without a manifest attempt were read from the run named by the job-level anchor attempt. They are now recovered from the listed source run that supplied the command, following carried attempt IDs to their original run.
- **SSH metadata read errors are reported as "job is not running"** (`internal/executor/ssh.go`, `readSSHMetadata`): fixed to treat only missing metadata as "job is not running" while preserving distinct errors for malformed or unreadable state.
- **`state.WriteJSON` ignores the configured shared/private file modes** (`internal/state/store.go`, `cmd/rotari/state_store.go`): fixed to use the configured `DirectoryMode()` / `FileMode()` instead of hard-coded private defaults.
- **Production logging depends directly on the Go test runtime** (`cmd/rotari/color.go`, `jobLogf`): fixed by removing the `testing.Testing()` dependency and exposing an explicit package-level log hook that tests can override without affecting production behavior.
