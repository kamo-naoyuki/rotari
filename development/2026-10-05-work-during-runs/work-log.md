# Work log

## 2026-10-06 — Phase 1 source planning and inspection

Implemented after the queue/recovery work:

- D8: saved-run sources are transformed in memory under the project state lock and passed to `Runner.Begin` as the run snapshot. `queue.json` is neither overwritten at copy time nor consumed at Begin; the run's queue-based sources still consume the next queue normally. `run --overwrite` / `retry --overwrite` are rejected.
- D3: previews and run completion messages report failed/unfinished latest-run jobs not represented in a non-empty queue, and provide the `copy --failed --unfinished --append` route. MCP preview/start return the same source run and omitted job IDs.
- Active/interrupted `show` presents the run and non-empty next queue separately; JSON retains the run snapshot in `commands` and adds `next_queue`.
- Updated contracts, guides, generated CLI references, golden output, and conformance coverage.

Validation completed:

- Focused source/snapshot, omission reporting, CLI/MCP, active/interrupted inspection, contract status, and golden tests passed.
- Relevant package tests and lifecycle/interface/coordination conformance passed with `-count=1`.
- Pre-commit checks passed after formatting the generated Python CLI schema.
- `scripts/check.sh --short` passed.
- `GOFLAGS='-timeout=40m' scripts/check.sh` passed (vet, normal tests, and race tests). The timeout was extended because the previous race run of the flag-pair suite exceeded Go's default ten minutes.
- Python client suite: 29 tests passed; generated CLI/API references and README synchronization checks passed.

Commit tracking: jj change `rzyrsrpw` (saved-run snapshots, retry source reporting,
and separate active-run/next-queue inspection).

## 2026-10-06 — Phase 2 active-run retry

- **Change:** Added a durable, versioned retry request/response channel in each
  active run; reopened final run-owned jobs in the engine without resetting
  attempt numbering; reopened blocked `DependsOn` descendants; and hid
  superseded results while the accepted attempt is pending. Exposed the shared
  selector and request flow through CLI `retry`, MCP preview/apply tools, and
  the Web UI/API. Documented same-run behavior and the explicit end-race error.
- **Reason:** Implement Phase 2 of the plan while preserving one run per
  project and avoiding silent fallback to a new run.
- **Plan impact:** Phase 2 is complete. Project-selection friction is moved to
  the separate Phase 3 plan at
  [development/2026-10-06-project-selection/plan.md](../2026-10-06-project-selection/plan.md).
  The end-race, whole-array, and eligibility rules have unit coverage; the
  related contracts remain partial pending broader end-to-end coverage.
- **Validation:** Focused CLI, core package, MCP, Web, state-version, selector,
  and lifecycle conformance tests passed. `scripts/check.sh --short` and
  `GOFLAGS='-timeout=40m' scripts/check.sh` passed, including race tests.
- **Remaining:** Broader conformance for end-race and option/array variants;
  see RUN-15 and SAFE-11 in `contracts/README.md`.

Commit: jj change `rpkwxuzr`, commit `025e2a33`, 2026-10-06 12:37:26.

## 2026-10-06 — Reconsider active retry configuration limits

- **Change:** Updated the umbrella and Phase 3 plans to prefer a bounded
  relaxation of run immutability: a selected final job may receive a revised
  execution definition for its next attempt, while the initial run membership,
  dependency graph, and array/matrix topology remain fixed. Listed the
  per-attempt provenance, mutable-field, scheduler-option, and explicit-mode
  questions that must be resolved before implementation.
- **Reason:** Same-definition active retry cannot correct a bad command,
  environment, or executor option during a long-running run; this limits the
  feature's value for fix-and-retry workflows.
- **Plan impact:** D10 now has a preferred design direction, not a complete
  implementation specification. The user clarified that changing run-wide
  settings while running is unnecessary; only a selected final job's next
  attempt may receive per-job overrides. Phase 3's active-retry inference
  remains paused until the request semantics are settled.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Define patchable per-job execution fields and which scheduler
  options can validly override lane defaults for that job; run-wide settings
  remain immutable. Then implement attempt-level provenance and views. D9 must
  be updated to match the final retry mode.

Commit: 2026-10-06 16:56:51 +09:00 `2621b67e`.

## 2026-10-06 — Scope candidate per-attempt retry overrides

- **Change:** Added a candidate V1 allowlist for revised retry attempts:
  command, working directory, job environment, timeout, executor, and
  per-job executor options. Kept job identity, dependencies, arrays/matrices,
  run context, run-level retry policy, concurrency, scheduler lane defaults,
  and the next queue fixed. Documented the scheduler-option empty/inherit
  ambiguity that an explicit replacement/clear representation must resolve.
- **Reason:** Make the user's preferred bounded immutability relaxation
  concrete without making active runs generally mutable.
