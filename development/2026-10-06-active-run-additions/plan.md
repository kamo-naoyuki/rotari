# Plan: Move to Run-Owned Pending Work

**Created:** 2026-10-06

**Status:** Paused alternative; no implementation should start from this plan until the run/retry product decision is revisited. The user now questions whether immediate submission weakens the meaning of a run and makes retry confusing, and whether same-run retry of a finished job during unrelated work is useful. The `start` / `submit` / `wait` and queue-retirement design remains documented as an option, not the preferred direction. The current queue/run behavior remains the default. Same-run active retry is already shipped, so removal/deprecation requires a separate compatibility decision.

## Goal

Replace the separate next-run queue with pending work owned by the execution run. `start` opens a run from a workflow manifest (or explicitly opens an empty session) and begins execution immediately for ready jobs. While admission remains open, `submit` adds new jobs to that run and schedules them as soon as dependencies permit. `wait` seals admission and waits for submitted work to finish. Pending definitions, execution attempts, and results share one run identity and history. A manifest is the pre-run authoring/review format; it is not a hidden mutable project queue.

A late submission is accepted atomically as a batch or rejected atomically. Its target run ID is explicit and is checked again at commit. If the run ends or begins cancellation before acceptance, report that exact outcome; never fall back to the queue or a new run.

## Target product behavior (proposal)

| Concern | V1 proposal |
| --- | --- |
| Target | `submit` adds work to the open run; it never stages work for a future run. Require an exact run/project target or define one unambiguous project-scoped rule. |
| Bootstrap | `start [MANIFEST]` opens a run from a validated manifest, or opens an empty session if that form is chosen, and begins ready jobs immediately. |
| No open run | `submit` fails with guidance to use `start`; it does not create a hidden draft or implicitly stage work. |
| Existing open run | `start` seals the prior run before opening a new one. Decide whether it waits for prior jobs to finish or rejects while they remain active; do not allow overlapping coordinators by accident. |
| Wait | `wait` seals the current run against new submissions, then waits for its accepted work to finish. Clarify whether `wait --run-id` may wait on an already-sealed run. |
| Admission lifetime | An open run continues accepting additions even when currently drained, until an explicit seal/finish boundary. An inactivity timeout is not the default. |
| Job edits | No edits after admission. `change` edits the manifest before `run`; a submitted or finished job's definition remains immutable. |
| Run settings | Working directory, caller environment, executor lanes, concurrency, and run-wide options remain those captured by the original run. Per-job fields use the existing job-definition semantics. |
| Dependencies | A new job may depend on a named job already in this run or another job in the same atomic submission. Forward references to a later request are rejected. Existing jobs and dependency edges are not changed. |
| Groups | Manifest and active additions must preserve current arrays/matrices as complete groups. Adding members to a group after part of it has started is not allowed initially; group-topology rules need a dedicated decision. |
| Results | Every admitted job is new work and is scheduled for execution; no result is carried from the queue or another run. A final failed prerequisite blocks a `DependsOn` dependent; `DependsOnFinished` follows existing readiness semantics. |
| Queue | No project-level next-run queue in the target model. Pre-run composition/review uses a versioned user-owned workflow manifest. Legacy queue data exists only during a compatibility/migration period; it is never silently executed or discarded. |
| Retry | A finished run's retry continues to create a successor run with carried results. Same-run active retry remains a separate transient-failure operation unless later evidence favors a change. Retry is not an implicit seal. |
| Script/client lifetime and cancellation scope | Once a submission is durably accepted, killing the calling script/client does not cancel it. Run-level cancel stops every unfinished job accepted into that session. Rotari already supports cancelling selected jobs with repeated `--job-id` values (and an array ID selects its unfinished tasks), so separate submission-group IDs are not needed merely to cancel one script's jobs; the submitter can retain the returned job IDs. The UI/CLI must show an open run left behind before `wait`. |
| Interfaces | One shared admission service and identical run lifecycle in CLI, Web, MCP, and Python. No interface may keep a hidden staging collection after queue removal. |

The initial-work bootstrap, exact CLI spelling, seal operation (including the role of `reset`), run-scoped revision, and group-topology policy are decisions to confirm before implementation. Do not infer a target from project state without an explicit and consistent rule.

