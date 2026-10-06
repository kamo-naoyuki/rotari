# Plan: Reconsider the Queue and Run Execution Model

**Created:** 2026-10-06

**Status:** Product direction is reconsidered; do not implement queue removal, the `start` / `submit` / `wait` replacement, or further in-run retry changes yet. The user identified that immediate submission may weaken the run abstraction and make retry confusing, and now questions the value of retrying a finished job while unrelated jobs are still running. Keep the current run/queue model as the default for now. Same-run active retry has already shipped; removing or deprecating it is a separate compatibility decision, not part of this pause. The alternatives and prior proposal remain documented for reference, but are no longer a preferred direction.

## Question

Which model should rotari make primary?

1. **Run-based batches:** build a set of jobs, start a run that owns an immutable snapshot, and make later edits part of the next run.
2. **Immediate submission:** adding a job makes it eligible for execution now, rather than waiting for an explicit `run` over a prepared queue.
3. **Run-centric open execution session (paused alternative):** `start [MANIFEST]` opens a run and starts ready work immediately; `submit` adds jobs to that run while it remains open; `wait` closes admission and waits for accepted work. A late job may depend on an earlier submitted job. The run itself owns a **pending / not-yet-started** state, rather than using a separate next-run queue. Initial jobs are authored/reviewed in a manifest or submitted after opening an empty run. This is not currently selected because it risks weakening the user's run/retry mental model.

The current implementation is run-based. That is a fact about the shipped design, not a conclusion that it is the best user model. The review starts from user workflows and invariants rather than assuming Phase 1–4 are the desired final architecture.

The current shipped behavior remains the baseline during this review. In
particular, same-run active retry is already implemented; “it may be
unnecessary” is a reason to evaluate its user value, not permission to remove
it without an explicit compatibility decision.

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

### C. Open execution session (streaming run with run-owned pending work)

- An explicit `run` opens an execution session and immediately starts its ready jobs; membership can grow while the session is open.
- A job submitted while a session is open joins that session and starts as soon as its dependencies permit. A later job may depend on an earlier job by ID/name; if the prerequisite is still pending/running it waits, if successful it may start immediately, and if finally failed it is blocked under the existing DAG rule.
- There is no separate next-run queue. A submission without an open session must either open a new session explicitly or fail with instructions to start one; do not silently create an invisible staging queue. Pending work is explicit state owned by a particular run, not another destination alongside the active run.
- Each accepted job has a clear lifecycle such as `pending` (definition accepted, no attempt started), `running` (an attempt has started), and a terminal result. `pending` is the run's not-yet-started work; it must not be confused with the existing next-run queue or with a scheduler-submitted-but-not-yet-running attempt unless the UI labels those states separately.
- `copy --run-id SOURCE` could add selected cloned jobs to a named open target run as `pending`. It must specify whether copied successful results are carried or deliberately reset so they execute again, and how job IDs/dependencies are remapped or collision-checked when source and target belong to the same run history.
- No submitted-job editing is assumed necessary in the current proposal. Keep `change` as a pre-execution draft operation unless a concrete workflow justifies a separate active-run mutation feature. If later required, never mutate a running attempt; preserve the exact definition/result of completed attempts and define an explicit next-attempt revision separately from ordinary `change`.
- The session has an accepting/sealed/finished lifecycle. It is not a continuously accepting queue after it is sealed.
- Run-wide environment, executor lanes, and concurrency are fixed at session start; job-level definitions can be supplied per appended job.
- The run snapshot becomes an append-only event/revision history while open, then a stable final snapshot when sealed.

This was previously preferred because the queue structure seemed less intuitive and manifests appeared sufficient for pre-run editing. That preference is paused: the user now questions whether immediate submissions make run membership and retry less understandable. Do not treat manifest bootstrap or queue migration as implementation work until the run/retry trade-off is resolved.

### Existing capabilities at risk under the open-run model

The model is not a pure improvement unless these current workflows are either
deliberately dropped or replaced. The key trade-off is that a run-owned pending
state handles work for the **current** open run; it does not automatically
provide a place to prepare a **different next run** while that run is active.

