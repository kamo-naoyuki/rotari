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
