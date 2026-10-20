# Plan: Verified Unlock of Interrupted Runs

Created: 2026-10-20
Status: Phases 1a, 1b, 2, and 3 implemented and validated. Phase 4 not started.

Phase 1a stores coordinator identity in the existing run context rather than
a separate run-directory filename, avoiding collisions with arbitrary job IDs.
`info` exposes the shared read-only assessment independently of the run lock.
The management completion marker is informational and never overrides process
evidence. Phase 1a left unlock policy and job-liveness checks unchanged.
CLI-21 documents its observational output; unlock contracts were unchanged then.
Validation: targeted tests, changed-file pre-commit, short checks, and the full
normal/race check passed. An initial unrelated cancellation timing failure and
its isolated passing repetitions are recorded in [ISSUES.md](../ISSUES.md).

Phase 1b selects `--assume-stopped` (`assume_stopped` in MCP/Python), not a
blanket force switch: unknown ownership needs explicit operator attestation,
while verified live ownership cannot be bypassed. The wording can also cover
unknown execution evidence in later phases without weakening positive-live
refusals. CLI `--dry-run`/`--json`, MCP preview/apply, and Python use the same
policy result; preview is read-only and apply reassesses under the state lock.
Refusals include codes and available host/namespace identity. Authorization
evidence is saved in the existing context rather than reserving another path.
This records attested recovery without calling it automatic verification.
Job termination is explicitly unchecked (`execution_checked: false`); summary
repair and executor probes remain later work. SAFE-4, COORD-3, SAFE-7,
SAFE-13, SAFE-14 and the automation migration guide describe this boundary.
Validation: targeted/package tests, changed-file pre-commit, short checks,
67 Python tests with the real binary, generated schema/reference checks, and
the isolated full normal/race check passed. The lockless-live regression
fails on pre-change `d1d69523`. Unrelated jj-read and positional-selector
intermittent failures are recorded in [ISSUES.md](../ISSUES.md); implementation
history and rationale are in [work-log.md](work-log.md).

Phase 2 repairs matching interrupted metadata to `finished` only after
validating the complete expanded command/result inventory, carried origins
and status marks, and all relevant attempts, including incomplete launches.
Preview/apply share `action` (`finalize` or `release`) and the repair reason
and execution inventory. A complete summary with started/uncertain work needs
explicit execution attestation; otherwise it refuses with `execution_unknown`,
not a silent release. Only established never-launched entries need no
execution attestation. Normal, cancel, timeout, SSH and scheduler wrapper
records establish outcomes, not managed-execution termination, so no
terminal-record-only fast path was added. Repair preserves summary/results,
commands, origins, run ID and finish time, cleans up applicable client status,
and never replays completion hooks. Partial cleanup can be retried.
`Finish` keeps removing its own lock on failure: durable ownership protects
live supervisors, while interrupted metadata remains explicitly repairable.
SAFE-15 and the operations guide record the boundary and wrapper audit.
Validation: the motivating unit test failed before the fix; plain and inherited
array binary regressions also fail on pre-change `435eea35`. Expanded-unit,
origin/attempt, cleanup-retry, ownership, CLI/MCP and flag-pair tests passed,
as did changed-file pre-commit, short checks, the final full normal/race check,
67 Python tests with the final binary, and generated schema/reference checks.
Execution probes remain Phase 3; ordinary release policy is tightened further
in Phase 4. Reasons and validation details are in [work-log.md](work-log.md).

Phase 3 adds executor-owned read-only local/scheduler probes shared by `info`
and finalization repair. New local attempts publish host/namespace before
launch, then PID/start time in the existing `job.json`; incomplete publication
stays unknown instead of losing the running process handle. Group existence
with matching leader identity is active; leaderless/reused/unverifiable live
groups are possibly live and unknown. Only the recorded host/namespace may
establish a missing group stopped. Escaped descendants remain outside scope.
Scheduler probes reuse backend parsers, validate native IDs, require terminal
accounting plus a successful non-active queue observation for stopped, batch
SGE's whole queue including pending jobs, and bound commands/output/rate waits.
All relevant attempts and carried origins remain in the inventory.
The reason for separating this evidence from terminal outcomes is unchanged:
a completed command does not establish that its managed execution stopped.
Positive activity blocks repair even with attestation; unknown needs explicit
attestation; all stopped units permit repair without execution attestation.
`execution_checked` describes a nonempty fully classified finalization
inventory, not permission or success. Ordinary release remains unchecked,
and SSH probing plus snapshot/probe/revalidation outside the state lock remain
Phase 4. SAFE-16 and the operations guide state these limits.
The motivating unit regression failed before implementation, and binary
conformance failed on pre-change `53f68884`. Local/scheduler evidence tables,
shared CLI/MCP policy, all unlock flag pairs, changed-file pre-commit, short
checks, and the final full normal/race check passed. All 67 Python tests
passed with the final binary; generated schema/reference checks passed.
Earlier full checks exposed intermittent unchanged copy-selector and retry
source-hint fixtures; three isolated repeats of each affected case passed,
and both observations are tracked in [ISSUES.md](../ISSUES.md).

