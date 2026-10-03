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

## Read projections and isolated file-output adapters

**Commit:** `413f129` — 2026-10-03T04:49:44+09:00

- **Change:** Added `check` and one-run `lineage` to the shared read adapter,
  and isolated `config`/`export` file adapters. Compared both orders' file
  contents, presence, and modes; rejected operations must leave no output,
  and all file-adapter operations must preserve queue/history. Added standalone
  samples and witnesses for three formats, deep checking a missing executable,
  quiet/JSON precedence, run source versus a distinct queue with output, and
  notification/template content. Updated CLI-1 bookkeeping, plan, and coverage.
- **Reason:** Broaden coverage without mistaking read-only success or stdout
  equality for evidence of option effects. Output reset prevents config's
  overwrite protection from contaminating the second order.
- **Plan impact:** Added 57 pairs: 50 accepted and 7 explicitly rejected.
  Now 819 pairs execute in six commands, accepting 513 and rejecting 306 in
  1,638 order-check invocations. Measured read loop: 38.25 seconds; file loop:
  6.40 seconds, including their separate fixtures. No new production bug,
  behavior change, compatibility declaration, or equality exception.
- **Validation:** Focused added cases, all pair-prefixed tests, interface/root
  conformance packages, and contract/layout checks passed uncached. Focused
  race checks passed (interface 72.59 seconds, root conformance 6.01 seconds).
  Short checks initially failed on JavaScript syntax errors in concurrently
  edited Web assets, then passed on recheck. Full checks failed first in Web
  rendering tests with `Unexpected end of input`, then on recheck in
  `TestOrbitGameInteraction` (exit 8). The full script did not reach race;
  focused race was run separately, not presented as a full-script pass.
  Original logs and exit codes were retained. Web files were not changed by
  this work. Gofmt, Markdown diagnostics, relative links, and diff checks
  passed. Pre-commit remains unavailable.
- **Remaining:** 5,662 pairs need safe mutation/execution/control adapters;
  observation gaps and lineage/config mode limits are in the coverage notes.
  Later milestones remain untouched.

## Restored mutation adapters for remove, reset, and delete

**Commit:** `2dea6b8` — 2026-10-03T05:33:44+09:00

- **Change:** Added a mutation adapter that restores the whole environment
  root (paths, contents, modes, directory times) before each invocation and
  compares process results and the resulting tree in both flag orders, with
  only `meta.json`'s `updated_at` dropped. Rejections and dry runs must leave
  the tree byte-identical; guard revisions are checked against `check --json`.
  Added effect witnesses for `remove` selectors, definition filters, long-form
  aliases, selector-filter intersections, `--run-id` restoration, `reset
  --quiet`, and `delete --run-id`/`--all`. Updated SEL-7 bookkeeping, the
  plan, coverage notes, and an open issue.
- **Reason:** Extend pair checks to commands that change queue and history,
  where equal stdout says nothing about effects.
- **Plan impact:** 178 pairs (158 accepted, 20 explicitly rejected); 997 of
  6,481 pairs now execute, 5,484 remain. A full root rewrite per invocation
  took 5m13s on NFS (about 0.7s of each pair was restoring); a differential
  restore verified by a stat walk takes 41-43s. No option was silently
  ignored and no production code changed. Triage recorded one open
  diagnostic issue: `remove` rejects two selector kinds, including `--all`
  with `--filter-stage`, with only its usage line. Two harness defects were
  repaired, not allowlisted: an assumed running supervisor at shutdown, and an
  exclusion sample (`setup`) no selector picks.
- **Validation:** Mutation pairs, effect witnesses, and inventory passed
  uncached. In temporary worktrees, a build ignoring remove filters failed the
  effect checks, and one ignoring them only with a direct selector failed
  eight intersection checks; both worktrees were removed. Interface and root
  conformance packages and focused race passed. `scripts/check.sh --short`
  and the full `scripts/check.sh`, including race, passed. Gofmt, vet,
  diagnostics, and diff checks passed; pre-commit remains unavailable.
- **Remaining:** Extend the mutation adapter to `add`, `change`, `copy`, and
  `import` (1,520 pairs), then run/control adapters; the `remove` diagnostic
  issue in `development/ISSUES.md`.

## Add, change, copy, and import pair adapter

**Commit:** `34803f61` — 2026-10-03T12:16:29+09:00

- **Change:** Added the remaining queue-edit adapters for `add`, `change`,
  `copy`, and `import`, using the NFS-aware fixture restore from the prior
  milestone. Added representative field/effect witnesses, copy result/stage
  intersection, import JSON preview/apply, guarded revision verification, and
  ID-preserving normalization. Registered copy selectors under SEL-4 and the
  guarded import witness under CLI-7.
- **Reason:** These commands accounted for 1,520 inventoried pairs and mutate
  persistent queue or history state; order/output alone cannot show whether
  their options took effect.
- **Plan impact:** 1,520 pairs, 1,187 accepted and 333 explicitly rejected in
  3,040 invocations; total executable coverage is 2,517 pairs in 13 commands,
  leaving 3,964 deferred. No production bug or equivalence exception found.
