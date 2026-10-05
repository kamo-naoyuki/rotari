# Plan: Work on a Project While Its Run Is Active

**Created:** 2026-10-05

**Status:** Proposed; no implementation started.

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

## Scope and order

1. **Edit the queue while a run is active.** The run takes the queue when it
   starts, so `add`, `change`, `remove`, `copy`, and `import` can prepare the
   next run.
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
- Allowing `run`, `reset`, or `delete` on a running project. They stay
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
- `unlock` / `reset --recover` (`project.RecoverInterrupted`) keep the
  retained queue, which is today the interrupted run's queue (SAFE-4).
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
- While the project is running, `add`, `change`, `remove`, `copy`, and
  `import` edit the queue for the next run. `run`, `reset`, and `delete`
  stay rejected; their message says that the queue can be edited.
- When the run finishes, the queue is left as edited; it is no longer cleared.
- Interrupted projects keep rejecting queue edits until recovered (SAFE-3),
  because recovery decides what the queue holds.

### Phase 1: Implementation

1. **Snapshot at `Begin`.** Write the queue to `runs/<run-id>/commands.json`
   before taking the run lock, and clear `queue.json` commands after the
   metadata says running, all under the state lock the caller already holds.
   A crash before the clear leaves today's state (queue still filled). On a
   `Begin` failure after the clear, restore the queue.
2. **Execute from the snapshot.** `Execute` reads the run's `commands.json`
   instead of `queue.json`. `RunSource` copying, fingerprint matching, and
   origin recording keep writing the same snapshot. Active runs then always
   have `commands.json`; update the consistency rule in
   [contracts/04-coordination-and-safety.md](../../contracts/04-coordination-and-safety.md)
   that lets an active run lack it.
3. **Stop clearing at finish.** `state.FinalizeRun` updates metadata only;
   `Finalize` no longer writes the queue.
4. **Split the idle check.** Replace the single `EnsureIdle` gate for queue
   edits with a shared `EnsureQueueEditable` (idle or running; not
   interrupted) in `internal/project`, used by `Edit` / `EditQueue`. Keep
   `EnsureIdle` for `run`, `reset`, `delete`, and `PreviewRun`. Queue edits
   during a run must not write the `collecting` phase; only an idle edit sets
   it. Keep the per-command operation names so messages stay specific.
5. **Selector resolution.** Queue-editing commands (`change`, `remove`, and
   `show` of queued jobs) must find jobs in the queue while the project runs.
   Inspection commands keep resolving the active run first (decision D2).
6. **Recovery.** With the run's commands in `commands.json`, the queue no
   longer explains an interrupted run. `unlock` and `reset --recover` keep the
   current queue; when it is empty they restore the interrupted run's
   commands into it, which preserves today's result for users who did not
   edit during the run (decision D1). The interrupted-run message names
   `retry --run-id RUN` as the way to resume that run's work.
7. **Retry source.** A queue edited during a run is not "a queue restored
   earlier". `RunSource` must not treat it as one (decision D3).

### Phase 1: Interfaces

- CLI: the commands above, `check` / `show` output for a running project with
  a non-empty queue (show both the active run and the next queue).
- Web UI / API: the queue view of a running project becomes editable; the run
  view is unchanged. Check every handler that calls the shared gate.
- MCP: `rotari_import` and the preview/revision flows
  (`rotari_preview_run`, `rotari_start_run`). A run changes the revision, so
  a preview made before a run starts must still fail its guard.
- Python client: wraps the CLI; update its docs and tests for the new
  acceptance.

### Phase 1: Contracts and documentation

- Rewrite SAFE-2 and the state table in
  [contracts/04-coordination-and-safety.md](../../contracts/04-coordination-and-safety.md);
  add an ID for "a run takes the queue at start and leaves later edits".
  Update SAFE-4 for recovery.
- [docs/CONCEPTS.md](../../docs/CONCEPTS.md): the queue roles per state and
  the run flow diagram (`queue.json: empty` happens at start, not at finish).
- [docs/FAQ.md](../../docs/FAQ.md): the "another project" answer and the
  idle/running queue-first explanation.
- [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md): `Begin`, `Execute`, and
  `Finish` steps.

### Phase 1: Tests

- `internal/projectrun`: `Begin` snapshots and clears; `Execute` ignores
  `queue.json`; `Finalize` keeps edits; `Begin` failure restores the queue.
- `internal/project`: the edit gate for idle, running, cancelling, and
  interrupted projects; no phase write while running.
- Conformance: through the binary, add a job while a run is active (sync and
  async), let the run finish, and check that the run executed only its own
  jobs and the queue holds the added job. Cover each queue-editing command and
  confirm `run`, `reset`, and `delete` are still rejected. Cover interrupted
  recovery with an empty and a non-empty queue. Web API rows for the same.

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

## Decisions to make

- **D1** Recovery with an edited queue: keep the edits and restore the
  interrupted run only into an empty queue (recommended), always restore, or
  never restore.
- **D2** Selector resolution while running: queue-editing commands look in
  the queue; inspection commands keep the active run first (recommended).
- **D3** `retry` after a run when the queue was edited during it: record in
  the queue which run it was restored from and treat only such a queue as
  restored (recommended), or require `--run-id` when the queue is not empty.
  First check what `retry --failed` does today with a queue of new jobs.
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
