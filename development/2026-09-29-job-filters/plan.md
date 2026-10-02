# Plan: Generalized Job Filters

**Created:** 2026-09-29
**Work history:** [work-log.md](work-log.md)

## Purpose and scope

Generalize result-only selection into reusable job predicates, so users can select work by result, definition, execution facts, diagnosis, and declared-file freshness. The user remains responsible for choosing what to run; rotari does not infer workflow dependencies or choose work automatically. The primary scope is CLI and the generated Python client. Web/API filters and cron scheduling are separate work.

All filter options use `--filter-*`; negations use `--filter-not-*`. Existing `--failed`, `--unfinished`, `--success`, `--stage`, and `--matrix` remain shorthand aliases. Direct selectors (such as `--job-id` and `--job-name`) stay distinct from filters.

## Combination rules

- Repeated values for one predicate are OR; different predicates are AND.
- Result selectors retain their existing OR behavior.
- Negated values exclude all listed matches. A missing attribute matches neither a positive nor a negative predicate.
- Direct selectors are mutually exclusive with filters. Time and duration bounds are not repeatable.
- Evaluate the latest attempt that produced the selected result. For carried results, use facts from the origin attempt.
- `show` switches to the appropriate queue/run view when a filter requires it; an option must not be silently ignored.

## Planned filter families

- **Result:** `result`, `exit-code`, and `failure-kind` (`timeout`, `cancelled`, `blocked`, `oom`, `signal`, `error`).
- **Diagnosis:** stable rule slugs; recompute from the latest attempt's log with current rules. Only failed jobs have a diagnosis attribute. Values prefer exact slug matches, then case-insensitive display-name substring matches.
- **Definition:** command regex, stage/matrix, and reference-run fingerprint selectors (`changed`, `new`).
- **Execution:** host glob, started/finished time bounds, duration bounds, and job state for control operations.
- **Declared-file freshness:** repeatable `--require-file` and `--produce-file` declarations, with `outdated` and `unproduced` predicates. Resolve declared paths relative to the job working directory and expand declared environment/matrix/array values. Evaluate files on the caller's host; shared visibility is required for remote executors. These declarations select work only and do not create dependencies or validate inputs before execution.

Use one `internal/jobfilter` evaluator. It owns predicate semantics; adapters provide job definition, result, execution, and lazy expensive facts. It must not import job status, executor, or diagnosis packages. `Filter.Selects` owns individual selection and `SelectsArray` aggregates task results as a whole. Callers must not reproduce these rules.

## Job control

`cancel`, `suspend`, and `resume` use the shared selection rules. Accept `--job-name`, applicable definition/execution filters, and `--filter-state`. `suspend`/`resume` reject pending-only selection. A zero-match filtered control request is an error, never a whole-run cancel. Resolve the target set once before confirmation and pass that same set to execution. Non-TTY use requires `--yes`; `cancel --wait` is incompatible with filters. Preserve the existing behavior when no filter is supplied.

## Implementation and validation

The implementation has landed in stages: shared evaluator and aliases; command predicates; result and execution facts; diagnosis and fingerprint selectors; job-control integration; and array-aware aggregation. Existing commits cover much of the CLI and Python schema path. Before additional work:

1. Verify the current CLI spec, Python generation, docs, and contracts against the implemented options.
2. Check `show`, `copy`, `run`/`retry`, queue editing, control commands, arrays, carried jobs, and Web/API siblings where relevant.
3. Add table-driven unit and conformance tests at the shared boundary. Test excluded jobs as well as selected jobs.
4. Update `contracts/06-selectors.md`, its status table, user docs, generated CLI reference, and Python client output as applicable.
5. Run focused tests first, then relevant package/conformance tests and `scripts/check.sh`.

## Explicit non-goals

No Web filter support in this plan; no cron, active-time-window predicate, inferred dependencies, automatic outdated-only execution, target/pattern workflow language, pre-run required-file validation, saved-diagnosis matching, universal `--not`, one generic `--filter KEY=VALUE`, or user-provided predicate shell command.
