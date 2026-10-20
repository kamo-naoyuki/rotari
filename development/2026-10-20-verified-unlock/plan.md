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
  API and UI have no unlock, and the Python client wraps the CLI. The CLI
  has no confirmation or dry run: running the command is the operator's
  word. MCP splits it into a preview and an apply guarded by `if_revision`.
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

Evidence precedence is shared across all interfaces: trustworthy positive-live
evidence for the selected execution or coordinator always blocks recovery,
even when a terminal record disagrees. Preserve and display the conflict,
but do not downgrade it to an overridable unknown. Only conflicts without
trustworthy positive-live evidence can use the unknown/attestation path.
Validate execution identity and observation freshness before applying this
rule; an unrelated reused PID or stale scheduler snapshot is not proof of
current activity. Live evidence whose identity cannot be validated, such as
a live process group without a recorded start time, is "possibly live": it
blocks normal unlock but is an overridable unknown, shown as possibly live,
not a non-overridable block.

### Coordinator identity

Today only the run lock records the coordinator's host, PID, and PID
namespace. Once a reader or `Finish` removes the lock, nothing says which
process owned the run, so its liveness cannot be checked: in the motivating
case the supervisor kept running after its lock was removed. Phase 1a adds a
record in the run directory, written when the run starts, holding the
coordinator's host, PID namespace, PID, and process start time, and marked
when the coordinator exits normally. A run without that record (older runs)
has unverifiable ownership. A missing lock is not evidence that its former
owner stopped. Reuse [`state.ProcessStart`](../../internal/state/lock.go) and
`state.PIDNamespace` for the identity, and the rule in
[`attachment/origin.go`](../../internal/attachment/origin.go) that a PID
without a recorded or readable start time may still be the same process;
`LockInfo` itself records no start time today. A normal-exit marker records
completed management work, not proof that the process has already exited;
define the point after which it cannot dispatch or finalize before trusting
that marker.

### Execution evidence

| Execution | Existing state | Evidence and limits |
| --- | --- | --- |
| Scheduler | `job.json` holds the native job/task ID; [`ProbeSchedulerJob`](../../internal/executor/scheduler_probe.go) runs the queue and accounting queries but returns raw output. | Classify with the existing `scheduler_status` parsers, not in callers. Trustworthy active queue state blocks unlock even if a terminal record disagrees. A terminal accounting record establishes termination within the scheduler's documented scope; a wrapper record alone does not prove the allocation stopped. Absence from the queue alone, expired accounting, and query failure do not prove termination. Process groups on compute nodes cannot be inspected from the submitting host, and without a timeout the wrapper does not lead its own group, so scheduler termination evidence comes from the scheduler only. SGE's queue query lists all jobs, so batch per run. |
| Local | Only a `pid` file; no host, PID namespace, or process start time per attempt. The run's context host is known. | A matching terminal wrapper result is outcome evidence, not proof that the process group is empty. Check the process group only from the run's host and PID namespace, as `info` does for the host. Without a recorded process start time, PID-group reuse cannot be told apart, so a live group is "possibly live" (blocks normal unlock, overridable); with a matching recorded start time it is positively live (blocks, not overridable). A missing group is "stopped" only when host and namespace match. Unrecorded namespace identity is unknown, not an assumed match. Children remaining in the group must be checked; children that left it (for example with `setsid`) cannot be detected by that check, a documented limit. Phase 3 adds the start time (`state.ProcessStart`) and namespace (`state.PIDNamespace`) to new attempts. |
| SSH | `job.json` holds the host, SSH options, and a remote token that cancel already uses to find the remote process. | A terminal result supplies outcome evidence; termination needs evidence covering the managed remote execution. Run a bounded remote query by token, like the cancel script, with a timeout, and audit whether it covers surviving children; the remote process group is defined only when the wrapper leads it (with a timeout), so state the query's scope per case. Death of the local SSH client proves nothing about the remote process. Records without a token remain unknown. |

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

A dry run performs the assessment but does not recover or persist it. The
CLI `unlock` has no dry run today (only `--basedir`, `--project-name`, and
`--run-id`), and neither has the CLI-wrapping Python client; only MCP has a
preview step (`rotari_preview_unlock`, then `rotari_unlock` with
`if_revision`).
Phase 1b adds a CLI `--dry-run` and its Python projection, matching the MCP
preview's states and evidence, together with the override.
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

Audit the wrapper's terminal status record first, but do not treat it as
proof that its process group or scheduler allocation has stopped. In the
normal path, the wrapper waits for its direct command; that command can exit
while background children remain even in the same process group. In the
cancel-grace and timeout-watchdog paths, the wrapper can write terminal status
before issuing the final kill. A terminal file can therefore be observable
while managed execution is still alive.