## Purpose

Let rotari verify whether an interrupted run's jobs have stopped before
unlocking the project. Users should not have to reconstruct scheduler commands,
inspect process IDs manually, or confuse successful computation with incomplete
run finalization.

This is narrower than
[supervisor restart and run recovery](../2026-10-09-supervisor-restart/plan.md):
it does not restart monitoring, resume dispatch, or execute any job.

## Current behavior and motivating case

- [Project inspection](../../internal/project/inspect.go) derives interruption
  from project metadata and the absence of a live coordinator lock.
- [Run phase resolution](../../internal/project/run_phase.go) gives that
  interruption precedence over a valid run summary.
- The interrupted-run detail (SAFE-7) checks saved terminal status files.
  Missing or non-terminal records mean that jobs might still be running; it
  does not query their execution backends.
- `info` (CLI-21) already checks local unfinished attempts with
  [`LocalProcessGroupAlive`](../../internal/executor/local.go) when the run's
  host matches, and counts everything else as unknown. Unlock does not use it.
- [Unlock](../../internal/project/unlock.go) (SAFE-4) rejects a locally live
  coordinator, but removes a lock from another host or PID namespace on the
  operator's word (COORD-3), even when jobs might still be running. It leaves
  the queue and run history intact. Only the CLI and MCP expose it; the Web
  API and UI have no unlock, and the Python client wraps the CLI.
- [`Finish`](../../internal/projectrun/lifecycle.go) writes the summary, then
  `Finalize` requires the run lock to still name the run. When the lock is
  gone, `Finalize` fails and metadata stays `running`; `Finish` removes its
  own lock regardless, so the project reads as interrupted.

The motivating run, `20261016-020229-e87d4799`, has a successful summary, a
terminal wrapper record, and saved end-of-run load information, but project
metadata still says running and the execution lock is absent. Its cause is
recorded in [ISSUES.md](../ISSUES.md) and fixed by `fb93fccc`: a reader in
another PID namespace took the live lock as stale and removed it, so the
run's `Finalize` failed at the end. What remains is that rotari has no way
out of this state except an unverified `unlock`, which leaves the run
interrupted although its summary is complete.

## Scope and non-goals

In scope:

- Repair of a run whose summary is complete but whose finalization failed.
- Read-only, per-execution stop verification with explicit evidence.
- A coordinator identity record that survives the lock's removal.
- Verified unlock, shared by every interface that exposes unlock.
- A documented, explicit operator override for unverifiable executions.

Out of scope:

- Automatic unlock as a side effect of `run`, `retry`, or ordinary status reads.
- Killing jobs, restarting supervisors, submitting jobs, or resuming a run.
- Recovering lost outputs or inventing successful results from process absence.
- General exactly-once submission or safe takeover of an unreachable owner.
- Adding unlock to the Web UI; see Open decisions.

## Verification model

Use one shared assessment over the run's execution units, not an independent
rule in each interface. Separate these facts:

1. Coordinator ownership: stopped, live, or unverifiable.
2. Execution liveness: stopped, active (including queued/suspended), or unknown.
3. Recorded outcome: success, failure, cancellation, unfinished, or unreadable.

Stopping does not imply success. A saved successful summary alone does not
establish that every external execution has stopped.

Each assessment includes job/task and attempt identity, backend, evidence
source, observation time, and an actionable reason. Errors and conflicting
evidence are visible, not converted to "stopped."

### Coordinator identity

Today only the run lock records the coordinator's host, PID, and PID
namespace. Once a reader or `Finish` removes the lock, nothing says which
process owned the run, so its liveness cannot be checked: in the motivating
case the supervisor kept running after its lock was removed. Phase 1 adds a
record in the run directory, written when the run starts, holding the
coordinator's host, PID namespace, PID, and process start time, and marked
when the coordinator exits normally. A run without that record (older runs)
has unverifiable ownership. A missing lock is not evidence that its former
owner stopped. A normal-exit marker records completed management work, not
proof that the process has already exited; define the point after which it
cannot dispatch or finalize before trusting that marker.

