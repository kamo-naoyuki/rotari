# Plan: Unified Run and Wait Attachment

Created: 2026-10-09
Status: Implemented for local CLI attachment flow; validation and multi-host
filesystem testing remain partial

## Purpose

Make synchronous `rotari run` / `retry` and `rotari wait` use the same
attachment lifecycle after a run has started. Waiting on a background run
must restore the same client-side control state as a synchronous run, rather
than being classified as observation only.

Keep the startup pipe. Removing it is not necessary for this unification.

Related work: [run visibility and disconnect behavior](../2026-10-08-run-visibility-and-disconnect/plan.md).
That plan's exclusion of `wait` from attached clients is superseded by the
direction here; its other status and recovery work remains separate.

## Agreed direction

- A pipe carries the initial request and acceptance or rejection response.
  Acceptance includes the resolved run ID. It is not the steady-state progress
  or control channel.
- Synchronous `run` starts a run and enters the shared attachment operation.
  `retry` uses the same path. `run --async` returns after acceptance without
  entering a steady-state attachment.
- `wait` resolves existing runs and enters that same attachment operation.
  It does not start or re-execute them.
- Both attached clients read persisted progress and finalized results, use the
  same cancellation boundary, and follow the same disconnect policy.
- A run is attached while at least one valid client attachment exists, whether
  that client entered through `run`, `retry`, or `wait`.
- Launch mode (`sync` / `async`) is history, not the current attachment state.
  An asynchronously launched run can subsequently be attached by `wait`.
- Web and read-only MCP consumers are not attachments merely because they read
  state. Starting a run asynchronously through an API does not attach it.

## Implemented architecture

- The startup pipe now carries the request and one explicit acceptance or
  rejection. `Accepted` includes the committed `RunID`; after acceptance, pipe
  close is normal and cannot detach or cancel execution. Sync requests reserve
  a session before launch and bind it to that run before the acceptance.
- `run` / `retry` and `wait` enter the same `followRunWithOutput` path. Initial
  runs read the journal from its beginning; waiters take the existing
  attach-time progress snapshot and skip older events. Both use the same
  renderer, cancellation operation, result/phase authority, and terminal
  controls. Async starts still return without a session.
- `internal/attachment` stores independent client records and process-held
  advisory locks in `.rotari-attachments/`. New run locks do not write the
  legacy `ClientAttached` boolean; aggregate current attachment is derived from
  sessions, with a fallback only for active runs created before the marker.
- The supervisor polls every 100 ms. A held lock means live even while the
  process is stopped. When the lock is free on the local host, Linux
  `/proc/PID/stat` start time is compared to avoid PID-reuse errors; if process
  identity is unavailable, liveness is conservative. Remote host identity
  cannot be verified and remains attached/unknown rather than being declared
  dead. Stale disconnect-cancel records are retained until cancellation
  succeeds (or the fixed run has already ended), so a transient cancellation
  error can be retried.
- Implicit waiters recheck and reserve candidates under the project state lock.
  Explicit waiters may join an existing session; releasing one client cannot
  erase another. Supplying `--project-name` is an explicit target and retains
  the attached warning; only a genuinely targetless `wait` silently reserves
  eligible detached candidates. Web and MCP readers remain observers, though shared run-view
  projections use current CLI attachment state.
- A bound session whose startup acknowledgement is lost is not retried. Client
  cleanup releases its process lock but preserves the bound record so the
  supervisor can apply the configured policy. Unbound reservations are removed
  on startup failure.

The progress journal remains best-effort and collision-tolerant. Both entry
points fall back to authoritative summary/phase polling if journal reads fail.

## Target responsibilities

| Responsibility | Owner / boundary |
| --- | --- |
| Start one supervisor, inherit cwd/environment, submit request, receive acceptance | Startup transport in `internal/server`, wired by the CLI |
| Execute jobs and finalize the run | Existing supervisor and `internal/projectrun` lifecycle |
| Register, validate, and release client sessions; derive attachment state | One shared attachment package, independent of CLI rendering |
| Read incremental progress and authoritative final state | Shared follow operation using existing state and run-phase helpers |
| Handle terminal input, signals, quiet/JSON output, and labels | CLI adapter used by both `run` and `wait` |
| Request cancellation of a specific run | Existing `internal/jobcontrol` boundary; no duplicated job-killing logic |
| Project attachment status into CLI and Web views | Shared run-view projection |