### Preserve the meaning of `run` and `retry`

Immediate submission risks turning a run from a prepared batch into an open
time interval whose membership can span scripts. Keep the run ID as the
stable history, settings, dependency, and whole-run cancellation unit. `wait`
seals membership before the run becomes a stable source for comparison or
successor-run retry. A submitter exiting before `wait` does not cancel work;
the still-open run must remain discoverable and closable by another client.

Keep finished-run `retry` as a new successor run with carried results unless
there is an explicit product decision to change it. The existing active retry
reopens final jobs in the same run; if retained, give it a distinct explicit
form (such as `retry-active`) rather than making `retry` mean “same run” or
“successor run” according to hidden session state. Decide if run-wide cancel
stops all unfinished work in the run. For a narrower selection, existing
repeated job-ID cancellation can select a submitter's jobs if `submit` returns
their accepted IDs; no new submission-group identity is needed for this.

**Required examples before implementation:** two scripts submit to one open
run; the first script exits before `wait`; a user cancels the run; `wait`
seals and finishes it; a later retry starts a successor run; and an active
same-run retry is invoked if that capability remains. Each example must state
job membership, cancellation scope, and returned run ID.

## Explicit non-goals

- Changing retry's established successor-run semantics for settled runs, unless separately justified.
- Allowing two coordinators or two runs to execute the same project concurrently.
- An automatic inactivity timeout as the default admission boundary.
- Changing a job after submission, whether pending, running, or finished.
- Retrying a failed job with a changed command or executor definition.
- Changing a submitted job's definition or retrying it with changed command/options.
- Widening an existing array, adding members to an existing matrix/stage, or changing existing dependencies.
- Changing run-wide context, environment, concurrency, executor lanes, or scheduler options after run start.
- Reusing a browser session, current project, or implicit “only active run” as an active-add target.
- Silently losing existing `queue.json` data during migration.

## Architecture findings that shape the work

- `cmdAdd` currently builds the job definition and calls `queueops.Editor.Add`; it does not communicate with the supervisor. Its existing path must remain intact for ordinary `add`.
- `Runner.Begin` snapshots the initial commands; `Runner.Execute` reads and transforms that run snapshot once. The engine never rereads `queue.json`.
- `ExecuteJobs` currently initializes membership maps and totals once, then exits when `running == 0 && delayed == 0`. It can receive manual-retry events only while that loop is alive. New admission requires an event-loop input and a drained-but-accepting lifecycle that waits for add/seal events, but not a scheduler rewrite.
- `Dispatcher` and executor lanes are already long-lived and accept repeated starts. Keep one dispatcher per run; do not recreate it for each admission.
- Active retry provides a useful cross-process file-request pattern: exact run ID, state-lock validation, supervisor polling, engine event, canonical response, and an end fence. Its job-ID-only payload, pending-result marker, and response type are retry-specific and must not be reused as if they already represented new membership.
- Run subdirectories are interpreted as job IDs. Any new protocol metadata must use validated/reserved root-level filenames, not a `requests/` directory.
- `commands.json` is written/updated by run setup and contains the initial run membership. An admission commit must not race a setup rewrite or let an external client rewrite the snapshot while the engine is running.

## Design decisions required before implementation

