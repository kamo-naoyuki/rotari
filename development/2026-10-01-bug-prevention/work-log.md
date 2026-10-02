# Work History: Preventing Recurring Boundary Bugs

See [plan.md](plan.md) for current scope and status. The original one-line notes did not preserve test commands/results; no pass is inferred from test additions.

## Expand conformance coverage and repository guidance

**Commits:** 2026-10-02 00:01:17 `acc10bb`; 2026-10-02 00:04:13 `830c625`; 2026-10-02 00:29:39 `2311d20`; 2026-10-02 00:51:10 `0cb4c3b`.

**Change:** Added lifecycle/selector conformance, older-attempt CLI coverage, cross-command exit-code filter coverage, and repository guidance for preventing recurring bugs and testing package/interface boundaries. Commit `2311d20` also advances the [job-filter plan](../2026-09-29-job-filters/work-log.md).

**Reason:** Make observable contracts and historical-result behavior harder to regress, and record recurring development lessons.

**Plan impact:** Advanced contract-coverage work and added repository-level development/testing guidance.

**Validation:** Commits add conformance tests; original notes do not record whether tests ran or passed.

**Remaining:** Continue from the partial contract rows in `contracts/README.md`; do not mistake test additions for full coverage or a successful run.