Choose the new package/API names during implementation. Keep dependencies
one-way: attachment and state logic must not import the CLI or its renderers.
The shared operation is an internal API, not an invocation of `cmdWait` or a
recursive CLI call.

## Lifecycle and race requirements

### Startup handoff

1. The client starts the supervisor and submits the request through the pipe.
2. The supervisor validates and begins exactly one run under existing locks.
3. It acknowledges the committed start with a run ID, or returns a start error.
4. For synchronous execution, responsibility for client liveness transfers to
   the common attachment mechanism. For asynchronous execution, no attachment
   remains.
5. Closing the startup pipe after acceptance is normal protocol completion,
   not an unexpected disconnect or an implicit cancellation.

Define the acceptance point and handoff atomically enough that a client dying
between acceptance and attachment registration cannot leave a permanent
attached record or silently bypass a configured cancel-on-disconnect policy.
An initial session identity may be reserved before the request and transferred
at acceptance. Do not change the public sync request into an async request
without separately preserving launch history and this handoff policy.

No automatic retry of an ambiguously accepted start: a lost acknowledgement
must not cause duplicate execution. Diagnose the uncertain outcome using the
known request/session identity and persisted run state.

### Attachment sessions

- Use per-client identities, not a single boolean overwritten by each waiter.
  Detaching one client must not detach another live client.
- Derive aggregate `attached` from valid sessions through one implementation;
  do not maintain an independently authoritative attachment count or a second
  competing boolean in the run lock.
- Register and release against a fixed run ID. Completion or replacement of a
  project's active run must never attach to or cancel the replacement run.
- Explicit detach, timeout, and `--until-failure` release only the caller's
  sessions and leave execution running.
- Ctrl-C requests cancellation of every still-active selected run, returns
  130, and releases the caller's sessions. Ctrl-D releases them without cancel.
- Unexpected disconnect defaults to detach; `--disconnect-action cancel`
  applies to both entry points. A suspended client is not a dead client.
- Finished runs return their saved results without creating a live attachment.
  Supervisor death remains interruption, not client detach or successful
  completion.

Liveness must be observable without relying on the dying client's cleanup.
Choose and test process-held locks, process identity checks, or renewable
leases before implementing storage. Account for PID reuse, SIGKILL, suspension,
remote clients, and the supported shared filesystems. A remote PID is not
locally verifiable; an uncertain session must not be treated as proved dead.

For cancel-on-disconnect, a surviving coordinator must detect lost sessions;
client-side signal handlers alone cannot provide that guarantee for SIGKILL.
Detection latency and uncertainty must be specified rather than claiming the
immediate EOF guarantee of the old steady-state pipe.

### Progress and results

- Use the existing finalized summary and run-phase checks as completion
  authority. Missing progress or a lost session is not proof of completion.
- Share the follower implementation while retaining appropriate entry cursors:
  initial `run` must not miss events produced before startup acceptance;
  `wait` keeps its attach snapshot and skips preceding event history.
- Drain final events before printing completion. Preserve quiet, JSON,
  diagnosis, exit-code, multi-run labeling, and output ordering contracts.
- Decide whether journal failure retains documented result-only fallback or
  becomes a start-time requirement. Preserve arbitrary valid job IDs; put new
  metadata in a collision-safe namespace instead of reserving new job names
  casually. Per-event `fsync` is not automatically required.

## Resolved design decisions and limits

1. Session liveness uses per-client `flock` plus host, PID, and Linux process
  start-time identity, not a heartbeat. The supervisor's scan interval is
  100 ms; actual detection also includes filesystem and job-control latency.
  A stopped process keeps its lock. Remote clients and malformed/unreadable
  session state are uncertainty, never proof of death. Shared/network
  filesystems with reliable cross-host advisory locks may give stronger
  behavior, but multi-host lock semantics have not been integration-tested.
2. Multiple explicit attachments are allowed. The current set is derived from
  sessions, with no count. Ctrl-D, timeout, and early-failure return release
  only their own session; Ctrl-C cancels the fixed selected run IDs. Implicit
  selection uses an under-lock recheck-and-reserve so concurrent waiters do
  not both claim the same unattached run.
3. The initiating client status remains launch/detach history. Active aggregate
  attachment comes from sessions; the run-lock `client_attached` field is no
  longer written by new runs and is read only for pre-marker compatibility.
  CLI and Web use the shared run-view projection. Unknown remote liveness is
  not shown as proved detached.
