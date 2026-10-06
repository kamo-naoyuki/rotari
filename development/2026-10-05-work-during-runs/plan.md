# Plan: Work on a Project While Its Run Is Active

**Created:** 2026-10-05

**Status:** Phase 1 is implemented. Phase 2's same-run retry implementation
is committed and validated. The user now prefers a run-centric immediate
execution model without a project-level next-run queue, using manifests for
pre-run authoring and run-owned pending work for execution. The staged
transition is planned in
[development/2026-10-06-active-run-additions/plan.md](../2026-10-06-active-run-additions/plan.md).
D10's submitted-job definition revision is deferred. Manifest bootstrap,
run sealing, retry lifecycle, and migration compatibility remain to be
specified. The broader comparison is tracked in
[development/2026-10-06-execution-model-review/plan.md](../2026-10-06-execution-model-review/plan.md).
Phase 3 has its own planning at
[development/2026-10-06-project-selection/plan.md](../2026-10-06-project-selection/plan.md)
but its implementation, together with Phase 4 implementation, is paused
until the active-add contract and project-targeting interactions are settled.
D1 to D5, D7, D8,
and D9 describe current or previously decided run-based behavior; D10 is
deferred and D11 is updated below for run-owned pending work. Queue removal is
the preferred product direction; the remaining gates concern workflow parity,
run lifecycle, and safe migration.

## Purpose

A project runs at most one run at a time, and while it runs, every queue edit
is rejected (SAFE-2). The second rule, not the first, causes most of the
waiting: users cannot prepare the next batch, add a forgotten job, or rerun a
job that failed early until the whole run ends. The current workaround,
creating another project, splits related history. With multiple projects,
some commands require `--project-name`; Phase 3's separate audit will identify
which commands have a safely inferable target.

This plan began from the run-based approach: remove waiting without allowing
two coordinators to execute one project concurrently. One active run per
project keeps result resolution linear, `show` / `wait` / `cancel` defaults
unambiguous, and the three-state lock and recovery model. User feedback has
raised a more basic question, however: whether the run/next-queue split itself
is the right primary model when users need to add or correct work during
execution. Compare that approach with immediate submission and an open
execution session in the linked review before treating the current split as a
long-term design commitment.

### Current shipped model: the queue is the next run before it starts

This describes the behavior implemented in Phases 1–2; whether it remains the
target architecture is open in the linked execution-model review.

A run is one object that moves through **queued → running → finished**. An
interrupted run is a running run that failed to reach finished; `unlock`
finishes it.

| State | Count | Operations |
| --- | --- | --- |
| queued | 0..1 | `add`, `change`, `remove`, `copy`, `import`, `reset` |
| running | 0..1 | inspect, `cancel`, `suspend`, `resume`, in-run retry |
| interrupted | 0..1 | inspect, `unlock` |
| finished | any | inspect, compare, source of `copy` / `retry -r` |

Counts are per project. The queued run is what the docs call the queue;
in-run retry is phase 2.

A project has at most one queued run, its next one, so the current storage
already fits the model: the queued run lives in the project's single
`queue.json`, and only started runs get a run ID and a directory under
`runs/`, so history never contains runs that did not start. Starting a run
moves the queued run into `runs/<run-id>/` and leaves the queue empty. The
plan changes when that move happens and what may touch each state; it does
not change file layout, the `queue` term, or command names.

The decisions below follow from the model: the queued run and the
running or interrupted run are different objects, so editing one never
waits on the other (phase 1, D7); recovery finishes the interrupted run
without touching the queued one (D1); and a run built from a saved run goes
straight to running without passing through the queue (D8).

## Scope and order

1. **Edit the queue while a run is active.** The run takes the queue when it
   starts, so the queue only ever holds the next run's jobs. `add`, `change`,
   `remove`, `copy`, `import`, and `reset` can then edit it in any project
   state, and recovering an interrupted run no longer involves the queue.
2. **Retry jobs inside the active run.** `retry --failed` (or retry of
   selected jobs) on a running project starts new attempts in that run, the
  same way `run --retry` does, instead of being rejected. The implementation
  currently retries the run's existing job definition. Editing a submitted
  job or changing its next-attempt definition is not included absent a
  concrete requirement.