1. **Bootstrap and admission boundary:** define how `start [MANIFEST]` creates a run, when the supervisor starts accepting submissions (after initial snapshot transformation and engine initialization), and how `wait` seals admission before waiting for completion.
2. **Canonical acceptance record:** choose the durable commit point for a request. A response file alone is insufficient: a crash after acceptance but before response must not lose or duplicate accepted jobs. Prefer a per-request immutable accepted-membership record with request ID, sequence/revision, normalized expanded job definitions, and acceptance time; the coordinator writes it before changing in-memory engine state. Final snapshots/readers combine initial membership and committed additions. Validate reserved filenames and state-version behavior.
3. **Atomic batch validation:** validate names, IDs, expanded task IDs, commands, paths, executors, and the combined dependency graph against current committed membership plus the whole incoming batch. Commit all jobs or none. A single event-loop writer serializes admissions; external goroutines must not mutate engine maps.
4. **Dependency/group policy:** use shared model validation/expansion. Existing groups may be dependencies if their membership is stable, but may not be expanded by an admission. V1 rejects unknown/future names and cycles. Decide whether existing `ValidateJobs` can validate combined run membership without imposing queue-only defaults.
5. **Run-scoped revision:** project revision currently hashes queue and metadata, not active run membership. Define an admission sequence/revision scoped to `(project, run ID)` for preview/apply and stale-request detection; do not make unrelated next-queue edits invalidate an active-run preview.
6. **Request idempotency and unknown timeout:** a caller timeout means acceptance is unknown. A stable request ID must allow lookup/retry of the same request and reject reuse with a different payload. Do not automatically submit a second ID after timeout.
7. **Compatibility/capabilities:** prevent a new client from writing a request an older supervisor silently ignores. Define the active-run admission capability/version marker and the explicit unsupported-version response. Consider state version bump from current version 3; decide the exact migration policy only after file format is selected.
8. **Progress/status and cancellation:** define dynamic totals, pending counts, run summaries, job listing, `wait`, export, timeline, and notification behavior when membership grows. “Accepted” means durably in the run, not started or succeeded. Preserve existing whole-run cancellation and selected-job cancellation via repeated job IDs; dynamically accepted but not-yet-started jobs must also be cancellable by their returned IDs. An array job ID should continue selecting its unfinished tasks.
9. **Cancellation and locking:** serialize admission commit with cancellation/finalization under the project state lock; never hold that lock while waiting for a supervisor response. A cancel that wins first rejects the addition; an accepted addition that wins first is included in cancellation and recovery semantics.
10. **CLI/API contract:** define `start [MANIFEST]`, `submit` syntax and run targeting, behavior when no run is open, dry-run behavior, run admission revision option, response shape (request ID, accepted job IDs, sequence), and errors. Preview must inspect the exact run and must not write queue state.
11. **Arrays/matrices:** decide whether V1 must include whole new groups. If so, promote group expansion from follow-up into the first phase and add aggregate/native-array conformance before implementation; existing group topology remains immutable either way.

## Implementation phases and checkpoints

### Phase 0 — Freeze the queue-less user contract

Write a decision table for: `start [MANIFEST]`, `submit` to an open run, `submit` when no run is open, drained/open versus sealed, `wait` sealing and waiting, cancellation/end races, new-work result policy, dependencies, and duplicate/unknown request behavior. Under the existing one-active-run invariant, when `start` encounters an open run it may seal it but must not start another coordinator while prior jobs are still executing. Recommended default: seal and wait for that run to finish, then create the new run; explicitly compare this with failing fast or allowing multiple concurrent runs. Decide whether `reset` remains a legacy queue operation only during migration or is retired; never overload it silently. Submitted-job changes are excluded.

**Checkpoint:** CLI, Web, MCP, Python, and Web read-side behavior can be described consistently, including accepted work after client exit, run-wide cancellation, and cancellation by selected returned job IDs. Resolve whether complete new arrays/matrices are accepted in the first release and define manifest bootstrap. No runtime edits yet.

### Phase 1 — Model validation and engine admission event

Likely files:

- `internal/model/model.go`, `internal/model/dependencies.go`: reuse or add combined-membership validation/expansion while keeping existing selector/dependency rules centralized.
- `internal/run/engine.go`: introduce a typed add request/event; process it in the existing event loop; validate/commit before mutating `jobsByID`, `jobsByName`, `waiting`, results/finality, and dynamic `total`; schedule newly ready work through the existing dispatcher.
- `internal/run/dispatch.go`: expected to need little or no structural change; verify ordering, lane capacity, and newly started jobs.

Tests first: plain independent admission; pending/running/successful/failed dependencies for both dependency kinds; same-batch dependencies; unknown name; cycle; duplicate name/ID; all-or-none rejection; admission interleaved with result/retry-delay events; dynamic progress; no concurrent map mutation. Keep ordinary `ExecuteJobs` callers and result selection unchanged.

**Checkpoint:** engine tests prove deterministic state changes and existing run/active-retry tests still pass. If combined dependency validation requires broad model refactoring, stop and split that refactor before proceeding.

### Phase 2 — Durable request protocol and open-session lifecycle

Likely files:

