# Plan: Add New Jobs to an Active Run

**Created:** 2026-10-06

**Status:** Detailed proposal for review; implementation has not started. The recommended first step is a bounded, explicitly targeted append to an already-running run. Keep the existing next-run queue and all ordinary `add` / `change` behavior unchanged. Do not implement a queue-less open session, active-job editing, or definition-changing retry in this work. This is an incremental way to validate the value and interface cost of late additions before deciding whether the queue/run model should be replaced.

## Goal

Allow a user to submit a new job to a specific active run while other work in that run is still executing. The new job becomes run-owned not-yet-started work, uses the active run's fixed context/settings, and participates in the same dependency-aware scheduler. It is new work; it is not copied from the next-run queue and does not inherit a prior result implicitly.

A late submission is accepted atomically as a batch or rejected atomically. Its target run ID is explicit and is checked again at commit. If the run ends or begins cancellation before acceptance, report that exact outcome; never fall back to the queue or a new run.

## Recommended first-version behavior

| Concern | V1 proposal |
| --- | --- |
| Target | `add --run-id RUN_ID` explicitly selects active-run admission. Without `--run-id`, `add` keeps editing the next-run queue exactly as today. |
| Eligible run state | Only the exact currently running run while its engine is accepting work. No submission to idle, interrupted, cancelling, sealed, or finished runs. |
| Boundedness | The run accepts additions only while its current execution loop is alive. If all current work drains and the run is ending, a concurrent submission may be rejected as ended. Do not keep an empty run alive waiting for future work in V1. |
| Job edits | None after admission. `change` remains pre-execution queue/draft editing. A submitted or finished job's definition is immutable. |
| Run settings | Working directory, caller environment, executor lanes, concurrency, and run-wide options remain those captured by the original run. Per-job fields use the existing job-definition semantics. |
| Dependencies | A new job may depend on a named job already in this run or another job in the same atomic submission. Forward references to a later request are rejected. Existing jobs and dependency edges are not changed. |
| Groups | V1 starts with plain jobs. Existing array/matrix topology is immutable; adding complete new groups is a follow-up phase after plain-job admission is proven. Never extend an existing array, matrix, or stage in V1. |
| Results | Every admitted job is new work and is scheduled for execution; no result is carried from the queue or another run. A final failed prerequisite blocks a `DependsOn` dependent; `DependsOnFinished` follows existing readiness semantics. |
| Queue | The next-run queue is not read for selection, changed, consumed, or replaced by active admission. It remains an independent, explicitly separate destination until a later model decision. |
| Interfaces | Shared admission service and matching target/race semantics. CLI is the first adapter; Web, MCP, and Python follow before the feature is considered complete for the supported product surface. |

The exact CLI spelling, preview/revision parameter, and whether plain-job-only V1 is useful enough for an initial release are decisions to confirm before Phase 1 implementation. Do not silently overload a queue-targeting flag or infer the target from project state.

## Explicit non-goals

- Removing `queue.json` or changing `reset`, `run`, `retry`, or current queue-first selection semantics.
- Allowing two coordinators or two runs to execute the same project concurrently.
- Allowing additions after the run has drained or ended; no open-ended accepting session or idle timeout.
- Changing a job after submission, whether pending, running, or finished.
- Retrying a failed job with a changed command or executor definition.
- Promoting jobs from the next-run queue into the active run. `copy` continues to target its current queue workflow.
- Widening an existing array, adding members to an existing matrix/stage, or changing existing dependencies.
- Changing run-wide context, environment, concurrency, executor lanes, or scheduler options after run start.
- Reusing a browser session, current project, or implicit “only active run” as an active-add target.

## Architecture findings that shape the work

