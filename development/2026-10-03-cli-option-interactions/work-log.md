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

## Wait adapter and per-package runtime budget

**Commit:** `dd5cd67` — 2026-10-03T17:27:54+09:00

- **Change:** Added all 21 `wait` pairs in both orders and seven standalone
  samples. Completed failed runs must return the fixture's exit 1 with empty
  stderr, correct text/JSON output, and unchanged state. Synthetic remote
  locks without a summary demonstrate timeout, CLI timeout precedence over
  config/environment, and early final-failure output in text and JSON.
  Added CLI-12 and extended RUN-6 coverage; no production semantics changed.
- **Reason:** Extend control coverage safely while preventing the combined
  interface package from exceeding Go's default ten-minute timeout.
- **Plan impact:** 5,859 executed of 6,481 pairs; 622 deferred. The user chose
  package splitting rather than changing CI timeouts. Fixture/schema/bounded
  invocation/NFS restoration moved to `conformance/support/pairs.go`; edit
  suites moved to `pairedits`, run/recovery/wait suites to `pairruns`, while
  read/file/projection checks stay in the parent group. Contract discovery
  already recurses into child packages, so layout mapping is unchanged.
  All 24 pre-existing pair test functions are retained, with no short/race
  exclusions and no change to CI timeout settings.
- **Validation:** Wait samples, 21 pairs, semantic witnesses, focused race,
  contract/layout, document links, and uncached architecture checks passed.
  Before splitting, default-timeout short checks failed at 606.14s, while
  a 30-minute-budget run passed; logs retain the failure and original exit 1.
  After splitting, uncached normal packages passed in 104.87/354.09/156.48s;
  uncached race packages passed in 85.56/414.96/148.14s. Full
  `scripts/check.sh` passed with the default timeout, including vet and race,
  exit 0. Gofmt and diff checks passed; pre-commit is unavailable. Unrelated
  projection edits were excluded from the commit.
- **Remaining:** 622 control/external pairs and the documented observation
  gaps, including actual run synchronization and retry-in-progress behavior.

## Isolated GC and server registry adapters

**Commit:** `e65349a` — 2026-10-03T17:48:57+09:00

- **Change:** Added three GC and three server flag pairs (18 invocations,
  since server pairs run under both status and list), plus standalone samples.
  A synthetic registry distinguishes existing run data, orphan runs, missing
  basedirs, malformed records, and stale server records. Tree comparisons
  require previews and rejections to preserve everything; GC apply and server
  list may remove only their specifically expected records. An alternate
  empty-registry witness proves config selection and CLI masterdir precedence
  in both flag orders. No supervisor starts or receives a signal.
- **Reason:** Expand remaining adapters without external daemons or risking
  live jobs. Server's schema combines subcommand flags, so unsupported flags
  are diagnosed by the actual subcommand parser, not accepted generically.
- **Plan impact:** 5,865/6,481 pairs execute across nineteen commands;
  616 remain (532 cancel/suspend/resume, 84 web/MCP/diagnose). Server shutdown
  and live lease effects remain separate from this status/list matrix.
- **Validation:** Focused samples, all six pairs, location effects, and
  inventory passed normally and uncached with race (4.20s focused package).
  Contract/layout and document-link tests passed. Full `scripts/check.sh`
  passed with the default timeout, including vet, normal, and race, exit 0;
  interface packages took 76.79/385.99/132.79s normal and
  175.50/524.28/253.32s race. Independent review found no blockers. Gofmt,
  new-file diagnostics and diff checks passed; pre-commit remains unavailable.
  Parallel option-description, queueops/resolve, projection, and artifact
  plan changes were excluded from this work's commits.
- **Remaining:** 616 pairs and all documented mode/semantic gaps. Next:
  finite MCP stdin/stdout requests, then isolated Web and diagnosis adapters.

## MCP initialization and tool-discovery pair

**Commit:** `0ea7fc4` — 2026-10-03T18:09:08+09:00

- **Change:** Added the one `mcp` pair (`--config`/`--masterdir`) in both
  orders and standalone samples. The public subprocess receives only
  JSON-RPC initialize, initialized notification, and tools/list. It matches
  replies by ID while ignoring notifications, compares server identity,
  capabilities, and sorted tool names, then closes stdin and requires a clean
  bounded exit with empty stderr and unchanged fixture state.
- **Reason:** Exercise stdio-mode option parsing and protocol startup without
  calling any project-writing or run-control MCP tool. Existing `startMCP`
  fixes arguments and lacks bounded stderr/process cleanup, so the isolated
  handshake uses its own bounded lifecycle helper.
- **Plan impact:** Inventory is 5,866/6,481 pairs across twenty commands;
  615 remain. This does not prove `--masterdir` precedence inside tools,
  registry-derived tool effects, or any MCP operation behavior.
- **Validation:** Focused handshake and standalone samples passed normally and
  with race (3.14s); inventory, contract/layout, and document links passed.
  Full `scripts/check.sh` after adding this adapter passed vet, normal,
  and race with the default timeout. Interface packages took
  108.24/365.50/163.75s normal and 84.04/452.51/162.45s race.
  Gofmt and diff checks passed; pre-commit unavailable. Unrelated projection
  and artifact-discovery edits were excluded from the implementation commit.