- `internal/state/attempts.go`, `internal/state/run_files.go`, `internal/state/store.go`: reserved root protocol names, validation, version/capability compatibility, atomic per-file persistence.
- `internal/jobcontrol/retry_request.go` and adjacent `jobcontrol` files: extract shared request polling/response/idempotency mechanics only where responsibilities genuinely overlap; add admission-specific typed payloads and exact-run submit/lookup APIs. Do not generalize the retry domain selection into a generic opaque framework.
- `internal/projectrun/execute.go`, `internal/projectrun/lifecycle.go`: establish accepting/sealed/drained/finished lifecycle after setup, attach watcher to the engine, fence shutdown before summary finalization.
- A focused package/file may own canonical admission records and run revision; preserve one-way dependencies and `internal/archtest` boundaries.

Crash/race tests: request before commit; committed admission before response; response before engine event; duplicate same request; same ID/different payload; concurrent append; cancellation race; finish/end race; timeout and later lookup; malformed/newer request; coordinator interruption after accepted pending work; client/script exit after acceptance; ensure no response wait holds the state lock. Recovery must reconstruct every accepted job exactly once from durable run-owned records, and caller exit must not implicitly cancel the run.

**Checkpoint:** a protocol-level test proves accepted membership is canonical independent of response delivery and can be reconstructed after restart/unlock. Do not proceed based solely on happy-path CLI tests.

### Phase 3 — Manifest bootstrap, `projectrun` integration, and CLI

Likely files:

- CLI command paths (`cmd/rotari/run_command.go` or a new `start` command, plus a new `submit` command reusing definition parsing from `cmd/rotari/add.go`), CLI spec source (`cmd/rotari/cli_spec.go`), generated reference via its generator: start from a validated manifest and submit to the exact open run; do not preserve queue-mode `add` as the final behavior. Return accepted IDs and request ID.
- `internal/projectrun/execute.go`, `internal/projectrun/validate.go`, `internal/projectrun/artifacts.go`, `internal/run/summary.go`: prepare added jobs with the original run's context/environment, validate against fixed run options, record artifacts, include final accepted membership in summary.
- `internal/runview/run.go`, `internal/web/loader.go`, `internal/web/timeline.go`, `internal/resolve/resolve.go`: expose committed membership and distinguish accepted-not-started from running/scheduler-submitted states; avoid changing existing selector precedence.
- `internal/project/edit.go`: keep project queue revision semantics unchanged; implement separate run admission sequence as designed.

CLI conformance: manifest and empty-session `start`; `submit` with an open/no-open run; `wait` seals and waits; `start` while an earlier run is open or sealed-but-running; wrong/ended/interrupted/cancelling/sealed target; no hidden queue write; no second supervisor; job executes with run context rather than submitter shell context; admission is always new work even for filtered runs; summary and show include the accepted job; kill the submitter after acceptance and verify work continues; cancel the run ID and verify every accepted job is included; cancel one newly accepted pending job by returned ID while another continues; cancel an array job ID and verify all its unfinished tasks are selected; append/seal race rejection with no fallback.

**Checkpoint:** package tests, targeted CLI tests, then binary conformance and coordination tests. Ensure ordinary `add`, `run`, `retry`, `copy`, and `reset` regressions remain unchanged.

### Phase 4 — Web, MCP, Python, and complete read surfaces

Likely files:

- `internal/webui/webui.go`, `internal/webui/assets/web_app_actions.js`, relevant web app view/log/timeline assets: active-run add endpoint/form, exact run selection, preview/acceptance/unknown-timeout feedback, dynamic job counts.
- `internal/mcp/server.go`, `internal/mcp/write.go`: active-add preview/apply tools with exact run ID, revision, accepted IDs, request ID; preserve redaction and capability errors.
- `python/rotari/client.py`, Python object/API tests and generated docs/schema: run-bound addition method and structured result, or clearly documented explicit `add(run_id=...)`; retain CLI as behavior source of truth.
- `docs/MCP.md`, `docs/PYTHON_CLIENT.md`, generated `docs/CLI_REFERENCE.md`, `docs/RUNNING.md`, `docs/INSPECT.md`, `docs/FAQ.md`, `docs/ARCHITECTURE.md`, relevant contracts and conformance layout.