Document separate evidence requirements for normal completion, cancellation,
and timeout, including the wrapper, managed children, and any scheduler
allocation. A fast path must establish the required scope of termination,
not merely agreement between summary and wrapper exit codes. If current
records cannot establish it, require a backend/process-group check or explicit
attestation rather than assuming that a wrapper-only fast path exists.
Coordinator-recorded cancellations and results reconstructed after a lost
wrapper do not establish termination either. `FinalizePendingResults` entries
need no stop check only when the inventory establishes they never launched.

The motivating run predates the coordinator record, so its ownership is
unverifiable: Phase 2 repairs it only with explicit attestation of the
coordinator. Its terminal wrapper record establishes the recorded command
outcome, but any remaining uncertainty about managed execution termination
needs additional verification or explicit attestation as well.

Without a complete trustworthy summary, unlock only releases the interrupted
project for future work. Preserve failed/unfinished results; never fabricate a
summary or execute missing jobs. Explain that retry is a separate action.

## Unknown evidence and compatibility

Normal unlock refuses unknown or conflicting evidence (see the rationale
below). Conflicts containing trustworthy positive-live evidence are blocking,
not overridable unknowns; possibly-live evidence is an overridable unknown.
An explicit override records the operator's attestation and the unresolved
evidence; it cannot bypass positively live ownership or
execution. One unknown execution blocks recovery of the whole run unless
explicitly overridden; no partial unlock is introduced.

The default tightens once, in Phase 1b, for ownership: from then on a lock
from another host or PID namespace, or a run without a coordinator record,
needs the override. Execution evidence joins the same refusal in Phase 4.
Before Phase 1b, unlock behaves as today; Phase 1a changes no behavior.
Phase 1b therefore carries the SAFE-4/COORD-3 contract change, the
migration guide for automation users, and machine-readable refusal reasons
in CLI, MCP, and Python. Do not silently infer consent in automation.

Older runs without coordinator records and executions with insufficient
identity may need the override. Measure how often this occurs to prioritize
backend support and migration guidance, not to relax the default.

When ownership is unknown only because the coordinator ran on another host
or PID namespace, the refusal names that host and namespace and explains that
running `unlock` on the original host in the same PID namespace may allow
verification if the recorded identity is still inspectable. Moving to the
host alone is insufficient; an old container/namespace may no longer be
accessible. Do not recommend recreating a container as equivalent identity.
On shared file systems, unlocking from another login node is common, so give
this actionable guidance before the explicit-attestation alternative.

## Open decisions

1. **Override option and attestation format.** Choose a consistent explicit
   interface for CLI, MCP, and Python, including dry-run behavior; decide
   in Phase 1b.
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

### Tighten the default in one named phase

Requiring attestation for unknown ownership is a user-visible change to
SAFE-4 and COORD-3: a lock from another host, accepted today on the
operator's word, would be refused. If the plan only says that Phase 4
documents the migration, Phase 1 could ship that refusal without its
contract change, refusal reasons, or migration guide, and automation would
break with nothing to follow. Naming Phase 1b as the point where the
default tightens keeps the contract, documentation, and the three
interfaces in the same change as the behavior. Execution evidence tightens
again in Phase 4, which carries its own contract change.

### Add the CLI dry run with the override

The CLI `unlock` has no way to see what it would do: it has no dry run and
no confirmation, so today running it is the operator's attestation. Once
unknown evidence is refused and the override is required, a CLI user or
script must be able to read the refusal reasons and evidence before
choosing to attest; otherwise the only way to learn them is to attempt the
unlock. MCP already separates preview and apply, so the CLI catches up in
the same phase that introduces the override, and both report one result
rather than two classifications.

### Split Phase 1 into identity and override

Phase 1 combined assessment types, policy, persisted identity, and an
override across three interfaces. Persisting the coordinator record has to
start as early as possible, because only runs started after it ships can be
verified; it changes no behavior and is low risk. The override interface,
by contrast, is visible and hard to change once automation depends on it.
Splitting them lets the record ship first (1a) and gather runs that carry
it, while the override (1b) is designed with real refusal cases at hand.
The override is still designed for both ownership and execution evidence,
so Phase 4 reuses it rather than reshaping it.

### State that the motivating run needs attestation

The motivating run started before any coordinator record exists, so no
later phase can verify its ownership retroactively. Without saying so, a
Phase 2 acceptance test might expect that run to repair without the
override, and either fail or tempt a weaker ownership check to make it
pass. The plan therefore expects coordinator attestation, plus verification
or attestation for managed execution termination. Its wrapper record alone
does not remove the latter requirement.