3. **Reduce the cost of switching projects.** Tracked separately in
  [development/2026-10-06-project-selection/plan.md](../2026-10-06-project-selection/plan.md):
  measure where multiple projects force `--project-name`, then remove only
  the cases that have one unambiguous answer.
4. **Move to run-owned pending work** (preferred direction; contract
  confirmation required). `start [MANIFEST]` opens a run and starts ready
  jobs immediately; `submit` adds work to that open run; `wait` seals
  admission and waits for completion. No next-run queue. Keep `change` on
  manifests before execution. Follow the detailed transition proposal in
  [development/2026-10-06-active-run-additions/plan.md](../2026-10-06-active-run-additions/plan.md).

Phases 1 and 2 were independent and shipped separately. Phase 3's repository
audit is complete, but implementation remains paused until its project
resolution assumptions are reconciled with run-scoped submission. Phase 4 can
reuse the existing request-channel pattern, but run membership commit,
open/seal lifecycle, and migration are new work.

### Non-goals

- Two concurrent runs of one project. Retry and late additions go into the
  active run instead (phases 2 and 4).
- Multiple coordinators for one project. The target is one open run that
  accepts additions until sealed, not concurrent execution sessions.
- A global, mutable "current project" shared by every shell.
- Starting a second run or deleting an open/interrupted run. Run-scoped
  addition and sealing are explicit; concurrent coordinators remain forbidden.
- Changing the executor, environment, or concurrency of a run that has
  already started.

## Current implementation

- `project.EnsureIdle` in
  [internal/project/inspect.go](../../internal/project/inspect.go) rejects
  running and interrupted projects for operations that must not run then.
  Queue edits instead use `project.EditQueue` in
  [internal/project/edit.go](../../internal/project/edit.go), which checks
  consistency but permits an active or interrupted run; `project.Edit` keeps
  the idle-only gate for `delete`.
  `PreviewRun` in [internal/projectrun/plan.go](../../internal/projectrun/plan.go)
  and the supervisor in [internal/supervisor/run.go](../../internal/supervisor/run.go)
  call it directly.
- Idle edits write `meta.json` phase `collecting` (`writeIdleQueueWith`).
  During a run the phase is `running` or `cancelling`, and the engine's
  `Stopped` callback polls it to detect cancellation.
- `Runner.Begin` in
  [internal/projectrun/lifecycle.go](../../internal/projectrun/lifecycle.go)
  writes the context and `commands.json` snapshot, takes `running.lock`,
  registers the run, marks the project running, and empties the queue while
  retaining its defaults.
- `Runner.Execute` in
  [internal/projectrun/execute.go](../../internal/projectrun/execute.go)
  reads the run's snapshot rather than `queue.json`; it updates that snapshot
  with origins and carried-result information before execution.
- `Runner.Finalize` calls `state.FinalizeRun` in
  [internal/state/run_files.go](../../internal/state/run_files.go), marks run
  metadata finished, and leaves the next queue untouched.
- `RunSource` in [internal/projectrun/source.go](../../internal/projectrun/source.go)
  identifies the source run and source policy; the supervisor resolves
  `copy-if-empty` under the state lock. A saved run is copied into memory by
  `queueops.Editor.CopySnapshot` and passed to `Runner.Begin` as the run's
  snapshot, never written into `queue.json`.
- Queue edits while active or interrupted belong to the next run. `reset`
  clears only that queue in every project state without changing run metadata;
  `--recover` and `ROTARI_RESET_RECOVER` fail with a hint to use `unlock`.
  `project.Unlock` is the shared recovery path, keeps the queue, and reports
  jobs that may still be running. MCP exposes preview/apply unlock tools and
  the Python client exposes `unlock`.
- An interrupted run always has `commands.json`: the consistency check
  (`state.ValidateRunDirectory`) rejects one without it. Every recoverable
  interrupted run can therefore be resumed from the run itself with
  `retry --run-id RUN`.
- `resolve.defaultJobs` in [internal/resolve/resolve.go](../../internal/resolve/resolve.go)
  looks only in the active run while a project is running or interrupted,
  and only in the queue (then the latest run) while it is idle.
