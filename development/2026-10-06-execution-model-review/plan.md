# Plan: Reconsider the Queue and Run Execution Model

**Created:** 2026-10-06

**Status:** Exploration. The leading hypothesis is now queue-less immediate execution: `run` starts jobs now and accepts additional submissions while its execution session is open; a later job may depend on an earlier submitted job. This is not yet the final decision. The initial-job submission contract and session close rule (explicit command versus time boundary) remain open. Phase 3 and Phase 4 implementation remain paused. The Phase 2 retry code is shipped, but its longer-term role is open.

## Question

Which model should rotari make primary?

1. **Run-based batches:** build a set of jobs, start a run that owns an immutable snapshot, and make later edits part of the next run.
2. **Immediate submission:** adding a job makes it eligible for execution now, rather than waiting for an explicit `run` over a prepared queue.
3. **Queue-less open run (current leading hypothesis):** `run` starts execution immediately; jobs submitted while its session remains open join that same run and become eligible as soon as dependencies permit. A late job may depend on an earlier submitted job. There is no separate next-run queue. The way the initial set of jobs enters `run` must be designed explicitly; silently retaining `add` as a staging queue would not satisfy the queue-less premise.

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

- An explicit `run` opens an execution session and immediately starts its ready jobs; membership can grow while the session is open.
- A job submitted while a session is open joins that session and starts as soon as its dependencies permit. A later job may depend on an earlier job by ID/name; if the prerequisite is still pending/running it waits, if successful it may start immediately, and if finally failed it is blocked under the existing DAG rule.
- There is no separate next-run queue. A submission without an open session must either open a new session explicitly or fail with instructions to start one; do not silently create an invisible staging queue.
- The session has an accepting/sealed/finished lifecycle. It is not a continuously accepting queue after it is sealed.
- Run-wide environment, executor lanes, and concurrency are fixed at session start; job-level definitions can be supplied per appended job.
- The run snapshot becomes an append-only event/revision history while open, then a stable final snapshot when sealed.

This is the current leading hypothesis: it keeps useful run grouping and the existing DAG model while eliminating the second workspace (next queue versus active run). It does require defining how the initial jobs are supplied and when the session stops accepting new work.

### Initial jobs when there is no queue

The queue-less model changes the current `add`-then-`run` workflow, so choose
one explicit bootstrap before implementation:

- `run` takes an initial workflow/manifest or command set and starts it;
	subsequent explicit submissions target that open run.
- `run` opens an empty session and `add --run-id RUN` submits each initial
	job. This makes `run` a session-opening command rather than a batch command.
- `add` without an open session creates/starts an execution session, while
	`run` becomes an explicit wait/seal operation. This is immediate, but changes
	the most fundamental meaning of `run` and needs strong compatibility review.

Do not claim the queue is removed if an implicit pending collection remains
somewhere under another name. If pre-run review and all-or-nothing validation
remain required, a versioned manifest passed to `run` may be clearer than a
hidden queue.

### D. Hybrid explicit targeting

- Keep the run snapshot and next-run queue model.
- Keep ordinary `add`/`change` targeting the next queue.
- Add an explicit operation for the active run, such as `add --run-id RUN` or `submit --run-id RUN`, and a narrowly explicit per-attempt revision request for failed jobs.
- All active-run changes are auditable and name the exact run; no operation changes target merely because a run happens to be active.

This is not a fourth execution engine so much as a compatibility path between A and C. It may preserve simple batch use while making in-progress corrections possible, at the cost of two visible workspaces and more explicit commands.

## Open-run boundary: when does accepting work stop?

An open run cannot finish merely because its current pending/running count
reaches zero: a user may be about to submit another dependent job. Compare:

1. **Explicit boundary command:** the session remains accepting until the
	user closes/rotates it. Candidate syntax includes `run finish` /
	`run seal --run-id RUN`, or reusing `reset` because users already reset
	before preparing a new work cycle. If `reset` is considered, define whether
	it seals the current session or seals it and opens a fresh one. It must not
	cancel accepted jobs, erase their history, or drop accepted work. This is a
	semantic change from today's `reset`, which only clears the next-run queue;
	the command name and existing contract need explicit review. A foreground
	`run` can wait for closure, and a detached session can be closed later.
	Explicit closure gives a deterministic boundary, at the cost of one
	lifecycle action and the possibility of a forgotten open run.
2. **Inactivity timeout:** stop accepting after a configured quiet interval.
	This can make one-shot work convenient but makes latency a dependency
	contract: a dependent arriving just after the deadline is rejected, and
	different workloads need different intervals. It must not silently infer
	completion from scheduler queue emptiness. If considered, require an
	explicit timeout value or clearly visible default and report the exact
	closing deadline.
