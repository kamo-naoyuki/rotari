# Verified unlock implementation history

## Phase 1a: durable coordinator identity and read-only assessment

Commit: `ae046803`

Git date: 2026-10-20 11:46:05 +0900

- **Change:** Store the coordinator's host, PID namespace, PID, process start
  time, and recording time in the existing run context before execution.
  Record successful management completion after finalization and lock cleanup.
  Add a shared `project.AssessCoordinator` and expose its state, reason,
  observation time, and identity in text/JSON `info` output. Add lifecycle,
  evidence-table, CLI, and binary conformance tests; update CLI-21, operations,
  architecture, and the contract coverage table.
- **Reason:** A removed run lock must not erase the identity needed to inspect
  a still-live coordinator. Reusing context avoids reserving another filename
  that could collide with a valid job ID. Completion is informational and
  cannot override actual process evidence. Missing legacy identity and
  uncheckable host/namespace/start-time evidence stay unknown. A malformed
  context now fails finalization explicitly rather than silently overwriting
  the coordinator record.
- **Plan impact:** Phase 1a is implemented; unlock policy, job-liveness rules,
  automatic recovery, and existing run results are unchanged. The read-only
  `info` contract is extended, without tightening recovery contracts yet.
  User-authored pending plan revisions were left uncommitted; only this
  implementation's status paragraph was included.
- **Validation:** Focused coordinator/lifecycle/CLI tests and
  `TestInfoCoordinatorIdentityWithoutRunLock` passed. Changed-file pre-commit
  and `scripts/check.sh --short` passed. The final `scripts/check.sh` passed
  vet, Web type checking, all normal tests (including conformance), and all
  race tests. The first full check failed an unchanged executor test at
  508.794469 ms against a 500 ms bound; three isolated repeats and the next
  full check passed. This observation is tracked in
  [ISSUES.md](../ISSUES.md). The context-preservation regression was also
  run against pre-change `93863df6` in a temporary worktree: it failed because
  `Finish` returned nil; it passes after the change. That worktree was removed.
  Validation used existing cached Go 1.25.8 and Node 22.23.2 tools; no
  dependencies were installed.
- **Remaining:** Phase 1b (explicit attestation and tightened ownership
  policy), Phase 2 (attested finalization repair), Phase 3 (execution
  assessment), and Phase 4 (full verified unlock). The unrelated cancellation
  timing observation remains open.

## Phase 1b: explicit ownership attestation and nonmutating unlock preview

Commit: `b2de2f48`

Git date: 2026-10-20 12:27:53 +0900

- **Change:** Gate shared `project.Unlock` on the durable coordinator
  assessment even without a run lock. Verified live ownership always refuses;
  unknown ownership requires `--assume-stopped` (`assume_stopped` in MCP and
  Python). Add CLI `--dry-run` and `--json`, Python projections, and the same
  permission, refusal codes, identity, observation and revision evidence in
  MCP preview/apply. A blocked preview is a successful inspection; a refused
  apply is an error, with structured evidence retained in MCP tool errors.
  Apply reassesses ownership under the state lock. Record recovery
  authorization in the existing context, preserving coordinator identity,
  queue and result history. Update SAFE-4, COORD-3, SAFE-7, add SAFE-13/14,
  regenerate schema/reference data, and document automation migration.
- **Reason:** An absent lock must not bypass a live durable owner. Conversely,
  a PID without a validated process start time is only possibly live, so it
  belongs to the overridable unknown path, not the non-overridable live path.
  `--assume-stopped` expresses attestation rather than a blanket force switch
  and can also cover unknown execution evidence later. It has no automatic
  configuration/environment equivalent. Reusing context avoids another
  reserved run-root filename and retains the distinction between attested
  recovery and verification. The authorization record precedes recovery;
  subsequent I/O failure still reports failure, not successful recovery.
- **Boundary:** Execution termination is still not checked. CLI/MCP/Python
  machine-readable output says `execution_checked: false`, including when all
  jobs have terminal records. Operators must independently confirm jobs and
  managed descendants stopped. No automatic unlock, job killing, resubmission,
  summary-based finalization repair, or Web unlock action was added.
