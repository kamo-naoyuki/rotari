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