- `cmdAdd` currently builds the job definition and calls `queueops.Editor.Add`; it does not communicate with the supervisor. Its existing path must remain intact for ordinary `add`.
- `Runner.Begin` snapshots the initial commands; `Runner.Execute` reads and transforms that run snapshot once. The engine never rereads `queue.json`.
- `ExecuteJobs` currently initializes membership maps and totals once, then exits when `running == 0 && delayed == 0`. It can receive manual-retry events only while that loop is alive. New admission therefore requires an event-loop input and a bounded ending race, but not a scheduler rewrite.
- `Dispatcher` and executor lanes are already long-lived and accept repeated starts. Keep one dispatcher per run; do not recreate it for each admission.
- Active retry provides a useful cross-process file-request pattern: exact run ID, state-lock validation, supervisor polling, engine event, canonical response, and an end fence. Its job-ID-only payload, pending-result marker, and response type are retry-specific and must not be reused as if they already represented new membership.
- Run subdirectories are interpreted as job IDs. Any new protocol metadata must use validated/reserved root-level filenames, not a `requests/` directory.
- `commands.json` is written/updated by run setup and contains the initial run membership. An admission commit must not race a setup rewrite or let an external client rewrite the snapshot while the engine is running.

## Design decisions required before implementation

1. **Admission boundary:** precisely define when the supervisor starts accepting requests (after initial snapshot transformation and engine initialization) and when it closes admission relative to run finalization.
2. **Canonical acceptance record:** choose the durable commit point for a request. A response file alone is insufficient: a crash after acceptance but before response must not lose or duplicate accepted jobs. Prefer a per-request immutable accepted-membership record with request ID, sequence/revision, normalized expanded job definitions, and acceptance time; the coordinator writes it before changing in-memory engine state. Final snapshots/readers combine initial membership and committed additions. Validate reserved filenames and state-version behavior.
3. **Atomic batch validation:** validate names, IDs, expanded task IDs, commands, paths, executors, and the combined dependency graph against current committed membership plus the whole incoming batch. Commit all jobs or none. A single event-loop writer serializes admissions; external goroutines must not mutate engine maps.
4. **Dependency/group policy:** use shared model validation/expansion. Existing groups may be dependencies if their membership is stable, but may not be expanded by an admission. V1 rejects unknown/future names and cycles. Decide whether existing `ValidateJobs` can validate combined run membership without imposing queue-only defaults.
5. **Run-scoped revision:** project revision currently hashes queue and metadata, not active run membership. Define an admission sequence/revision scoped to `(project, run ID)` for preview/apply and stale-request detection; do not make unrelated next-queue edits invalidate an active-run preview.
6. **Request idempotency and unknown timeout:** a caller timeout means acceptance is unknown. A stable request ID must allow lookup/retry of the same request and reject reuse with a different payload. Do not automatically submit a second ID after timeout.
7. **Compatibility/capabilities:** prevent a new client from writing a request an older supervisor silently ignores. Define the active-run admission capability/version marker and the explicit unsupported-version response. Consider state version bump from current version 3; decide the exact migration policy only after file format is selected.
8. **Progress/status semantics:** define dynamic totals, pending counts, run summaries, job listing, `wait`, export, timeline, and notification behavior when membership grows. “Accepted” means durably in the run, not started or succeeded.
9. **Cancellation and locking:** serialize admission commit with cancellation/finalization under the project state lock; never hold that lock while waiting for a supervisor response. A cancel that wins first rejects the addition; an accepted addition that wins first is included in cancellation and recovery semantics.
10. **CLI/API contract:** decide `add --run-id RUN`, dry-run behavior, run admission revision option, response shape (request ID, accepted job IDs, sequence), and errors. Active-add preview must inspect the exact active run and must not write queue state.
11. **Arrays/matrices:** decide whether V1 must include whole new groups. If so, promote group expansion from follow-up into the first phase and add aggregate/native-array conformance before implementation; existing group topology remains immutable either way.

## Implementation phases and checkpoints

### Phase 0 — Freeze the user contract