- **Validation:** The shared policy table covers 32 combinations of identity,
  lock presence and attestation; binary/MCP conformance covers 24 ownership and
  attestation combinations. All 21 unlock flag pairs run in both orders, with
  separate standalone samples and safety cases. Targeted tests, complete
  affected-package tests, changed-file pre-commit, `scripts/check.sh --short`,
  and the final isolated `scripts/check.sh` passed, including normal/race
  conformance. All 67 Python tests passed with the real isolated binary.
  Generated Python metadata matches the binary schema; the generated CLI
  reference and documentation links passed their checks.
  `TestCmdUnlockRejectsLocklessLiveCoordinator` was run against pre-change
  `d1d69523` in a temporary worktree and failed because unlock actually
  recovered the live lockless run; it passes after the change.
  The first full check failed an unchanged jj revision test with
  `jj: signal: killed`; three isolated repetitions passed. The next full
  race check missed the second run in two unchanged positional selector
  cases; three isolated race repetitions of the affected cases and
  `wait/latest_run` passed. The final full rerun passed without weakening
  any thresholds or changing those implementations/tests. Both observations
  are recorded in [ISSUES.md](../ISSUES.md).
- **Isolation:** Full validation used a worktree containing only this phase's
  changes, with existing cached Go/Node tools and dependencies. Concurrent
  lifecycle-filter work and the user's pending plan revisions were excluded;
  only this phase's status/rationale paragraph was staged from the plan.
- **Remaining:** Phase 2 adds attested finalization repair; Phase 3 adds
  execution probes; Phase 4 applies full verified-unlock policy. The unrelated
  intermittent-test observations and stale coverage-prose counts remain open.

## Phase 2: attested finalization repair

Commit: `ca3b5c6f`

Git date: 2026-10-20 13:56:48 +0900

- **Change:** Assess finalization repair in shared `project.Unlock`. Validate
  the selected summary against the expanded command snapshot, including
  arrays, sparse arrays and matrix members, exact result coverage, attempt
  associations, carried origins and explicit status marks. Inventory earlier
  attempts and incomplete launch records, not just the final outcome.
  CLI/Python JSON and MCP preview/apply share `action`, `repair`, reasons and
  execution identities. Human CLI output distinguishes finalization from
  release. Add SAFE-15, update related contracts and their conformance map,
  regenerate CLI metadata/reference, and update recovery and architecture
  documentation.
- **Reason:** Terminal outcome files cannot establish termination of a
  wrapper, its managed descendants/process group, or scheduler allocation.
  The wrapper audit found normal completion waits only for the direct
  command; cancel-grace and timeout escalation can write terminal status
  before the final kill. SSH wait observes the SSH client, while scheduler
  wait can return a wrapper result without a fresh allocation query.
  Therefore every started/uncertain unit requires explicit `--assume-stopped`
  for repair in this phase. A complete summary without that attestation
  refuses with `execution_unknown`, rather than silently releasing.
  Only entries established never launched avoid execution attestation.
  Verified live coordinator ownership still blocks, with or without a lock
  and regardless of attestation.
- **Recovery:** Use `state.FinalizeRun` with the recorded summary finish
  time, complete applicable client status, and preserve the original summary,
  results, commands, origins, run ID and next queue. Do not invoke
  `projectrun.Runner.Finalize` or replay notification/completion hooks.
  Authorization evidence precedes lock removal, client cleanup and metadata
  finalization; later I/O failure is reported, and partial cleanup can be
  retried. Missing/inconsistent summaries explain a release to collecting;
  malformed/unreadable repair evidence is an explicit error.
- **Preserved decision:** Keep `projectrun.Finish` removing its own lock on
  finalization failure. A dead supervisor should leave interrupted metadata,
  not an apparently running lock. The durable coordinator record already
  protects a live supervisor independently of that lock.
- **Origins:** Preserve exact-attempt and unpinned-origin semantics. Move
  the existing array-task origin rule to `model.TaskOrigin`, with the run
  helper delegating to it, so planning and repair do not independently
  implement inheritance. Expanded repair tasks can follow an array-level
  origin even without a per-task map. Export the existing run-specific
  attempt decoder to reuse validation for non-timestamp run IDs without
  changing ordinary attempt discovery. The generic string-origin lookup's
  separate legacy-array limitation is recorded in [ISSUES.md](../ISSUES.md);
  other readers' behavior is intentionally unchanged.