- **Remaining:** 615 web/diagnosis/control/external pairs and the MCP tool-side
  master-directory observation gap.

## Web static-export pair adapter and explicit mode guards

**Commit:** `8fe6304` — 2026-10-03T21:29:53+09:00

- **Change:** Added a separate `pairweb` package with all 28 Web pairs in
  both orders and eight standalone samples. Static exports compare asset paths
  and modes, preserve source state, and independently check the notifications
  toggle. Static mode now rejects explicitly supplied host, port, auth-token,
  and allow-control options from CLI, environment, or configuration. Updated
  WEB-3/4, schema descriptions, generated CLI references, guides, and coverage.
- **Reason:** Before the fix, static exports accepted all four live-server-only
  options and silently ignored them. Random HTML session tokens prohibit a
  raw byte-equality oracle; the notification witness checks meaningful content.
- **Plan impact:** Public schema inventory confirms 5,894 executable pairs of
  6,481, with 587 deferred (532 control and 55 diagnose). There are four pair
  packages. This adapter never starts an HTTP server. The inventory, rather
  than earlier handoff arithmetic, is the source of these totals.
- **Validation:** Focused Web tests passed uncached (16.40s normal, 16.65s
  race), as did existing CLI Web mode tests, contract/layout, inventory,
  document links, generated CLI reference synchronization, and diff checks.
  The full script passed vet and normal tests but exited 1 during race in
  `TestAsyncStartHintsWork`, reading a missing `commands.json` immediately
  after async start. The subsequent short script exited 1 for the same error;
  all four pair packages passed. Isolated lifecycle checks have both failed
  and passed, including the latest race check; this is not a full-suite pass
  or a resolved readiness defect. Original full/short logs and exit codes
  were retained, and the issue is recorded in `development/ISSUES.md`.
  Unrelated projection and configuration-link changes remain uncommitted.
- **Remaining:** Diagnose and control adapters, live-server behavior, and the
  semantic/mode gaps listed in the coverage notes.

## Diagnose pair adapter and rule-mode option guard

**Commit:** `fb96c72` — 2026-10-04T00:02:58+09:00

- **Change:** Added `pairdiagnose`: all 55 `diagnose` pairs in both orders and
  standalone samples. LLM-mode invocations go only to an in-process fake
  endpoint that checks the model, language prompt, endpoint path, and
  authentication; local-rule invocations make no request, and the fixture
  must stay unchanged. `diagnose --rules` now rejects `--provider`,
  `--endpoint`, `--model`, and `--language` from CLI, environment, or config
  (CLI-13). Updated the schema description, generated CLI references, guide,
  coverage, and plan; added CLI-4's newly covering test to its status row.
- **Reason:** Before the fix, `--rules` accepted all four LLM options and
  silently ignored them (exit 0).
- **Plan impact:** 6,294/6,826 pairs executed; 532 job-control pairs remained.
- **Validation:** The rule-option test failed on the pre-fix commit in a
  temporary worktree for all four CLI options and the config source, then
  passed. Pair package normal and race, focused CLI diagnose tests, inventory,
  contract/layout, document links, generated CLI reference check, and
  `scripts/check.sh --short` passed (exit 0). The full race script was run
  with the following job-control work.
- **Remaining:** Job-control pairs (next entry).

## Job-control pair adapter and stage/matrix exclusion

**Commit:** `626607e` — 2026-10-04T01:17:35+09:00

- **Change:** Added `pairjobcontrol`: all 171 `suspend`, 171 `resume`, and
  190 `cancel` pairs in both orders against live local runs of five sleeping
  jobs. An independent SEL-8/SEL-12 model predicts the jobs acted on or the
  diagnosed rejection. Suspend/resume share one run reset before each
  invocation; acting cancels get fresh runs, two pairs at a time; expected
  rejections share a run that must stay untouched. Every run is reaped.
  `jobcontrol.Controller.Select` now rejects a stage scope with a matrix
  scope for every caller. Updated SEL-12, coverage, plan, and architecture.
- **Reason:** The adapter found `--stage`/`--filter-stage` with
  `--matrix`/`--filter-matrix` acting on the stage's jobs and silently
  ignoring the matrix, while `show`, `run`, and `copy` reject it.
- **Plan impact:** All 6,826 inventoried pairs now execute; none are deferred.
  A repeated single-value option keeping its last value was recorded in
  `development/ISSUES.md`.
- **Validation:** Before the fix, all three commands failed exactly the four
  stage/matrix pairs (exit 1); after it, the package passed normally and with
  race (141s race), and no sleeping job remained. Unit, inventory,
  contract/layout, and link tests passed. A first full `scripts/check.sh`
  failed in race with five-second invocation timeouts across several pair
  packages while cancels ran four pairs at a time; parallelism was lowered.
  A second failed because a duplicated `package` line appeared in a new file;
  it was removed. The final full `scripts/check.sh` passed (vet, normal,
  race; exit 0). Logs are retained.
- **Remaining:** Semantic/mode gaps listed in the coverage notes.
