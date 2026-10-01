# Plan: Preventing Recurrent Boundary Bugs

## Positioning

This plan responds to the recurring bugs recorded in
[ISSUES.md](ISSUES.md). The issue is not simply a lack of unit tests. The
same classes of defects repeatedly appear at boundaries:

- state transitions between client, supervisor, wrapper, lock, and summary;
- different projections of the same state in CLI, Web, and Python interfaces;
- selector resolution across project, run, job, and attempt IDs;
- missing, stale, older, or partially finalized state;
- behavior that is specified in contracts but only partially checked by
  conformance tests.

The goal is to make these boundaries explicit, executable, and difficult to
regress.

## Root-cause hypothesis

A feature is currently considered sufficiently tested when its owning package
and a nearby command test pass. That is inadequate for behavior whose meaning
is defined across packages or processes. The resulting pattern is:

```text
implementation
  -> package tests pass
  -> an untested boundary is exercised later
  -> a CLI/Web/concurrency/history bug is found
  -> a local regression test is added after the fact
```

This plan changes the completion rule to:

```text
contract
  -> shared implementation boundary
  -> package test
  -> external conformance test
  -> repeated/concurrent check where timing matters
  -> documentation and contract status updated
```

## Goals

- Convert the highest-risk pending contracts into executable conformance tests.
- Test state transitions using observable barriers instead of incidental sleeps.
- Verify that CLI, Web API, and Python-facing behavior resolve and report the
  same persisted state.
- Exercise malformed, stale, older-attempt, interrupted, and partially written
  state explicitly.
- Make unresolved contract coverage visible in the contract status table.
- Preserve the filesystem as the source of truth and keep tests independent of
  implementation details where possible.

## Non-goals

- Rewriting all existing tests.
- Adding a new test framework or external dependency.
- Making every test exhaustive across every executor immediately.
- Fixing unrelated issues discovered while implementing this plan.
- Treating a passing repeated test as proof that a race cannot exist.

## Decisions

### Contract coverage is a release gate

A behavior-changing change is complete only when its affected contract rows
have a conformance test, or the remaining gap is explicitly documented as
`partial` with a named follow-up. `pending` is reserved for contracts with no
external behavioral test.

The first targets are:

1. `RUN-4`: new-run fingerprint matching.
2. `SEL-12`: job-control names and filters.
3. The remaining lifecycle and state-resolution rows marked `partial`.

### Test at the public boundary

Package tests remain useful for deterministic algorithms. Conformance tests
must be preferred for behavior that crosses a process, filesystem, CLI, Web
API, or generated client boundary. A package test may explain a rule; it does
not replace the external test for that rule.

### Synchronize on state, not elapsed time

Fixtures must wait for a state that the next operation depends on, such as:

- a run lock containing the expected run ID;
- an attempt directory and submission marker;
- a wrapper status showing executor ownership;
- a finished summary and finalized project metadata;
- a scheduler state transition.

Time limits remain only as failure deadlines. A fixed sleep must not be the
condition that makes a test proceed.

### One owner for one semantic rule

Selector resolution, status fallback, attempt selection, and command metadata
must have one shared implementation. Tests should call the owning boundary and
then compare all projections that promise the same meaning.

## Implementation phases

### Phase 1: Contract gap closure

Add conformance coverage for `RUN-4` and `SEL-12`, then update
`contracts/README.md` from `pending` to `conformance` only when the full rule is
covered.

`RUN-4` cases:

- Job ID/Origin matching takes priority over fingerprint matching.
- Changed IDs with the same command match by fingerprint.
- Duplicate fingerprints match by queue occurrence.
- A count mismatch does not partially match any unit of that fingerprint.
- Command, explicit environment, working directory, array task, and matrix
  values affect the fingerprint as specified.
- Job name, executor, timeout, retry, DAG, and range definition do not affect
  the fingerprint.
- Missing historical input is ineligible for fallback matching.

`SEL-12` cases:

- repeated job names combine by OR;
- names select array tasks individually;
- unknown names fail without affecting any job;
- stage and matrix scope narrow the selection;
- command, host, time, duration, and state filters narrow the selection;
- negated stage and matrix filters exclude matching jobs;
- `cancel`, `suspend`, and `resume` apply the same selection rules;
- `suspend` and `resume` reject pending-only selections;
- confirmation happens before signalling and the selected set is not
  reevaluated afterward;
- filters and names are mutually exclusive with job IDs and `cancel --wait`.

Exit criteria:

- `RUN-4` and `SEL-12` no longer have `pending` status.
- Tests run through the built binary and use isolated filesystem state.
- The tests assert both positive selection and non-interference with excluded
  jobs.