Write a decision table for: ordinary `add` versus active-targeted `add`, running versus ending/cancelling run, queue independence, new-work result policy, dependencies, and duplicate/unknown request behavior. Specify bounded admission (no waiting after drain) separately from any future open/seal session. Confirm no active `change` requirement.

**Checkpoint:** CLI, Web, MCP, Python, and Web read-side behavior can be described consistently. Resolve whether V1 is plain-only or includes complete new arrays/matrices. No runtime edits yet.

### Phase 1 — Model validation and engine admission event

Likely files:

- `internal/model/model.go`, `internal/model/dependencies.go`: reuse or add combined-membership validation/expansion while keeping existing selector/dependency rules centralized.
- `internal/run/engine.go`: introduce a typed add request/event; process it in the existing event loop; validate/commit before mutating `jobsByID`, `jobsByName`, `waiting`, results/finality, and dynamic `total`; schedule newly ready work through the existing dispatcher.
- `internal/run/dispatch.go`: expected to need little or no structural change; verify ordering, lane capacity, and newly started jobs.

Tests first: plain independent admission; pending/running/successful/failed dependencies for both dependency kinds; same-batch dependencies; unknown name; cycle; duplicate name/ID; all-or-none rejection; admission interleaved with result/retry-delay events; dynamic progress; no concurrent map mutation. Keep ordinary `ExecuteJobs` callers and result selection unchanged.

**Checkpoint:** engine tests prove deterministic state changes and existing run/active-retry tests still pass. If combined dependency validation requires broad model refactoring, stop and split that refactor before proceeding.

### Phase 2 — Durable request protocol and admission commit

Likely files:

- `internal/state/attempts.go`, `internal/state/run_files.go`, `internal/state/store.go`: reserved root protocol names, validation, version/capability compatibility, atomic per-file persistence.
- `internal/jobcontrol/retry_request.go` and adjacent `jobcontrol` files: extract shared request polling/response/idempotency mechanics only where responsibilities genuinely overlap; add admission-specific typed payloads and exact-run submit/lookup APIs. Do not generalize the retry domain selection into a generic opaque framework.
- `internal/projectrun/execute.go`, `internal/projectrun/lifecycle.go`: establish admission accepting/closed lifecycle after setup, attach watcher to the engine, fence shutdown before summary finalization.
- A focused package/file may own canonical admission records and run revision; preserve one-way dependencies and `internal/archtest` boundaries.

Crash/race tests: request before commit; committed admission before response; response before engine event; duplicate same request; same ID/different payload; concurrent append; cancellation race; finish/end race; timeout and later lookup; malformed/newer request; coordinator interruption after accepted pending work; ensure no response wait holds the state lock. Recovery must reconstruct every accepted job exactly once from durable run-owned records.

**Checkpoint:** a protocol-level test proves accepted membership is canonical independent of response delivery and can be reconstructed after restart/unlock. Do not proceed based solely on happy-path CLI tests.

### Phase 3 — `projectrun` integration and CLI

Likely files:

- `cmd/rotari/add.go`, CLI spec source (`cmd/rotari/cli_spec.go`), generated reference via its generator: parse explicit active-run mode, preserve queue-mode behavior, return accepted IDs and request ID.
- `internal/projectrun/execute.go`, `internal/projectrun/validate.go`, `internal/projectrun/artifacts.go`, `internal/run/summary.go`: prepare added jobs with the original run's context/environment, validate against fixed run options, record artifacts, include final accepted membership in summary.
- `internal/runview/run.go`, `internal/web/loader.go`, `internal/web/timeline.go`, `internal/resolve/resolve.go`: expose committed membership and distinguish accepted-not-started from running/scheduler-submitted states; avoid changing existing selector precedence.
- `internal/project/edit.go`: keep project queue revision semantics unchanged; implement separate run admission sequence as designed.

