# Artifact candidate discovery work log

## Classifier and argument discovery (phases 0 and 1)

**Commit:** `2bb43fc` — 2026-10-04T14:22:47+09:00

**Change:** Added `internal/artifact` with the shared rule-based classifier
(PATH-R2 to R5, exclusions PATH-X1 to X3), the interpreter and launcher
recognizer for PATH-X4 (`sh`/`dash`/`bash`/`zsh`, Python, Perl, Node behind
`env`, `timeout`, and `srun`), and `FromJob`, which extracts candidates from
argv, environment values, and log destinations (PATH-D1) with provenance and
deduplication. Added an archtest boundary rule (no rotari imports, no file
access), the package to docs/ARCHITECTURE.md, and the rule to
contracts/00-overview.md.

**Reason:** Phases 0 and 1 of the plan: turn the rules into executable
fixtures and implement argument discovery once, before any lifecycle wiring.

**Plan impact:** Phase 0 rule details were decided and recorded in
plan.md ("Phase 0 decisions"): the log-destination base is the effective
working directory for every executor, `key=value` splitting requires an
identifier key, single-dash option words are skipped, recognized program words
are not candidates, launcher nesting stops at four, Perl switch clusters are
unsupported, additional PATH-X1/X2 cases, search-path lists are skipped, and
the candidate shape, order, and limit.

**Validation:** `go test -count=1 -v ./internal/artifact` passed (130
subtests counted), plus `go vet ./internal/artifact`, `./internal/archtest`,
and `./internal/doclinks`. `gofmt -l` was clean. `pre-commit` is not
installed in this environment.

**Remaining:** None for these phases.

## Configuration file inspection (phase 2)

**Commit:** `293a173` — 2026-10-04T14:25:41+09:00

**Change:** Added `artifact.Discover` and `ConfigReferences`, which walk YAML
(node API, aliases not followed, custom tags skipped, multi-document), JSON,
and TOML within size, depth, and value budgets. Added
`internal/artifactsource.Read`, the only file access of discovery: regular
files only, symlinks followed, `O_NONBLOCK` against FIFOs, 1 MiB limit.
Updated docs/ARCHITECTURE.md and the plan.

**Reason:** Phase 2: directly referenced configuration files.

**Plan impact:** Phase 2 decisions recorded in plan.md (reader split,
which candidates are read, symlink policy without confinement, budgets,
location syntax, and diagnostics that carry no source text).

**Validation:** Uncached `go test -v` of `./internal/artifact` and
`./internal/artifactsource` passed, including a FIFO test that would block on a
plain open, and `./internal/archtest` and `./internal/doclinks` passed.

**Remaining:** None for this phase.

## Attempt records and carried lookup (phase 3)

**Commits:**

- `110cfea` — 2026-10-04T14:31:45+09:00
- `4c6f6b4` — 2026-10-04T14:34:02+09:00

**Change:** `projectrun.Runner.Execute` now creates an `artifactRecorder`:
discovery runs in the engine's `AssignAttemptID` callback, and the record is
written to the attempt's `artifacts.json` from the dispatcher's start
callback. It reads the job's own environment (not run variables), and SSH
attempts do not read configuration files. Added `artifact.Record`,
`DiscoveryVersion`, `state.ArtifactsFileName`, and `jobstatus.Artifacts`,
which follows carried origins through `locateAttempt`. The second commit makes
a panic in discovery a logged warning instead of stopping the run. Updated the
state layout in contracts/00-overview.md and docs/ARCHITECTURE.md.

**Reason:** Phase 3: attempt-bound resolution and storage shared by every
job creation path, without touching `QueuedCommand` or fingerprints.

**Plan impact:** Phase 3 decisions recorded in plan.md. Queue-time
candidates stay a pure computation (`FromJob`) and are not stored.

**Validation:** `TestExecuteRecordsArtifactCandidatesPerAttempt` runs real
local jobs (plain, two array tasks, env-distinguished members, a retried job
with two attempts, a relative working directory) and passed, along with the
SSH and never-started recorder tests and `jobstatus` tests for the latest
attempt, an older attempt, carried results, old state, and a corrupt record.
`go vet ./internal/...` and uncached `go test ./internal/...` passed.

**Remaining:** No CLI or Web view reads the records yet; that belongs to a
viewing design outside this plan.

## Contract and documentation (phase 5)

**Commit:** `c0ee1d0` — 2026-10-04T14:33:14+09:00

**Change:** Added RUN-9 to contracts/02-run-lifecycle-and-execution.md and
its status row, the binary conformance test
`TestStartedAttemptRecordsArtifactCandidates`, and a user note in
docs/INSPECT.md.

**Reason:** Phase 5: document the implemented guarantee and its limits.

**Plan impact:** Phases 0 to 3 and 5 are done; phase 4 (shell inspection,
PATH-R1 and PATH-E1) stays deferred.

**Validation:** The conformance test passed. `TestContractStatus`,
`TestConformanceLayout`, and `./internal/doclinks` passed.
`scripts/check.sh --short` passed. It started before `4c6f6b4` was
committed, so the recover change was checked separately with
`go test -run Artifact ./internal/projectrun`. A full `scripts/check.sh`
(vet, test, race), started after `4c6f6b4`, passed with "all checks passed".

**Remaining:** Phase 4 (shell inspection) when the plan resumes it.

## Shared-filesystem configuration reads for SSH

**Commit:** `a8223c3` — 2026-10-04T14:55:52+09:00

**Change:** Removed the SSH exception from `artifactRecorder`: SSH attempts
read referenced configuration files from the supervisor's host like local and
scheduler attempts. The recorder test now runs for local, SSH, and Slurm and
checks that a file the supervisor cannot read gets a diagnostic while its
path stays a candidate. Updated docs/INSPECT.md and the phase 3 decision in
plan.md.

**Reason:** The user chose the shared-filesystem assumption for SSH too: a
file that is not found is simply not loaded.

**Plan impact:** Reverses the phase 3 "Execution hosts" decision for SSH;
the plan records the revision.

**Validation:** `go test -count=1` of `./internal/projectrun` (including
`TestArtifactRecorderReadsSourcesForEveryExecutor/{local,ssh,slurm}`, checked
with `-v`), `./internal/doclinks`, `./internal/archtest`, and
`TestStartedAttemptRecordsArtifactCandidates` passed. `go vet` and `gofmt`
were clean. A full `scripts/check.sh` was not rerun for this change.

**Remaining:** None.
