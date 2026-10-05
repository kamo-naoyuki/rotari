# Plan: Work on a Project While Its Run Is Active

**Created:** 2026-10-05

**Status:** Proposed; no implementation started. D1, D3, D7, and D8 settled on 2026-10-06.

## Purpose

A project runs at most one run at a time, and while it runs, every queue edit
is rejected (SAFE-2). The second rule, not the first, causes most of the
waiting: users cannot prepare the next batch, add a forgotten job, or rerun a
job that failed early until the whole run ends. The current workaround,
creating another project, splits related history and then requires
`--project-name` on every command because automatic project selection stops
working once a state directory has more than one project.

This plan removes that waiting without allowing two runs of one project.
One run per project stays: it keeps result resolution linear (one latest
attempt per job, one active run), keeps `show` / `wait` / `cancel` defaults
unambiguous, and keeps the three-state lock and recovery model. Work that
needs to run now is moved into the active run instead of into a second run.

### Model: the queue is the next run before it starts

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
   same way `run --retry` does, instead of being rejected.
3. **Reduce the cost of switching projects.** Measure where multiple
   projects force `--project-name`, then remove the cases that have one
   unambiguous answer.
4. **Add jobs to the active run** (conditional). Only if phases 1 and 2 leave
   a demonstrated need, let new jobs join the active run, reusing the phase 2
   request channel.

Phases 1 and 2 are independent and can ship in either order. Phase 3 starts
with measurement after phase 1, because phase 1 removes the most common
reason to create a second project. Phase 4 depends on phase 2.

### Non-goals

- Two concurrent runs of one project. Retry and late additions go into the
  active run instead (phases 2 and 4).
- A continuously accepting queue as the default mode. The FAQ answer that
  rotari does not provide one stays true through phase 3; phase 4 revisits it
  only for an explicit request.
- A global, mutable "current project" shared by every shell.
- Allowing `run` or `delete` on a running or interrupted project. They stay
  rejected; only queue edits change in phase 1.
- Changing the executor, environment, or concurrency of a run that has
  already started.

## Current implementation

- `project.EnsureIdle` in
  [internal/project/inspect.go](../../internal/project/inspect.go) rejects
  running and interrupted projects. Every queue edit reaches it through
  `project.Edit` / `project.EditQueue` in
  [internal/project/edit.go](../../internal/project/edit.go), and
  `PreviewRun` in [internal/projectrun/plan.go](../../internal/projectrun/plan.go)
  and the supervisor in [internal/supervisor/run.go](../../internal/supervisor/run.go)
  call it directly.
- Idle edits write `meta.json` phase `collecting` (`writeIdleQueueWith`).
  During a run the phase is `running` or `cancelling`, and the engine's
  `Stopped` callback polls it to detect cancellation.
- `Runner.Begin` in
  [internal/projectrun/lifecycle.go](../../internal/projectrun/lifecycle.go)
  writes the context, takes `running.lock`, registers the run, and marks the
  project running. It does not snapshot the queue.
- `Runner.Execute` in
  [internal/projectrun/execute.go](../../internal/projectrun/execute.go)
  reads `queue.json` in the supervisor, possibly well after `Begin` for an
  async run, and writes `runs/<run-id>/commands.json`.
- `Runner.Finalize` clears `queue.json` commands (`state.FinalizeRun` in
  [internal/state/run_files.go](../../internal/state/run_files.go)) on the
  assumption that the queue is the one the run consumed.
- `RunSource` in [internal/projectrun/source.go](../../internal/projectrun/source.go)
  decides where `retry` gets its jobs: a result selection copies the last run
  only into an empty queue, so a queue restored and edited earlier is kept.
- The queue of an interrupted run is "retained" only because `Finalize`,
  which would have cleared it, never ran. `unlock` (`project.RecoverInterrupted`)
  leaves it in place (SAFE-4). `reset` on an interrupted project discards it
  and recovers the run in one step, after a confirmation that the run's jobs
  stopped; `--recover` / `ROTARI_RESET_RECOVER` gives that confirmation
  without a prompt (SAFE-5, SAFE-6), and the confirmation reports jobs that
  still look running (SAFE-7). MCP recovers only through
  `rotari_preview_reset` / `rotari_reset` ([internal/mcp/reset.go](../../internal/mcp/reset.go));
  the Python client exposes the same option.
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
5. **Selector resolution.** Queue-editing commands (`change`, `remove`, and
   `show` of queued jobs) must find jobs in the queue while a run is active or
   interrupted. Inspection commands keep resolving that run first (decision
   D2).