Tests: all interfaces target the same exact run and return equivalent accepted IDs; read-only/static Web rejects writes; auth/control behavior; MCP preview stale revision; Python argv/result; show/jobs/export/timeline/summary/wait agree on accepted membership; no uncommitted request is visible as a run job.

**Checkpoint:** parity across CLI, Web API/UI, MCP, and Python. Add contract IDs and conformance `covers` entries matching `contracts/README.md`.

### Phase 5 — Complete group support and retire queue compatibility

Only after plain-job admission, manifest bootstrap, and open/seal lifecycle work:

- support an entire new array/matrix group atomically, if required;
- continue to reject changes to groups already present in the run;
- test sparse/dense arrays, matrix×array, native scheduler arrays, selection, cancellation, aggregate status, export/import, and ID collisions.

Migrate legacy queue workflows to manifests, deprecate queue-backed `add`/`copy`/`change` paths, and remove their persistence/read paths only after the compatibility gates below pass. Define final `reset` behavior explicitly; a dedicated `run finish` / `seal` is preferred over silently reinterpreting `reset`.

## Acceptance criteria

- Active admission always names and revalidates one exact run; it cannot target another run after a race.
- `run` starts ready jobs immediately; an open run owns all pending work and accepts later additions until explicitly sealed.
- There is no project-owned next-run queue in the target state. Existing queue data is migrated or exported without loss and never silently executed.
- Once acceptance is durable, client/script lifetime does not control job lifetime. Run-level cancel still groups all accepted work under the session's run ID.
- Preserve existing whole-run and selected-job cancellation scopes. Do not add submission-group IDs just for cancellation: repeated `--job-id` selectors already cancel arbitrary subsets, provided `submit` returns the accepted job/task IDs consistently.
- Active addition never mutates `queue.json` and never starts a second coordinator.
- Each accepted request is atomic, idempotent, durably recoverable, and visible exactly once.
- New jobs use the original run's context/settings and shared dependency semantics.
- Running/finished job definitions, run-wide settings, existing dependencies, and existing array/matrix topology are not changed.
- Progress, results, summaries, inspection, and all supported interfaces agree on accepted membership.
- Relevant tests, contracts, docs, generated references, and `scripts/check.sh` pass before implementation is declared complete.

## Migration roadmap to retire the queue

The user clarified the product premise: the queue abstraction is less
intuitive than immediate submission, and manifest authoring is sufficient for
the uncommon pre-run copy/edit workflow. Queue retirement is therefore the
intended destination, not a hypothesis that must wait for a bounded-add
experiment. The remaining gates are about preserving behavior, defining the
run lifecycle, and migrating safely.

Do not ship a permanent two-target UX in which some `add` invocations stage
queue work and others submit to an active run. Implementation can be staged
internally, but queue compatibility is temporary and must have a removal
milestone.

### Gate 0 — Confirm replacement workflows, not queue popularity

Inventory how manifests cover current pre-run composition, preview, saved-run
copy, editing, import/export, and defaults. Identify unsupported cases, but
do not require proof that queue use is frequent before planning its
replacement. If a capability lacks a manifest equivalent, design that
equivalent or explicitly retire it before deleting queue behavior.

### Stage 1 — Decide the final run lifecycle

Choose how a new execution starts, how long it accepts additions, and what
closes admission before changing the queue contract:

- Prefer an explicit `run finish` / `run seal --run-id RUN` boundary for
  deterministic closure. Decide explicitly whether the user's familiar
  `reset` habit should become an alias, stay a queue-only legacy operation
  during migration, or be retired. Never silently reinterpret reset as
  canceling or deleting accepted work.
- Define `open`, `drained-but-accepting`, `sealed-but-running`, `finished`,
  `cancelling`, and `interrupted` states, including `run`, `wait`, `cancel`,
  `unlock`, and recovery behavior. Do not infer closure from an empty scheduler
  queue or inactivity unless an explicit timeout policy is separately chosen.
- Define the state when the process running a shell script exits before
  `wait`: accepted work continues under the supervisor and run ID, but the
  session may remain open. Specify how `show`/Web indicate this, how another
  client can seal it, and whether any explicit recovery command is needed.
- Keep `retry`'s successor-run semantics for finished runs unless evidence
  justifies changing it. Decide whether the shipped same-run active retry
  remains for transient failures; do not make retry an implicit seal operation.

