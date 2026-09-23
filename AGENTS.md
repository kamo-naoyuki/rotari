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

Read the relevant implementation and tests first.

If the change affects any of the following, also read `docs/INTERNALS.md`:

* job state
* scheduling
* queue behavior
* result or status resolution
* persistent state
* filesystem behavior
* CLI behavior
* web/API behavior

When unsure whether a change affects these areas, read `docs/INTERNALS.md`.

## Project invariants

These rules are important and must not be violated.

### Status / result resolution

`showJob` / `showRun` and `loadWebJobs` use the same fallback chain.

If the fallback behavior changes, inspect and update all relevant implementations.
Do not change only one side.

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

## Documentation

A user-visible behavior change may require updates to:

* `README.md`
* `docs/FAQ.md`
* `docs/INTERNALS.md`

When an unrelated bug, design concern, or technical debt is discovered during
work, record it in `ISSUES.md`. Remove the item when it is resolved, or move it
to the `Resolved` section when a short record is useful.

Examples include changes to:

* CLI options
* job/run semantics
* scheduling behavior
* queue behavior
* status or result resolution
* path rules
* web/API behavior

When behavior changes, inspect all three documents and update every affected document. When the behavior is covered by `docs/INTERNALS.md` or `docs/internals/`, keep the relevant internal note current and add or update links to the representative implementation and tests in the same change.

## Testing

When changing behavior:

1. Find the existing tests covering the affected behavior.
2. Add or update tests when appropriate.
3. Run the smallest relevant test set first.
4. Run broader tests when the change may affect other components.

Do not modify tests merely to make them pass.
Tests should reflect the intended behavior.

If tests cannot be run, report that explicitly and explain why.

From the repository root, prefer this validation order:

1. Run the smallest relevant test first, for example `go test ./cmd/rotari -run TestName`.
2. Run the package tests with `go test ./cmd/rotari`.
3. For broader validation, run `go test ./...` and `go vet ./...`.

If the Go cache is unavailable in the environment, retry with
`GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache GOPROXY=off`.

## Before finishing

Before reporting a task as complete:

1. Inspect the final diff.
2. Verify that the requested behavior is implemented.
3. Check for unrelated changes.
4. Run relevant tests.
5. Check relevant documentation when behavior changed.
6. Re-check the project invariants above for affected code.
7. Do not claim tests passed unless they were actually run.

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