- **Validation:** The motivating unit regression was written first and
  failed because unlock returned metadata to collecting instead of finished.
  Unit cases cover incomplete/duplicate/mismatched results and commands,
  expanded units, never-launched cancellation/dependency blocking, earlier
  attempts/incomplete launches, source cycles, accepted and marked results,
  pinned/unpinned origins, cleanup retry, stale previews, malformed records,
  and config-directory inventory. An unpinned-origin case also failed before
  its validation correction. Terminal normal/cancel/timeout records remain
  unknown across local, SSH, Slurm and SGE examples. CLI/MCP conformance
  checks 12 repair/release variants, including inherited carried arrays,
  preserved files/queue/finish time, nonmutation/refusal and repeated recovery.
  Existing ownership cases and all unlock flag pairs/samples/safety cases
  passed. Plain and inherited-array binary regressions failed on pre-change
  `435eea35` in a temporary worktree, then passed with this implementation;
  that worktree was removed.
  Affected package tests, changed-file pre-commit, `scripts/check.sh --short`,
  and the final `scripts/check.sh` passed, including vet, Web typecheck and
  all normal/race conformance suites. The full check was rerun after the
  final origin corrections. All 67 Python tests passed with the final binary;
  generated schema/reference checks passed. An initial package test used the
  system Node 12 and failed parsing existing Web scripts; the configured
  cached Node 22 run passed without changing those tests or scripts.
- **Isolation and boundary:** Only this phase's plan status/rationale and
  generated attestation-description hunks were committed. User plan revisions
  and the pending generated lifecycle-filter description remain unstaged;
  the concurrent CI-workflow commit is not part of this work item. No
  production run was unlocked. Execution probes, automatic recovery, killing
  or resubmitting work, and a Web unlock action are not implemented.
  `execution_checked` remains false. Next is Phase 3's read-only executor
  assessment; Phase 4 tightens ordinary release against execution evidence.

## Phase 3: executor-owned read-only execution assessment

Commit: `83e9782d`

Git date: 2026-10-20 15:04:47 +0900

- **Change:** Add bounded local, Slurm, PBS, SGE and LSF execution probes in
  the executor package. Share the complete attempt/carried-origin inventory
  and probe results between `project.AssessExecutions`, `info` and complete
  finalization repair. Expose stopped/active/unknown counts and per-unit
  source, native identity, observation time, scope, possible activity and
  recorded-outcome conflicts. Keep existing outcome counts separate.
  CLI, MCP and Python use the same project recovery decision. Add SAFE-16,
  update related contracts and coverage links, and document the interfaces
  and limitations.
- **Local identity reason:** Publish host and PID namespace before launch
  and PID/process-start identity after launch, in the existing `job.json`.
  A prelaunch write failure prevents execution. A post-start write failure
  is explicitly logged without discarding the live process handle; the
  incomplete durable identity remains unknown to later inspection.
  Matching host/namespace and a missing group establish stopped within the
  recorded process-group scope. An existing group is active only with
  matching leader identity; missing/reused leader identity, permission
  failures and another host/namespace remain unknown rather than claiming
  termination. Descendants that leave the group are outside this scope.
- **Scheduler reason:** Reuse executor backend parsers instead of deciding
  scheduler states in CLI/project callers. Validate native identity and exact
  matching records. A fresh active queue or accounting record blocks repair;
  stopped requires a successful classifiable non-active queue observation
  plus matching terminal accounting. Missing jobs, expired accounting,
  failed queries and ambiguous output remain unknown, never a success-shaped
  absence. Preserve queue/accounting conflicts and query errors even when
  positive activity wins. Include pending SGE XML entries and share its
  whole-queue query across distinct native IDs, not cached per-job verdicts.
  Do not treat LSF's history-only `bhist` fallback as verified accounting.
- **Bounds:** Limit each query to five seconds, each assessment batch to
  thirty seconds, and captured output to 64 KiB. Reuse the scheduler rate
  gate with context-aware waits. Kill only the inspection helper's process
  group on query cancellation and bound its pipe wait; never signal the
  inspected execution. An exhausted budget remains explicit unknown.
- **Recovery reason:** A saved terminal result and execution termination
  are independent facts. Probe all relevant attempts, including earlier
  attempts, incomplete launches, array/matrix members and carried origins.
  Positive activity cannot be overridden by `--assume-stopped`. Unknown
  execution still requires attestation; all-stopped evidence removes only
  execution attestation, not the independent coordinator requirement.
  `execution_checked` means a nonempty complete finalization inventory was
  classified active/stopped within the stated scopes; it does not mean all
  stopped or recovery allowed. Empty established-never-launched inventory
  and ordinary release do not claim execution verification.