3. **Hybrid:** explicit close/reset remains authoritative; an optional
	inactivity timeout is only a safety net, with a visible warning/countdown
	and an extension mechanism. This keeps deterministic explicit closure
	available but adds policy/UI complexity.

**Working recommendation:** prefer an explicit user-controlled boundary over
automatic timeout, because it avoids making scheduler timing determine whether
a dependency can be added. Reusing `reset` is worth evaluating against a
separate `finish`/`seal` action since it matches the user's existing cycle
habit, but it must be clear that reset closes admission rather than discarding
running/accepted jobs. Consider an opt-in idle timeout only if users
demonstrate that remembering to close a session is a real problem. Before
choosing, define what foreground `run` does
when current jobs drain but admission remains open, how `wait` distinguishes
drained-but-accepting from finished, what `cancel` does to admission, and what
`unlock` reports after a coordinator dies. Closure and submission must be
serialized: a request is either admitted to the named open run or rejected as
closed, never accepted after the summary is finalized.

### Existing commands that already signal a boundary

`reset` and `retry` are not neutral edits in the current user vocabulary:

- `reset` means discard the not-yet-run batch. In the current queue model it
  clears only the next-run queue, even while another run is active.
- `retry` normally means take a finished run's failed/unfinished work and
  create a successor run. The shipped active-retry path is an exception: it
  revises/reopens work inside the same run.

The queue-less model should decide whether these commands express lifecycle
boundaries rather than add a new boundary verb by default. Candidate semantics
to compare:

1. `reset` seals the current session (or explicitly rotates it to a new
   session) without canceling accepted work or erasing history. This matches
   the user's existing “reset between cycles” habit, but changes today's
   queue-only contract and must make “seal only” versus “seal and start next”
   unmistakable.
2. `retry` closes/admission-seals the source session and creates a successor
   run for selected failed/unfinished jobs after the source's accepted work
   settles. This preserves retry's usual new-run/history meaning, but cannot
   promise an immediate retry while unrelated jobs are still running unless
   the system allows concurrent sessions or queues the successor visibly.
3. `retry` remains an in-session attempt revision. This is faster for transient
   failures, but callers must understand that retry no longer creates a new
   run in this mode; an explicit modifier or separate command may be needed
   to distinguish it from successor-run retry.

For each option, define how the source run is sealed, what happens to later
submissions, when the retry attempt may start, how corrected job definitions
are recorded, and what run ID/history the user receives. Do not use `reset` or
`retry` as implicit session-boundary commands without updating their contract,
CLI help, Web/MCP labels, and Python behavior together.

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
3. A fast job fails due to a typo or bad per-job Slurm option while unrelated jobs remain slow; correct only that job and rerun it. In the open-run model, decide whether this is a revised attempt in the same session or a new job that depends on/duplicates the failed one.
4. A job fails because an external service was transiently unavailable; rerun unchanged while the rest of the run continues.
5. Add a job that depends on a running job, a successful job, and a failed-final job; define readiness and blocking in each case. DAGs are compatible with late submission: forward references to jobs not yet submitted are a separate question and may be rejected while backward dependencies remain supported.
6. Widen an array/matrix or add one member after some members have completed; define selection, aggregate status, and comparison semantics.
7. Kill the coordinator while work is running and while a late-add/retry request is being accepted; recover without losing, duplicating, or misattributing jobs.
8. Start two unrelated projects and operate on one with explicit IDs; ensure no model creates ambiguous defaults.

For every scenario record: desired user action, selected model behavior, persisted records, failure/race behavior, and interface burden.

## Decisions to make

- Is the primary abstraction a **run**, an **open execution session**, or an **individual submitted job**?
- If the queue-less open-run model is selected, which operation opens the session, and does `add` submit immediately only while a session is open?
- How is the open run sealed: explicit command, inactivity timeout, or a hybrid?
- Should `reset` be the explicit run boundary, and if so does it seal only or
  seal-and-open the next session? How does that coexist with the current
  queue-only reset contract and preserve accepted work/history?
- Should `retry` be a session/run boundary that starts a successor run, or an
  in-session re-execution? If both are needed, what explicit form distinguishes
  them and what happens to the source session's admission state?
- Are dependencies only allowed on already submitted jobs, or may a submitted job name a future prerequisite?
- If there is no next-run queue, how are users expected to stage/preview a large batch before execution? Is a manifest or `run` plan still the batch-start interface?
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

1. A workflow comparison table populated with a recommendation for A, B, C, or D, including the user's queue-less open-run hypothesis and the run-boundary choice.
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
