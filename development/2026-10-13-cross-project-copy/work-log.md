# Work log: Copy and Retry Into Another Project

## `copy --to-project` (Phase 1)

**Commits:** `1d8e828f` (2026-10-14 03:50:28 +0900)

**Change:** `queueops.Editor.CopyTo` copies from a run of the source project
into another project's queue, creating it like `add` (reserved-name check, no
directory on a dry run or a rejected source); `Copy` delegates to it and
`CopySnapshot` takes the destination. `queueedit.Run.Foreign` makes copies
from another project carry no `Origin`/`TaskOrigins` and be marked
unfinished. CLI `copy --to-project` and the Web API `/api/copy` `to_project`
use the same path. `projectrun.RunSource` no longer rejects a result
selection in a project without a previous run. Contract RUN-18; docs in
RECOVERING, FAQ, ARCHITECTURE; regenerated CLI reference, Python schema, and
golden schema; flag-pair inventory and samples.

**Reason:** Plan D1–D7: rerunning a finished run's failures beside a running
project needed hand-built jobs in another project.

**Plan impact:** Phase 1 done. RUN-18 replaces the planned SAFE-8 note. The
`RunSource` change was found while testing `retry` in a new destination.

**Validation:** `go test -short ./...` passed except
`TestShowReportsElapsedAndQuietTime`, which fails the same way on an
unmodified HEAD worktree (the job's quiet time reads `14m 07s` 15 s after
start in this sandbox's TMPDIR). Path-element conformance for the CLI and Web
destination, `TestCopyIntoAnotherProject` (fails with the unfinished mark
removed), and the `pairedits` copy pairs passed.

**Remaining:** None for this phase.

## `run`/`retry --to-project` and the rejection hint (Phases 2–3)

**Commits:** `3d861301` (2026-10-14 04:09:54 +0900)

**Change:** `PlanRequest.SourceProject` and `server.Request.SourceProject`
(protocol 13). `projectrun.Runner.foreignSnapshot` copies the source run's
selected jobs with `queueedit.CopyRequest.PartialArray` narrowing arrays,
then clears the selection; `PlannedRun.Request` returns it, and the
supervisor applies it before `Execute` so execution does not reselect. The
CLI resolves the source's latest run when `--run-id` is absent, checks the
destination is idle, rejects `--match-by`, and plans once before starting
the supervisor. `project.EnsureIdle` returns `*project.RunningError`; the
`retry` rejection adds the `--to-project` command. Contracts RUN-19 and
SAFE-2; docs as in Phase 1 plus python-api.

**Reason:** Plan D8–D9.

**Plan impact:** Phases 2 and 3 done. `--to-project` is on `run` too, since
the commands share their options. MCP takes no destination.

**Validation:** `go vet ./...`; `go test -short ./...` passed except the same
`TestShowReportsElapsedAndQuietTime`; new tests
`TestCopyPartialArrayNarrowsToSelectedTasks`,
`TestPlanRunFromAnotherProjectAppliesTheSelectionToTheSource`,
`TestRetryInAnotherProject`, and the SAFE-2 hint check in
`TestRunningProjectRejectsSecondRunAndDelete` passed, as did `pairruns`.
The race suite was not run.

**Remaining:** Phase 4 (provenance) is not scheduled.