### Execution evidence

| Execution | Existing state | Evidence and limits |
| --- | --- | --- |
| Scheduler | `job.json` holds the native job/task ID; [`ProbeSchedulerJob`](../../internal/executor/scheduler_probe.go) runs the queue and accounting queries but returns raw output. | Classify with the existing `scheduler_status` parsers, not in callers. Active queue state blocks unlock. A terminal accounting record or matching terminal wrapper result establishes termination; contradictory evidence remains unknown. Absence from the queue alone, expired accounting, and query failure do not prove termination. SGE's queue query lists all jobs, so batch per run. |
| Local | Only a `pid` file; no host, PID namespace, or process start time per attempt. The run's context host is known. | Prefer a matching terminal wrapper result. Otherwise check the process group only from the run's host and PID namespace, as `info` does for the host. Without a recorded process start time, PID-group reuse cannot be told apart, so a live group is "active or reused" (blocks) and a missing group is "stopped" only when host and namespace match. Unrecorded namespace identity is unknown, not an assumed match. A child that left the process group (for example with `setsid`) cannot be detected; document this limit. Phase 3 adds the start time and namespace to new attempts. |
| SSH | `job.json` holds the host, SSH options, and a remote token that cancel already uses to find the remote process. | Prefer a matching terminal result. Otherwise run a bounded remote query by token, like the cancel script, with a timeout. Death of the local SSH client proves nothing about the remote process. Records without a token remain unknown. |

Inventory executions from command snapshots and attempt/submission records,
not only directories already containing a command file. Cover plain jobs,
array tasks, matrix members, retries, and carried results. Carried results are
not new executions to probe. Include any earlier attempt that could still be
active. Missing directories, incomplete launch records, and the crash window
between external acceptance and saving the native identity must not produce
an empty inventory interpreted as safely stopped.

Use bounded queries, backend batching where available, and existing scheduler
rate limits. Do not require extra dependencies. Unsupported verification must
explicitly return unknown.

## Proposed user flow

1. A read-only assessment shows execution counts, evidence, and blockers.
   Extend `info`'s existing per-attempt check (CLI-21) with the shared
   assessment rather than adding a second classifier; select exact options
   after reviewing its output and the MCP preview (SAFE-7).
2. Normal `unlock` obtains a fresh assessment:
   - All managed executions stopped and coordinator safely excluded: recover.
   - Any active execution or live coordinator: refuse and identify it.
   - Any unknown/conflicting evidence: refuse and explain the missing or
     conflicting evidence and the explicit operator-override path.
3. An explicit override permits an operator to attest to stopping unverifiable
   executions. It must not bypass a positively live coordinator or execution.
   The option name and confirmation UX must be consistent across CLI, MCP,
   and the Python client.
4. Record recovery evidence and any override without modifying historical
   job outcomes. Keep credentials and unnecessary backend output out of it.

`--dry-run` performs the assessment but does not recover or persist it.
Machine-readable responses expose the same states and evidence as text.
Do not add expensive external probes to project lists, routine page
refreshes, or `jobs`/`show`; label earlier observations as stale.

## Ownership, races, and persistence

- Snapshot the selected run and execution identities under the state lock,
  then release it while performing potentially slow queries.
- Before applying recovery, reacquire the state lock and revalidate the
  selected run, metadata, run lock, coordinator record, and execution
  inventory. Refuse or reassess on change; never unlock a replacement run
  using an old assessment.
- A coordinator whose identity cannot be checked from here (another host or
  PID namespace, or no record) remains a blocker without explicit operator
  attestation; a heartbeat timeout is not sufficient proof.
- Prevent concurrent recoveries and normal starts from interleaving. An
  external actor can still requeue or manipulate scheduler jobs; document the
  boundary rather than promising an atomic transaction with the scheduler.
- Make recovery repeatable after partial writes. Do not consume the next queue,
  overwrite commands/origins, or resend completion notifications.

## Completed work with incomplete finalization

Expose this as a recovery explanation initially, rather than immediately
introducing a new public lifecycle enum.

Repair requires all of the following, not just terminal results among the
entries present in a summary:

- The summary is valid and names the selected run; project metadata still
  names that interrupted run as `running` or `cancelling`.