### Audit the wrapper record, but not as termination proof

"Only proven fast paths" leaves Phase 2 without a starting point. The wrapper
record is execution-side evidence and is present in the motivating run, so
it is a useful starting point for an audit, not a promised fast path.

Reading [the wrapper implementation](../../internal/executor/wrapper.go)
shows why the stronger claim was removed: normal completion waits for the
direct command, not every process in its group. A background child can
outlive that command without leaving the group. The cancellation grace timer
and timeout watchdog also write terminal status before issuing their final
kill; a crash or delay between those operations leaves an observable terminal
record without proven termination. Even the wrapper's own terminal record
must therefore be evaluated by path and scope. Coordinator-recorded results
provide no stronger guarantee.

This distinction may defer automatic repair to the execution-assessment
phase. That is preferable to declaring safety based on exit-code agreement
while duplicate work can still overlap surviving processes.

### Do not turn positive-live conflicts into overridable unknowns

A wrapper may report finished while the matching scheduler job is still
running, or a matching local process group may still contain children. If
all contradictions become unknown, the override intended for missing evidence
could bypass known activity, contradicting the plan's live-execution refusal.
Keep trustworthy positive-live evidence blocking regardless of terminal
evidence, and display both observations. Identity and freshness checks avoid
confusing reused identifiers or historical observations with current activity.

### Treat unvalidated live evidence as overridable

Every local attempt recorded before Phase 3 lacks a process start time, so a
live process group under its PID may be the job or an unrelated process that
reused the ID. Treating that as trustworthy positive-live evidence would make
it non-overridable, and an unrelated long-lived process could then keep the
run unrecoverable indefinitely, with no action available to the operator.
Treating it as stopped would hide a job that may still be running. "Possibly
live" keeps normal unlock refusing and displays the observation, while the
explicit override remains available, consistent with the rule that
identity must be validated before evidence counts as positive-live. Once
the start time is recorded and matches, the evidence becomes positive-live
and stops being overridable.

### Say that Phase 2 repairs with attestation

The wrapper audit showed that its terminal record does not establish
termination, and Phase 2 has no backend or process-group assessment. So in
Phase 2 almost every repair needs attestation of execution termination as
well as of legacy ownership. Describing Phase 2 as if evidence-based fast
paths were expected would set an acceptance target it cannot meet, and could
invite weakening the evidence rules to meet it. Phase 2 still has its own
value: an attested repair restores the run to `finished` with its summary,
instead of the collecting state that unlock leaves today. Evidence-based fast
paths move to Phase 3, where the assessment that provides that evidence
exists.

### Scope process-group evidence to where it is defined

A process-group check is meaningful only where rotari knows the group and
can inspect it. The local executor starts the wrapper in its own group, but
a scheduler or SSH wrapper leads its own group only when a timeout is set,
and compute-node groups are not visible from the submitting host. Without
stating this, wrapper and children tests could be read as applying to every
executor and lead to probes that inspect an unrelated group. Scheduler
evidence therefore comes from the scheduler, SSH evidence from the token
query with a stated scope, and the process-group tests apply to local
attempts.

### Reuse existing process identity helpers

`state.ProcessStart`, `state.PIDNamespace`, and the attachment rule for a
missing start time already decide whether a PID is still the same process.
A second implementation for coordinators or local attempts would be the
kind of drift [AGENTS.md](../../AGENTS.md) warns about: one rule in several
places. The plan names them so that each phase extends them.

### Point refusals to the coordinator's host

Shared file systems make it ordinary to run rotari from a different login
node than the one that started the run. Ownership is then unknown only
because of where the check runs, not because evidence is missing. Naming
the host and PID namespace can make a refusal actionable, reducing operator
attestation where verification is possible. A PID only identifies a process
within its namespace: reaching the original host does not grant access to a
former container's namespace. Guidance must be conditional on access to that
same namespace and recorded identity, not promise that changing hosts solves
every remote-ownership refusal.

## Contracts affected