- **Plan impact:** Candidate field boundary is documented, not yet a settled
  implementation contract. Run-wide Slurm/PBS/LSF/SGE settings remain
  immutable; only supported per-job options may override defaults for the
  selected next attempt.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Confirm the V1 allowlist and define request/CLI/Web/MCP shapes,
  atomic validation, per-attempt provenance, and read-side presentation.

Commit: 2026-10-06 17:03:55 +09:00 `ee118402`.

## 2026-10-06 — Record the two-workspace interaction

- **Change:** Added D11 to call out that active retry revisions and Phase 4
  queue-to-run promotion create distinct next-queue and active-run write
  targets. Phase 4's sketch now requires explicit targeting, defines promotion
  consumption and collision questions, and preserves the initial snapshot via
  append-only revision records. Phase 3 active-retry inference is gated on
  resolving this model.
- **Reason:** The user identified that allowing both queue edits and active-run
  edits may make the queue/target model confusing.
- **Plan impact:** No runtime behavior changed. D11 requires queue commands to
  keep targeting the next run and active-run mutations to be explicit; if that
  distinction is not understandable across interfaces, defer active-run
  mutation features.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Decide promotion semantics, collision handling, and whether
  the two-target UI/CLI model is sufficiently clear before implementing D10 or
  Phase 4.

Commit: 2026-10-06 17:07:08 +09:00 `a62c4a2f`.

## 2026-10-06 — Reopen the foundational execution model

- **Change:** Added a separate comparison plan for run-based batches,
  immediate independent jobs, open execution sessions, and explicit-target
  hybrid operations. Reframed the queue/run model as the currently shipped
  design rather than a settled architectural premise, and gated Phase 3/4
  expansion on selecting the primary model.
- **Reason:** The user questioned whether both next-queue edits and active-run
  edits make the queue an awkward second work target, especially when retry
  cannot fix job configuration. That calls for comparing the execution models
  before extending either feature.
- **Plan impact:** No runtime behavior changed. Phase 3 project-selection
  inference and Phase 4 late additions are paused pending the model comparison;
  D10/D11 remain provisional. The review is at
  [development/2026-10-06-execution-model-review/plan.md](../2026-10-06-execution-model-review/plan.md).
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Populate workflow comparisons and choose a target model before
  revising D9–D11 or writing an implementation plan.

Commit: 2026-10-06 17:15:57 +09:00 `5cd12e49`.

## 2026-10-06 — Explore queue-less immediate execution

- **Change:** Expanded the execution-model review around the user's leading
  hypothesis: `run` starts work immediately, admits later jobs into that same
  execution session, and does not keep a separate next-run queue. Clarified
  that a late job may depend on already submitted jobs under the existing DAG
  readiness rules. Added the missing bootstrap question for initial jobs and
  compared explicit session close with inactivity-timeout closure.
- **Reason:** Reconsider the root work model instead of layering queue and
  active-run mutation features on top of one another.
- **Plan impact:** No runtime behavior changed. The open-run model is a leading
  hypothesis, not a final decision. The working recommendation is explicit
  close first; a timeout is optional only if users demonstrate a need. Phase 3
  and Phase 4 remain paused pending the model choice.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Compare the workflow scenarios, decide how `run` receives its
  initial jobs without a hidden queue, and select the session closure contract.

Commit: 2026-10-06 17:24:17 +09:00 `490a355e`.

## 2026-10-06 — Assess copy/change in a queue-less model

- **Change:** Added a dedicated compatibility review for `copy` and `change`
  to the execution-model plan. It distinguishes queue staging (which disappears
  in a queue-less model) from cloning saved job definitions, editing before
  execution, previewing, and preserving provenance. It notes existing docs,
  examples, and conformance coverage without assuming actual usage frequency.
- **Reason:** The user raised that `copy`/`change` may be uncommon enough to
  retire if the queue goes away.
- **Plan impact:** No commands are deprecated and no runtime behavior changes.
  Any queue-less decision must say how the documented copy/fix/rerun workflow
  is preserved or intentionally retired across interfaces.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Decide whether saved-run cloning and pre-run editing have a
  replacement that is simpler than retaining queue-based operations.

Commit: 2026-10-06 17:31:46 +09:00 `82993756`.

## 2026-10-06 — Model the queue as run-owned pending work

- **Change:** Refined the open-run candidate: remove the separate next-run
  queue workspace, but retain an explicit `pending`/not-yet-started lifecycle
  state owned by a run. Added candidate semantics for copying selected jobs
  into an open run as pending and limiting `change` to definitions whose first
  attempt has not started. A started attempt remains immutable; correcting it
  requires a separately specified retry/revision operation.
- **Reason:** The user proposed treating today's queue as the run's pending
  state, with `copy` adding pending work and `change` editing work that has not
  started, rather than maintaining two separate edit targets.