| Current capability | What the open-run model changes or can lose | What would preserve it |
| --- | --- | --- |
| Prepare a next batch while the current run is active (`add`/`change` edit the next queue) | If all pending work must belong to the sole active run, users cannot independently stage a successor batch while current work is running. They must wait until the run is sealed/finished, or risk adding work to the wrong lifecycle. | Permit a clearly separate successor draft (manifest or named session), or explicitly accept losing concurrent next-batch preparation. A draft is still a second workspace, even if it is not called a queue. |
| Review and validate a whole batch before any execution | Immediate admission can start ready jobs before the user has submitted or reviewed the rest; errors in a later submission cannot roll back already-started jobs. | Keep a manifest/plan preview with whole-batch validation and an explicit start/apply step. Otherwise document that validation is incremental and partial execution is possible. |
| Choose run-wide context and scheduling settings before start | Late submissions join a run whose working context, environment, concurrency, executor lanes, and run-wide scheduler options were fixed at opening. A late job may be unable to use a different run-level configuration. | Keep run settings immutable and require job-level overrides where supported; if a distinct environment/lane is needed, start a separate session after sealing or support multiple explicit sessions without violating project coordination. |
| Stable run membership and a single complete run snapshot | Membership becomes time-dependent and append-only while accepting work. A run may be drained but still open, so “all jobs done” no longer means “run complete.” | Persist admission events/revisions, distinguish drained/open from sealed/finished in every interface, and define one final snapshot boundary. |
| Copy a saved run, edit it, then selectively retry while carrying other results | Copying into an active run changes that run's membership and must resolve source/target ID collisions, dependency references, and whether successful results are carried or rerun. A saved run is no longer simply a template for a clean successor. | Make copy's target run explicit; define new IDs/origin links, result carry rules, and whether a successor can be prepared before the current run ends. |
| Arrays/matrices have known membership for aggregate selection and atomic retry | Appending tasks or matrix members after some members start/finish changes aggregate status, “whole array” behavior, selection scope, and comparison against earlier snapshots. | Either freeze group topology once any member starts, or version membership and specify atomic admission, selectors, aggregation, export/import, and retries for each version. |
| `retry` creates a successor run with carried-forward successes and distinct history | Same-session retries blur the boundary between a completed result and the current run's final result, even when the job definition is unchanged. | Preserve successor-run retry for stable comparison, or define an explicit in-session retry generation with clear historical views. |
| `run`, `wait`, and scripts observe a finite operation | If the session remains open after jobs drain, a foreground `run`/`wait` may wait indefinitely, or return while accepting work continues; scripts need an explicit close protocol. | Make seal/finish a first-class, deterministic operation and define foreground, detached, cancellation, and recovery behavior around it. |

These are possible losses, not inevitable ones: manifests, explicit successor
sessions, immutable attempt records, and versioned group membership can retain
many capabilities. But preserving next-run preparation as a separately
editable draft means there are still two workspaces conceptually; the design
should state whether that is acceptable rather than hiding it behind a new
name. Current queue editing, preview, and retry behavior is described in
[RUNNING.md](../../docs/RUNNING.md) and [RECOVERING.md](../../docs/RECOVERING.md);
contract changes would also need updates to the corresponding run lifecycle
and selector contracts.

### Potential gains of run-owned pending work

The proposal's value is not “pending disappears”; it is that pending work and
the execution/history unit that will own it become the same target.

- **One in-project mutation target:** while a run is open, `copy` and late
  `add` operations target that explicitly identified run,
  rather than silently editing a separate next-run queue. This removes the
  need to explain which of two mutable project workspaces an operation affects.
- **Late work can join the actual workflow:** a newly submitted job can depend
  on earlier pending, running, or successful jobs and become runnable as soon
  as its dependencies permit. It need not wait for the current run to finish
  just because its definition arrived later.
- **No queue promotion/copy step for current-run additions:** if work belongs
  to the active workflow, it can be submitted there directly instead of being
  staged for a later run and then reconciled or copied. The distinction
  “current run” versus “successor work” still needs to be explicit.
- **A coherent live workflow view:** pending, running, finished, and blocked
  jobs can be shown together with one run ID and one DAG, including who is
  waiting on whom. This can make incremental workflows easier to inspect than
  showing active-run state beside an unrelated next queue.

These gains apply only when new work belongs to the currently open workflow.
They do not make next-run preparation unnecessary, make whole-batch validation
possible after execution has started, or remove the need for a run boundary.
The user's preference is to remove the separate queue because the immediate
execution model is easier to understand; manifests cover pre-run authoring.
Validation should now focus on whether the manifest and lifecycle replacement
preserves required workflows, not on whether to keep queue and active-run
targets as permanent parallel modes. Active-job editing is not counted as a
gain unless the user later identifies a concrete need for it.

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

Be precise about what “remove the queue” means: this candidate removes the
separate **next-run staging workspace**, not the pending state every scheduler
needs before an attempt starts. If pre-run review and all-or-nothing validation
remain required, a versioned manifest passed to `run` may be clearer than a
second mutable workspace.

### D. Hybrid explicit targeting (migration-only fallback)

