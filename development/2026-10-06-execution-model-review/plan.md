# Plan: Reconsider the Queue and Run Execution Model

**Created:** 2026-10-06

**Status:** Exploration. No target model has been selected. Phase 3 project-resolution implementation and Phase 4 late additions are paused until this review decides whether the run-based model remains the right foundation. The Phase 2 same-run retry code is shipped; its longer-term role is part of this review, not a commitment to expand it.

## Question

Which model should rotari make primary?

1. **Run-based batches:** build a set of jobs, start a run that owns an immutable snapshot, and make later edits part of the next run.
2. **Immediate submission:** adding a job makes it eligible for execution now, rather than waiting for an explicit `run` over a prepared queue.

The current implementation is run-based. That is a fact about the shipped design, not a conclusion that it is the best user model. The review starts from user workflows and invariants rather than assuming Phase 1–4 are the desired final architecture.

## Candidate models

### A. Explicit run snapshots (current model)

- `add`/`change` edit the next-run queue.
- `run` snapshots the queue and starts one run for the project.
- A run has stable membership, dependencies, run context, and run-level settings.
- Queue edits during a run prepare the next run.
- Run-level retry and the shipped active retry add attempts within that run; the shipped active retry currently reuses the job definition captured by the run.
- Run completion closes a history unit; `wait`, `show`, cancellation, and recovery have a clear run ID.

### B. Immediate independent jobs

- Adding a job submits it immediately.
- Jobs are primarily independent execution records; a collection of jobs may not have one shared start/end time or run-wide configuration.
- Dependencies, arrays/matrices, whole-workflow previews, and all-or-nothing submission need additional grouping semantics.
- This is the simplest literal interpretation of “add means start now,” but may fit rotari's DAG and batch scheduler features poorly.

### C. Open execution session (streaming run)

- An explicit `run` starts an execution session, but its membership can grow while it is open.
- `add` to an active session, or an explicit `submit --run-id`, validates and appends jobs to that session; queue edits remain separately targeted at the next session.
- The session has an explicit accepting/sealed/finished lifecycle. It is not a continuously accepting queue unless the user opts into that mode.
- Run-wide environment, executor lanes, and concurrency are fixed at session start; job-level definitions can be supplied per appended job.
- The run snapshot becomes an append-only event/revision history while open, then a stable final snapshot when sealed.

This may retain useful run grouping while removing the “wait for the batch to finish before adding work to it” problem, but it does not remove the need to distinguish the next-run queue from the active session.

### D. Hybrid explicit targeting

- Keep the run snapshot and next-run queue model.
- Keep ordinary `add`/`change` targeting the next queue.
- Add an explicit operation for the active run, such as `add --run-id RUN` or `submit --run-id RUN`, and a narrowly explicit per-attempt revision request for failed jobs.
- All active-run changes are auditable and name the exact run; no operation changes target merely because a run happens to be active.

This is not a fourth execution engine so much as a compatibility path between A and C. It may preserve simple batch use while making in-progress corrections possible, at the cost of two visible workspaces and more explicit commands.

## Comparison criteria

Evaluate each model against concrete workflows, not just feature counts:

| Criterion | Questions to answer |
| --- | --- |
| User mental model | What does `add` promise: stage work or submit it? How does a user know where a change will go? |
| Fix-and-retry | Can a user correct a command or Slurm option after an early failure without waiting for unrelated slow jobs? Is a new run acceptable? |
| Dependencies | Can a new job depend on existing pending/running/final jobs? What happens if a prerequisite has already failed? |
| Arrays and matrices | Can a user add tasks or members after start? Are whole-group operations atomic? What defines membership and range? |
| Run settings | Which settings are fixed at start, and which can be job-specific? Can new jobs use different executor lanes without altering existing work? |
| Scheduling | How do concurrency limits, scheduler submit options, throttling, and native arrays apply to late work? |
| Preview and validation | Can rotari validate the whole intended workflow before any work starts, or are failures necessarily incremental? |
| History and provenance | What is a run/result unit? How are dynamically added jobs and revised attempts represented and compared? |
| Cancellation and recovery | What does cancel stop? What does unlock recover if a coordinator dies while accepting work? |
| Interfaces | Can CLI, Web, MCP, and Python expose the same target and state transitions without implicit routing? |
| Compatibility | Which current scripts depend on `add` not launching work, batch previews, or one run summary? |
| Complexity cost | What new state machine, persistence protocol, UI concepts, and contracts does each model require? |

## Required workflow scenarios

Use these scenarios to compare the models:

1. Queue ten independent jobs, inspect/preview them, then run all with a chosen concurrency and environment.
2. Start a slow run, add a forgotten independent job, and decide whether it should start now or belong to the next run.
3. A fast job fails due to a typo or bad per-job Slurm option while unrelated jobs remain slow; correct only that job and rerun it.
4. A job fails because an external service was transiently unavailable; rerun unchanged while the rest of the run continues.
5. Add a job that depends on a running job, a successful job, and a failed-final job; define readiness and blocking in each case.
6. Widen an array/matrix or add one member after some members have completed; define selection, aggregate status, and comparison semantics.
7. Kill the coordinator while work is running and while a late-add/retry request is being accepted; recover without losing, duplicating, or misattributing jobs.
8. Start two unrelated projects and operate on one with explicit IDs; ensure no model creates ambiguous defaults.

For every scenario record: desired user action, selected model behavior, persisted records, failure/race behavior, and interface burden.

## Decisions to make

- Is the primary abstraction a **run**, an **open execution session**, or an **individual submitted job**?
- Does `add` remain a queue-only operation, or may it submit work? If both meanings exist, what explicit syntax names the target?
- Is an active retry without definition changes valuable enough to keep? If retry may change one job's next attempt, is that an explicit attempt revision or a new run?
- Are run-wide executor settings intentionally immutable? The current preference is yes; determine whether job-specific executor/option overrides cover the real correction cases.
- Is there a need for a continuously accepting mode, or only a way to append work to a bounded active session?
- Which current Phase 3 and Phase 4 items survive under the selected model?

## Constraints and non-negotiable safety properties

- No two coordinators may execute one project concurrently unless a separate design explicitly replaces that invariant.
- Never silently accept an option while applying it to a different target than the user intended.
- A request must identify its target run/session and be revalidated atomically; after an end race, do not fall back to another run.
- Preserve job-level provenance: every attempt must expose the exact definition and settings it used.
- Queue-next and active-work edits, if both remain, need distinct explicit targets and stable preview/apply semantics.
- Apply shared selection, status, path-validation, and result-resolution rules once at their owning package boundaries.

## Deliverables

1. A workflow comparison table populated with a recommendation for A, B, C, or D.
2. A decision record for run/queue/session semantics, retry revisions, active additions, and whether the existing active retry should remain.
3. A revised umbrella plan that marks superseded Phase 2–4 work as retained, replaced, deferred, or removed; do not leave contradictory phase descriptions.
4. Only after the decision: a separate implementation plan for structural changes, with package boundaries, persistence format, migration/versioning, contracts, interface matrix, and tests.

No runtime or user-facing behavior changes are part of this exploration plan.

## Related plans

- [Original queue/active-run plan and implementation history](../2026-10-05-work-during-runs/plan.md)
- [Project-selection audit](../2026-10-06-project-selection/plan.md)
- [Resolution contracts](../../contracts/01-resolution-and-config.md)
- [Run lifecycle contracts](../../contracts/02-run-lifecycle-and-execution.md)
- [Coordination and safety contracts](../../contracts/04-coordination-and-safety.md)
- [Development planning conventions](../README.md)