- `run.ExecuteJobs` in [internal/run/engine.go](../../internal/run/engine.go)
  is an in-memory event loop. A job is final once it succeeds or fails with
  no retries left; `DependsOn` dependents of a final failure are finalized as
  `blocked by failed dependency`; the loop returns when nothing is running or
  waiting for a retry delay. `FinalResult` is documented as called exactly
  once per job.
- The supervisor protocol has only `OpRun`
  ([internal/server/protocol.go](../../internal/server/protocol.go)). Cancel,
  suspend, and resume act through files and signals in
  [internal/jobcontrol/jobcontrol.go](../../internal/jobcontrol/jobcontrol.go)
  (`markCancelling`, `Controller.Select`), so they work from the Web UI and
  without the supervisor socket.
- Job IDs come from `makeJobID` (random), so jobs added during a run do not
  collide with the active run's IDs.

## Phase 1: Queue edits during a run

### Phase 1: Behavior

- `run` / `retry` take the queue when the run starts: the queue's commands
  move into the run's snapshot and `queue.json` is left empty, keeping its
  queue-level defaults (`default_executor`, `default_executor_options`).
- The queue holds only the next run's jobs and has no relation to the active
  or interrupted run. `add`, `change`, `remove`, `copy`, `import`, and `reset`
  edit it whether the project is idle, running, or interrupted.
- `run` and `delete` stay rejected while a project is running or
  interrupted; their message says that the queue can still be edited.
- When the run finishes, the queue is left as edited; it is no longer cleared.
- **Recovery is `unlock` alone.** It confirms that the interrupted run's jobs
  stopped and returns the project to idle; it does not touch the queue. The
  interrupted run's work is resumed from the run itself, for example with
  `retry --run-id RUN --unfinished`, and the interrupted-run message and
  `unlock` output name that command.
- **A run built from a saved run bypasses the queue** (decision D8).
  `retry --run-id RUN`, and `retry` / `run --failed` on an empty queue, copy
  the source run with the same copy rule as today but into the new run's
  snapshot, not into `queue.json`. The queue is left as it is, so jobs added
  during a run are not replaced. `retry --overwrite` no longer applies and
  fails with an error. `copy` still writes the queue, for the copy, `change`,
  then `retry` flow. A run that fails before starting leaves nothing in the
  queue; use `copy` to keep the jobs for editing.
- **`reset` only clears the queue.** It needs no confirmation about running
  jobs in any state. `reset --recover` and `ROTARI_RESET_RECOVER` are
  removed: they fail with an error that names `unlock`, rather than being
  accepted and ignored.
- **`retry` keeps its source rule** (decision D3, settled): a non-empty queue
  is used as is, and each job finds its earlier result through its `Origin`
  or `--match-by` (default `id-and-fingerprint`); on an empty queue the run
  is built from the latest run (D8). Jobs added during a run are in the same position
  as jobs added after a run today, so no new rule is needed. Because the
  latest run's failures are then not rerun unless they are in the queue,
  `retry` on a non-empty queue says which source it used, how many of the
  latest run's failed or unfinished jobs it leaves out, and how to include
  them (`copy --failed --append`, then `retry`). `--dry-run` shows the same.

### Phase 1: Implementation

1. **Snapshot at `Begin`.** Write the queue to `runs/<run-id>/commands.json`
   before taking the run lock, and clear `queue.json` commands after the
   metadata says running, all under the state lock the caller already holds.
   A crash before the clear leaves a filled queue beside an interrupted run;
   since recovery no longer reads the queue, that only leaves the user's jobs
   queued. On a `Begin` failure after the clear, restore the queue.
2. **Execute from the snapshot.** `Execute` reads the run's `commands.json`
   instead of `queue.json`. `RunSource` copying, fingerprint matching, and
   origin recording keep writing the same snapshot. Active runs then always
   have `commands.json`; update the consistency rule in
   [contracts/04-coordination-and-safety.md](../../contracts/04-coordination-and-safety.md)
   that lets an active run lack it.
3. **Stop clearing at finish.** `state.FinalizeRun` updates metadata only;
   `Finalize` no longer writes the queue.