- **Plan impact:** The leading hypothesis is now “run-centric with run-owned
  pending work,” not literally “no pending collection.” Copy must define
  carried results and ID/dependency collision behavior. Reset must not conflate
  clearing pending jobs with sealing admission. No runtime behavior changed.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Define pending versus executor-accepted-but-not-launched state,
  copy semantics for successful jobs, and whether `change` is allowed after a
  session is sealed but before a pending job starts.

## 2026-10-06 — Consider `reset` as an open-run boundary

- **Change:** Added reuse of `reset` as a candidate explicit session boundary
  to the execution-model comparison, alongside a dedicated `run finish` /
  `run seal` command and timeout-based closure. The plan now states that any
  reset-based seal must preserve accepted jobs and history, and must reconcile
  the current queue-only reset contract.
- **Reason:** The user noted that their workflow already calls `reset` between
  work cycles, so the existing command may provide a familiar run boundary.
- **Plan impact:** No behavior change or command decision. Whether reset seals
  an open run, seals and opens the next session, or remains queue-only is an
  explicit question in the model review.
- **Validation:** Documentation-only; no tests run.
- **Remaining:** Compare the reset boundary with a dedicated seal command and
  timeout/hybrid behavior before selecting the open-session lifecycle.

## 2026-10-06 — Allow change on finished jobs

- **Change:** Updated the execution-model proposal so `change` may revise a
  finished job's definition for a subsequent attempt, in addition to editing
  a job that has never started. A running attempt remains immutable; the
  completed attempt's definition and result stay in history.
- **Reason:** The user clarified that `change` should also work for finished
  jobs, not only for not-yet-started jobs.
- **Plan impact:** The state model must distinguish never-started work from a
  finished job whose next-attempt definition has been revised. The plan
  recommends keeping `change` as an edit and `retry` as the execution trigger,
  but records automatic-start versus explicit-retry as an open decision.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Decide whether `change` on a finished job makes a retry
  immediately eligible or requires explicit `retry`, including behavior when
  the session is sealed.

## 2026-10-06 — Audit capabilities at risk in the open-run model

- **Change:** Added a capability-loss table to the execution-model review for
  next-batch preparation during an active run, whole-batch preview/validation,
  run-wide context selection, stable membership, copy/result carry, array and
  matrix topology, successor-run retry history, and finite `run`/`wait`
  behavior.
- **Reason:** The user asked whether the proposed model could remove things
  that work today.
- **Plan impact:** The open-run model may reduce target ambiguity but can also
  remove independent next-run staging and batch semantics. Manifests or
  successor drafts can preserve those capabilities, but are still a second
  conceptual workspace; the decision should be explicit.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Compare the trade-off against concrete workflows and decide
  which capabilities must be preserved versus intentionally dropped.

## 2026-10-06 — Identify gains of run-owned pending work

- **Change:** Added the corresponding benefits to the model review: one
  explicit in-project mutation target, late DAG submissions into the active
  workflow, corrected retries without waiting for unrelated work, fewer
  queue-to-run promotion steps, and a unified live DAG/status view.
- **Reason:** The user asked what the proposed change gains, after reviewing
  the capabilities it may put at risk.
- **Plan impact:** The benefit is framed as unifying the target, not removing
  pending work. These gains apply to current-workflow additions and do not
  solve next-run preparation or whole-batch validation after execution starts.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Decide whether these gains justify losing or externalizing a
  separately prepared successor batch.

## 2026-10-06 — Defer editing jobs after submission

- **Change:** Revised the execution-model review to stop treating active-run
  `change` or changed-definition retries as requirements or benefits. Keep
  `change` pre-execution unless a concrete use case demonstrates a need to
  revise a submitted job. The retry comparison now concerns same-definition
  in-run attempts versus successor-run history.
- **Reason:** The user suspects editing an already-submitted job is not needed
  in the first place.
- **Plan impact:** This removes one major motivation for moving queue state
  into an open run. Late additions remain a possible benefit, but whether they
  alone justify the model change is now an explicit decision. No runtime
  behavior is changed.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Identify a recurring late-add workflow and compare it with
  preserving the existing queue and run boundary before recommending a model.

## 2026-10-06 — Concretize the active-run additions plan

- **Change:** Added a detailed implementation proposal for bounded,
  exact-run-targeted active additions. It preserves normal queue operations,
  excludes submitted-job edits and queue promotion, defines an all-or-none
  admission contract, and maps model/engine, durable protocol, projectrun/CLI,
  Web/MCP/Python, tests, contracts, and later array/open-session phases.
- **Reason:** The user asked to proceed and make the plan concrete enough to
  organize what would change.
