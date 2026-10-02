# CLI option interaction work log

## Planning

**Commit:** `996781b` — 2026-10-03T03:38:42+09:00

- **Change:** Added this work's plan, prioritizing schema inventory, safe
  pair execution, observable selection witnesses, and initial failure triage.
- **Reason:** Recurring option-combination and interface/documentation bugs
  need systematic checks without assuming equal output proves ignored flags.
- **Plan impact:** Established a phased rollout; compatibility declarations,
  documentation examples, and interface parity remain follow-ups.
- **Validation:** Diff/Markdown/link checks, `scripts/check.sh --short`, and
  `scripts/check.sh` passed. Pre-commit was unavailable.
- **Remaining:** Implementation and initial execution, subsequently started
  below.

## Correct result selection for run-wide logs

**Commit:** `e14514b` — 2026-10-03T04:21:29+09:00

- **Change:** The CLI forwards parsed failed-result selection to its existing
  run-wide log selector. Expanded the unit test to both orders and short/long
  selectors; added `TestShowLogResultSelection` for older/latest runs and
  carried successful output. Updated SEL-11, the inspection guide, and the
  resolved issue record. No Web/Python implementation changed: Web logs name
  one job, and Python structured `show` uses JSON.
- **Reason:** The new output/selector witness found `--logs --failed` and
  `--logs --filter-result failed` accepted but printing successful jobs.
- **Plan impact:** First confirmed production bug from semantic pair checks;
  fixed rather than adding an equality exception or compatibility table.
- **Validation:** Focused unit and conformance regressions failed before the
  fix with successful jobs in output (exit 1 for both test runs), then passed.
  CLI/interface packages and root conformance checks passed uncached. The
  complete `scripts/check.sh` including race passed. Initial short checks
  failed in concurrently edited wait-selection tests, with `project ... has
  no active run`; a later short check passed. Original failure logs and exit
  codes were retained; this is not attributed to flaky pair tests. Pre-commit
  is not installed; gofmt and diff checks were run instead.
- **Remaining:** The staged pair-check rollout described in
  [plan.md](plan.md) and
  [coverage notes](../../conformance/03-interfaces/flag-pair-coverage.md).

## Schema inventory and read-only interaction checks

**Commit:** `644ebae` — 2026-10-03T04:22:15+09:00

- **Change:** Added all-command pair inventory, standalone sample checks,
  read-only `show`/`jobs` adapters, output/selector witnesses, two narrowly
  reasoned equivalences, coverage notes, and updated plan/SEL-11 bookkeeping.
- **Reason:** Separate systematic robustness/order checks from semantic ignore
  detection, which requires a distinguishing fixture and independent renderer.
- **Plan impact:** Generated 6,481 pairs. Executed 762 pairs in both orders;
  the final measured run accepted 463 and explicitly rejected 299 in 1,524
  invocations (29.84 seconds including setup). Ran 92 output/selector cases
  and the formatted jobs window witness. Milestone 1 remains partial.
- **Validation:** Inventory, samples, observation, and complete pair tests
  passed uncached. `json+failed` passed on current production code and failed
  with the expected selected-ID mismatch on `fddf05a`, before the historical
  fix. Current test/support files supplied harness compatibility only; no old
  production file changed, and the temporary worktree was removed. CLI/root
  conformance/interface package checks passed. Full `scripts/check.sh` passed
  including race (interface package: 84.33 seconds normal, 132.05 seconds race).
  Short checks passed on recheck after initial unrelated wait-selection
  failures; see the preceding entry. Final contract/layout checks passed.
  Gofmt, diff, Markdown diagnostics, and relative-link checks passed;
  pre-commit is unavailable.
- **Remaining:** 5,719 deferred pairs need isolated command adapters. Positive
  queue, active/unfinished, host/time/diagnosis, direct-selector, and interface
  witnesses remain explicitly documented in the coverage notes. Later
  milestones have not started.