CLI conformance: active sync and async run; explicit correct run ID; wrong/ended/interrupted/cancelling run; unrelated queued jobs remain byte-for-byte unchanged; no second supervisor; job executes with run context rather than submitter shell context; admission is always new work even for filtered runs; summary and show include the accepted job; cancel after acceptance; end-race rejection with no fallback.

**Checkpoint:** package tests, targeted CLI tests, then binary conformance and coordination tests. Ensure ordinary `add`, `run`, `retry`, `copy`, and `reset` regressions remain unchanged.

### Phase 4 — Web, MCP, Python, and complete read surfaces

Likely files:

- `internal/webui/webui.go`, `internal/webui/assets/web_app_actions.js`, relevant web app view/log/timeline assets: active-run add endpoint/form, exact run selection, preview/acceptance/unknown-timeout feedback, dynamic job counts.
- `internal/mcp/server.go`, `internal/mcp/write.go`: active-add preview/apply tools with exact run ID, revision, accepted IDs, request ID; preserve redaction and capability errors.
- `python/rotari/client.py`, Python object/API tests and generated docs/schema: run-bound addition method and structured result, or clearly documented explicit `add(run_id=...)`; retain CLI as behavior source of truth.
- `docs/MCP.md`, `docs/PYTHON_CLIENT.md`, generated `docs/CLI_REFERENCE.md`, `docs/RUNNING.md`, `docs/INSPECT.md`, `docs/FAQ.md`, `docs/ARCHITECTURE.md`, relevant contracts and conformance layout.

Tests: all interfaces target the same exact run and return equivalent accepted IDs; read-only/static Web rejects writes; auth/control behavior; MCP preview stale revision; Python argv/result; show/jobs/export/timeline/summary/wait agree on accepted membership; no uncommitted request is visible as a run job.

**Checkpoint:** parity across CLI, Web API/UI, MCP, and Python. Add contract IDs and conformance `covers` entries matching `contracts/README.md`.

### Phase 5 — Consider arrays/matrices and open/seal separately

Only after plain-job admission is validated:

- support an entire new array/matrix group atomically, if required;
- continue to reject changes to groups already present in the run;
- test sparse/dense arrays, matrix×array, native scheduler arrays, selection, cancellation, aggregate status, export/import, and ID collisions.

Separately decide whether users need admission while no jobs are running. If yes, add accepting/sealed/drained lifecycle, explicit close (`finish`/`seal` or a separately decided `reset` meaning), empty sessions, foreground `run`/`wait`, cancellation, recovery, and UI state. This is a second lifecycle project, not a flag added to bounded append. Keep current `reset` queue-clearing behavior until a contract change is explicitly approved.

## Acceptance criteria

- Active admission always names and revalidates one exact run; it cannot target another run after a race.
- Existing ordinary queue editing remains unchanged and does not implicitly submit.
- Active addition never mutates `queue.json` and never starts a second coordinator.
- Each accepted request is atomic, idempotent, durably recoverable, and visible exactly once.
- New jobs use the original run's context/settings and shared dependency semantics.
- Running/finished job definitions, run-wide settings, existing dependencies, and existing array/matrix topology are not changed.
- Progress, results, summaries, inspection, and all supported interfaces agree on accepted membership.
- Relevant tests, contracts, docs, generated references, and `scripts/check.sh` pass before implementation is declared complete.

## Decision after V1

Collect concrete experience with active additions. Then decide independently:

1. Are late additions common/useful enough to keep?
2. Is explicit `--run-id` targeting clear, or does the second queue/run workspace remain confusing in practice?
3. Is open admission after drain actually needed?
4. Does evidence justify replacing the next-run queue with run-owned pending work?

Do not remove queue-based workflows merely because active admission exists.

## Related plans

- [Execution-model review](../2026-10-06-execution-model-review/plan.md)
- [Work during runs umbrella](../2026-10-05-work-during-runs/plan.md)
- [Project-selection audit](../2026-10-06-project-selection/plan.md)
