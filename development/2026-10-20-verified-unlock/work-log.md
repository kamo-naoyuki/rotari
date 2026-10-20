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
