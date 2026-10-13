# Plan: Copy and Retry Into Another Project

**Created:** 2026-10-13
**Status:** Phases 1–3 done; Phase 4 not scheduled. See [work-log.md](work-log.md).
**Related:** [Work on a project while its run is active](../2026-10-05-work-during-runs/plan.md),
[project selection](../2026-10-06-project-selection/plan.md),
[shared experiment record](../2026-10-10-shared-experiment-record/plan.md)
(cross-project comparison finding)

## Purpose

A project has at most one active run (CORE-5). While it runs, `retry` and
`run` are rejected (SAFE-2), and `copy` can only fill the project's own next
queue, which waits for the active run to end. A user who wants to rerun the
failed jobs of an earlier run *now*, beside the active run, has to rebuild the
jobs in another project by hand. The FAQ already names another project as the
way to run work concurrently; this plan makes that one command.

1. `copy --to-project NAME` copies jobs from a saved run of the source project
   into the queue of another project, creating it when absent.
2. `retry --to-project NAME` starts a run in another project from a saved
   run's failed and unfinished jobs, without carrying any result.
3. When `retry` is rejected because its project is running, the message names
   the `--to-project` form.

## Non-goals

- Carrying results across projects (option (b) of the discussion). Every
  reader of a job origin resolves its run ID inside the reading project's
  `runs/` directory, so a cross-project origin would be misread or fail; see
  "Current implementation". Supporting it means adding a project to
  `model.JobOrigin` and teaching every reader, which touches the result
  resolution invariant (`internal/jobstatus`). Not pursued.
- An interactive "create a new project?" prompt when the project is locked.
  It could only appear on a terminal (SAFE-6), so the CLI, Web API, MCP, and
  Python client would behave differently, and `copy` is never rejected by an
  active run in the first place. The rejection message names the command
  instead (Phase 3).
- Automatically generated destination names (`-r NEW` or similar). `-r` is
  already `--run-id`, and generated names scatter history across projects
  nobody chose.
- Copying the source project's *queue* to another project, and a hint for
  `run` (as opposed to `retry`) rejections. `copy` reads saved runs only.
- A destination in another basedir.
- Concurrent runs within one project (unchanged non-goal of the
  work-during-runs plan).
- A Web UI control for the destination. The Web API accepts it (Phase 1); a
  UI control is a separate decision.
- Cross-project `lineage` / `diff`. Tracked by the shared-experiment-record
  plan; Phase 4 below only records provenance that would enable it.

## Current implementation

- `cmd/rotari/copy.go` (`cmdCopy`) resolves one project from `--basedir` /
  `-p` and passes it to `queueops.Editor.Copy(baseDir, projectName, runID,
  request)` in [internal/queueops/copy.go](../../internal/queueops/copy.go).
  Source and destination are the same `state.ProjectPaths`: `Copy` takes the
  destination's state lock through `project.EditQueueGuarded`, and
  `CopySnapshot` reads the source run from the same `RunsDir`, after
  `project.EnsureRunSettled`.
- `queueedit.Copy` in [internal/queueedit/copy.go](../../internal/queueedit/copy.go)
  selects jobs (`jobfilter.Filter.Selects` / `SelectsArray`), checks omitted
  prerequisites against the source results (`checkOmittedDependencies`), keeps
  source job IDs unless they collide, clears marks, and writes
  `Origin` / `TaskOrigins` naming the source run (`commandOrigin`,
  `taskOrigins`).
- Origins are resolved in the reading project only:
  `run.originResolver.result` loads `RunResults(origin.RunID)` from the
  project's runs ([internal/run/rerun.go](../../internal/run/rerun.go));
  `projectrun.originResults.FilterJob`, `Origin`, and
  `LoadOriginAttemptResult` join `paths.RunsDir` with the origin's run ID
  ([internal/projectrun/origins.go](../../internal/projectrun/origins.go));
  `jobstatus.FilterJob` / `Artifacts` take one `runsDir`
  ([internal/jobstatus](../../internal/jobstatus)); `show` logs for carried
  jobs (`printCarriedForwardOutput` in
  [cmd/rotari/show.go](../../cmd/rotari/show.go)) and
  `runlineage` follow `Origin.RunID` the same way.
- A queued job without an origin falls back to the reference run's result
  by job ID, or fails planning with `ErrNoReferenceRun` when there is none
  (the "filtered run" rules in
  [contracts/02-run-lifecycle-and-execution.md](../../contracts/02-run-lifecycle-and-execution.md)).