- **Plan impact:** Recommended incremental experiment is explicit
  `add --run-id RUN` while the run is executing, not a queue-less migration.
  Bounded append does not wait after work drains; open/seal semantics and
  whether to remove the queue remain later decisions. V1 group scope and
  protocol/revision details remain decision gates before implementation.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Confirm V1 command shape and plain-job versus new-group scope,
  admission record/revision design, then update contracts before coding.

## 2026-10-06 — Map a possible queue-retirement path

- **Change:** Added a staged roadmap after bounded active-add V1: gather
  evidence that the queue itself is a problem; decide open/seal/retry lifecycle;
  replace pre-run queue workflows with a visible, previewable manifest flow;
  provide opt-in compatibility and lossless migration; remove queue-backed
  execution only after interface and conformance gates pass.
- **Reason:** The user asked for the path from the current proposal through to
  possible queue removal.
- **Plan impact:** Queue retirement is explicitly not part of V1. The roadmap
  preserves pre-run editing and whole-batch review unless deliberately
  replaced, forbids silently changing `add` semantics or reinterpreting
  `reset`, and includes stop conditions where retaining a draft workflow is
  the better choice.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** First confirm V1 active-add contract; only after real usage
  evaluate the evidence gate for queue retirement.

## 2026-10-06 — Prefer immediate run-owned execution

- **Change:** Updated the execution-model review, active-run plan, and umbrella
  plan to treat a queue-less run-owned pending model as the preferred product
  direction. The user clarified that the motivation is a simpler mental model
  for immediate submission/execution; queue `copy`/`change` are uncommon and
  manifests are sufficient for pre-run authoring. A bounded add that preserves
  a permanent queue is no longer the target UX.
- **Reason:** The earlier framing overemphasized whether the queue was used and
  underweighted that the queue abstraction itself is less understandable.
- **Plan impact:** `run` starts ready manifest work and remains open to later
  additions until an explicit boundary; the run owns pending jobs. Submitted
  job edits remain out of scope. Manifest bootstrap, `add` without an open
  run, `reset` versus `finish`/`seal`, retry interaction, and lossless queue
  migration remain design gates. Queue removal is intended, but no runtime
  behavior has changed.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Settle the run state machine and manifest start contract
  before implementation planning is considered complete.

## 2026-10-06 — Sketch `start` / `submit` / `wait` commands

- **Change:** Added the user's proposed lifecycle vocabulary: `start` opens a
  run and seals a previous open run first; `submit` adds work to the open run
  for immediate dependency-aware scheduling; `wait` seals admission and waits
  for accepted work to finish. Propagated it into the transition, model-review,
  and umbrella plans.
- **Reason:** The user described the command model they are considering.
- **Plan impact:** Added a key invariant question: `start` cannot safely open a
  second coordinator while a prior run still executes. The proposed default is
  to seal and wait for the prior run before opening the next; fail-fast and
  multi-active-run alternatives remain for comparison. `submit` with no open
  run and manifest versus empty-session bootstrap also need a contract.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Confirm whether `start` should wait for the prior run to
  finish after sealing, and whether initial work enters through a manifest or
  an empty session followed by `submit`.

## 2026-10-06 — Preserve run-level cancellation across script exit

- **Change:** Added a lifecycle requirement and comparison scenario: once a
  submission is durably accepted, a submitter script exiting before `wait`
  does not cancel the work; all accepted jobs remain grouped under one run ID
  and can be cancelled together. Added visibility/sealing requirements for a
  run left open by that script exit.
- **Reason:** The user identified that the current `run` batch model survives
  a script ending before `wait` and provides one run-level cancellation
  target; this is a benefit the proposed model must preserve.
- **Plan impact:** Open-session behavior now distinguishes client lifetime
  from run lifetime. An abandoned open run is an operational state to surface
  and explicitly seal/recover, not a reason to cancel accepted work.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Specify the user-visible status and cleanup action for an
  open run whose submitter has exited, while preserving run-wide cancel.

## 2026-10-06 — Reassess run and retry identity under immediate submission

- **Change:** Added a central risk analysis: when an open run accumulates
  submissions over time, `run` may stop feeling like one prepared batch and
  `retry` may no longer have an obvious selection boundary. The plans now
  require `wait` to seal a stable source, preserve finished-run retry as a
  successor run, and make same-run active retry an explicit distinct form if
  retained. They also require deciding whether multiple scripts share one
  whole-run cancellation unit or need explicit submission groups.
- **Reason:** The user noted that immediate execution may make `run` less
  salient and make retry harder to understand.
- **Plan impact:** This is now a decision gate that can reject the proposed
  model if run membership, retry selection, returned run IDs, and cancel scope
  cannot be explained consistently with concrete examples.
- **Validation:** Documentation-only; `git diff --check` passed. No tests run.
- **Remaining:** Write/validate examples for multiple submitters, client exit,
  wait/seal, whole-run cancellation, active retry, and successor-run retry.
