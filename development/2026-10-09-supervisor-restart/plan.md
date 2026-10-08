# Plan: Supervisor Restart and Run Recovery

Created: 2026-10-09
Status: Deferred; design outline only, implementation not started

## Purpose and scope

Allow an explicitly restarted supervisor to take over the same interrupted
run, reconcile its existing jobs, continue eligible work, and finalize it.
Restarting management is not retrying the run in a new run directory, and it
must not re-execute jobs merely because their old coordinator disappeared.

This is separate from the
[unified run/wait attachment plan](../2026-10-09-unified-run-attachment/plan.md).
That plan unifies clients following a run; this plan reconstructs execution
management. Startup pipes may remain in both designs. Neither plan requires a
permanent server supervising the supervisor.

Related work: [run visibility and disconnect behavior](../2026-10-08-run-visibility-and-disconnect/plan.md).

## Current baseline

- One supervisor manages a run; dispatch and executor monitoring happen in its
  goroutines, not in separate per-job rotari management processes.
- Local jobs use independent shell wrappers and can outlive a killed
  supervisor, recording results without it. Normal local monitoring still
  waits on the original child process.
- Scheduler executors persist native job IDs and poll scheduler/wrapper state.
- SSH execution currently depends on a local `ssh` child and an in-memory
  process registry; its wait path cannot reconstruct that registry after a
  supervisor restart.
- Commands and run context are saved, but this does not establish that all
  execution settings, intermediate results, retries, and dispatch decisions
  required for recovery are persisted.
- Existing recovery is operator-driven; `unlock` does not kill or reconcile
  surviving jobs, and there is no automatic supervisor restart.

Representative boundaries:
[run lifecycle](../../internal/projectrun/lifecycle.go),
[local execution](../../internal/executor/local.go),
[SSH execution](../../internal/executor/ssh.go), and
[scheduler monitoring](../../internal/executor/scheduler_shared.go).
The current behavior is documented in
[durability and coordination contracts](../../contracts/04-coordination-and-safety.md).
This plan does not change those contracts yet.

## Design direction

Prefer recoverable execution identities and durable results over adding an
unbounded chain of watchdog servers. Do not change the existing policy to kill
every job when the supervisor dies as part of this work.

Begin with explicit manual recovery and a defined host/executor support
matrix. Same-host recovery and limited executors are candidate first scope,
not a promise that every interrupted run is recoverable. Automatic restart,
cross-host takeover, and transparent client recovery remain separate decisions.

## Required implementation boundaries

### 1. Load an existing run without starting a new one

Introduce an explicit recovery entry point for a fixed run ID. It must not
call new-run creation in a way that consumes the current queue or replaces
commands, origins, run identity, or timestamps.

Audit saved data and persist what is missing:

- Effective execution plan, commands, executor options, concurrency limits,
  dependency rules, retry/timeout policies, and working context.
- Attempt identities, submission/launch state, native execution identifiers,
  completed and carried results, retry budgets, and cancellation state.
- Required environment semantics. Avoid silently substituting the recovery
  caller's cwd/config/environment; define how necessary environment data is
  retained without casually persisting credentials or other secrets.

Version the recovery state and define behavior for old or incomplete records.
Unsupported recovery must fail diagnostically rather than guess.

### 2. Acquire exclusive management ownership

Serialize recovery with existing project/run coordination. Two recovery
clients must not start two active managers, and an old supervisor must not
resume dispatch or finalization after a takeover.

- Verify the old owner's identity and death, not just a reused numeric PID.
- Reject takeover while the old owner is alive or cannot be safely excluded.
- Transfer the run lock/owner record without exposing the project as idle.
- Recheck the selected run and metadata under the lock; do not recover a
  replacement run or consume jobs added to the queue during interruption.
- Define owner generations/fencing if any form of uncertain-host takeover is
  later supported. A remote PID or expired heartbeat alone is not proof of
  death, and a generation field alone does not stop an old owner issuing
  external submissions.

### 3. Reattach executor monitoring to existing execution

| Executor | Recovery requirement |
| --- | --- |
| Scheduler | Load the saved native job/array identity; resume queue/accounting and wrapper-result checks without submitting again. Treat unavailable or expired scheduler records as uncertain, not automatically unstarted. |
| Local | Reconcile wrapper results and a verifiable execution identity on the original host. A new supervisor cannot ordinarily `Wait()` on the old supervisor's children; monitoring must not depend on that relationship. |
| SSH | Add remote execution identity and independently accessible status/results/control. Reconstruct monitoring with new SSH connections instead of relying on the original local child and in-memory registry. |

Reuse existing executor cancellation rules, including host ownership checks.
Define how logs, timeout enforcement, cancellation, and process-group identity
survive handover. Neither killing the local SSH client nor finding a numeric
PID is sufficient evidence about the remote command.

### 4. Handle the launch/submission crash window

Persist an attempt's launch intent before the external side effect, then
record and validate its execution identity after launch/acceptance. Consider
crashes at each boundary, including:

- Scheduler accepted a job before its native ID was saved.
- Local or remote command started before its identity was committed.
- Cancellation was requested but not yet acknowledged by the executor.
- An attempt finished before its result/retry decision was committed.

Where supported, correlate a unique launch token with the external execution
and reconcile that token after restart. Launch intent alone cannot prove that
submission did or did not happen. Without a safe reconciliation mechanism,
leave the attempt uncertain and require explicit operator resolution; do not
promise general exactly-once submission or blindly retry.

### 5. Reconcile before continuing dispatch

| Verified state | Action |
| --- | --- |
| Finished | Import the terminal result for the correct attempt. |
| Running/queued externally | Resume monitoring; account for it in concurrency limits. |
| Never launched | Dispatch when dependencies, policy, and capacity permit. |
| Unknown or conflicting | Hold affected work and diagnose; do not duplicate execution. |

Rebuild dependency readiness, active slots, completed results, retry budgets,
and cancellation decisions before releasing new work. Use the same execution
rules as a normal run, not a separate recovery scheduler. Do not count carried
results as new execution or revive cancelled work.

Make reconciliation repeatable if recovery itself crashes. Finalization must
also tolerate a saved summary with incomplete metadata/lock cleanup without
re-running jobs or duplicating final side effects. Define the treatment of
notifications and other irreversible completion effects explicitly.

### 6. Integrate client and status behavior

- Keep the same run ID throughout recovery; client attachment and management
  ownership remain different concepts.
- Distinguish interrupted, recovering, resumed, and recovery-blocked states
  in shared projections where needed. These are proposed semantics, not new
  runtime labels introduced by this document.
- Decide whether an already waiting client exits on supervisor death or can
  opt to wait through manual recovery, and how that wait is bounded.
- If attachments survive handover, validate their liveness and disconnect
  policy instead of replaying stale client history as a live connection.
- Do not silently make `wait`, `retry`, or `unlock` start recovery.

## Open decisions

1. First supported executors and hosts: scheduler-first versus local-first;
   behavior when one run mixes supported and unsupported executors.
2. Effective-plan/environment persistence, state versioning, and migration.
3. Execution identity and launch-token mechanisms for each executor, including
   process identity reuse and scheduler accounting expiry.
4. Operator interface for requesting recovery and resolving uncertain attempts;
   command names/options are not decided by this plan.
5. Whether independent verified work may continue while other attempts remain
   uncertain, or the initial implementation blocks the entire run.
6. Ownership proof/fencing requirements, completion side effects, and client
   waiting/attachment policy during interruption and recovery.

## Implementation phases when resumed

1. **Audit and specify.** Trace existing saved state and crash boundaries;
   resolve the initial support matrix and write executor recovery contracts.
2. **Persist recovery inputs.** Add versioned plan/attempt records and safe
   execution identities with focused tests before implementing takeover.
3. **Implement one executor's reconciliation.** Reattach without creating a
   new attempt; test both confirmed and uncertain cases.
4. **Recover the run engine.** Add exclusive manual takeover, reconstruct
   dispatch state, and share execution/finalization rules with normal runs.
5. **Expand deliberately.** Add other executors, mixed runs, client integration,
   public status projections, contracts, and operator documentation.

Each phase needs compilation/package-test checkpoints. Preserve existing
behavior until the new explicit recovery path is intentionally introduced.

## Validation and acceptance criteria

- Fault-inject before/after launch intent, external acceptance, identity write,
  result recording, retry decision, summary write, and lock release. Use
  synchronization markers, not timing-only crash tests.
- Kill the original supervisor while jobs remain active; verify recovery
  monitors the same executions, never duplicates them, and eventually produces
  a correct summary under the original run ID.
- Test concurrent recovery, live old owner, unverifiable remote owner, stale
  or reused PID, unsupported state version, incomplete records, and another
  crash during recovery.
- Cover plain jobs, array tasks/whole arrays, matrix members, dependencies,
  multiple failing attempts, retry budgets, carried results, cancellation,
  additions to the queue, mixed executors, and distinct native identities.
- Verify local/remote logs and results after handover, scheduler accounting
  lag/expiry, host mismatch, and inability to determine execution state.
- Recheck CLI, Web/API, MCP, and Python behavior wherever status or control
  contracts change. Add binary/Web conformance and update contract coverage
  IDs alongside the implementation; do not change current contracts now.
- Review architecture, running/recovery/inspection guides, FAQ, and durability
  contracts when implementation lands. Validate focused tests first, then
  package/conformance tests, formatting, `scripts/check.sh --short`, and
  `scripts/check.sh` including race detection.

## Non-goals and current status

- No automatic restart, extra permanent watchdog server, cross-host failover,
  forced adoption of uncertain jobs, or kill-on-supervisor-death policy.
- No transparent recovery from host loss or arbitrary network partitions.
- Implementation is postponed. This document records a separate future work
  item; it makes no runtime changes and claims no recovery tests have passed.
