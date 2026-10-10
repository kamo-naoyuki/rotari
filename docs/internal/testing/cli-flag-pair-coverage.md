# CLI flag-pair coverage

The interface conformance suite inventories and executes CLI flag pairs from
`schema --json`. Its shared harness (schema inventory types, finished-run
fixture, bounded invocation, and NFS-aware tree restoration) is
[conformance/support/pairs.go](../../../conformance/support/pairs.go). The suites
run in six packages so the full pair matrix does not put any single package
near Go's default ten-minute test timeout:

- [conformance/03-interfaces](../../../conformance/03-interfaces/): inventory,
  read-only pairs, selection witnesses, config/export, and projections.
- [pairedits](../../../conformance/03-interfaces/pairedits/): restored mutation
  and edit adapters and their effect witnesses.
- [pairruns](../../../conformance/03-interfaces/pairruns/): `run`/`retry`
  previews, `unlock`, `wait`, and `note`.
- [pairweb](../../../conformance/03-interfaces/pairweb/): safe static exports;
  it never starts an HTTP server.
- [pairjobcontrol](../../../conformance/03-interfaces/pairjobcontrol/):
  `cancel`, `suspend`, and `resume` against live local runs that are cancelled
  and reaped afterward.

Tests obtain flags from the built binary's `schema --json`, not from CLI
implementation imports. The staged rollout is tracked in the
[development plan](../../../development/2026-10-03-cli-option-interactions/plan.md).

## Current layers

- `TestCLIFlagPairInventory` enumerates all 6,902 unordered flag-name pairs
  from the command schema. Sorted flag-name fingerprints require an explicit
  coverage review when a command or flag is added, removed, or renamed. This
  is not exhaustive value coverage; changes to descriptions/types without a
  name change are not detected by the fingerprint.
- `TestCLIFlagPairs` executes both orders of all `show`, `basedirs`, `projects`,
  `runs`, `jobs`, `check`, `info`, and `lineage` pairs, from the same read-only fixture.
  `check` has a runnable restored queue, and `lineage` has a fixed run
  positional (no duplicate location flags are injected). It compares exit
  code, stdout, and stderr exactly. Completed jobs, a fixed fixture, and
  non-terminal output avoid dynamic elapsed-time and pager output. No fields or
  IDs are scrubbed. A five-second per-process deadline fails the case and
  kills/waits for the child; it is not an accepted outcome.
- `TestCLIFlagPairSamples` checks standalone samples, explicit boolean false,
  and representative repeated result/exit-code values. Only diagnosed mode
  incompatibilities are accepted as nonzero outcomes; parser/sample errors,
  missing state, panic/internal errors, and timeouts fail.
- `TestCLIFlagPairFiles` executes the 15 `config` and 21 `export` pairs in both
  orders (72 invocations). Each invocation starts without its isolated output
  directory. Observations include stdout/stderr, file presence, content, and
  permissions; queue/history contents and file modes must not change. Config
  uses a fallback output outside the basedir to avoid a prompt, except in list
  mode or when output itself is under test. Output samples use a TOML extension
  so notification output is valid; format samples explicitly select JSON.
  Only the exact notification-format rejection and export template/source
  usage rejection supplement the existing incompatibility classifier.
- `TestCLIFlagPairFileSamples` checks each advertised file-adapter flag alone.
  `TestCLIFlagPairFileObservability` compares stdout and file content for each
  of YAML, TOML, and JSON, using a neutral output suffix to prove explicit
  format selection rather than extension inference.
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
  previous post-invocation snapshot and then verifies every entry's type, mode,
  size, and time; on NFS a full rewrite per invocation took 5m13s, the
  differential restore 41s. Observations are the process result and the full
  tree, with only `meta.json`'s `updated_at` dropped. Rejections and dry runs
  must leave the tree byte-identical. A guard revision printed by
  `--dry-run`/`--if-revision` is checked against `check --json` on the resulting
  state before it is normalized for order comparison. The adapter supplies
  `remove --all` or `delete --run-id` only when no selector is under test. Two
  exact usage rejections supplement the classifier: `remove` with two selector
  kinds, and `delete --all` with `--run-id`.
- `TestCLIFlagPairMutationObservability` (SEL-7) checks that each `remove`
  selector and definition filter has an effect, that `--filter-stage` and
  `--filter-matrix` act as their short forms, and that every selector with
  every filter removes exactly the intersection of what each removes alone,
  in both orders. It requires each option to change some combination, so the
  fixture can tell an ignored option apart. It also checks that `--run-id`
  restores that run's snapshot, `reset --quiet` keeps history and suppresses
  output, and `delete --run-id` removes one run while `--all` removes every
  run. In temporary worktrees, a build that ignores remove filters everywhere
  failed the effect checks, and one that ignores them only with a direct
  selector failed eight intersection checks.