### Phase 2: Lifecycle barrier fixtures

Create shared conformance helpers for the lifecycle states currently repeated
across tests. Each helper should expose an explicit condition and a timeout:

- `waitForRunState`
- `waitForAttemptSubmitted`
- `waitForAttemptRunning`
- `waitForAttemptFinished`
- `waitForProjectFinalized`

Replace test sleeps only where they currently control correctness or hide a
race. Keep polling interval and deadline in the helper, not scattered through
individual tests.

Test event permutations for:

- client disconnect before and after supervisor startup;
- supervisor termination before and after job submission;
- job startup racing with cancel, suspend, resume, reset, or unlock;
- summary creation racing with finalization and lock removal;
- output written immediately before the finished marker;
- stale lock and newer state-version files.

Exit criteria:

- lifecycle tests proceed only after an observed prerequisite;
- interrupted and finalized runs have distinct assertions;
- repeated lifecycle tests produce the same result without increasing sleeps.

### Phase 3: Cross-interface projection tests

For representative persisted states, compare the result of:

- human-readable CLI output;
- CLI JSON output;
- Web API output;
- Web UI loader projections;
- Python client behavior where CLI options are generated or transformed.

Start with status, result fallback, timestamps, selectors, job control, and
run finalization. The fixture should write or create one state, then query all
relevant interfaces rather than creating separate approximate states.

Exit criteria:

- shared status and selector contracts have at least one CLI/Web comparison;
- JSON and human output agree on state, run ID, result, and selected jobs;
- Python tests verify both accepted options and rejected/unsupported options.

### Phase 4: Invalid and historical state matrix

Build a small matrix of persisted state variants:

- missing summary or commands file;
- malformed JSON;
- newer state version;
- older attempt selected while a newer attempt exists;
- interrupted run with and without a leftover job;
- missing attempt result;
- blocked job without an attempt;
- stale registry entry or missing registry entry;
- queue and run with different project or run selectors.

For each variant, specify whether the command should fail, show an unfinished
state, use the fallback chain, or skip the record. Cover `show`, `jobs`,
`wait`, `run`, `retry`, `copy`, and Web loading where applicable.

Exit criteria:

- each fallback rule has a representative external test;
- newer state is never silently treated as missing;
- older attempts never inherit the latest attempt's result.

### Phase 5: Repeated and race-sensitive CI checks

Keep the normal check fast, then add a focused repeated job for high-risk
packages and conformance groups:

- lifecycle and coordination conformance with `-count=10`;
- selector conformance with `-count=10`;
- race-enabled package tests for supervisor, state, jobcontrol, and projectrun;
- periodic stress runs with larger counts for nightly or pre-release checks.

Failures must preserve the complete command output and original exit status.
Do not classify a failure as flaky until the failing test and condition are
identified.

Exit criteria:

- the repeated job is deterministic about reporting failures;
- known timing-sensitive tests have a named barrier or documented external
  dependency;
- a recurring failure creates a concrete issue with reproduction data rather
  than only an intermittent-test note.

## Required change checklist

For every behavior change affecting state, scheduling, selectors, persistence,
CLI, Web, or Python:

- [ ] Read the relevant contract and architecture entry.
- [ ] Identify the owning implementation boundary.
- [ ] Add or update the package test for the local rule.
- [ ] Add or update an external conformance test.
- [ ] Check CLI, Web, and Python projections when applicable.
- [ ] Add a barrier for any process or timing dependency.
- [ ] Update the contract status row and representative test links.
- [ ] Update user-facing documentation when behavior changes.
- [ ] Run the smallest focused test first.
- [ ] Run package tests, conformance tests, and final checks appropriate to the
      affected area.
- [ ] Inspect the final diff for unrelated changes.

## Progress

- [ ] Phase 1: `RUN-4` conformance coverage.
- [ ] Phase 1: `SEL-12` conformance coverage.
- [ ] Phase 2: shared lifecycle barrier fixtures.
- [ ] Phase 3: CLI/Web/Python projection comparisons.
- [ ] Phase 4: invalid and historical state matrix.
- [ ] Phase 5: repeated and race-sensitive CI checks.

## Completion criteria

This plan is complete when:

- all currently pending high-risk contracts have external tests;
- lifecycle tests synchronize on persisted state rather than fixed timing;
- representative CLI, Web, and Python paths are checked against the same
  underlying state;
- invalid, interrupted, stale, and historical states have explicit expected
  behavior;
- repeated CI checks cover the known race-sensitive slices;
- `ISSUES.md` records only unresolved work with a reproduction condition,
  root cause, invariant, regression test, and affected interfaces;
- `go test ./...`, relevant conformance tests, and `scripts/check.sh` pass.
