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