- Additional witnesses prove deep checks reject a missing executable even
  with quiet/JSON output, quiet success text stays suppressed while requested
  check JSON remains emitted, export's explicit run source wins over a
  distinct current queue with file output, and template/notification modes
  produce their own distinct content. These are not blanket equal-output
  exceptions.
- `TestCLIFlagPairEdits` executes all 351 `add`, 780 `change`, 496 `copy`, and
  21 `import` pairs (1,648 pairs; 3,296 invocations). Each flag order starts
  from the restored root, and rejected edits/previews must leave it
  byte-identical. Import plan revisions are checked against `check` before
  normalization. Newly allocated command/matrix IDs use a one-to-one mapping
  in encounter order; existing fixture IDs and provenance remain literal. A
  normalization check guards against lost identity/collision distinctions.
- Edit effect witnesses verify 19 added command fields survive `--quiet`, ten
  change settings have visible values, copy's failed/stage selection is the
  same intersection in both orders and in long forms, and JSON import preview
  leaves state unchanged while overwrite applies the manifest. A separate add
  witness verifies `--matrix-exclude` persists its rule and removes the
  requested combination. These focused witnesses supplement the pair loop,
  rather than treating a successful exit as proof that each option took effect.
- `TestCLIFlagPairPreviews` executes all 1,830 `run` and 1,830 `retry` pairs
  in both orders through `--dry-run`. Four concurrent cases compare output and
  exit status and verify that fixture state stays unchanged. Standalone
  samples and dedicated async-conflict, run-name, and selection witnesses
  supplement this mode-specific matrix; no job or scheduler starts.
- `TestCLIFlagPairWait` executes all 28 `wait` pairs in both orders against a
  single finished failed run, with an explicit two-second timeout on every
  invocation. The expected exit 1 is the fixture run's result, not a diagnosed
  option rejection; stderr must remain empty, output must match the selected
  text/JSON mode (`--quiet` suppresses text completion but preserves JSON), and
  the fixture tree must remain unchanged. Separate synthetic-running witnesses
  remove the summary and add a remote lock without starting a process: one
  confirms timeout and CLI-over-environment/config precedence while retaining
  persisted progress, and `--until-failure` returns early with final failure
  groups in text and JSON. These checks do not exercise live-run
  synchronization or retrying failures.
- `TestCLIFlagPairRegistries` executes three `gc` and three `server` pairs
  (18 invocations: server pairs run for both `status` and `list`). GC previews
  preserve the entire tree; apply removes only the orphan run and missing
  basedir records, keeping existing run data and malformed records. Server
  list removes only a synthetic stale server record; status reports no running
  supervisor. Unsupported subcommand flags must produce the exact parser
  rejection category and preserve state. No supervisor starts or receives a
  signal. An alternate-registry witness proves config selection and explicit
  masterdir precedence in both orders. Shutdown and live lease behavior are
  still outside this adapter.
- `TestCLIFlagPairWeb` executes all 45 `web` pairs in static-export mode, with
  generated output compared by relative asset paths and permissions in both
  orders. Server-only options (`--host`, `--port`, `--auth-token`,
  `--allow-control`, and `--artifact-root`) are explicitly rejected whenever
  supplied by the CLI, environment, or config rather than silently ignored.
  `--notifications` is checked as a content-affecting option on the exported
  browser toggle. `web` never starts a live HTTP server in this adapter.
- `TestCLIFlagPairSuspend`, `TestCLIFlagPairResume`, and
  `TestCLIFlagPairCancel` execute all 171 `suspend`, 171 `resume`, and 190
  `cancel` pairs in both orders against live local runs of five sleeping jobs
  (a two-task array, a single job, and a two-member matrix in three stages).
  Every sample selects a distinguishing subset, and an independent model of
  SEL-8/SEL-12 predicts the jobs acted on or the diagnosed rejection. Suspend
  and resume share one run and reset every job's state before each invocation,
  observing `scheduler_status.json`; cancels expected to act each get a fresh
  run and are observed through `finished_at`, while expected rejections share
  one run that must stay untouched. `--yes` is injected unless under test (the
  terminal prompt is covered by the selector table). Every run is cancelled
  and its processes reaped when its test ends. This adapter found `--stage`
  with `--matrix` silently ignoring the matrix.
- `TestCLIFlagPairMCP` checks the `--config`/`--masterdir` pair and standalone
  samples by sending only JSON-RPC `initialize` and `tools/list`. It compares
  server identity/capabilities and sorted tool names, ignores asynchronous
  notifications, closes stdin after the final reply, and requires a clean
  bounded exit, empty stderr, and an unchanged fixture. No MCP tool is called,
  so tool-side effects and masterdir precedence inside tools remain untested.
- `TestCLIFlagPairNote` adds a note to the finished fixture run with each of
  the three `note` pairs in both orders from the restored fixture, and checks
  that each invocation appends exactly its note. Notes carry the time they
  were added, so the orders are compared by note text, not byte for byte.
