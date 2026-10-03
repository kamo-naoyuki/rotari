# Plan: Schema-Driven CLI Option Interaction Checks

**Created:** 2026-10-03

**Status:** In progress; read-only adapters and initial triage completed.

## Purpose

Prevent recurring CLI boundary bugs by exercising option interactions through
the built binary, starting with every unordered pair of flags advertised by
`rotari schema --json`. The first milestone includes running the new suite and
classifying failures, not merely adding a generator.

The resolved records in [../ISSUES.md](../ISSUES.md) include:

- `show --json` silently ignoring `--failed`;
- positional selector and output-option rules differing between commands
  (SEL-10);
- the documented `jobs --since 7d` being rejected;
- Python dropping unknown options or incorrectly encoding
  `partial_array=False`.

These examples motivate pair coverage and interface/documentation checks.
They do not establish that future bugs cannot require three or more options.

## Scope and priority

1. **Implement schema-driven flag-pair and silent-ignore checks** in
   `conformance/03-interfaces`, run them, and triage their findings.
2. **Consider declarative compatibility rules** only after the first milestone
   supplies evidence of repeated validation drift.
3. **Validate documented command examples**, beginning with argument acceptance.
4. **Compare CLI, Python, and Web API selections** on identical state.

This document is the planning deliverable. It does not change public behavior,
add tests, declare any newly discovered bug, or modify current contracts.

### Non-goals for the first milestone

- Exhaustive combinations of three or more flags, or all values of every flag.
- Redesigning CLI parsing, adding dependencies, or adding compatibility metadata
  before its need is demonstrated.
- Treating every equal output as proof of a silently ignored option.
- Invoking real schedulers, SSH hosts, external LLM services, or notifications.
- Replacing existing focused regression tests or selector contract tables.

## Existing implementation and integration points

- [CLI specifications](../../cmd/rotari/cli_spec.go) define commands, flags,
  value names, enumerated values, repetition, and positional syntax.
- [Schema output](../../cmd/rotari/schema.go) publishes schema version 1 with
  commands, subcommands, positional descriptions, and flag metadata. It does
  not expose complete types, safe sample values, compatibility constraints,
  or side-effect classifications. `CommandLineOnly` is not exported as a
  boolean; its effect includes omitting the flag's environment mapping.
- [Interface option tests](../../conformance/03-interfaces/options_test.go)
  already obtain command coverage from the binary's schema for precedence
  checks. Extend that approach without importing CLI implementation packages.
- [Conformance support](../../conformance/support/support.go) supplies the
  binary, isolated environments, build-input tracking, and captured exit code,
  stdout, and stderr.
- [Selector fixture](../../conformance/06-selectors/selector_fixture_test.go)
  and [selector table](../../conformance/06-selectors/selector_test.go) cover
  plain jobs, arrays, matrices, multiple projects, older/latest runs, and
  carried/executed results. The fixture is currently private to its package;
  it cannot be directly called by the interface test package.
- [Contracts](../../contracts/README.md) describe coverage bookkeeping and the
  standard-library-only external boundary for conformance tests.

## Milestone 1: All flag pairs and ignore detection

### 1. Inventory and deterministic generation

Read `rotari schema --json` from the binary built by the conformance harness.
Validate the schema version and sort commands and flag names for stable case
identities. For each command with n flags, enumerate n(n-1)/2 distinct pairs.
This is all flag-name pairs, not a covering array over all flag-value domains.

The proposed approximately 1,800 pairs for `run` and 10,000 overall are sizing
estimates, not measured facts. Record the actual inventory, invocation count,
fixture setup cost, and elapsed time before setting the CI budget. Single-flag
baselines, both orders, multiple fixtures, and values increase invocation count.

Maintain a small test-owned sample-value registry and command adapters for
required positionals, subcommands, safe setup, and observation. Enumerations
can use schema `values`; paths, IDs, durations, regular expressions, and
executor options require valid fixture-aware samples. Include explicit false
for boolean samples and representative repeated-value cases where relevant.
Use distinct non-default values when checking observability.

