# Plan: Preventing Recurring Boundary Bugs

## Purpose

Recurring defects have crossed package, process, persistence, and interface boundaries. Typical examples include inconsistent status projections across CLI/Web/Python, stale or partially finalized run state, selectors resolved differently by callers, and lifecycle tests that rely on elapsed time rather than observed state. The goal is to make these boundaries explicit, testable, and difficult to regress.

## Working principles

- A behavior change is complete only when its owning implementation, focused tests, external conformance coverage, and affected contract/documentation agree.
- Test cross-process and public behavior at the CLI, Web API, or persisted-file boundary. Unit tests remain appropriate for deterministic local rules.
- Synchronize on observable state (locks, markers, summaries, scheduler state), not fixed sleeps. Deadlines are failure bounds only.
- Implement each semantic rule once. Callers provide facts; they must not independently recreate status, selection, or resolution rules.
- Preserve full failure output and exit status when investigating intermittent tests. Do not label a failure flaky without reproducing and identifying its cause.

## Scope and phases

### 1. Close contract coverage gaps

Prioritize high-risk `pending` and `partial` rules in `contracts/README.md`, beginning with fingerprint matching (`RUN-4`) and job-control filters (`SEL-12`). Tests should cover positive selection and non-interference, through the built binary and isolated filesystem state. Update contract status only when `covers` calls fully represent the rule; document any remaining deviation and its follow-up.

### 2. Make lifecycle fixtures state-driven

Create reusable wait helpers for run lock, attempt submission/start/finish, and project finalization. Replace sleeps only when they control correctness or mask a race. Exercise disconnects, supervisor/job startup and termination order, control operations racing with execution, finalization/lock removal, and stale or newer state files.

### 3. Compare interface projections

For the same persisted fixture, compare CLI text/JSON, Web API/UI loading, and Python client behavior where applicable. Start with status/result fallback, timestamps, selection, job control, and finalization. Avoid approximate, separately constructed states.

### 4. Cover invalid and historical state

Specify behavior for missing or malformed state, unsupported newer versions, older attempts, interrupted runs, absent results, blocked jobs, stale registries, and differing queue/run selectors. Cover all affected readers and operations (`show`, `jobs`, `wait`, `run`, `retry`, `copy`, and Web loading).

### 5. Repeat race-sensitive checks

Keep normal checks fast, and add focused repeated/race-enabled coverage for lifecycle, coordination, and selector paths. Repetition is a detector, not proof of race-freedom; record reproducible conditions and barriers.

## Change checklist

- [ ] Read the relevant contract and architecture notes.
- [ ] Identify the single owning implementation boundary.
- [ ] Add focused unit tests and public conformance tests as appropriate.
- [ ] Check sibling variants: plain jobs, arrays, matrix members, carried/executed results, older/latest attempts, CLI, server, Web, and Python.
- [ ] Update contract status and user-facing documentation.
- [ ] Run the smallest focused test first, then broader checks justified by scope.
- [ ] Inspect the final diff for unrelated changes.

## Current status

The conformance suite now has lifecycle and selector coverage, and its organization follows contract topics. Pending contract rows have been addressed; partial coverage remains and is tracked in `contracts/README.md`. Continue from the remaining gaps there rather than treating this plan as a claim that every boundary is complete.