- Its results cover the command snapshot's expanded jobs, array tasks, and
  matrix members, with no missing or duplicate execution units and no invalid
  attempt associations. Validate carried results against their origins.
- Executed results, carried results, and blocked/cancelled jobs that never
  started are distinguished. Earlier attempts and incomplete launch records
  cannot be hidden by a terminal result for the latest attempt.
- Coordinator ownership is safely excluded, or the operator explicitly
  attests to unverifiable ownership. A positively live owner always blocks.
- Every started execution has sufficient termination evidence. Any unknown
  requires explicit operator attestation; positively active execution blocks.

Only then reconcile metadata with the summary through `state.FinalizeRun`
and complete applicable client-state cleanup. Preserve the original result,
run ID, and recorded finish time. Do not replay completion notifications or
other non-idempotent hooks.

Backend queries may be omitted only for execution paths whose persisted
evidence establishes termination. Phase 2 must audit and test this property:
[`execute.go`](../../internal/projectrun/execute.go) writes the summary after
the dispatch loop returns, but that alone does not prove backend termination.
In particular, inspect scheduler/SSH cancellation while external processes
are still ending, retries, and `FinalizePendingResults` for jobs that never
started. Document the evidence that permits each fast path. Paths without
that guarantee remain blocked until Phase 3 assessment or explicit operator
attestation; do not ship an unchecked summary-only fallback.

Without a complete trustworthy summary, unlock only releases the interrupted
project for future work. Preserve failed/unfinished results; never fabricate a
summary or execute missing jobs. Explain that retry is a separate action.

## Unknown evidence and compatibility

Normal unlock refuses unknown or conflicting evidence. An explicit override
records the operator's attestation and the unresolved evidence; it cannot
bypass positively live ownership or execution. One unknown execution blocks
recovery of the whole run unless explicitly overridden; no partial unlock
is introduced.

This tightens the existing operator-directed behavior (SAFE-7 and COORD-3).
Older runs without coordinator records and executions with insufficient
identity may need the override. Measure how often this occurs to prioritize
backend support and migration guidance, not to relax the default. Update MCP,
Python, and script examples with machine-readable refusal reasons and the
explicit attestation option. Do not silently infer consent in automation.

## Open decisions

1. **Override option and attestation format.** Choose a consistent explicit
   interface for CLI, MCP, and Python, including dry-run behavior.
2. **Whether the Web API exposes assessment.** It has none today; adding Web
   unlock remains outside this plan.
3. **Coordinator record format** and how it relates to the lock and
   `stale_lock_removed.json`.

## Decision rationale (2026-10-20 revision)

These decisions follow the plan review. They describe design reasoning, not
implementation or validation already completed.

### Establish coordinator identity before finalization repair

The motivating failure demonstrates that removal of the run lock can occur
while its supervisor is still running. Existing unlock checks use that lock;
when it is missing, those checks cannot exclude the original supervisor.
Therefore finalization repair cannot safely depend only on the existing
coordinator checks while postponing durable identity to a later phase.

Move coordinator identity and its shared policy to Phase 1, before repair.
Preserve an explicit operator-attestation path for older runs that cannot
gain trustworthy historical identity retroactively. This adds prerequisite
work, but avoids exposing an apparently verified repair that has not actually
excluded a live owner. An exit marker alone is not a substitute for defining
when the owner has irrevocably stopped management work.

### Validate summary coverage, not only its present entries

Every result present in a summary can be terminal while an expected job/task
is missing. The command snapshot and execution inventory therefore need to
be checked against the summary, including arrays, matrix members, attempts,
and carried origins. Otherwise an incomplete snapshot could be promoted to a
finished run.

Also, a terminal outcome is not automatically termination evidence: a
cancellation may have been recorded before the external execution stopped.
The dispatch loop returning is a useful place to investigate, not a proof
for all executor paths. Permit backend-query omission only after auditing
and testing the relevant path; retain unknown or explicit attestation
otherwise. This keeps the fast repair path without assuming the property
that the feature is supposed to verify.

### Refuse unknown evidence by default

The purpose is to remove the need for users to interpret uncertain liveness
themselves. A warning followed by ordinary unlock would preserve the most
important weakness of the existing flow. Query failures, missing identities,
and other-host checks provide no affirmative evidence of stopping, even if
they are common.

Require explicit attestation for unknown evidence and never allow it to
override positively live ownership or execution. Treat legacy compatibility
as a migration and interface-design concern, not evidence of safety.
Measurements of unknown rates should prioritize better verification support,
not silently change the default to permissive behavior.

