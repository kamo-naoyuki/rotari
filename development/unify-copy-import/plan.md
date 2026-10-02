# Plan: Unifying `copy` and `import`

## Goal

Make imported queues use the same run/result rules as copied queues. Remove the separate Force mechanism, allow explicit status marking, and leave the decision to execute or carry a result to the existing run filters.

## Decisions and behavior

- An unfiltered `run` executes every job, including jobs from an imported queue.
- Remove queue-level Force/Accepted fields and `WorkflowImport`. A changed job keeps its result/provenance; `run` filters determine execution.
- Replace those markers with `MarkedStatus` and per-array-task `TaskMarkedStatus`. Supported values are `success`, `failed`, `cancelled`, and `unfinished`. `unfinished` clears the effective result; other marks rewrite the result for selection and carry. A mark requires an existing result except `unfinished`.
- Preserve the result's `Origin`; it continues to identify the source attempt.
- Convert old state version 1 queue fields when reading: `force` becomes `unfinished` (priority over `accepted`), `accepted` becomes `success`, and `workflow_import` is discarded. Do not rewrite old run files. Keep `JobResult.Accepted` for the user-facing accepted-result display.
- For filtered reruns, expand downstream work through both `--depends-on` and `--depends-on-finished`.
- Change `change` to support `--status` and `--clear-status`; allow equivalent manifest status marking. Import dry-run reports resulting queue status, not the old execute/reuse/accept actions.
- Update queue source-status displays to reflect marked status. Do not add status editing to `/api/change` as part of this work.

## Implementation areas

Centralize status resolution/marking in `internal/model`; migrate legacy queue data in `internal/state`; use the shared status rule in rerun planning, workflow reconciliation, queue edits, project state, and CLI rendering. Remove parallel import-specific planning/carry paths. Update state-version contracts and conformance, docs, generated CLI/schema output, and relevant golden data.

## Status

The main status-handling unification and Force removal landed. Subsequent work included package-boundary cleanup and workflow import/reconciliation fixes. Fingerprint matching is a separate follow-up and is documented in [its plan](../fingerprint-matching/plan.md). Treat this plan as completed for its original scope; use new focused plans for additional semantics rather than reopening the unification task.

## Validation

Keep tests for status marking, legacy state conversion, imported unfiltered execution, filtered downstream expansion, queue display, and workflow reconciliation. Run focused package tests, conformance for affected contracts, then the repository checks.
