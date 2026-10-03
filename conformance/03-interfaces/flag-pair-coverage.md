# CLI flag-pair coverage

The entry point is [flag_pairs_test.go](flag_pairs_test.go), with isolated file
adapters in [flag_pair_files_test.go](flag_pair_files_test.go), restored
mutation adapters in [flag_pair_mutations_test.go](flag_pair_mutations_test.go),
and additional witnesses in [flag_pair_projections_test.go](flag_pair_projections_test.go). Tests obtain flags
from the built binary's `schema --json`, not from CLI implementation imports.
The staged rollout is tracked in the
[development plan](../../development/2026-10-03-cli-option-interactions/plan.md).

## Current layers

- `TestCLIFlagPairInventory` enumerates all 6,481 unordered flag-name pairs.
  Sorted flag-name fingerprints require an explicit coverage review when a
  command or flag is added, removed, or renamed. This is not exhaustive value
  coverage; changes to descriptions/types without a name change are not
  detected by the fingerprint.
- `TestCLIFlagPairs` executes both orders of all 741 `show`, 21 `jobs`, 15
  `check`, and 6 `lineage` pairs, from the same read-only fixture, for 1,566
  command invocations. `check` has a runnable restored queue, and `lineage`
  has a fixed run positional (no duplicate location flags are injected). It
  compares exit code, stdout, and stderr exactly. Completed jobs, a fixed
  fixture, and non-terminal output avoid dynamic elapsed-time and pager output.
  No fields or IDs are scrubbed. A five-second per-process deadline fails the
  case and kills/waits for the child; it is not an accepted outcome.
- `TestCLIFlagPairSamples` checks standalone samples, explicit boolean false,
  and representative repeated result/exit-code values. Only diagnosed mode
  incompatibilities are accepted as nonzero outcomes; parser/sample errors,
  missing state, panic/internal errors, and timeouts fail.
- `TestCLIFlagPairFiles` executes the 15 `config` and 21 `export` pairs in
  both orders (72 invocations). Each invocation starts without its isolated
  output directory. Observations include stdout/stderr, file presence, content,
  and permissions; queue/history contents and file modes must not change.
  Config uses a fallback output outside the basedir to avoid a prompt, except
  in list mode or when output itself is under test. Output samples use a TOML
  extension so notification output is valid; format samples explicitly select
  JSON. Only the exact notification-format rejection and export template/source
  usage rejection supplement the existing incompatibility classifier.
- `TestCLIFlagPairFileSamples` checks each advertised file-adapter flag alone.
  `TestCLIFlagPairFileObservability` compares stdout and file content for each
  of YAML, TOML, and JSON, using a neutral output suffix to prove explicit format
  selection rather than extension inference.
- `TestCLIFlagPairObservability` checks 23 selection flags against JSON, logs,
  failed logs, and report output (92 combinations), plus `jobs --format` with
  two `--since` values. Accepted combinations must match the job table's IDs
  and have a distinguishing witness. Explicit incompatibility errors are
  valid outcomes, not successful selection checks.
- `TestCLIFlagPairMutations` executes the 136 `remove`, 21 `reset`, and 21
  `delete` pairs in both orders (356 invocations) from a fixture with two
  finished runs and a restored queue plus one queue-only job. Before every
  invocation the whole environment root is restored to the initial snapshot:
  paths, contents, modes, and modification times (latest-run lookup reads
  directory times). Restoration rewrites only entries that differ from the
  previous post-invocation snapshot and then verifies every entry's type,
  mode, size, and time; on NFS a full rewrite per invocation took 5m13s,
  the differential restore 41s. Observations are the process result and the
  full tree, with only `meta.json`'s `updated_at` dropped. Rejections and dry
  runs must leave the tree byte-identical. A guard revision printed by
  `--dry-run`/`--if-revision` is checked against `check --json` on the
  resulting state before it is normalized for order comparison. The adapter
  supplies `remove --all` or `delete --run-id` only when no selector is under
  test. Two exact usage rejections supplement the classifier: `remove` with
  two selector kinds, and `delete --all` with `--run-id`.