### Keep repair separate from execution recovery and Web expansion

Successful work with incomplete finalization needs management-state repair,
not another training run or supervisor takeover. Preserve results, queue
contents, and original times, and avoid replaying notifications. A separate
supervisor-restart plan owns continued execution management.

The existing CLI/MCP unlock paths and CLI-wrapping Python client are enough
to deliver this capability initially. Adding Web controls would expand the
interface and testing scope without resolving the ownership and termination
questions. Web assessment remains an open extension; Web unlock is excluded.

## Contracts affected

- SAFE-4 and COORD-3: other-host/namespace recovery gains verification and an
  explicit override.
- SAFE-7: the warning becomes the shared assessment's evidence.
- CLI-21: `info`'s local check moves to the shared assessment.
- RES-7 and CORE-5 remain; new rules are needed for finalization repair and
  for the override. Give them IDs when their phase starts.

## Implementation phases

Each phase is a separate change with its own tests and documentation.

1. **Coordinator identity and policy foundation.** Define shared assessment
   types and recovery policy. Persist coordinator identity before execution,
   and assess it even when the run lock is absent. Test live, stopped,
   reused-PID, other-host/namespace, and legacy missing-record cases. Select
   the override interface and implement explicit attestation for unknown
   ownership across CLI, MCP, and Python before Phase 2 uses it.
2. **Guarded finalization repair.** Recover a run with a complete matching
   summary to `finished` instead of collecting, subject to the completeness,
   ownership, and termination requirements above. Reproduce the motivating
   state in a failing test first. Implement only proven evidence-based fast
   paths; other cases require explicit attestation or remain blocked pending
   Phase 3. Record whether `Finish` should keep removing its lock on failure.
   Add a contract rule and conformance row. Do not treat this phase as full
   executor verification.
3. **Read-only execution assessment.** Add executor-owned classified probes
   under the shared policy, starting with local and schedulers. Reuse
   `info`'s logic, add process start time and PID namespace to new local
   attempts, and wire `info` and MCP preview to the shared assessment.
4. **Full verified unlock.** Make `unlock` use fresh execution assessment,
   revalidate under the state lock, persist evidence and overrides, and add
   SSH token queries. Reuse Phase 1's attestation interface for unknown
   executions. Update CLI, MCP, and Python together; every intermediate
   phase must label unsupported checks rather than claim full verification.
5. **Documentation.** Each phase updates recovery guides (`docs/OPERATIONS.md`,
   `docs/FAQ.md`), CLI/API documentation, `docs/ARCHITECTURE.md`, contracts,
   and conformance mappings for what it changes; Phase 4 documents the
   migration for automation users.

## Validation and acceptance criteria

- Phase 1: removing the run lock does not hide a live coordinator; missing
  legacy identity requires explicit attestation and never implies death.
- Phase 2: the successful-summary/running-metadata state recovers to
  `finished` without execution, queue changes, result loss, or duplicate
  notifications; an incomplete or mismatched summary still recovers to
  collecting only after the same applicable recovery safety checks. Missing
  or duplicate results, invalid attempt/origin associations, and cancellation
  before external termination do not qualify for summary-based repair.
  Live ownership/execution refuses even with an override; unknown ownership
  or termination evidence requires explicit attestation.
- Table tests: terminal/active/unknown/conflicting evidence; query timeout,
  permission error, unavailable backend, missing identity, and old records.
- Inventory tests: plain/array/matrix, earlier retries, carried results,
  incomplete submission, and missing directories. Distinguish task IDs and
  outcomes in fixtures.
- Local tests: PID-group reuse with and without a recorded start time,
  namespace/host mismatch, live managed children, and terminal wrapper
  records. SSH without a remote token stays unknown.
- Race tests: run replacement, inventory mutation during queries, concurrent
  unlock/start, and failure between recovery writes.
- Conformance rows: CLI/MCP agreement, refusal for active executions, refusal
  for unknown evidence without explicit override, dry-run, and finalization
  repair. Check the Python client where those operations are exposed.
- Bug fixes require a failing regression before implementation. Run targeted
  package tests first, formatting/pre-commit on changed files, then
  `scripts/check.sh --short`, `go test ./conformance`, and `scripts/check.sh`.

Completion means users can understand why recovery is allowed or blocked and
recover supported stopped executions without manual backend inspection.
Unknown cases remain explicit and safe; no automatic rerun is introduced.
