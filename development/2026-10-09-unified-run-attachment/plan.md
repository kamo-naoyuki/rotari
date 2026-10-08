# Plan: Unified Run and Wait Attachment

Created: 2026-10-09
Status: Planned; no runtime changes made by this plan

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

## Current implementation and gap

The synchronous client uses `Client.StreamRun` in
[client.go](../../internal/server/client.go) for pipe progress and control.
[serve.go](../../internal/server/serve.go) watches that connection and applies
detach/cancel policy. In contrast,
[wait.go](../../cmd/rotari/wait.go) follows the progress journal and invokes
`jobcontrol.Controller.Cancel` for explicit cancellation.

The commands already share the progress renderer and completion formatting,
but not a client-session lifecycle. `wait` already supports Ctrl-C cancellation
and Ctrl-D detach; this plan must preserve those behaviors, not add them as if
they were absent. Its attachment is not currently included in the persisted
initiating-client state introduced by the related visibility work.

The progress journal is currently best-effort: a write failure or a colliding
job ID can disable it. Replacing pipe progress makes that limitation affect
both commands and therefore requires an explicit policy.

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

## Open design decisions

1. Session storage/liveness mechanism, remote-host guarantees, and disconnect
   detection latency, including how suspended clients remain attached.
2. Multiple explicit attachments: recommended direction is to allow them,
   count all live sessions, and let an explicit Ctrl-C cancel the shared run.
   Implicit `wait` should skip runs with any valid attachment. Specify how
   concurrent implicit waiters select and register without accidental races.
3. Representation of current attachment versus launch/detach history in JSON
   and text, including compatibility with existing run state and uncertain
   remote sessions.
4. Startup acceptance/handoff ordering and handling a lost acknowledgement.
5. Progress-journal failure/collision policy once all clients depend on it.

## Implementation phases

1. **Specify and test sessions.** Resolve the open liveness and handoff
   decisions. Add failing tests for `async -> wait -> attached`, independent
   detach with multiple sessions, and implicit selection while another waiter
   is attached. Preserve existing terminal-control tests.
2. **Extract the shared attachment boundary.** Move follow/control lifecycle
   out of command-specific code; route `wait` through it. Add shared status
   projection and liveness handling with package-test checkpoints.
3. **Switch synchronous run/retry.** Keep startup pipes, acknowledge run ID,
   and enter the same attachment operation. Stop steady-state pipe streaming
   and pipe-based control after a safe handoff. Retain one supervisor per run,
   project exclusion, cwd/environment inheritance, and async startup errors.
4. **Remove obsolete paths and document contracts.** Remove duplicate
   stream/control state, align CLI/Web projections and generated schemas if
   affected, and update relevant contracts and guides. Do not preserve two
   runtime attachment implementations as a permanent compatibility layer.

## Validation

- Exercise sync `run` and `async -> wait` with equivalent inputs: progress,
  Ctrl-C job cancellation, Ctrl-D detach, unexpected disconnect policies,
  exit status, finalization, quiet, and JSON.
- Cover retry, multiple selected runs, multiple clients on one run, timeout,
  `--until-failure`, completed/interrupted runs, attach versus completion, and
  old run versus replacement run races.
- Use real subprocess/PTY tests for signals and terminal behavior, including
  SIGKILL, SIGHUP, SIGTERM, and suspension. Channel-injection tests alone are
  insufficient evidence for disconnect handling.
- Test refusal/early supervisor death, acknowledgement loss, handoff death,
  journal failure, metadata/job-name collisions, stale sessions, and supported
  remote/shared-filesystem cases without timing-only assertions.
- Cover plain jobs, array tasks/whole arrays, and matrix members where the
  cancellation/selection boundary is affected; use existing shared operations.
- Add binary/Web conformance for the changed attachment meaning, and update
  contract IDs/status mapping with implementation. Inspect Python/MCP clients
  for any changed public fields or startup response assumptions.
- Review `docs/ARCHITECTURE.md`, `docs/RUNNING.md`, `docs/INSPECT.md`,
  `docs/CLI_REFERENCE.md`, `docs/FAQ.md`, and relevant contracts when code lands.
- Run focused tests first, affected package tests and conformance next, then
  pre-commit on changed files, `scripts/check.sh --short`, and `scripts/check.sh`.

## Non-goals and status

- No removal of startup pipes, permanent daemon, new socket service, or
  automatic supervisor restart/recovery is required by this plan.
- Attachment does not recreate a job terminal or forward job stdin.
- No implementation, runtime contract change, or claim of passing runtime
  tests is made by this document. The parallel run-visibility work remains
  outside this planning change.