4. **One gate for queue edits.** Queue edits (`project.Edit` / `EditQueue`,
   and `reset`) no longer call `EnsureIdle`; they only require consistent
   project state. An edit writes the `collecting` phase only when the project
   is idle; a running or interrupted project's phase is left as is, since the
   engine polls it for cancellation and it marks the interrupted run. Keep
   `EnsureIdle` for `run`, `delete`, and `PreviewRun`. Keep the per-command
   operation names so messages stay specific.
5. **Selector resolution (D2).** `resolve.defaultJobs` drops its
   running/interrupted branch: in every project state, a job selector looks
   in the queue first, then in the project's active, interrupted, or latest
   run, as idle projects do today. `show`, `copy`, and `run` / `retry`
   share it. Job control (`cancel`, `suspend`, `resume`, and phase 2's
   in-run retry) keeps resolving in the active run through
   `resolve.JobSelection`, because it acts on running jobs. `show` marks a
   queued match as queued, and `--run-id` selects the run's job when a name
   is in both.
6. **Copy sources.** `copy` and `retry --run-id` from the active run, or from
   an interrupted run before `unlock`, are rejected: their results are not
   final and their jobs may still run.
7. **Recovery.** `project.Unlock` is the shared recovery operation; it keeps
  the next queue. The still-running-jobs report (SAFE-7) is on `unlock` and
  the MCP unlock preview, never on `reset`.
8. **`reset`.** Remove the interrupted-run branch, the confirmation, and the
   `--recover` option and environment variable; reject them with an error
   naming `unlock`. Update the schema-driven CLI reference.
9. **Retry source message.** In `RunSource`'s caller, when a selection uses a
   non-empty queue, report the source and the latest run's failed or
   unfinished jobs that are not in the queue, without changing which jobs
   run.
10. **Runs from a saved run.** When `RunSource` says to copy first, build the
    copied queue in memory with the same copy function, as `--dry-run`
    already does (`runQueue` in
    [cmd/rotari/run_command.go](../../cmd/rotari/run_command.go), passed to
    `PreviewRun`), and hand it to `Begin` as the run's snapshot instead of
    writing `queue.json`. Remove the overwrite prompt and reject
    `--overwrite` on `run` / `retry`. Apply the same path to MCP
    `rotari_start_run` ([internal/mcp/write.go](../../internal/mcp/write.go))
    and the Web UI retry action, so every interface shares it. The revision
    guard no longer changes between the copy and the start.

### Phase 1: Interfaces

- CLI: the commands above, `check` / `show` output for a running or
  interrupted project with a non-empty queue (show both the run and the next
  queue), `unlock` output, and `reset` option removal.
- Web UI / API: the queue view of a running or interrupted project becomes
  editable; the run view stays distinct and `show`-equivalent inspection
  surfaces both run and next queue. Existing Web `Create queue` / `Append to
  queue` controls remain copy operations; the run view's retry command uses
  the D8 `retry --run-id` behavior. There is no Web API to start a run, so do
  not turn copy controls into a new start API as part of this phase.
- MCP: `rotari_preview_reset` / `rotari_reset` become queue-only, so add
  `rotari_preview_unlock` / `rotari_unlock` (or equivalent) to keep a
  recovery path with the running-jobs report. Check `rotari_import` and the
  preview/revision flows (`rotari_preview_run`, `rotari_start_run`): a run
  changes the revision, so a preview made before a run starts must still fail
  its guard.
- Python client: remove the recover option from `reset`, add or document
  `unlock`, and update its tests for the new acceptance.

### Phase 1: Contracts and documentation

- Rewrite the state table and SAFE-2 to SAFE-7 in
  [contracts/04-coordination-and-safety.md](../../contracts/04-coordination-and-safety.md):
  queue edits and `reset` in every state, `run` / `delete` still rejected,
  recovery by `unlock` only, no `reset` confirmation. Add an ID for "a run
  takes the queue at start and leaves later edits".
- The `retry` source rule and its new message in
  [contracts/02-run-lifecycle-and-execution.md](../../contracts/02-run-lifecycle-and-execution.md)
  or [contracts/06-selectors.md](../../contracts/06-selectors.md), wherever
  the rule is stated now.
- [docs/CONCEPTS.md](../../docs/CONCEPTS.md): the queue roles per state and
  the run flow diagram (`queue.json: empty` happens at start, not at finish).
