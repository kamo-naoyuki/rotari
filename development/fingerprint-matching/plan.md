# Plan: Matching Jobs by Fingerprint

## Purpose

Match equivalent work across runs even when Job IDs or names change, while keeping Job IDs as random unique identifiers rather than content identities. This is a follow-up to [copy/import behavior unification](../unify-copy-import/plan.md).

## Decisions

- Fingerprint matching applies when creating a run from a new queue. `copy` and `import` keep their explicit provenance behavior.
- Supported modes are `job-id`, `fingerprint`, and `job-id + fingerprint` (default). In the combined mode, resolve explicit Job ID/Origin matches first, then fingerprint only for unmatched execution units. Never overwrite explicit provenance.
- `latest` means the latest finalized run; active/interrupted runs are excluded unless a run ID is explicitly selected.
- Recompute fingerprints from the current queue and historical `commands.json`; do not persist them. Ignore historical execution units with no current counterpart.
- Matching and execution/result carry are separate. A match supplies an Origin to the existing status and run-filter logic; it does not itself decide whether work runs.

## Fingerprint inputs

Include the argv elements as provided, explicitly declared job environment values, explicitly declared working directory (normalizing `.` and `..` without resolving against the caller's cwd), expanded matrix values, and expanded array task number. Exclude Job ID, name, executor/options, timeout, retry settings, array/matrix range definitions, DAG/dependencies, caller environment, and caller cwd. Use a canonical representation and SHA-256; never rely on Go map iteration order or default struct JSON ordering.

For duplicate work, match by `(fingerprint, occurrence index)` after removing units already matched by Job ID/Origin. If old and new occurrence counts differ for a fingerprint, match none of that fingerprint group; do not guess. Handle regular jobs, array tasks, and matrix leaves at their expanded execution-unit level.

## Integration

1. Calculate fingerprints deterministically and test the canonical payload.
2. Build historical candidates from `commands.json`, including resolved result/attempt facts; match candidates and attach `JobOrigin`/`TaskOrigins`.
3. Pass the resulting queue to the existing run planner. Preserve existing success carry, failure retry, status marking, dependency expansion, and downstream rerun behavior.
4. Expose the match mode through `--match-by`. A future plan/dry-run projection should identify source run/job/attempt, match outcome, and final planned status.
5. Document the contract and add CLI/conformance coverage for changed IDs, duplicates, count mismatches, array/matrix expansion, explicit origins, and carry behavior.

## Current status

Canonical fingerprint calculation, priority matching, occurrence matching, reference-run integration, and `--match-by` propagation have landed. Tests cover missing history, directory normalization, array ranges, non-input metadata exclusion, matching priority, occurrence, and deterministic result ordering. The plan/dry-run match explanation, array/matrix end-to-end conformance, and user-facing documentation remain follow-up areas; verify current coverage before extending the implementation.

## Completion criteria

- Equivalent work with changed IDs matches safely in the combined mode.
- Duplicate/count-mismatch cases never create ambiguous matches.
- Existing explicit provenance and carry rules remain authoritative.
- Unmatched and ambiguous cases are visible in an execution plan before being relied on operationally.
- Focused tests, relevant conformance, and the repository checks pass.