- **Validation:** Standalone schema-flag samples and focused semantic witnesses
  passed; all 1,520 edit pairs passed in both orders (4m01s normal). The
  uncached interface package passed in 358.36s; the full script's normal and
  race interface runs passed in 438.88s and 451.11s. Final focused race
  verification passed in 335.13s, including every edit pair (5m30s), ID
  normalization, copy selection, and import JSON observations. Contract/layout
  checks passed after restoring their correspondence to actual `covers` calls.
  The first contract check encountered a concurrently added DUR-7 test before
  its definition existed; no unrelated contract was edited to suppress it.
  `scripts/check.sh --short` passed. The full script exited 1 during race:
  concurrently edited carried-result code had missing symbols and mismatched
  `runlineage.IsCarried` signatures across jobstatus/projectrun/runview/Web.
  This is not a full-suite pass; original logs and exit codes were retained.
  Gofmt, editor diagnostics, diff checks and relative links passed. Pre-commit
  remains unavailable. Concurrent carried-result/MCP and projection edits are
  excluded from this work's commit.
- **Remaining:** Run/control adapters, more semantic value/boolean/repeated
  witnesses, and all gaps recorded in the coverage notes.

## Run and retry dry-run pair adapter

**Commit:** `09d19f3` — 2026-10-03T14:01:57+09:00

- **Change:** Added schema-sampled `run`/`retry` pairs through `--dry-run`, with
  a semaphore limiting concurrent subprocesses and full fixture non-interference
  checks. Added standalone value samples, result/stage intersection and
  partial-array plan witnesses, run-name output, and async/dry-run regression.
  Updated CLI-8/9/10, selector/guard contract status, running guide, and issue
  history.
- **Reason:** Complete the next highest-priority command group while ensuring
  pair checks do not launch jobs or contact schedulers.
- **Plan impact:** Executed all 3,315 `run`/`retry` pairs in both orders;
  3,016 accepted, 299 explicitly rejected (6,630 invocations). A run took
  88.95s at concurrency four. Total executable inventory is 5,832/6,481, with
  649 control/external pairs deferred.
- **Validation:** Before the fix, `run` and `retry` with `--async --dry-run`
  returned exit 0 in both orders and printed a plan. The focused regression
  failed for all four cases, then passed after a shared guard was added.
  The run/retry pair matrix, samples, selector/partial-array witness, run-name
  witness and CLI contract/layout checks passed. The latest focused matrix
  passed in 114.83s (package 118.58s). An unlimited-parallel trial timed out
  previews; the suite bounds concurrency at four and then passed.
  An earlier short check passed, but subsequent short and full checks failed
  while a concurrent undocumented-option guard misidentified commands as
  `config` and refused retry's existing result selectors. The full check also
  hit the interface package's default ten-minute timeout in an existing show
  observability case; it did not reach race. These are not full-check passes,
  nor the carried-result race build failure from the preceding edit phase.
  Full logs retain the original failures. After the owning thread removed
  the guard, the final `GOFLAGS='-timeout=30m' scripts/check.sh` passed
  (exit 0), including vet, normal tests, and race. The interface package
  passed in 568.32s normal and 676.47s race; the larger package timeout
  accommodates the expanded matrix without dropping cases. Contract and
  document-link tests, gofmt, and diff checks also passed. Pre-commit is
  unavailable. Unrelated projection edits are excluded from the commit.
- **Remaining:** 649 pairs in control/daemon/external commands, retry-specific
  selection expectations, actual execution and scheduler effects, plus the
  gaps listed in the coverage report.

## Unlock recovery pair adapter

**Commit:** `29d5f7f` — 2026-10-03T15:32:40+09:00

- **Change:** Added all six schema-advertised `unlock` pairs in both orders
  and four standalone samples. Each invocation restores the finished fixture,
  seeds coherent interrupted metadata and a synthetic stale lock, then checks
  lock removal, recovery metadata, and unchanged queue/history/registry state.
  Safety witnesses check live-local refusal with the test process's PID,
  an existing sibling run mismatch, remote recovery, and lockless recovery,
  each in both project/run flag orders. Registered the tests under SAFE-4.
- **Reason:** Begin control-command coverage without launching live jobs or
  sending control signals. Existing fixture execution stops before synthetic
  recovery cases begin; public CLI subprocesses keep the five-second bound.
- **Plan impact:** Six accepted pairs, twelve pair-loop invocations; inventory
  now executes 5,838/6,481 pairs across sixteen commands, leaving 643 deferred.
  No production behavior changed and no ignored-option bug was found.
  Location samples still share fixture defaults and an empty config; they do
  not independently prove override or config effects. Next adapter: `wait`.
- **Validation:** Focused samples, all pairs, safety cases, inventory,
  contract/layout, and document-link checks passed. The pair test took 2.85s
  including setup in one focused run. Final focused race after extracting
  helpers to resolve complexity diagnostics passed uncached (10.92s package).
  The first short check exited 1 when the existing file-pair snapshot lost
  `server.pid` between enumeration and stat; recorded in `../ISSUES.md`, with
  no unsupported root-cause claim. Its complete log and exit code remain
  retained. A subsequent short check passed. Full checks with
  `GOFLAGS='-timeout=30m' scripts/check.sh` passed vet, normal, and race
  (exit 0; interface package 528.73s normal, 632.87s race), before the final
  behavior-preserving helper extraction. Final focused race and contract/link
  rechecks passed afterward. Gofmt, diff checks, and new-file diagnostics
  passed; pre-commit remains unavailable. Unrelated projection edits were
  excluded from the commit.
- **Remaining:** 643 control/external pairs, alternate-location/config
  witnesses, and the existing observation gaps. Investigate file-pair fixture
  quiescence separately; a successful rerun does not resolve that issue.