- [docs/RECOVERING.md](../../docs/RECOVERING.md) and
  [docs/INSPECT.md](../../docs/INSPECT.md): interrupted runs are resumed with
  `unlock` then `retry --run-id`; the retained queue and `reset --recover`
  are gone; `retry` on a non-empty queue; `retry -r` no longer replaces the
  queue and has no `--overwrite`.
- [docs/FAQ.md](../../docs/FAQ.md): the "another project" answer, the
  idle/running queue-first explanation, and the interrupted-run answers.
- [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md): `Begin`, `Execute`, and
  `Finish` steps.

### Phase 1: Tests

- `internal/projectrun`: `Begin` snapshots and clears; `Execute` ignores
  `queue.json`; `Finalize` keeps edits; `Begin` failure restores the queue.
- `internal/project`: the edit gate for idle, running, cancelling, and
  interrupted projects; the phase is written only when idle; recovery leaves
  the queue untouched.
- Conformance: through the binary, add a job while a run is active (sync and
  async), let the run finish, and check that the run executed only its own
  jobs and the queue holds the added job. Cover each queue-editing command and
  `reset` in running and interrupted projects, and confirm `run` and `delete`
  are still rejected. Cover `unlock` followed by `retry --run-id RUN
  --unfinished`, `reset --recover` failing with the `unlock` hint, copying
  from an active or interrupted run being rejected, and the `retry` message
  for a non-empty queue. Check that `retry --run-id` with jobs in the queue
  leaves them queued and runs only the source run's jobs, and that
  `retry --overwrite` fails. Web API and MCP rows for the same.

## Phase 2: Retry inside the active run

### Phase 2: Behavior

`rotari retry` with a result selection or job IDs, on a running project,
starts a new attempt for each selected job in the active run and returns once
the run accepts the request. On an idle project it keeps creating a new run.
The output states which happened. If the run ends before it accepts the
request, the command fails instead of starting a new run (D5).

- Selection uses the existing rule: `jobcontrol.Controller.Select` over
  `jobfilter.Filter.Selects` / `SelectsArray`. A job is eligible only when its
  result in this run is final; running jobs and failures awaiting an
  automatic retry are not.
- Jobs carried from earlier runs are not executed by this run. Selecting one
  is an error that names it, not a silent skip.
- Dependents blocked by a retried job return to waiting and run if it now
  succeeds. `DependsOnFinished` dependents that already ran keep their
  results; the documentation says so.
- Arrays follow the idle `retry` rule: without `--partial-array` the whole
  array is rerun; with it, only the selected tasks.
- Automatic retries need no special case: a job is final only after its
  retry limit is used, so a failed manual attempt is final too.
- Run-level options (`--executor`, executor options, `--env`, concurrency,
  `--retry`, `--run-name`, `--match-by`, a `--run-id` other than the active
  run) cannot apply to a started run and fail with an error.
- A cancelling run rejects the request.

### Phase 2: Implementation

1. **Request channel (D4).** Requests are files under
   `runs/<run-id>/retry_request-<id>.json` (with `retry_accepting.json` open
   while the run accepts requests), the way `cancel` marks the project in
   `meta.json`; the supervisor has no endpoint other processes can reach.
   - The requester (CLI, Web UI process, MCP) takes the state lock, checks
     that the lock and metadata name this run as running (not cancelling),
     resolves the selection to job IDs with the shared rule
     (`jobcontrol.Controller.Select` over `jobfilter`), and writes
     `retry_request-<id>.json` with the job IDs, `--partial-array`, and the
     request time.
   - A supervisor goroutine polls the directory about once a second (no
     file-notification dependency). It decides eligibility, which only it
     can do because a failure awaiting an automatic retry looks final on
     disk: the result must be final, executed by this run, and not carried.
     It passes accepted jobs to the engine and writes
     `retry_request-<id>.response.json` with accepted job IDs and each rejected
     job with its reason.
   - The requester waits for the response with a timeout and prints both
     lists, so no selected job is dropped silently.
   - Request and response files stay in the run as the record of manual
     retries. Their format is new run state: give it a state version and a
     contract entry.
   - Phase 4 adds a request kind to the same directory.
