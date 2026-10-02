# Work History: Unifying Copy and Import

See [plan.md](plan.md) for current scope and status. The original one-line notes did not preserve test commands/results; test additions are not treated as evidence of a pass.

## Establish shared queue/run behavior and package boundaries

**Commits:** 2026-09-25 17:09:55 `3751b61`; 2026-09-25 18:21:23 `9fddce5`; 2026-09-25 22:18:15 `f1ef5e6`; 2026-09-26 11:20:14 `7ad44de`.

**Change:** Added matrix provenance/import carry tests, direct planning for new imported jobs, moved rerun planning/copy/workflow reconciliation into internal packages, and moved project state and idle queue edits into `internal/project`.

**Reason:** Align copied and imported queues with shared run/result behavior and separate domain operations from CLI adapters.

**Plan impact:** Added coverage and moved major queue, workflow, and project responsibilities into internal packages.

**Validation:** Commits add unit coverage, but historical notes do not name the test commands or results.

**Remaining:** Unify status marking and legacy-state behavior across copy, import, run planning, and display.

## Unify status handling and document edited-job behavior

**Commits:** 2026-09-29 02:51:35 `4e674fd`; 2026-09-29 04:55:01 `bf7045c`.

**Change:** Unified copy/import status handling and removed the separate `Force` path; clarified through contracts, conformance, and `docs/RUNNING.md` that edited jobs retain their result after `Force` removal.

**Reason:** Queue status and execution selection belong to shared result/run-filter rules, not parallel import-specific logic.

**Plan impact:** Completed the original status-unification scope and documented the user-visible behavior.

**Validation:** Commits include test/conformance changes, but prior notes did not record executed commands/results.

**Remaining:** The plan marks its original scope complete. Fingerprint matching is tracked separately; no other follow-up was recorded.