- `TestCLIFlagPairUnlock` executes all six `unlock` pairs in both orders from
  an identical synthetic interrupted state. It uses a stale local lock for an
  already-finished fixture run, so no process or scheduler is running. Each
  successful recovery removes that lock, returns metadata to `collecting`,
  retains the recorded last-run ID, and leaves queue and run history unchanged.
  The pair adapter therefore validates parser/order behavior and this recovery
  path. `TestCLIFlagPairUnlockSafety` also checks both orders of project/run
  flags for live-local refusal using the test process's PID, existing sibling
  run mismatch, remote-lock recovery, and recovery without a lock. These
  synthetic cases do not start or signal a real coordinator.

The fixture uses public `add`/`run`/`copy` commands. It contains successes,
distinct failure codes (1, 3, 7, 9), two failing tasks in a three-task array,
two matrix members, three stages, and different stdout/stderr content. The
robustness tests also restore a real queue so queue scopes resolve. These are
read-only tests after setup, so baselines do not need separate state copies.

Run a single pair using the subtest path (the plus sign must be escaped in a Go
test regular expression), or run the complete `TestCLIFlagPair` prefix with
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

Every pair is inventoried, and all twenty-seven commands with at least two
advertised flags have adapters; no generated pair is deferred.

| Commands | Pairs | Required next work |
| --- | ---: | --- |
| `schema`, `completion`, `guide`, `version`, `env` | 0 | Fewer than two advertised flags; subcommand/positional coverage is separate |

The 7,085 executed pairs consist of 809 read-only, 36 file-output, 172
queue-mutation, 1,648 edit, 3,782 run/retry previews, 6 unlock, 45 wait,
6 gc/server, 1 MCP, 45 web static-export, 532 job-control, and 3 note pairs. The edit
pair loop accepted 1,309 and explicitly rejected 339 pairs in 3,296
invocations; one run took 43s on the edit loop alone.

The run/retry dry-run pair loop accepted 3,322 and explicitly rejected 338
pairs, with 7,320 invocations in 19.9 seconds, after `retry`'s spec came to
list every `run` option it takes (61 flags, as `run` had then; 62 with
`--note`). This excludes actual
execution, supervisor/async lifecycle, scheduler submission, and host effects.

## Remaining observation gaps

- Unlock location samples use the fixture's existing basedir/project defaults
  and an empty explicit config. Their success does not independently prove
  location override or config precedence; distinguishing alternate-location
  witnesses remain to be added. No-op coverage remains in existing SAFE-4
  tests, not the generated recovery matrix.
- Run/retry pairs only preview plans. `--async` with `--dry-run` is explicitly
  rejected after this adapter exposed it being accepted and ignored; see CLI-8.
  Every generated pair containing async therefore exercises this early mode
  rejection, not the other flag's planning behavior. The full matrix injects
  dry-run for safety; it is flag-pair coverage in that mode, not unrestricted
  two-option execution coverage. Run naming has a dedicated preview witness.
  Runtime presentation/settings not visible in a job plan (such as quiet
  output or scheduler settings when a local executor is selected) still need
  mode-aware witnesses. Actual sync/async execution, scheduler submission,
  retries, cancellation, and host effects remain deferred.
- Edit samples represent a single value and a finished source fixture.
  Clear/set precedence, dependency-cycle effects, append-vs-overwrite collision
  behavior, source/destination conflicts, stdin import, new-project creation,
  repeated values, and boolean-false behavior need additional witnesses. Copy
  filters for host/time/diagnosis and unfinished may give a precise
  empty-selection error; that is an explicit rejection, not positive semantic
  coverage. The injected overwrite mode does not test omission/confirmation.
- Only the finished run view is used for mode/selector comparisons. Queue-only
  `filter-new`/`filter-changed` are checked for explicit rejection in that
  view, not for semantic selection on a modified queue.
- Filters rejected with output modes have rejection coverage, not a positive
  selection witness. All-host and broad time-window samples are useful for
  parsing/order but would need additional fixtures to prove narrowing.
- The generic fixture does not cover unfinished jobs, missing hosts, changed
  definitions, diagnosed failures, older runs, or carried results. The focused
  [log regression](../../../conformance/03-interfaces/log_selection_test.go)
  covers older/latest runs and carried successful output; existing selector
  tables remain authoritative for their broader variants. This suite is not a
  substitute for those tables.
- Direct job/task/attempt selectors, streaming behavior, location precedence,
  and equivalent routes through Python/Web need dedicated semantic checks.

The table is an independent rendering oracle, not a complete independent
implementation of selection. A bug shared by table and output can escape;
contract tables with explicit expected IDs are still necessary.

## Historical triage

The staged rollout findings and fix chronology are recorded separately in
[CLI flag-pair implementation triage](cli-flag-pair-triage.md).
