# AGENTS.md

## Project overview

`rotari` is a tool for managing and running jobs.

Use `rotari` for the tool name in code, commands, inline code, and mid-sentence
prose. At the beginning of an English sentence, capitalize it as `Rotari`.

Before making changes, inspect the relevant existing implementation and tests.
Do not assume behavior from names alone.

## General rules

* Keep changes focused on the requested task.
* Do not refactor unrelated code.
* Do not introduce new dependencies unless necessary for the task.
* Preserve existing public behavior unless the task explicitly requires changing it.
* Follow existing code style, but do not preserve an unhealthy architecture merely for compatibility.
* When the task is refactoring, prioritize clear responsibilities, one-way dependencies, and appropriate package boundaries over minimal file churn.
* Prefer incremental structural changes with compile/test checkpoints over broad unverified rewrites.
* Do not modify generated files unless the task explicitly requires it.
* Do not edit `README.md` directly. The `<!-- BEGIN GETTING STARTED -->` section is generated from `docs/GETTING_STARTED.md`; edit that source document and run `python3 scripts/sync_readme.py` instead.

### Refactoring priorities

When the user asks for refactoring, assess the architecture before applying
local cleanups. Prefer this order when applicable:

1. Establish clear package and dependency boundaries.
2. Separate domain types from CLI, Web, persistence, and executor adapters.
3. Remove duplicated logic and consolidate shared behavior.
4. Improve local readability and naming.

Moving code between packages is an intended refactoring, not an unrelated
change, when it makes dependencies and responsibilities easier to understand.
Do not stop at moving code between files in the same package if the main
problem is package-level coupling.

## Before changing code

Read the relevant implementation and tests first. To find them, use
`docs/ARCHITECTURE.md`, which maps processes, packages, `cmd/rotari` files,
and the path each main command takes through the code.

`contracts/` holds the behavior rotari promises and must keep; `docs/` holds
user guides and design notes. `conformance/` tests the contracts through the
built binary and the Web API.

If the change affects any of the following, also read `contracts/README.md`:

* job state
* scheduling
* queue behavior
* result or status resolution
* persistent state
* filesystem behavior
* CLI behavior
* web/API behavior

When unsure whether a change affects these areas, read `contracts/README.md`.

## Project invariants

These rules are important and must not be violated.

### Status / result resolution

`showJob` / `showRun` and `loadWebJobs` use the same fallback chain, which is
implemented once in `internal/jobstatus`. Change the chain there rather than in
a caller.

If the fallback behavior changes, inspect and update all relevant implementations.
Do not change only one side.

### Job selection

Whether a job or array task is selected by a result selection (`--failed`,
`--success`, `--unfinished`) and the per-job `--filter-*` conditions is
decided once, by `jobfilter.Filter.Selects`, and for an array job decided as
a whole, by `jobfilter.Filter.SelectsArray` from its tasks. `show`, `copy`,
and `run` / `retry` (including each array task under `--partial-array`) call
them; do not re-implement a condition or aggregate task results in a caller.
Callers only supply the job's facts.

### Paths

Path elements must be treated as arbitrary strings and must not be interpreted as filesystem paths.