6. **Copy sources.** `copy` and `retry --run-id` from the active run, or from
   an interrupted run before `unlock`, are rejected: their results are not
   final and their jobs may still run.
7. **Recovery.** `project.RecoverInterrupted` loses its discard-queue mode.
   Move the still-running-jobs report (SAFE-7) from `reset` to `unlock`.
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
  editable; the run view is unchanged. Check every handler that calls the
  shared gate.
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
The output states which happened.

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

1. **Request channel (decision D4).** Recommended: a file-based request under
   `runs/<run-id>/`, written under the state lock after checking that the
   lock and metadata name this run as running, the same way `cancel` marks a
   project. A supervisor goroutine picks requests up and answers each one in a
   response file (accepted job IDs or a reason). This works from the CLI, the
   Web UI process, and MCP without a new supervisor operation.
2. **Engine event.** Add an event to `run.ExecuteJobs` that reopens final
   jobs: clear `final`, return them and their blocked `DependsOn` dependents
   to `waiting`, and continue attempt numbering. The loop must also wake for
   requests, not only for job results.
3. **End-of-run race.** The engine can return between the request check and
   pickup. `Finalize` answers any unanswered request with "run ended" under
   the state lock, and the CLI then starts a normal retry run and says so
   (decision D5). The CLI waits for a response with a timeout and reports a
   missing supervisor as an error.
4. **Final-result callbacks.** `FinalResult` becomes "once per final result":
   diagnosis, notification hooks, and `Observer.Finished` see a reopened job's
   new final result. Check the progress display, which may now see
   `completed` decrease.

### Phase 2: Interfaces

CLI `retry`; a Web UI action on failed jobs of the active run and its API;
an MCP tool or an extension of the existing run tool; the Python client.

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
- Request/response handling, including the end-of-run race and a cancelling
  run.
- Conformance: a run with one fast-failing job and one slow job; `retry
  --failed` while the slow job runs reruns only the failed job in the same
  run; option rejection rows; the same through the Web API.

## Phase 3: Cost of switching projects

Start by measuring, after phase 1:

1. List every command that fails with "multiple projects exist" when more than
   one project exists, and for each one, whether one answer is unambiguous.
2. For commands that act on an active run (`cancel`, `suspend`, `resume`,
   `retry` in phase 2), use the only running project when exactly one runs,
   as `wait` already does. Several running projects stay an error that lists
   them.
3. Consider a per-directory project pin that follows the existing
   `./.rotari-state` precedent: a file in the working directory naming the
   project, resolved after `--project-name` and `ROTARI_PROJECT_NAME`. Decide
   only if step 1 shows that queue-editing commands are still the main cost
   (decision D6).

Changes to resolution order go into `contracts/01-resolution-and-config.md`
(RES rules), `docs/CONCEPTS.md` "State and project resolution", and the
resolution conformance tests, for every command that shares the rule.

## Phase 4: Add jobs to the active run (conditional)

Proceed only if users still need queued work to start before the active run
ends after phases 1 and 2. Sketch:

- A request over the phase 2 channel moves selected queued jobs into the
  active run's `commands.json` and the engine's pending set.
- New jobs may depend on jobs in the run; a dependency on a final failure
  blocks them at once.
- The run snapshot stops being immutable while the run is active; it is
  immutable once the run finishes. This changes the run contract and needs
  its own decision record.
- If the run has already ended, the request fails and the jobs stay queued.

## Decisions

Settled on 2026-10-06:

- **D1** Recovery does not touch the queue. The queue was retained only
  because `Finalize` did not run, and every recoverable interrupted run has
  `commands.json`, so `unlock` returns the project to idle and the run's work
  is resumed with `retry --run-id RUN`. The queue is never restored from the
  interrupted run.
- **D3** `retry` keeps its source rule (non-empty queue as is, matched by
  `Origin` or fingerprint; empty queue: built from the latest run) and
  reports what a non-empty queue leaves out. A special case for queues edited
  during a run would make the same rule behave differently by history.
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

- **D2** Selector resolution while running: queue-editing commands look in
  the queue; inspection commands keep the active run first (recommended).
- **D4** Phase 2 transport: file-based request (recommended) or a new
  supervisor operation.
- **D5** End-of-run race: fall back to a new retry run and say so
  (recommended), or fail and ask the user to repeat the command.
- **D6** Whether to add a per-directory project pin.

## Validation for each phase

Follow AGENTS.md: focused package tests first, then package tests,
`pre-commit` on changed files, `scripts/check.sh --short`, `go test
./conformance`, and `scripts/check.sh` before finishing. Update the contract
status table in `contracts/README.md` together with each new `covers` call.
Record each phase's commits in `work-log.md` in this directory.
