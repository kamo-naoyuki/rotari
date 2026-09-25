# Issues

Track unresolved bugs, design concerns, and technical debt discovered while working on other tasks.

This file is not a replacement for GitHub issues. Remove an item when it has been resolved, or move it to `Resolved` when keeping a short record is useful.

## Open

<!-- Add items here as they are discovered. Include the relevant file or area when possible. -->

## Resolved

<!-- Keep only short records of resolved items when they may help prevent recurrence. -->

- **`/dev/null` stdin was treated as a terminal** (`cmd/rotari/terminal.go`, `isTerminal`): only `os.ModeCharDevice` was checked, so `reset` and `copy` with stdin from `/dev/null` prompted and failed with `EOF` instead of printing the non-interactive guidance. It now queries termios settings.
- **Copying part of a stage rejected its dependents** (`cmd/rotari/copy.go`, `copyRunToQueue`): the excluded-dependency check required every stage member to have succeeded, including the members being copied, so `retry` failed after any stage member failed. Only omitted members are checked now, and the stage dependency is kept while any member is copied.
- **The `example-shellcheck` pre-commit hook never ran** (`.pre-commit-config.yaml`): its `files: ^example\.sh$` pattern did not match `scripts/example*.sh`, and `bash -n a b` checks only the first file. The hook now matches every example script and checks each one separately.
- **Matrix base-name dependencies broke partial queue edits** (`internal/model/dependencies.go`, `ClearMatrixGroup`; `cmd/rotari/copy.go`): a dependency on a matrix base name stopped resolving once `change` or `remove` cleared the group's provenance, and `copy` (including `retry`) did not recognize base names at all. Clearing provenance now rewrites such dependencies to the remaining member names, and `copy` checks base names like stages.
- **Running an imported queue with new jobs failed from the CLI** (`cmd/rotari/run_selection.go`, `planImportedWorkflow`): the run server records the new run as `LastRunID` before planning, and jobs without an origin fell back to that run's missing summary. Origin-less imported jobs are now planned as new work without a previous-run lookup.
- **`show` job hid manual acceptance** (`cmd/rotari/show.go`, `showJobAttempt`): an accepted job printed only the ordinary carry note and the source attempt's failing exit code, while `show` run and the Web table showed `success (accepted)`. It now prints the accepted status and follows `Origin.AttemptID`.
- **Workflow import ignored local results of non-latest attempts** (`cmd/rotari/workflow_reconcile.go`, `resolveAttempt`): only wrapper `status.json` was read, so a local-executor attempt that was not the summary's latest failed with "has no completed result". It now reads the local `status` result first, matching run planning.
- **Workflow import rejected unchanged matrix runs** (`cmd/rotari/workflow_reconcile.go`, `reconcileCommandLeaves`): every matrix member was resolved against the group's job-level `attempt_id`, so any group with two or more successful combinations failed with "attempt belongs to job". Matrix members now use their own instance attempt or recover from the listed source run.
- **Workflow import of a filtered array retry silently accepted stale failed tasks** (`cmd/rotari/workflow_reconcile.go`, `listedCommandRun`): leaves without a manifest attempt were read from the run named by the job-level anchor attempt. They are now recovered from the listed source run that supplied the command, following carried attempt IDs to their original run.
- **SSH metadata read errors are reported as "job is not running"** (`internal/executor/ssh.go`, `readSSHMetadata`): fixed to treat only missing metadata as "job is not running" while preserving distinct errors for malformed or unreadable state.
- **`state.WriteJSON` ignores the configured shared/private file modes** (`internal/state/store.go`, `cmd/rotari/state_store.go`): fixed to use the configured `DirectoryMode()` / `FileMode()` instead of hard-coded private defaults.
- **Production logging depends directly on the Go test runtime** (`cmd/rotari/color.go`, `jobLogf`): fixed by removing the `testing.Testing()` dependency and exposing an explicit package-level log hook that tests can override without affecting production behavior.