Both `/` and `\` must be rejected where path separators are not allowed.

Path validation should be performed at the shared boundary rather than independently in each caller.

When changing path handling, inspect all relevant CLI, server, and web code paths.

### CLI / server / web behavior

When changing validation or user-visible behavior, check all relevant interfaces:

* CLI
* server
* web UI / API

Do not assume that fixing one interface fixes the others.

## Preventing bugs

Most bugs found so far were one rule implemented in several places that
drifted apart, or one path of a feature that nobody exercised. Design against
both.

* **One rule, one implementation.** When a rule (selection, status
  resolution, validation, path handling) applies in more than one command,
  view, or process, implement it once in the package that owns it and have
  callers supply only facts. Before adding or changing a condition, search
  for every place that evaluates the same rule and route them through the
  shared function. Keep lower-level helpers unexported when exporting them
  would let a caller bypass the shared function.
* **Enumerate the variants.** A change to job behavior must consider each
  variant that applies: plain job, array task, whole array (`--partial-array`),
  matrix member; queue view and run view; latest and older attempt; executed
  and carried result; CLI, server, Web API, and Python client. When fixing a
  bug in one variant, check its siblings for the same bug.
* **Do not decide per-job conditions on aggregates.** An aggregate result
  (for example, an array's) keeps only part of its members' facts. Evaluate
  per-job conditions on each member, then combine.
* **Never ignore an option silently.** An option that cannot apply in a mode
  or view must change the view to one where it applies, or fail with an
  error. It must not be accepted and ignored.

## Documentation

A user-visible behavior change may require updates to:

* `README.md`
* the user guides linked from the README `Documentation` section, such as
  `docs/CONCEPTS.md`, `docs/RUNNING.md`, or `docs/CONFIGURATION.md`
* `docs/FAQ.md`
* `contracts/README.md`
* `docs/ARCHITECTURE.md`, when a package, process role, or command flow it
  describes changes

When an unrelated bug, design concern, or technical debt is discovered during
work, record it in `development/ISSUES.md`. Remove the item when it is resolved, or move it
to the `Resolved` section when a short record is useful.

Examples include changes to:

* CLI options
* job/run semantics
* scheduling behavior
* queue behavior
* status or result resolution
* path rules
* web/API behavior

When behavior changes, inspect these documents and update every affected one. When the behavior is covered by `contracts/`, keep the relevant contract note current and add or update links to the representative implementation and tests in the same change.

## Testing

When changing behavior:

1. Find the existing tests covering the affected behavior.
2. Add or update tests when appropriate.
3. Run the smallest relevant test set first.
4. Run broader tests when the change may affect other components.

Do not modify tests merely to make them pass.
Tests should reflect the intended behavior.

When fixing a bug:

1. Write a test that reproduces it, and confirm the test fails for the
   reported reason before fixing the code.
2. When a test is added after the fix, show that it catches the bug: run it
   against the pre-fix commit in a temporary worktree
   (`git worktree add --detach "$TMPDIR/wt" COMMIT`), then remove the
   worktree.
3. Add a conformance row as well when the bug is visible through the binary
   or the Web API, so the bug is covered for every command that shares the
   rule, not only the one where it was found.

For behavior shared by several commands or paths, prefer a table test that
gives each command and variant the same input and expects the same selection.
Fixtures must contain data that tells cases apart: distinct exit codes,
more than one failing task, tasks with and without a host, carried and
executed results. A fixture where every failure looks the same cannot detect
two failures being mixed up.

Confirm that the tests you report actually ran. `go test -run` prints `ok`
when the pattern matches nothing, and a subtest name containing `/` adds a
level to the pattern (for example,
`-run 'TestSelectorTable/.*/exit_code'`). Use `-v` when in doubt.

`conformance/` checks contracts through the built binary and the Web API. A
refactoring must leave it passing without edits; change it only when a
contract itself changes, and run `go test ./conformance` for changes in the
areas listed under "Before changing code". Contract rules carry IDs such as
`RES-10`; a conformance test names the IDs it checks with `covers(t, "ID")`,
and the "Contract status" table in `contracts/README.md` must agree with those
calls. When a test starts checking a rule, update the rule's row, giving the
rule an ID first if it has none.

If tests cannot be run, report that explicitly and explain why.

From the repository root, prefer this validation order:

1. Run the smallest relevant test first, for example `go test ./cmd/rotari -run TestName`.
2. Run the package tests with `go test ./cmd/rotari`.
3. Run pre-commit on changed files for formatting, then run
   `scripts/check.sh --short` (`go vet ./...` and `go test -short ./...`).
4. Before finishing a change, run `scripts/check.sh`, which adds the race
   detector to match CI's Go checks.

## Before finishing

Before reporting a task as complete:

1. Inspect the final diff.
2. Verify that the requested behavior is implemented.
3. Check for unrelated changes.
4. Run relevant tests.
5. Check relevant documentation when behavior changed.
6. Re-check the project invariants above for affected code.
7. Do not claim tests passed unless they were actually run.

## Commits

* After finishing work, commit each work item separately.
* Include only changes from the current thread. Other work may be in progress
  in the same working tree; leave unrelated files and hunks unstaged, even in
  a file this thread also edited.

## Scope

Do not:

* perform unrelated refactoring
* rename public APIs without a requirement
* change behavior merely to make the implementation cleaner
* add dependencies without necessity
* modify unrelated documentation
* remove existing behavior without an explicit reason

When the task is ambiguous, prefer the smallest change that moves the code
toward clear responsibilities and one-way dependencies while preserving
existing behavior. If the request is specifically about structure or
refactoring, a larger package-level change is preferable to a sequence of
temporary file-only cleanups.