- Phase 1a: none.
- Phase 1b: SAFE-4 and COORD-3 (other-host/namespace and record-less runs
  need the override); new rules with IDs for the override and for the CLI
  dry run (SAFE-7's MCP preview gains the same result).
- Phase 2: a new rule with an ID for finalization repair; RES-7 and CORE-5
  remain.
- Phase 3: SAFE-7 (the warning becomes the shared assessment's evidence),
  CLI-21 (`info`'s local check moves to the shared assessment), and the
  Phase 2 repair rule (assessed termination replaces execution attestation).
- Phase 4: SAFE-4 again, for refusal on unknown execution evidence.

## Implementation phases

Each phase is a separate change with its own tests and documentation.

1. **Coordinator identity and policy foundation.**
   - **1a. Identity record.** Define shared assessment types and recovery
     policy. Persist coordinator identity before execution, and assess it
     even when the run lock is absent, reusing `state.ProcessStart` and
     `state.PIDNamespace`. Test live, stopped, reused-PID,
     other-host/namespace, and legacy missing-record cases. Expose the
     assessment read-only (for example in `info`); unlock behavior does not
     change.
   - **1b. Override and tightened default.** Select the override interface,
     designed for both ownership and execution evidence, and require it for
     unknown ownership across CLI, MCP, and Python. Refusals name the
     coordinator's host and namespace. Add the CLI `--dry-run` (new; the CLI
     has none today) and its Python projection, sharing the MCP preview's
     result. Update SAFE-4 and COORD-3, add the override and dry-run rules,
     and publish the migration guide for automation users in the same
     change.
2. **Attested finalization repair.** Recover a run with a complete matching
   summary to `finished` instead of collecting, subject to the completeness,
   ownership, and termination requirements above. Reproduce the motivating
   state in a failing test first. Expect execution termination to need
   explicit attestation in this phase: the wrapper audit separates
   direct-command outcome from managed-execution termination for normal,
   cancel, and timeout paths, and the terminal record alone does not
   establish termination. Only never-launched entries established by the
   inventory need no attestation. Evidence-based fast paths are added in
   Phase 3. Record whether `Finish` should keep removing its lock on
   failure. Add a contract rule and conformance row. Do not treat this phase
   as executor verification.
3. **Read-only execution assessment.** Add executor-owned classified probes
   under the shared policy, starting with local and schedulers. Reuse
   `info`'s logic, add process start time and PID namespace to new local
   attempts, and wire `info` and MCP preview to the shared assessment. Let
   finalization repair use assessed termination evidence, so a verified
   stopped run repairs without execution attestation.
4. **Full verified unlock.** Make `unlock` use fresh execution assessment,
   revalidate under the state lock, persist evidence and overrides, and add
   SSH token queries. Reuse Phase 1b's attestation interface for unknown
   executions. Update CLI, MCP, and Python together; every intermediate
   phase must label unsupported checks rather than claim full verification.
5. **Documentation.** Each phase updates recovery guides (`docs/OPERATIONS.md`,
   `docs/FAQ.md`), CLI/API documentation, `docs/ARCHITECTURE.md`, contracts,
   and conformance mappings for what it changes; Phase 1b documents the
   migration for automation users, and Phase 4 extends it to execution
   evidence.

## Validation and acceptance criteria

- Phase 1a: removing the run lock does not hide a live coordinator in the
  assessment; missing legacy identity is unknown and never implies death;
  unlock behavior is unchanged.
- Phase 1b: unknown ownership requires explicit attestation in CLI, MCP, and
  Python alike; the refusal names the coordinator's host and namespace;
  a live coordinator refuses even with the override. CLI `--dry-run`, the
  Python dry run, and the MCP preview report the same states and evidence
  and change nothing.
- Phase 2: the successful-summary/running-metadata state recovers to
  `finished` without execution, queue changes, result loss, or duplicate
  notifications; an incomplete or mismatched summary still recovers to
  collecting only after the same applicable recovery safety checks. Missing
  or duplicate results, invalid attempt/origin associations, and cancellation
  before external termination do not qualify for summary-based repair.
  Live ownership/execution refuses even with an override; unknown ownership
  or termination evidence requires explicit attestation. The motivating
  state without a coordinator record requires coordinator attestation and
  separate termination verification or attestation; a terminal wrapper record
  alone is insufficient. Never-launched results require inventory evidence,
  not fictitious process termination evidence.
  In Phase 2, executed jobs require execution attestation even with a
  terminal wrapper record.
- Phase 3: a run whose executions are assessed as stopped repairs without
  execution attestation.
- Wrapper tests (local attempts): normal command exit with a background child
  still in its process group; terminal status visible before final kill in
  cancellation and timeout paths; and children outside the managed process
  group. Scheduler and SSH tests check the scope their evidence states.
- Evidence precedence tests: matching wrapper-finished/scheduler-running and
  wrapper-finished/live-process-group (with matching start time) conflicts
  refuse even with override. A live group without a recorded start time is
  possibly live: refused without override, accepted with it. Stale
  observations or reused identities must not become positive-live proof.
- Refusal guidance tests: original host with a different or inaccessible PID
  namespace does not promise verification merely by moving to that host.
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