- `add` creates an absent project through `project.CreateQueueGuarded` and
  registers the basedir with `queueops.Editor.RegisterBaseDir` only when the
  edit is applied, so `--dry-run` creates nothing.
- `retry` is `runJobs` with failed + unfinished selection
  ([cmd/rotari/run_command.go](../../cmd/rotari/run_command.go)). The
  running-project rejection is `project.EnsureIdle(paths, "run")`, called from
  [internal/projectrun/plan.go](../../internal/projectrun/plan.go), with the
  message `project %q is running; run is not allowed (queue edits for the next
  run are)`. `retry --run-id` builds the run from the saved run without
  touching the queue (RUN-13), through `Editor.CopySnapshot`.
- Interfaces: Web API `POST /api/copy` in
  [internal/webui/webui.go](../../internal/webui/webui.go) calls
  `Editor.Copy`; the Python client's `copy` / `retry` are generated from
  `cmd/rotari/cli_spec.go` by `scripts/generate_python_cli.py`; MCP has no
  copy tool. `docs/CLI_REFERENCE.md` is generated by
  `scripts/generate_cli_reference.py`.

## Decisions

- **D1 Option.** `--to-project NAME` on `copy` and `retry`. CLI only, no
  environment default: an exported default would redirect every later copy
  or retry without the user seeing it.
- **D2 Roles.** The source project is selected exactly as today (`-p`, run ID
  resolution, config defaults); `--to-project` names only the destination.
  Both are in the same basedir.
