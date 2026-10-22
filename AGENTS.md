# AGENTS.md

## Project

`rotari` is a tool for managing and running jobs. Use `rotari` in code,
commands, inline code, and mid-sentence prose. At the beginning of an English
sentence, capitalize it as `Rotari`.

Before changing code, inspect the relevant implementation and tests. Use
`docs/ARCHITECTURE.md` to find the package and command paths; do not assume
behavior from names alone.

## Always

* Keep changes focused, preserve existing behavior unless requested, and do
  not introduce dependencies without need. Do not change tests merely to make
  them pass.
* Use Jujutsu (`jj`) for status, diffs, commits, and history changes, even
  when a colocated Git repository exists. Do not use Git to stage, commit,
  check out, or rewrite changes.
* Do not add `Co-authored-by` trailers to commit messages unless the user
  explicitly requests them.
* Before editing, run `jj status` and `jj workspace list`. For concurrent
  threads, use separate workspaces with explicit base revisions; do not edit
  another thread's workspace or rewrite its changes without coordination.
* Do not modify generated files unless explicitly required. Do not edit
  `README.md` directly: its Getting Started section is generated from
  `docs/GETTING_STARTED.md`; edit that source and run
  `python3 scripts/sync_readme.py`.
* Surface errors explicitly. Never silently ignore an option or return a
  success-shaped fallback for an error.
* When a change affects user-visible behavior, check all applicable
  interfaces: CLI, server, Web UI/API, and Python client.

## Refactoring

For refactoring, first assess package boundaries and dependencies. Prefer
separating domain types from CLI, Web, persistence, and executor adapters,
then consolidating duplicated logic, before local readability changes.
Moving code between packages is appropriate when it clarifies responsibility;
do not stop at moving it between files in the same package when the coupling
is at package level.

## Project invariants

* **Status and result resolution:** `showJob` / `showRun` and `loadWebJobs`
  share the fallback chain in `internal/jobstatus`. Change the shared chain,
  not one caller.
* **Job selection:** `jobfilter.Filter.Selects` owns `--failed`, `--success`,
  `--unfinished`, and per-job `--filter-*` selection. For arrays,
  `jobfilter.Filter.SelectsArray` decides selection from its tasks. `show`,
  `copy`, and `run` / `retry` (including `--partial-array`) use these shared
  functions; callers supply results rather than reimplementing conditions.
  Filter facts come from `jobstatus.FilterJob`, which follows carried jobs to
  the attempt that produced the result.
* **Paths:** Treat path elements as arbitrary strings, not filesystem paths.
  Reject both `/` and `\` where separators are disallowed, and validate at the
  shared boundary.
* **Shared behavior:** Implement each cross-command rule once in its owning
  package; keep lower-level helpers unexported when exposing them would let a
  caller bypass the shared rule. Enumerate applicable variants before
  changing behavior: plain jobs, array tasks/arrays, matrix members,
  queue/run views, latest/older attempts, executed/carried results, and all
  relevant interfaces. Never decide per-job conditions from aggregate
  results.

## Contracts and documentation

`contracts/` defines behavior rotari promises to keep; `docs/` contains user
guides and design notes. For changes to job state, scheduling, queue behavior,
result/status resolution, persistent state, filesystem behavior, CLI, or
Web/API behavior, read `contracts/README.md`. When unsure, read it.

When behavior changes, update the affected user documentation and contract
notes. Conformance tests cover contracts through the binary and Web API; keep
their contract IDs and the status table in `contracts/README.md` in sync.
When an unrelated bug, design concern, or technical debt is discovered,
record it in `development/ISSUES.md`.

## On-demand workflows

Load the matching skill for detailed procedures:

* `.claude/skills/behavior-change/SKILL.md` — behavior changes,
  contracts, documentation, and conformance.
* `.claude/skills/bug-fix/SKILL.md` — reproducing bugs and verifying
  regression tests.
* `.claude/skills/jj-workflow/SKILL.md` — workspaces, commits,
  bookmarks, and pushing.