Every advertised flag must have a sample strategy or an explicit unsupported
entry with a reason and follow-up. Schema additions must fail this inventory
check rather than silently reducing coverage. Track generated, executed,
unsupported, and intentionally exempt cases separately.

### 2. Safe execution and independent baselines

Run command, command+A, command+B, command+A+B, and command+B+A from equivalent
initial state. Cache read-only baselines where safe; never reuse mutable state
between variants. Do not inject a flag already under test as a duplicate
baseline flag, because last-value-wins parsing would corrupt the comparison.

- Start with read-only selectors and output modes (`show` and `jobs`), then
  expand adapters to mutation and execution commands without losing the full
  inventory of remaining pairs.
- Build fixtures using the public binary and persisted state. Extract only
  reusable fixture construction into `conformance/support` if needed; keep
  assertions and selector expectations in their existing packages.
- Strengthen fixture witnesses where required: distinct exit codes, multiple
  failing array tasks, tasks with/without a host, success/unfinished results,
  and carried/executed attempts. Equal-looking failures cannot detect the
  wrong task being selected.
- Use fresh state copies or reconstruction for queue edits and runs. Observe
  persisted effects as well as output. Never execute destructive operations
  against the developer's state.
- Use harmless local commands, bounded deadlines, process cleanup, and fake
  external adapters where suitable. Interactive, streaming, daemon, and
  external-integration commands need dedicated adapters or explicit deferred
  coverage, not an unbounded generic invocation.
- Do not add `--help` to behavior checks: it may bypass the validation or
  operation being tested.

### 3. Generic assertions and their limits

**Robustness:** no panic, internal error, signal termination, or timeout.
Accept successful completion or a diagnosed usage/compatibility rejection.
Do not equate every nonzero exit with a usage error: legitimate job failure,
missing state, external-service failure, and parser rejection can share an
exit code. Safe adapters must classify expected outcomes; an unclassified
error is a failure requiring investigation.

**Order independence:** A+B and B+A must have equivalent outcome categories,
normalized diagnostics, semantic output, and persisted effects. Normalize only
known nondeterminism such as generated IDs, timestamps, and temporary roots.
Preserve selected job/task identities, exit codes, and meaningful ordering.
Keep original arguments, exit status, stdout, and stderr in failure reports.

**Silent-ignore suspicion:** compare A with A+B and B with B+A. Equal
observations on a successful operation are suspicious when the added flag has
a demonstrated distinguishing effect on the chosen fixture. Prefer selected
job/task identities for selectors, rendered representation for output modes,
and persisted effects for mutation flags; raw stdout equality alone is not a
universal oracle.

Each observable flag should change its single-flag baseline on at least one
witness fixture. If it does not, improve the witness or report an observation
gap rather than declaring the flag ineffective. Redundant filters, disjoint
selections, defaults, empty results, and documented overrides can legitimately
produce equality. A successful pair that hides one flag's effect must either
gain a distinguishing witness, receive a reasoned exception, or remain a
failing/unclassified case. A rejection must explain the incompatibility and
must not silently perform a partial operation.

The checks use shared metamorphic rules, not individually authored expected
output for every pair. They still need small adapter-specific observation
rules; the schema alone cannot describe every command's semantics.

### 4. Reasoned exceptions, not blanket exclusions

Store intentional equivalences in a reviewable test data table, keyed by
command/subcommand, pair, sample values, fixture/mode, and assertion direction
where applicable. Each entry includes the reason, supporting contract or
documentation, and the precise assertion it exempts.

- Keep observation equivalence exceptions separate from safety/deferred
  execution entries and known bugs.
- Do not suppress panic/internal-error checks with an equality exception.
- Reject stale, unused, overly broad, or schema-unknown entries.
- Do not automatically accept all first-run failures into the allowlist.
- Do not use missing prerequisites or an inadequate fixture as evidence that
  ignoring an option is intentional.

### 5. First-run triage and bug fixes

For every failure, retain a minimal reproducible command, sample values,
fixture identity, both orders, outcome, raw output, and normalized observation.
Group it into one of four categories:

1. **Confirmed bug:** reduce the case, record it in `development/ISSUES.md`,
   identify the shared owning rule and sibling command/interface variants.
2. **Intentional equivalence:** document the semantics and add a narrow,
   reasoned exception.
3. **Harness/fixture defect:** repair sampling, isolation, normalization, or
   observability and rerun; do not add an exception.
4. **Unsupported execution path:** record its exact coverage gap, reason,
   safe-adapter follow-up, and impact on completion claims.

Commit each confirmed fix separately. First reproduce it with a failing focused
test, fix the shared implementation, then add/update a conformance row covering
the relevant commands and variants. If a regression test is written after the
fix, prove it fails against the pre-fix commit in a temporary worktree. Update
affected contract IDs/status and user documentation together with the fix.

### Milestone 1 acceptance criteria

- [ ] Schema-driven inventory accounts for every advertised command and pair.
- [ ] Deterministic case names allow running one command/pair with `go test -run`.
- [ ] The public-binary harness checks robustness and both flag orders safely.
- [ ] Ignore detection uses distinguishing witnesses and scoped exceptions.
- [ ] New/stale flags, sample gaps, and stale exception entries fail checks.
- [ ] Initial failures are all classified; no unresolved case is disguised as
      intentional behavior. Remaining bugs and deferred paths are explicit.
- [ ] Actual pair counts, coverage gaps, subprocess count, and runtime are
      recorded. A fast subset never substitutes for the full milestone report.
- [ ] Focused regression tests and shared-selector conformance rows accompany
      confirmed fixes; full validation passes for completed implementation.

## Follow-up milestones

### 2. Declarative option compatibility (decision gate)

Review the first-run findings before extending `cliFlagSpec`. If repeated
compatibility drift warrants it, declare incompatible flags and mode-specific
applicability in the CLI specification, route production validation through
one shared evaluator, and expose necessary metadata to the public schema.
Tests must read that public metadata rather than importing `cmd/rotari`.

Keep fixture witnesses and order/observability assertions independent of the
declarations: implementation and tests reading the same incorrect table must
not make the error invisible. Define schema versioning and check generated
Python metadata when the schema changes. Do not create a second compatibility
table solely for tests.

### 3. Execute documentation examples safely

Inventory `rotari ...` examples in docs, contracts, the README, and agent guides.
Parse fenced/multiline shell examples deliberately; account for placeholders,
shell variables, pipelines, prerequisite state, and external services. A regex
extracting single lines is insufficient.

Begin with an argument-acceptance check that does not use `--help` to hide
validation. Use sandboxed fixture execution where possible; decide whether a
side-effect-free parser/validator boundary is needed for unsafe commands.
Require explicit handling of non-executable examples and a regression example
for `jobs --since 7d`. Edit the getting-started source rather than the generated
README section when a documentation correction is necessary.

### 4. CLI / Python / Web API selection parity

Build one distinguishing persisted fixture and apply equivalent selection
conditions through the CLI, Python client, and supported Web endpoints. Compare
job and array-task identities, not separately reconstructed aggregate counts.
Reuse/extend the existing selector table expectations rather than reimplementing
selection rules. Cover plain jobs, arrays/partial arrays, matrix members,
queue/run views, older/latest attempts, and carried/executed results as applicable.

First map which operations and conditions each interface actually supports;
do not assume every CLI flag has a Web equivalent. Keep the Go conformance
suite standard-library-only and decide how existing Python tests participate
without requiring Python for every Go conformance invocation. Include explicit
boolean-false encoding and unknown-option rejection in Python coverage.

## Validation and tracking

For this planning-only change, inspect the diff, check Markdown whitespace and
relative links, and run repository checks required by `AGENTS.md`. Report these
separately from future test implementation; no flag-pair results exist yet.

For implementation, run the smallest command/pair test first with verbose
output to confirm it actually ran, then interface-package tests, selector tests
when fixture support changes, and contract/layout checks. Run the full
conformance packages (including topic subpackages), pre-commit on changed
files, `scripts/check.sh --short`, and finally `scripts/check.sh` including the
race detector. Measure the full new suite before deciding CI sharding or
short-mode policy; publish coverage differences if a subset is introduced.

