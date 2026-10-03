# CLI flag-pair coverage

The entry point is [flag_pairs_test.go](flag_pairs_test.go). Tests obtain flags
from the built binary's `schema --json`, not from CLI implementation imports.
The staged rollout is tracked in the
[development plan](../../development/2026-10-03-cli-option-interactions/plan.md).

## Current layers

- `TestCLIFlagPairInventory` enumerates all 6,481 unordered flag-name pairs.
  Sorted flag-name fingerprints require an explicit coverage review when a
  command or flag is added, removed, or renamed. This is not exhaustive value
  coverage; changes to descriptions/types without a name change are not
  detected by the fingerprint.
- `TestCLIFlagPairs` executes both orders of all 741 `show` pairs and 21 `jobs`
  pairs, from the same read-only fixture, for 1,524 command invocations. It
  compares exit code, stdout, and stderr exactly. Completed jobs, a fixed
  fixture, and non-terminal output avoid dynamic elapsed-time and pager output.
  No fields or IDs are scrubbed. A five-second per-process deadline fails the
  case and kills/waits for the child; it is not an accepted outcome.
- `TestCLIFlagPairSamples` checks standalone samples, explicit boolean false,
  and representative repeated result/exit-code values. Only diagnosed mode
  incompatibilities are accepted as nonzero outcomes; parser/sample errors,
  missing state, panic/internal errors, and timeouts fail.
- `TestCLIFlagPairObservability` checks 23 selection flags against JSON, logs,
  failed logs, and report output (92 combinations), plus `jobs --format` with
  two `--since` values. Accepted combinations must match the job table's IDs
  and have a distinguishing witness. Explicit incompatibility errors are
  valid outcomes, not successful selection checks.

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

Every pair is inventoried, but only `show` and `jobs` have generic execution
adapters. The remaining 5,719 pairs are **not executed** by this suite.

| Commands | Pairs | Required next work |
| --- | ---: | --- |
| `check`, `lineage` | 21 | Read-only projections with command-specific observation and safe sample values |
| `config`, `export` | 36 | Isolate output files and template modes; observe emitted file contents |
| `add`, `change`, `copy`, `delete`, `import`, `remove`, `reset` | 1,698 | Reconstruct independent queue/history state per variant; observe effects and guard rejections |
| `run`, `retry` | 3,315 | Independent run state, harmless execution, fake scheduler settings, async cleanup, persisted observations |
| `cancel`, `suspend`, `resume`, `unlock`, `wait` | 559 | Active/interrupted fixtures, barriers, signals, prompts, and bounded cleanup |
| `gc`, `server`, `web`, `mcp`, `diagnose` | 90 | Isolated registry/daemon/stdio/HTTP adapters; fake external diagnosis services |
| `schema`, `completion`, `guide`, `version`, `env` | 0 | Fewer than two advertised flags; subcommand/positional coverage is separate |

Deferred cases are not equality exceptions and are not counted as passes.
New commands/flags require revisiting this accounting. Subcommands are not
expanded by the inventory yet; the current execution targets have none.

## Remaining observation gaps

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