- **D3 Creation.** An absent destination is created as `add` creates one:
  same name validation (`model.ValidateReservedName` and path-element
  checks, rejecting `/` and `\`) at the shared boundary, created and
  registered only when the edit is applied, never by `--dry-run`.
- **D4 Same project.** `--to-project` naming the source project behaves
  exactly as if it were omitted, including origins.
- **D5 No cross-project origins; copied jobs are unfinished.** A copy into
  another project writes no `Origin` or `TaskOrigins` and marks each copied
  command `MarkedStatus = unfinished` (no `TaskMarkedStatus`). Without the
  mark, a filtered run in the destination (`retry -p DEST`) would look the
  job up by ID in the destination's reference run: a new project fails with
  `ErrNoReferenceRun`, and an older destination run that holds a success
  under the same, preserved job ID would be carried instead of executing
  the job. `run.originResolver.jobResult` returns unfinished for the mark
  before any lookup, so `run` and `retry` both execute every copied job.
  This is the same mark that replaced the queue's old `force` flag
  (`convertQueueV1` in [internal/state/store.go](../../internal/state/store.go)).
  Job IDs are kept unless they
  collide, as today. The decision belongs in `queueedit.Copy` (the source
  `Run` says whether it is in the destination project), not in each caller.
- **D6 Prerequisites.** The omitted-prerequisite check stays against the
  source run's results: a dropped prerequisite is one the source run
  completed. Copied jobs keep their working directory settings, so a job
  without an explicit one runs where `rotari run` is started, as today.
- **D7 Locks and guards.** Only the destination's state lock is taken. The
  source run must be settled (`EnsureRunSettled` on the source project); the
  source project itself may be running, which is the motivating case.
  `--append` / `--overwrite`, the terminal confirmation (SAFE-6), and
  `--if-revision` refer to the destination queue.
- **D8 `retry --to-project` semantics (option (a)).** It builds the new run
  directly in the destination from the source run's selected jobs, writing
  neither project's queue (as RUN-13 does for `--run-id`). The source is
  `--run-id` or the source project's latest settled run, never the source
  queue, which belongs to the source's next run. Nothing is carried: the new
  run contains only the selected jobs. With `--partial-array=true` (the
  default) an array is narrowed to its selected tasks, each decided by
  `jobfilter.Filter.Selects`; with `false` the whole array is decided by
  `SelectsArray`. The destination must be idle (SAFE-2 applies to it) and is
  created when absent. Executor and run settings come from the command line
  and the destination's configuration.
- **D9 Hint, not prompt.** When `retry` is rejected because its project is
  running, the message adds the `retry ... --to-project NEW_PROJECT` form.
  The `run` rejection is unchanged (non-goals).

## Phases

### Phase 1: `copy --to-project` (done)

As planned, with two changes. The contract is RUN-18 alone; SAFE-8 needed no
note, since a copy edits only the destination's queue. `projectrun.RunSource`
no longer rejects a result selection in a project without a previous run: it
plans from the queue, so `retry` in a new destination executes the copies
instead of failing, and planning still rejects an unmarked job that needs a
result.

1. Split source from destination in `queueops.Editor.Copy` / `CopySnapshot`:
   source project paths for `EnsureRunSettled`, `CheckRunVersions`, the
   snapshot, summary, timestamps, context, `FilterJob`, and attempt lookup;
   destination paths for the guarded edit, creation, and registration. Keep
   the run-startup caller of `CopySnapshot` on the same project.
2. Use `project.CreateQueueGuarded` for an absent destination (D3).
3. Omit origins and mark copied commands unfinished for a foreign source in
   `queueedit.Copy` (D5).
4. `cli_spec.go` and `cmdCopy`: parse `--to-project`, validate it, and print
   the destination in the success message and JSON. Regenerate the CLI
   reference and the Python client.
5. Web API: `POST /api/copy` accepts `to_project` with the same validation
   and the same `Editor.Copy` call.
6. Contracts: a new RUN rule for cross-project copy (D2–D7) and a SAFE-8 note
   that the destination's state decides what a queue edit may do.
   Docs: `docs/RUNNING.md` (running failed jobs beside an active run),
   the FAQ "another project" answer, `docs/ARCHITECTURE.md` copy flow.

### Phase 2: `retry --to-project` (done)

Implemented for `run` as well as `retry`: the two commands share their
options, and `run --to-project` is the same start with no default selection.
The planned request travels back to the supervisor as `PlannedRun.Request`,
so execution does not apply the selection a second time. The CLI plans once
before starting the supervisor, which would otherwise create the destination
for a rejected request. `--match-by` is rejected with the filters D8 names.
MCP's `rotari_start_run` takes no destination; MCP has no copy tool either.

1. In `projectrun` planning, resolve the source run in the source project and
   plan the run in the destination with `CopySnapshot` (foreign source, so no
   origins) and no reference run, so nothing is carried (D8).
2. Per-task narrowing for `--partial-array=true` through the shared filter
   functions; reject options that cannot apply instead of ignoring them.
3. The supervisor, lock, and `Begin` act on the destination only; the
   source project's queue, lock, and phase are untouched.
4. `cli_spec.go`, regenerated reference and Python client; contracts (new RUN
   rule, SAFE-2 note that the destination is the project checked); docs as in
   Phase 1.

### Phase 3: rejection hint (done)

`project.EnsureIdle` returns `*project.RunningError`, and the CLI adds the
hint to a `retry` rejection, keeping `--run-id` when one was given.

1. Give the `retry` rejection for a running project the D9 hint. The
   message is built where the operation is known (`retry` vs. `run`), not by
   making `EnsureIdle` aware of commands.
2. Update SAFE-2 and its conformance check.

### Phase 4 (not scheduled): provenance

Record, informationally, which project and run copied jobs or a cross-project
retry came from, without any reader resolving results through it. This needs
a state-format decision and would serve the cross-project `lineage` finding of
the shared-experiment-record plan. Decide only after Phases 1–3 are used.

## Tests and validation

Fixtures must tell cases apart: distinct exit codes, more than one failing
task, tasks with and without a host, carried and executed results in the
source run.

- `queueedit`: a foreign source writes no `Origin` / `TaskOrigins` and marks
  the command unfinished for plain jobs, arrays, and matrix members; the same
  project still writes origins and no mark; prerequisite checks and
  dependency rewriting are unchanged.
- `queueops`: copy into an absent project creates and registers it; `--dry-run`
  creates nothing; destination running is allowed for the next queue; source
  running with a settled latest run is allowed; source interrupted run is
  rejected; destination names with `/`, `\`, or a reserved name are rejected
  for CLI and Web API alike; `--if-revision` checks the destination.
- After a cross-project copy, both `run -p DEST` and `retry -p DEST` execute
  every copied job: into a new project (no reference run, no
  `ErrNoReferenceRun`), and into a project whose last run holds a success
  under the same job ID (not carried). Arrays execute every task.
- `retry --to-project`: plain job, array task with `--partial-array` true and
  false, whole array, matrix member, stage dependents; destination running is
  rejected and changes nothing; the new run's summary has no carried results;
  both queues are unchanged; `show`, `jobs`, and the Web API read the new run
  without following any origin into the source project.
- Phase 3: conformance check of the SAFE-2 message for `retry`, and that the
  `run` message is unchanged.
- Conformance: new RUN rows with `covers(t, ...)` and the "Contract status"
  table updated. The `pairedits` flag-pair suite enumerates the new flag
  automatically; watch its runtime, which already nears its timeout
  (see `development/ISSUES.md`).
- Run `go test` for the touched packages, `go test ./conformance`, and
  `scripts/check.sh` before each phase is reported done.