Update this plan as milestones advance. Record related implementation commits
and their actual Git timestamps in this directory's `work-log.md` once they
exist, following [development tracking](../README.md). Link to the broader
[boundary bug prevention plan](../2026-10-01-bug-prevention/plan.md) rather than
duplicating its lifecycle or status-projection work.

## Current status and next action

The current schema inventory generates 6,826 pairs, and adapters execute all
of them across 26 commands in six conformance packages: 783 read-only, 36
file-output, 178 queue mutations, 1,520 edits, 3,660 run/retry previews, 6
unlock, 21 wait, 6 registry, 1 MCP, 28 web static-export, 55 diagnose, and
532 job-control pairs. Registry
checks exercise GC preview/apply and server status/list; server shutdown and
live leases are not implied. MCP checks only initialization/tool discovery,
never a tool call. Web static mode rejects live-server-only options, and the
diagnose adapter routes all LLM calls to a local fake endpoint. Job-control
pairs act on live local runs and compare the jobs acted on with a model.

The pair tests share their public-binary harness through `conformance/support`.
Splitting the CLI checks across `03-interfaces`, `pairedits`, `pairruns`,
`pairweb`, `pairdiagnose`, and `pairjobcontrol` keeps each Go package below the default ten-minute
test timeout without excluding pairs or witnesses. One uncached normal run
took 104.87s, 354.09s, and 156.48s for the first three; the web suite takes
about 16s, diagnosis about 10s, and job control about 140s (race, cancels two
pairs at a time).

Run/retry preview checks accept 3,322 pairs and explicitly reject 338 in 7,320
invocations, measured at about 20 seconds with four concurrent cases. They never
start a job or scheduler. This suite runs in short and race mode; NFS fixture
state is restored differentially.

Semantic preview checks cover failed/stage intersection, per-task array
selection, and visible `--run-name`. The pair loop also checks the async/dry-run
rejection regardless of flag order; this was a reproduced silent ignore and is
now fixed at the shared run/retry parser boundary. Retry-specific selection
expectations, actual run lifecycle, scheduler submission, and host effects are
not implied by dry-run coverage.

A restored mutation adapter adds `remove`, `reset`, and `delete`: 178 pairs
(158 accepted, 20 explicitly rejected) in both orders, now 997 executed pairs
in nine commands. Each invocation starts from a byte-identical environment
root, including directory times; rejections and dry runs must change nothing,
and guard revisions must match `check`. On NFS, rewriting the whole root per
invocation took 5m13s, so restoration rewrites only what changed (41s).
Effect witnesses prove every `remove` selector and definition filter has an
effect and that each selector-filter combination removes the intersection;
modified builds in temporary worktrees that ignore filters, everywhere or
only with a direct selector, fail them. Triage found no ignored option. It
recorded one open diagnostic issue: `remove` rejects two selector kinds,
including `--all` with `--filter-stage`, with only its usage line.

The restored adapter now also covers `add`, `change`, `copy`, and `import`: 1,520 pairs
(1,187 accepted, 333 explicitly rejected) in 3,040 invocations. Total executed
coverage is 2,517 pairs across 13 commands; 3,964 remain. The edit pair loop took
4m01s in one uncached run, and the whole uncached interface package took 438.88s.
Semantic witnesses verify added/changed definition fields with quiet, copy's
result/stage intersection, import's JSON preview versus applied overwrite, and
restored-state behavior. Import revisions are checked against `check`; only new
IDs are normalized while original IDs/provenance are retained. Harness setup
failures were fixed and not waived as intended behavior.

See [coverage and triage](../../conformance/03-interfaces/flag-pair-coverage.md)
for measured results and explicit gaps. Milestone 1 pair execution is
complete; execution, scheduler submission,
async lifecycle, clear/set, active-state, provenance, boolean/repeated-value,
and cross-interface witnesses remain. Compatibility declarations are an
evidence-gated follow-up. Run/retry previews and synthetic wait locks do not
exercise live execution or scheduler/control integration.