- **Validation:** The motivating stopped-execution unit regression was
  written first and failed because repair still required attestation.
  Binary conformance also failed against pre-change `53f68884` because
  `info` had no execution/live-conflict assessment; the temporary worktree
  was removed. Local/scheduler evidence tables cover identity, namespace,
  permissions, leader reuse, partial publication, malformed records,
  native-ID ambiguity, accounting conflicts, limits and cancellation.
  The SGE batch test uses three distinct IDs with pending, stopped and
  running outcomes, asserting exactly one queue and one accounting query.
  Shared project tests cover earlier active attempts and summary-only
  conflicts. CLI/MCP conformance covers verified stopped plain, array,
  carried-array and matrix repairs, unknown legacy identities, and live
  conflicts refusing even with attestation without mutating recovery state.
  All unlock flag pairs/samples/safety cases passed. Their comparator now
  validates and normalizes only dynamic execution observation timestamps,
  retaining other evidence comparisons.
  Affected package tests, focused race tests, changed-file pre-commit,
  `scripts/check.sh --short`, and the final frozen-source
  `scripts/check.sh` passed, including vet, Web type checking and all normal
  and race suites. All 67 Python tests passed against the final binary;
  generated schema/reference checks passed.
- **Validation observations:** An earlier full race check failed unchanged
  `copy/job_ID` with a nonempty next queue; three isolated race repetitions
  of it and `copy/stage_and_job_ID` passed. A later full normal check found
  an empty retry source hint in the unchanged implicit-basedir fixture;
  three isolated repetitions passed. Both are recorded separately in
  [ISSUES.md](../ISSUES.md); their causes remain unconfirmed. The final full
  rerun passed without weakening assertions or changing these fixtures.
- **Isolation and boundary:** Commit only this phase's plan status/rationale
  and generated attestation-description hunks; preserve the user's other
  plan revisions and pending generated lifecycle-filter description.
  Concurrent artifact-launcher documentation commits are not this work item.
  No production run was unlocked and no dependencies were added. Scheduler
  probes use the current scheduler command environment, not a persisted
  authenticated cluster binding; fixture tests are not real-cluster
  certification. Legacy job-control and attachment PID checks remain a
  separate issue. Ordinary release, SSH token probing, and moving probes
  outside the state lock with snapshot/revalidation remain Phase 4.
  Automatic unlock, killing/resubmitting jobs and a Web unlock action remain
  out of scope.

## Phase 4: verified execution evidence for release and repair

Implementation commit: `a1bbb006`

- **Change:** Apply execution assessment to ordinary release as well as
  finalization repair. Run bounded execution probes outside the project state
  lock, then reacquire the lock and verify that the coordinator and all
  execution-relevant state are unchanged before applying the preview. Reject
  active execution even with `--assume-stopped`; require attestation for
  unknown evidence, while verified stopped evidence needs no execution
  attestation. Add an SSH probe keyed by a private per-attempt token, preserve
  remote process identity and finished markers after the SSH wrapper exits,
  and prune only old, verified-stopped remote state. Update CLI/MCP help,
  conformance, SAFE-4/7/16 and related operator, interface, and architecture
  documentation.
- **Reason:** A scheduler or SSH query can take long enough that holding the
  state lock would block unrelated inspection and coordination. Releasing that
  lock creates a time-of-check/time-of-use window, so the preview must be
  invalidated if the coordinator, selected run, carried origin, execution
  identity or recorded outcome changes. A positive active observation cannot
  be made safe by human attestation; missing or inconclusive evidence remains
  explicit unknown. SSH client exit alone does not prove the remote process
  group stopped, so remote evidence must outlive the client without exposing
  the private lookup token as a native execution ID.
- **Validation:** Focused project, executor, MCP and CLI tests passed; the
  CLI/MCP interface suite and Phase 4 active-execution coordination tests
  passed. Tests cover lock availability during a scheduler probe, stale
  preview rejection after execution identity changes, SSH token matching and
  ambiguous results, remote-state retention/pruning, and parity for ordinary
  release. The updated help golden passed, changed-file pre-commit passed,
  and Python tests passed (65 passed, 2 skipped because the optional binary
  environment was not configured). `scripts/check.sh --short` passed vet and
  Web typecheck but failed the unchanged `TestRunRecordsAJJWorkingCopy` after
  its `jj` subprocess was killed at 33 seconds; the isolated test passed in
  4.56 seconds. The first full check failed three unchanged timing-sensitive
  tests: the two `jj` revision-read tests and a local cancellation timing
  assertion. Isolated reruns passed in 4.56 seconds, 1.78 seconds and
  0.43 seconds respectively; details are recorded in [ISSUES.md](../ISSUES.md).
  `GOFLAGS=-p=1 scripts/check.sh` then passed vet, Web typecheck, all normal
  tests, and all race tests. A final review made stale-evidence expiry
  idempotent so revalidation does not duplicate its diagnostic. After that
  adjustment, the complete project, executor and MCP packages, the
  idempotence test under `-race`, and changed-file pre-commit all passed.
- **Isolation:** The user's pending plan revisions and concurrent
  `internal/launcher` work are not part of this phase and must remain
  unstaged. No production run was unlocked and no dependencies were added.