4. A sync client reserves identity before launching; the supervisor binds it
  before acceptance and returns the accepted run ID. Closing the pipe is not
  a disconnect. A lost/ambiguous acceptance is not retried; a bound stale
  reservation remains visible to the supervisor.
5. Progress remains best-effort. Valid job IDs are not reserved; a collision
  disables the journal, and readers retain result-only fallback. The journal
  carries the start event's run ID so the startup acceptance can identify the
  accepted run without making the stream itself the control channel.

Stale session records from a killed supervisor may remain with interrupted run
history; no session garbage collector is introduced here. A remote client
whose host cannot be probed may remain conservatively attached indefinitely.

## Implementation phases

1. **Specify and test sessions.** Complete: added session independence, async
  run followed by wait, implicit skip/reservation, suspension, SIGKILL, PID
  identity, and remote-uncertainty tests.
2. **Extract the shared attachment boundary.** Complete for session storage,
  aggregation, projection, and the shared CLI follow/control path. The
  progress-rendering adapter remains in `cmd/rotari`; internal attachment
  logic imports no CLI renderer.
3. **Switch synchronous run/retry.** Complete: startup-only pipe acceptance,
  pre-reserved session handoff, shared follower, fixed run-ID cancellation,
  and supervisor-side disconnect monitoring. Async startup remains
  one-response and does not attach.
4. **Remove obsolete paths and document contracts.** Complete for steady-state
  stream/control paths (`StreamRun`, server-side detach/cancel bytes and
  connection watchers, and the legacy status-update adapter were removed).
  The Ctrl-D byte remains only as terminal input recognition in the CLI.
  Architecture, running/inspection/FAQ guides, and CLI-19 contract notes were
  updated; no generated public schema changed.

## Validation

- Exercise sync `run` and `async -> wait` with equivalent inputs: progress,
  Ctrl-C job cancellation, Ctrl-D detach, unexpected disconnect policies,
  exit status, finalization, quiet, and JSON.
- Cover retry, multiple selected runs, multiple clients on one run, timeout,
  `--until-failure`, completed/interrupted runs, attach versus completion, and
  old run versus replacement run races.
- Use real subprocess/PTY tests for signals and terminal behavior, including
  SIGKILL, SIGHUP, SIGTERM, and suspension. Channel-injection tests alone are
  insufficient evidence for disconnect handling. Subprocess tests cover
  SIGKILL and suspension; a full cross-platform PTY and multi-host matrix is
  still outstanding.
- Test refusal/early supervisor death, acknowledgement loss, handoff death,
  journal failure, metadata/job-name collisions, stale sessions, and supported
  remote/shared-filesystem cases without timing-only assertions.
- Cover plain jobs, array tasks/whole arrays, and matrix members where the
  cancellation/selection boundary is affected; use existing shared operations.
- Add binary/Web conformance for the changed attachment meaning, and update
  contract IDs/status mapping with implementation. Inspect Python/MCP clients
  for any changed public fields or startup response assumptions.
- Reviewed/updated `docs/ARCHITECTURE.md`, `docs/RUNNING.md`,
  `docs/INSPECT.md`, `docs/FAQ.md`, and contracts. `docs/CLI_REFERENCE.md` and
  generated schemas needed no changes because no public option or response
  schema was added.
- Validation run during implementation: focused attachment, server,
  supervisor, projectrun, runview, and CLI package tests; doclinks/archtest;
  and full conformance passed. Review then found three issues, each fixed with
  a regression test shown to fail first: explicit `Session.Close` released its
  lock before removing its record (a scan could cancel an explicit detach),
  initiator status updates bypassed the project state lock, and a `wait`
  session wrote the session-only marker onto pre-marker runs, hiding a legacy
  sync client's lock-recorded attachment. Pre-commit on changed files,
  `scripts/check.sh --short`, and `scripts/check.sh` (with race detection)
  passed; the final full check was rerun after the last fix.

## Non-goals and remaining validation

- No removal of startup pipes, permanent daemon, new socket service, or
  automatic supervisor restart/recovery is required by this plan.
- Attachment does not recreate a job terminal or forward job stdin.
- Startup pipes, the one-supervisor-per-run model, and the existing
  run-visibility schema are retained. No supervisor restart/recovery or remote
  liveness service was added. Do not infer successful remote disconnect
  cancellation from same-host tests.
- This work does not include the separate supervisor-restart project. Its code
  and plan must remain untouched.