2. **Engine event.** Add an event to `run.ExecuteJobs` that reopens final
   jobs: clear `final`, return them and their blocked `DependsOn` dependents
   to `waiting`, and continue attempt numbering. The loop must also wake for
   requests, not only for job results.
3. **End-of-run race.** The engine can return between the request check and
   pickup. `Finalize` answers any unanswered request with "run ended" under
   the state lock. The requester then fails with a non-zero exit, naming the
   run that ended and saying that repeating the command starts a new retry
   run (D5); it never starts one itself. The requester waits for a response
   with a timeout and reports a missing supervisor as an error.
4. **Final-result callbacks.** `FinalResult` becomes "once per final result":
   diagnosis, notification hooks, and `Observer.Finished` see a reopened job's
   new final result. Check the progress display, which may now see
  `completed` decrease.
5. **Atomic arrays and state format.** A request records `partial_array`;
  whole-array retry is accepted only if every task in that array has a final
  result. Request, response, acceptance, and pending-result files carry the
  current `state_version`, and run readers refuse files from a newer version.

### Phase 2: Interfaces

CLI `retry`; a Web UI action on failed jobs of the active run and its API;
MCP preview/apply tools; and the Python client's returned `Run` behavior.

### Phase 2: Contracts and documentation

The retry and lifecycle rules in
[contracts/02-run-lifecycle-and-execution.md](../../contracts/02-run-lifecycle-and-execution.md),
SAFE-2 for `retry`, and `docs/RUNNING.md` (Automatic retries, plus a new
"Retry during a run" section), `docs/CLI_REFERENCE.md` (generated by
`scripts/generate_cli_reference.py`; regenerate, do not edit), and the FAQ.

### Phase 2: Tests

- `internal/run` table test over: plain job, array task, whole array,
  matrix member; failed, cancelled, succeeded (whole-array case), running,
  awaiting automatic retry, carried; `DependsOn` dependents blocked and
  `DependsOnFinished` dependents finished; distinct exit codes so results
  cannot be confused.
- Request/response handling, including a cancelling run and the end-of-run
  race: the request fails with the ended run's name and no new run starts.
- Conformance: a run with one fast-failing job and one slow job; `retry
  --failed` while the slow job runs reruns only the failed job in the same
  run; option rejection rows; the same through the Web API.

### Phase 2: Attempt revision review (D10) — deferred

The shipped active retry reopens a final job using the active run's existing
`JobSpec` and run settings. It helps when the failure may disappear without
changing that definition (for example, a transient external service failure),
but cannot correct a bad per-job executor option, command, or environment
before retrying. Editing the next queue does not mutate the active run's
in-memory execution plan.

The shipped same-run retry reuses the job definition captured by the run.
Changing a submitted job's definition for a later attempt is out of scope
unless a concrete workflow demonstrates that it is needed. If revisited, it
requires a separate decision about attempt-level provenance and presentation;
it is not a prerequisite for active additions.

**Cross-cutting concern:** ordinary queue edits and exact active-run additions
remain two explicit destinations in the proposed incremental phase. Do not
let `add`/`change` switch targets implicitly based on project state. The
active-add plan excludes promoting queue entries, so there is no queue
consumption/collision reconciliation between those destinations. Whether two
explicit destinations are still too confusing is to be evaluated after the
bounded feature is tried; do not remove the queue preemptively.

D10 is not a prerequisite for Phase 4. Keep the shipped same-definition
active retry behavior and its current contracts unchanged during active-add
work. Revisit retry scope only if a concrete workflow establishes a need to
change submitted job definitions.

## Phase 3: Project selection

Phase 3 has been split into the focused plan
[development/2026-10-06-project-selection/plan.md](../2026-10-06-project-selection/plan.md).
That plan owns the command inventory, measurement, active-run resolution
decisions, and D6's project-pin decision gate. This umbrella plan keeps the
phase number and points to the detailed work rather than duplicating it.

## Phase 4: Move to run-owned pending work (preferred direction)

The earlier bounded-add sketch that preserves a permanent next-run queue is no
longer the target UX. The proposed final model has no project-owned next-run
queue; manifests provide pre-run authoring, and all executable pending work
belongs to a named open run. Follow
[development/2026-10-06-active-run-additions/plan.md](../2026-10-06-active-run-additions/plan.md)
for lifecycle, persistence, package ownership, adapters, and tests.