- `TestCLIFlagPairMutationObservability` (SEL-7) checks that each `remove`
  selector and definition filter has an effect, that `--filter-stage` and
  `--filter-matrix` act as their short forms, and that every selector with
  every filter removes exactly the intersection of what each removes alone, in
  both orders. It requires each option to change some combination, so the
  fixture can tell an ignored option apart. It also checks that `--run-id`
  restores that run's snapshot, `reset --quiet` keeps history and suppresses
  output, and `delete --run-id` removes one run while `--all` removes every
  run. In temporary worktrees, a build that ignores remove filters everywhere
  failed the effect checks, and one that ignores them only with a direct
  selector failed eight intersection checks.
- Additional witnesses prove deep checks reject a missing executable even
  with quiet/JSON output, quiet success text stays suppressed while requested
  check JSON remains emitted, export's explicit run source wins over a distinct
  current queue with file output, and template/notification modes produce their
  own distinct content. These are not blanket equal-output exceptions.
- `TestCLIFlagPairEdits` executes all 300 `add`, 703 `change`, 496 `copy`, and
  21 `import` pairs (1,520 pairs; 3,040 invocations). Each flag order starts
  from the restored root, and rejected edits/previews must leave it byte-identical.
  Import plan revisions are checked against `check` before normalization. Newly
  allocated command/matrix IDs use a one-to-one mapping in encounter order;
  existing fixture IDs and provenance remain literal. A normalization check
  guards against lost identity/collision distinctions.
- Edit effect witnesses verify 19 added command fields survive `--quiet`,
  ten change settings have visible values, copy's failed/stage selection is the
  same intersection in both orders and in long forms, and JSON import preview
  leaves state unchanged while overwrite applies the manifest. These focused
  witnesses supplement the pair loop, rather than treating a successful exit as
  proof that each option took effect.

The fixture uses public `add`/`run`/`copy` commands. It contains successes,
distinct failure codes (1, 3, 7, 9), two failing tasks in a three-task array,
two matrix members, three stages, and different stdout/stderr content. The
robustness tests also restore a real queue so queue scopes resolve. These are
read-only tests after setup, so baselines do not need separate state copies.

Run a single pair using the subtest path (the plus sign must be escaped in a
Go test regular expression), or run the complete `TestCLIFlagPair` prefix with
verbose output. The tests report actual inventory and pair-loop runtime.
Neither short mode nor the race run silently drops pair cases.

## Intentional equivalences

The two reasoned entries in `pairEquivalences` are limited to failed-result
samples: `--failed-logs` already selects failed jobs, making `--failed` and
`--filter-result failed` redundant. They exempt only the baseline-change
assertion. Job-ID parity and robustness still apply. Unknown/duplicate entries,
empty reasons, a new rejection, or lost equivalence fail the checks.

Other equal results are not globally allowlisted. Most pairs currently have
only robustness/order coverage; they do not claim a semantic ignore oracle.

## Deferred command adapters

Every pair is inventoried, and thirteen commands have execution adapters. The
remaining 3,964 pairs are **not executed** by this suite.

| Commands | Pairs | Required next work |
| --- | ---: | --- |
| `run`, `retry` | 3,315 | Independent run state, harmless execution, fake scheduler settings, async cleanup, persisted observations |
| `cancel`, `suspend`, `resume`, `unlock`, `wait` | 559 | Active/interrupted fixtures, barriers, signals, prompts, and bounded cleanup |
| `gc`, `server`, `web`, `mcp`, `diagnose` | 90 | Isolated registry/daemon/stdio/HTTP adapters; fake external diagnosis services |
| `schema`, `completion`, `guide`, `version`, `env` | 0 | Fewer than two advertised flags; subcommand/positional coverage is separate |

The 2,517 executed pairs consist of 783 read-only, 36 file-output, 178
mutation, and 1,520 edit pairs. The edit pair loop accepted 1,187 and explicitly
rejected 333 pairs in 3,040 invocations; one run took 4m01s including setup.
The whole uncached interface package took 438.88s in the repository check.

## Remaining observation gaps