- Keep the run snapshot and next-run queue model.
- Keep ordinary `add`/`change` targeting the next queue.
- Add an explicit operation for the active run, such as `add --run-id RUN` or `submit --run-id RUN`. Do not add active-run `change` or revised-definition retry without a demonstrated use case.
- All active-run changes are auditable and name the exact run; no operation changes target merely because a run happens to be active.

This is not the preferred product model. It may be needed temporarily to migrate users, but should not become a permanent two-workspace design if the manifest/open-run model proves adequate.

### What happens to `copy` and `change`?

The preferred direction removes the separate next-run queue. The user reports
that queue `copy` / `change` are uncommon and a manifest is sufficient for
pre-run editing and reuse. The plan should therefore preserve those needed
capabilities through workflow manifests, rather than retain a queue solely to
keep those command forms unchanged.

In this model, the current **copy a saved run into the next-run queue, edit it,
then run/retry it** workflow cannot remain unchanged because that separate
destination no longer exists. The capability can instead target a run's
pending state, but this changes when execution may begin and does not imply
that a running process can be edited. That does not automatically mean every
capability behind `copy` and `change` should disappear:

- `copy` currently supports two useful concepts: clone a saved run's job
  definitions, and stage that clone for later execution. The staging part can
  become “add these cloned jobs as pending members of this run”; cloning and
  provenance still need clear source/target IDs and collision behavior.
- `change` currently edits queued definitions (and `change --run-id` restores
  a saved snapshot into the queue before editing). Preserve it as a
  pre-execution operation in the current proposal. The user questions whether
  changing a job after submission is needed, so do not extend `change` to
  active-run or finished jobs absent a concrete workflow that requires it.
- A manifest-based workflow could preserve review-before-execution:
  `export`/edit/`run` from a manifest or a new-session preview/apply API. This
  is an alternative interface, not a hidden replacement queue.

The repository documents and demonstrates `copy` + `change` as the fix-and-run
workflow in [RECOVERING.md](../../docs/RECOVERING.md), [FAQ.md](../../docs/FAQ.md),
and [examples/lineage.sh](../../examples/lineage.sh), and conformance covers
copy/selector behavior. This confirms it is a supported and tested capability,
not how frequently users rely on it; no usage data establishes its actual
frequency. If the queue-oriented commands are retired, inventory and replace
their user-facing purpose: restore/clone a saved workflow, edit before start,
preview the edited execution, and preserve stable provenance. Do not equate
“remove the queue” with “drop reproducible workflow editing” without deciding
whether that capability has an acceptable replacement.

Decision questions:

1. Is pre-run draft editing still required, or is direct immediate submission
   with per-job validation sufficient?
2. Does exporting/cloning a saved run into a manifest preserve the required
  copy, selector, origin, and provenance workflow? Any direct run-to-run copy
  can be added only if there is a concrete use case that manifests do not
  cover.
3. Is there a demonstrated workflow that requires changing a job after it has
   been submitted? Unless one is identified, keep `change` limited to
   pre-execution definitions and do not design revised-definition retries.
4. If the supported copy/change use case is removed, what replaces the
   documented fix-before-rerun flow in CLI, Web, MCP, and Python?

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

### Risk: immediate submission may weaken `run` and confuse `retry`

This is a primary product risk, not just a naming concern. In today's model,
one `run` roughly means “execute this prepared batch”; its ID is also a useful
unit for inspection, cancellation, comparison, and retry. In a `start` /
`submit` / `wait` model, membership accumulates over time and may combine
several scripts or unrelated submit calls. If `run` then means an open time
window rather than a known batch, users may no longer know what “retry this
run” selects or whether it includes later submissions.

The design must preserve a meaningful run boundary even after removing the
queue:

- A run ID remains the durable owner of accepted jobs, attempts, logs,
  dependencies, settings, and history. A script/client is not the run owner;
  its exit does not erase or cancel accepted work.
- `wait` seals membership, then waits for all accepted work. Only after this
  point is the run a stable retry/comparison source. A run left open after its
  submitter exits remains visible and can be sealed or cancelled by run ID.
- Finished-run retry should continue to mean “create a successor run from
  this stable source, carrying the unselected results forward,” unless the
  product deliberately chooses another meaning.
- The shipped active retry currently reopens final jobs in the same run. If
  that remains, distinguish it explicitly (for example, a separate
  `retry-active` operation) from retrying a sealed run into a successor. Do not
  let `retry` silently switch meanings based on whether a session happens to
  be open.