- Target an exact running `RUN_ID`; do not infer a target or fall back after
  an end/cancel race.
- `add` submits to the open run; when no run is open it must either open a
  session or fail with manifest/run-start guidance. It never stages work.
  `change` edits a manifest before run start; submitted-job edits are out of
  scope.
- Admit a whole new-job batch atomically through the existing supervisor and
  dispatcher. The request channel is reusable as a pattern, but durable
  membership commit and crash recovery are new design work.
- Start with plain jobs and DAG dependencies. Complete new array/matrix groups
  are a follow-up unless confirmed as a V1 requirement; never extend an
  existing group in V1.

Manifest bootstrap, explicit `finish`/`seal`, retry interaction, and legacy
queue migration are required design gates before implementation.

## Decisions

Settled on 2026-10-06:

- **D1** Recovery does not touch the queue. The queue was retained only
  because `Finalize` did not run, and every recoverable interrupted run has
  `commands.json`, so `unlock` returns the project to idle and the run's work
  is resumed with `retry --run-id RUN`. The queue is never restored from the
  interrupted run.
- **D2** Job selectors look in the queue first, then the run, in every
  project state, matching idle projects and `retry`'s queue-first source
  rule. Running-only resolution existed because the queue duplicated the
  running run; with the queue holding the next run, it would hide queued
  jobs. Job control keeps resolving in the active run.
- **D3** `retry` keeps its source rule (non-empty queue as is, matched by
  `Origin` or fingerprint; empty queue: built from the latest run) and
  reports what a non-empty queue leaves out. A special case for queues edited
  during a run would make the same rule behave differently by history.
- **D4** Phase 2 requests are files in `runs/<run-id>/` named
  `retry_request-<id>.json`, not a `requests/` subdirectory: a run's
  subdirectories are its job IDs, and any name is a valid job ID. They are
  polled by the supervisor, with a response file per request. The
  supervisor is reachable only through its parent's pipes, so a new
  supervisor operation would need a new listener (socket path, permissions,
  stale-socket cleanup) that works only on one host. File requests reach the
  supervisor from every interface and from other hosts on a shared state
  directory, like `cancel`; about a second of polling delay is acceptable
  for a retry.
- **D5** A retry request that the run cannot accept because the run ended
  fails with an error that names the ended run and says that repeating the
  command starts a new retry run. It does not fall back to a new run on its
  own: an in-run retry and a new run differ in history, carried results, and
  run settings, so the user chooses.
- **D7** Queue edits, including `reset`, are allowed while interrupted; `run`
  stays rejected until `unlock`. `reset --recover` is removed with an error
  naming `unlock`, because the queue it discarded is no longer the run's.
- **D8** A run built from a saved run (`retry --run-id`, and `retry` on an
  empty queue) takes the copy as its snapshot directly and leaves the queue
  untouched. In the model, it is a new run that goes straight to running;
  the queued run is a different object. `retry` keeps being copy then run,
  but the copy is no longer written to `queue.json`. Costs accepted: a run
  that fails before starting leaves no copied queue to edit, and
  `retry --overwrite` is removed.

Open:

- **D6** Whether to add a per-directory project pin; the decision gate is
  tracked in [development/2026-10-06-project-selection/plan.md](../2026-10-06-project-selection/plan.md).
- **D10** Submitted-job definition revision is deferred. The user questions
  whether editing a job after submission is needed. Keep the shipped active
  retry behavior (same run, original job definition) unless a concrete
  workflow demonstrates that changing a next-attempt definition is required.
- **D11** The preferred target model has one execution destination: the open
  run owns pending and started work. A manifest is the pre-run authoring
  format; there is no next-run queue to promote from. `add` targets the open
  run, and `change` edits the manifest before start. During migration, legacy
  queue operations must be explicitly marked transitional and must not route
  based implicitly on active project state.

## Validation for each phase

Follow AGENTS.md: focused package tests first, then package tests,
`pre-commit` on changed files, `scripts/check.sh --short`, `go test
./conformance`, and `scripts/check.sh` before finishing. Update the contract
status table in `contracts/README.md` together with each new `covers` call.
Record each phase's commits in `work-log.md` in this directory.