- Edit samples represent a single value and a finished source fixture. Clear/set
  precedence, dependency-cycle effects, append-vs-overwrite collision behavior,
  source/destination conflicts, stdin import, new-project creation, repeated
  values, and boolean-false behavior need additional witnesses. Copy filters for
  host/time/diagnosis and unfinished may give a precise empty-selection error;
  that is an explicit rejection, not positive semantic coverage. The injected
  overwrite mode does not test omission/confirmation.
- Only the finished run view is used for mode/selector comparisons. Queue-only
  `filter-new`/`filter-changed` are checked for explicit rejection in that view,
  not for semantic selection on a modified queue.
- Filters rejected with output modes have rejection coverage, not a positive
  selection witness. All-host and broad time-window samples are useful for
  parsing/order but would need additional fixtures to prove narrowing.
- The generic fixture does not cover unfinished jobs, missing hosts, changed
  definitions, diagnosed failures, older runs, or carried results. The focused
  [log regression](log_selection_test.go) covers older/latest runs and carried
  successful output; existing selector tables remain authoritative for their
  broader variants. This suite is not a substitute for those tables.
- Direct job/task/attempt selectors, streaming behavior, location precedence,
  and equivalent routes through Python/Web need dedicated semantic checks.
- The table is an independent rendering oracle, not a complete independent
  implementation of selection. A bug shared by table and output can escape;
  contract tables with explicit expected IDs are still necessary.

## Initial triage

Initial harness failures were an assumed single ID for matrix `add`, an invalid
diagnosis sample, missing queue scopes, and incomplete classification of
existing explicit mode rejections. These were repaired, not allowlisted.

The first semantic run exposed a confirmed bug in `show --logs`: both `--failed`
and `--filter-result failed` were accepted but ignored. The existing log
selector now receives the parsed failed selection. Focused unit and external
regressions failed before the fix, including carried output. See the record in
[development/ISSUES.md](../../development/ISSUES.md).

The JSON/failed witness also fails against `fddf05a`, immediately before the
historical JSON selection fix, with a job-ID mismatch (not a build failure).
Only current test code and test-harness support were copied into that temporary
worktree; the old production CLI was unchanged. The worktree was removed.

After the fix, a measured full pair run accepted 463 pairs and explicitly
rejected 299, with no unclassified/order failures. Its 1,524 invocations took
29.84 seconds including setup (other runs under concurrent load were slower).
This is an observation, not a fixed CI runtime bound.

No need for production compatibility declarations has been established yet.
Expand mutation/execution adapters and observation witnesses before claiming
the first milestone's full completion.

The next expansion added `check`, `lineage`, `config`, and `export`: 57 more
pairs (50 accepted, 7 explicitly rejected). The combined measured runs accepted
513 and rejected 306 of 819 pairs with 1,638 invocations. Read-only pairs took
38.25 seconds including setup, and file pairs took 6.40 seconds including their
own fixture. No new production bug or equality exception was found. No command
compatibility declaration was added; implementation behavior is unchanged.

The mutation expansion added `remove`, `reset`, and `delete`: 178 pairs (158
accepted, 20 explicitly rejected), 41 seconds with the differential restore.
No option was found silently ignored and no production code changed. Triage
found one diagnostic gap, recorded as an open issue: `remove` rejects two
selector kinds, including `--all` with `--filter-stage`, with only its usage
line. Two harness defects were repaired, not allowlisted: the fixture assumed a
supervisor was still running for `server shutdown`, and the first exclusion
sample excluded a stage no selector picks, so it could not detect an ignored
`--filter-not-stage`.

The edit expansion added `add`, `change`, `copy`, and `import`: 1,520 pairs
(1,187 accepted, 333 explicitly rejected) without a production bug or equality
exception. Initial failures were harness defects and were corrected: standalone
samples now restore state, preventing prior edits from clearing later matrix
selectors; add uses a name that does not collide with a matrix base name;
dependency validation failures are classified narrowly; import revisions are
verified before normalization; and newly allocated IDs are normalized without
rewriting existing IDs. `TestCLIFlagPairEditSamples` exercises every individual
schema flag. Tests for definition fields, copy selector intersection, JSON
preview, and `--quiet` provide semantic witnesses; the remaining cases above are
not claimed as ignored-option coverage.