**Gate:** a state-transition table and race contract covers append versus
seal, retry versus seal, cancel versus append, coordinator failure, and a run
that has no jobs yet. The user-visible run identity/history boundary is clear
in CLI, Web, MCP, and Python.

### Stage 2 — Replace queue staging before removing it

Identify what currently relies on a project-owned editable queue:

- composing multiple jobs, dependencies, arrays/matrices, and defaults before
  any work starts;
- whole-workflow validation and preview;
- saved-run `copy`, pre-run `change`, remove, import, and reset workflows;
- scripts that accumulate jobs with repeated `add` calls and then invoke
  `run` once.

Choose a visible pre-run workflow representation. The leading candidate is a
user-owned versioned workflow manifest that can be validated/previewed and
then passed to `run`; `copy` can export/clone a saved run into that manifest
and `change` can edit it before execution. This keeps draft editing without a
project-global queue. It is not acceptable to move `queue.json` unchanged to a
new hidden project file and call the system queue-less.

Specify whether a session starts from a manifest (`run WORKFLOW.yaml`) and
then accepts additional jobs, or whether an empty session is opened first and
initial jobs are submitted individually. Preserve whole-batch validation if
that is a required workflow; otherwise explicitly accept incremental partial
execution. Keep submitted-job definitions immutable: no active `change` is
introduced by this migration.

**Gate:** every current pre-run queue workflow has a named replacement or is
explicitly retired with user-visible migration instructions. Preview and
validation happen before execution for manifest-based starts.

### Stage 3 — Introduce the replacement with compatibility safeguards

Before changing defaults, provide an opt-in path and a migration/compatibility
period:

- Convert or export existing queued jobs, queue defaults, dependencies, and
  groups without starting them. Preserve IDs/provenance where the target
  format permits; report anything that cannot be represented.
- Make migration previewable and repeatable. Back up the original state and
  never silently discard or execute the old queue during conversion.
- Update CLI, Web, MCP, Python, workflow import/export, generated help, and
  examples together. Existing no-argument `add` behavior must not silently
  change from “stage” to “submit”; introduce an explicit mode/command first,
  then deprecate the old path with actionable errors and migration guidance.
- Keep mixed-version clients/supervisors from silently ignoring session or
  admission records. Add version/capability checks and a documented minimum
  compatible version.

**Gate:** migration fixtures cover non-empty queues, defaults, arrays,
matrices, dependencies, and project metadata; an interrupted migration can
resume without loss or duplicate execution. Legacy and new interface behavior
is explicitly tested during the transition.

### Stage 4 — Remove the queue as an execution workspace

Only after the replacement workflow is available and migration has shipped:

- stop writing and reading `queue.json` as an implicit next-run destination;
- route all accepted not-yet-started work through the exact run's canonical
  membership/admission records;
- remove queue-first selector branches, queue promotion/copy behavior, and
  queue-only revision assumptions only after contract and caller audits;
- retain compatibility export/import or a one-time migration command if
  needed, but ensure none acts as a hidden pending collection for execution;
- update contracts, conformance coverage, architecture maps, docs, Python
  schema/API, Web/MCP labels, examples, and static exports;
- remove old state only in a later, explicit cleanup after backups and
  migration support are no longer needed.

**Final gate:** a repository-wide search and conformance suite show no runtime
path that stages executable work outside a run. The only not-yet-started jobs
are durably owned by a named run; every run has an explicit admission boundary;
old state can be migrated without loss; all supported interfaces agree.

### Stop conditions

- If users still need to prepare a successor batch while another run is
  active, retain a separate user-owned manifest workflow. This preserves
  batch preparation without restoring a project-owned execution queue.
- If whole-workflow review is not replaceable, do not remove pre-run staging.
- If manifest import/export cannot preserve a required queue workflow, pause
  queue removal until the missing capability is designed; do not silently
  drop that workflow.
- Do not bundle queue retirement with changed-definition retries or unrelated
  project-selection work. Group expansion follows its own phase and tests.

## Related plans

- [Execution-model review](../2026-10-06-execution-model-review/plan.md)
- [Work during runs umbrella](../2026-10-05-work-during-runs/plan.md)
- [Project-selection audit](../2026-10-06-project-selection/plan.md)