- Run-wide cancel remains the one-action cancellation for all unfinished work
  in the session. For a narrower cancellation, Rotari already supports
  selecting individual jobs by ID (including repeated IDs); a new submission
  group ID is not required solely for cancellation. `submit` must return the
  admitted IDs so scripts can retain that selection.

**Decision gate:** before implementation, write examples showing `start`,
multiple `submit` calls from different scripts, one script exiting before
`wait`, `wait` sealing, run-wide cancel, retry of an open run, and retry of a
finished run. Each example must say exactly which jobs are included, which run
ID is returned, and whether a successor history unit is created. If those
answers are hard to explain, the immediate-submit UX may be less clear than
the queue model it replaces.

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
| Run/retry mental model | Does `run` still name a coherent unit when jobs arrive over time? Does `retry` create a successor from a sealed run, retry inside an open run, or both via explicit forms? |
| Cancellation and recovery | What does cancel stop? What does unlock recover if a coordinator dies while accepting work? |
| Script lifetime and cancellation | If a submitter script exits before `wait`, do accepted jobs continue? Existing run-wide cancel cancels all unfinished jobs, while repeated job-ID selectors can cancel an arbitrary subset (array ID selects its unfinished tasks). Does that remain clear for jobs submitted by multiple scripts, and how is an abandoned open session surfaced? |
| Interfaces | Can CLI, Web, MCP, and Python expose the same target and state transitions without implicit routing? |
| Compatibility | Which current scripts depend on `add` not launching work, batch previews, or one run summary? |
| Saved workflow editing | How much user-facing value is carried by `copy` + pre-execution `change`, and what replaces its clone/edit/preview steps if queues disappear? |
| Pending state | Is a job `pending` until its first attempt starts, and how is that distinct from an executor-accepted attempt waiting to launch? Are pending definitions immutable once admitted to a run? |
| Complexity cost | What new state machine, persistence protocol, UI concepts, and contracts does each model require? |

## Required workflow scenarios

Use these scenarios to compare the models:

1. Queue ten independent jobs, inspect/preview them, then run all with a chosen concurrency and environment.
2. Start a slow run, add a forgotten independent job, and decide whether it should start now or belong to the next run.
3. A fast job fails due to a typo or bad per-job Slurm option while unrelated jobs remain slow. First establish whether users need to correct the already-submitted job at all. If not, do not design active `change`; define the supported recovery path (for example, leave it failed and submit a separately identified replacement, or wait and prepare a successor run).
4. A fast job fails transiently while unrelated jobs are still running. Compare
  (a) same-run retry immediately, (b) wait for the run and use ordinary
  successor-run retry, and (c) leave it failed until the run ends. Decide
  whether the latency benefit of (a) is important enough to justify the
  additional retry state and any confusion with successor-run `retry`.
5. Add a job that depends on a running job, a successful job, and a failed-final job; define readiness and blocking in each case. DAGs are compatible with late submission: forward references to jobs not yet submitted are a separate question and may be rejected while backward dependencies remain supported.
6. Widen an array/matrix or add one member after some members have completed; define selection, aggregate status, and comparison semantics.
7. Kill the coordinator while work is running and while a late-add/retry request is being accepted; recover without losing, duplicating, or misattributing jobs.
8. Start two unrelated projects and operate on one with explicit IDs; ensure no model creates ambiguous defaults.
9. A script starts a run, submits several jobs, then is killed before `wait`. Accepted jobs must not be implicitly cancelled and remain cancellable by run ID. A second script submits unrelated work to the same open session: run cancellation intentionally reaches all unfinished jobs, while repeated returned job IDs can select only one script's subset if needed. Verify dynamically added pending jobs and arrays remain selectable through existing cancel behavior. Define how the still-open session is sealed or recovered.

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
- If there is no separate next-run queue, how are users expected to stage/preview a large batch before execution? Is a manifest or `run` plan still the batch-start interface, and how is run-owned pending work displayed?
- Is an active retry without definition changes valuable enough to keep? Treat definition-changing retries as out of scope unless a concrete workflow establishes their value.
- Is the already-shipped same-run retry useful enough to justify its distinct semantics, or should a future compatibility change remove/deprecate it? Do not silently conflate this with successor-run retry.
- Are run-wide executor settings intentionally immutable? The current preference is yes; determine whether job-specific executor/option overrides cover the real correction cases.
- Is there a need for a continuously accepting mode, or only a way to append work to a bounded active session?
- Is late addition to the active run valuable even if editing already-submitted jobs is explicitly out of scope? The proposed first step tests this while preserving current queue behavior.
- After bounded active-add experience, is there evidence that the separate next-run queue itself should be removed, or is explicit target selection sufficient?
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
