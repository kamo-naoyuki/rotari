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

## Executable contract examples

**Commit:** `54cf5b6` — 2026-10-04T15:01:46+09:00

**Change:** Added "Artifact candidate examples" under RUN-9 in
contracts/02-run-lifecycle-and-execution.md: a fixture `conf/train.yaml` and
two tables (12 recorded, 9 not recorded) of `rotari add` command lines with
the candidates each records and why. `TestArtifactCandidateExamples` in
conformance/02-lifecycle/artifacts_test.go parses that region, splits each
line with `sh`, adds every example to one project, runs it, and compares each
attempt's `artifacts.json` with its row. Shared the context and record readers
with `TestStartedAttemptRecordsArtifactCandidates`, and listed the new test in
the RUN-9 status row.

**Reason:** The user asked for registered and ignored cases written as
Markdown contract examples and executed by conformance, so developers can see
at a glance what happens in which case.

**Plan impact:** Phase 5 now has a document-driven contract; changing a row
changes the test.

**Validation:** `go test -count=1 -run
'TestArtifactCandidateExamples|TestStartedAttempt' -v ./conformance/02-lifecycle`
passed with all 21 example subtests listed. Temporarily changing the
`train results` row to expect `results` made the test fail with "recorded [],
contract says [\"results\"]"; the contract was then restored.
`TestContractStatus`, `TestConformanceLayout`, `./internal/doclinks`,
`go vet`, and `gofmt` passed. A full `scripts/check.sh` was not rerun.

**Remaining:** None. Examples need commands that are harmless to run (no
`srun`), because each row runs.

## Output and input keys in PATH-R5

**Commit:** `4879815` — 2026-10-04T15:11:38+09:00

**Change:** PATH-R5 also accepts a value under an output or input key
(`output`, `out`, `input`, or a key ending in `_output`, `_out`, `_input`)
unless the value is a format name (a PATH-R4 extension without its dot,
`text`, `html`, `xml`, `md`, `markdown`, `table`, `stdout`, `stderr`, `stdin`,
`-`). `DiscoveryVersion` is now 2. Added classifier fixtures, a recorded
contract example (`--output results --input data`), and a format key to the
not-recorded `--output png` example. Updated the conformance version check,
the plan's PATH-R5 text and anchor table, and docs/INSPECT.md.

**Reason:** The user noted that a bare directory name such as `--output
results` was never found. Since a false positive only costs a candidate that
cannot be shown, the user chose to add output/input keys while still skipping
format names.

**Plan impact:** Revises the PATH-R5 decision that generic `output`/`input`
names alone were insufficient. Bare positional names (`cp -r src results`)
remain undiscovered.

**Validation:** The seven new acceptance fixtures failed before the rule
change and passed after it. `go test -count=1` of `./internal/artifact`,
`./internal/projectrun`, `./internal/jobstatus`, and `./internal/doclinks`
passed. `TestArtifactCandidateExamples` (with the new rows listed under `-v`),
`TestStartedAttemptRecordsArtifactCandidates`, and `TestContractStatus`
passed. `go vet` and `gofmt` were clean. A full `scripts/check.sh` was not
rerun.

**Remaining:** None.

## Text commands (PATH-X5)

**Commit:** `778857d` — 2026-10-04T15:14:07+09:00

**Change:** Added PATH-X5: `echo` and `printf`, recognized by the shared
`recognizeCommand` at argv[0] or behind `env`, `timeout`, or `srun`, have
none of their arguments classified, and the launcher scan stops at them.
`DiscoveryVersion` is now 3. Added argument fixtures (direct, absolute
`/bin/echo`, `printf`, behind `env`, `srun` stopping at `echo`, and `echo`
as an ordinary argument of another command), moved the `echo bash -c
output.csv` contract example to "Not recorded", added a launcher example,
and updated the conformance version check, the plan (PATH-X5, the launcher
limitation and boundary tables), and docs/INSPECT.md.

**Reason:** The user asked to exclude `echo bash -c output.csv` by making
`echo` a known command; `printf` is the same kind of command.

**Plan impact:** Resolves the `srun echo bash -c output.csv` false-match
limitation.

**Validation:** Five new argument fixtures failed before the change (the
`srun` case after adding a trailing `x.csv` so it could tell the scan
stopping at `echo` from `bash -c` hiding the word) and passed after it.
`go test -count=1` of `./internal/artifact`, `./internal/projectrun`, and
`./internal/doclinks`, `TestArtifactCandidateExamples`,
`TestStartedAttemptRecordsArtifactCandidates`, and `TestContractStatus`
passed. `go vet` and `gofmt` were clean. A full `scripts/check.sh` was not
rerun.

**Remaining:** None.

## Configuration file contract examples

**Commit:** `a3d128c` — 2026-10-04T15:17:22+09:00

**Change:** Added a "Configuration files" table with nine fixture files to
the artifact candidate examples in contracts/02-run-lifecycle-and-execution.md:
nested YAML, skipped values, uncombined separate keys, aliases and custom
tags, JSON and TOML key order, a configuration named inside another (not
read), a parse failure, a missing file, and a YAML log destination (not read).

**Reason:** The user asked for configuration parsing examples in the
contract, executed like the argument examples.

**Plan impact:** None; the rows document the phase 2 behavior.

**Validation:** `go test -count=1 -run TestArtifactCandidateExamples -v
./conformance/02-lifecycle` passed with all ten new rows listed as
subtests. `./internal/doclinks` and `TestContractStatus` passed. No code
changed.

**Remaining:** None.

## Contract rule tables

**Commit:** `cfe7914` — 2026-10-04T15:21:23+09:00

**Change:** Rewrote the contract section as "Artifact candidate rules": tables
for where values are read (argv forms, environment, configuration, log
destinations), every exclusion (PATH-X1 to X3, X5), interpreter code operands
(PATH-X4, per interpreter), and every positive rule, listing all PATH-R4
extensions, all PATH-R5 key patterns, key normalization, and all format names;
then the remaining examples and the configuration file examples. Rows that
the rule tables now cover were removed from the examples. The conformance
harness now finds the command cell in tables of any layout and, for rows whose
first column names a `PATH-R` or `PATH-D` rule, checks that every recorded
candidate has a source accepted by that rule.

**Reason:** The user agreed to list every key pattern as rules rather than
scattered examples, keeping examples only for combined behavior.

**Plan impact:** None; the tables document existing behavior.

**Validation:** `TestArtifactCandidateExamples` passed with 64 row subtests
counted under `-v`. Relabeling the PATH-R2 row as PATH-R3 failed with
"/data/in was not recorded by PATH-R3"; the contract was restored.
`TestStartedAttemptRecordsArtifactCandidates`, `TestContractStatus`,
`TestConformanceLayout`, `./internal/doclinks`, `go vet`, and `gofmt`
passed. `srun` rows are left to package tests because running them would
submit jobs on a host with Slurm. A full `scripts/check.sh` was not rerun.

**Remaining:** None.

## Per-run configuration cache and size limit

**Commit:** `cd6a968` — 2026-10-04T15:33:26+09:00

**Change:** Added `artifactsource.Cache`, which parses a configuration file
once per version (path, size, modification time) for one run and caches
parse errors too; the run's `artifactRecorder` uses one per run.
`artifact.Discover` now takes a `ConfigReader` (`ParseSources` keeps the
byte-reader form). `MaxSourceBytes` is 256 KiB (was 1 MiB): a larger file is
rejected from its stat, without being read or parsed. `MaxSourceValues` is
50,000 (was 100,000). `DiscoveryVersion` is 4. Updated the plan and
docs/ARCHITECTURE.md.

**Reason:** Benchmarks showed YAML parsing at about 3.5 MB/s (4 KB in
1.3 ms, 100 KB in 30 ms) and discovery running once per attempt in the engine
loop, so a 1000-task array with a 100 KB configuration delayed submission by
about 30 s. The user also asked that abnormally large files stop analysis.

**Plan impact:** Revises the phase 2 budgets and the reader design.

**Validation:** The cache test (same size and time returns the cached
parse; a new time or size reparses; parse errors cached; an oversized file
that is unreadable is still rejected by size) and
`TestArtifactRecorderParsesAConfigOncePerRun` passed. The latter failed with
the uncached reader temporarily wired in. `./internal/artifact`,
`./internal/projectrun`, `./internal/jobstatus`, `./internal/archtest`,
`./internal/doclinks`, and the RUN-9 conformance tests passed. A full
`scripts/check.sh` (vet, test, race) started after this commit passed with
"all checks passed"; it also covers the commits since `4879815` that recorded
"full check not rerun".

**Remaining:** None.

## Shell code inspection (phase 4, steps a, c, d)

**Commit:** `9cffb94` — 2026-10-04T15:49:19+09:00

**Change:** Added `internal/artifact/shell.go`, which parses the code operand
of a recognized shell with `mvdan.cc/sh/v3/syntax` v3.13.1 (POSIX for
`sh`/`dash`, Bash, Zsh) and never runs it. Each simple command's words go
through the shared walker (`commandWords`, refactored out of `arguments`,
so the interpreter and text-command rules apply unchanged). Literal
file-opening redirection targets are PATH-R1 references with their operator.
Plain `$NAME`/`${NAME}` expand for `ROTARI_ARRAY_TASK_ID`, `ROTARI_JOB_DIR`,
and the job's own variables unless the source assigns the name (PATH-E1).
Other expansions, substitutions, globs, brace expansion, and a leading `~`
skip the word, and relative references after `cd`/`pushd`/`popd` are
skipped. Nested shells and quoted-delimiter heredocs read by a shell are
inspected up to three levels. The recognizer now reports the shell dialect,
script operand, and standard-input use. Configuration files named in shell
code are read. `artifact.Job.Variables` carries the attempt values, filled by
`artifactRecorder` from the prepared environment. Sources gain `direction`
and `expanded`. `DiscoveryVersion` is 5. Added the contract's "Shell source"
table (17 rows), updated the interpreter rows and examples whose shell code
is now inspected, added `TestShellVariablesDifferPerArrayTask`, and updated
`TestStartedAttemptRecordsArtifactCandidates`, whose `sh -c` body now
yields `code/unused.csv`. Updated the status row, docs/INSPECT.md,
docs/ARCHITECTURE.md, and the plan. `go.mod` gains `mvdan.cc/sh/v3`;
`golang.org/x/sys` rises to v0.42.0 by minimum version selection.

**Reason:** The user agreed to phase 4 after asking whether `echo aa >
output.txt` could be found; the dependency was confirmed with the plan. v3.14
requires Go 1.26, so v3.13.1 is the newest usable release.

**Plan impact:** Steps (a), (c), and (d) are done; (b), referenced script
files, remains. New decision: configuration files named in shell code are
read.

**Validation:** `TestShellInspection` (30 cases: the plan's redirection table,
heredoc cases, `cd`, globs, PATH-E1 forms, nesting, zsh) and
`TestShellProvenanceAndDiagnostics` passed; the one wrong expectation (an
absolute path is PATH-R2) was a test error. `TestDiscoverReadsConfigsNamedInShellSource`
failed before the change and passed after it.
`TestExecuteExpandsTaskVariablesInShellSource` (array tasks, members by
environment, attempt directory) passed once the test configured the run
directory name as real runs do. Conformance: `TestArtifactCandidateExamples`
(81 rows), `TestShellVariablesDifferPerArrayTask`, and
`TestStartedAttemptRecordsArtifactCandidates` passed; `TestContractStatus`,
`TestConformanceLayout`, `go vet ./internal/...`, `gofmt`, and the
artifact, artifactsource, projectrun, jobstatus, archtest, and doclinks
packages passed. A full `scripts/check.sh` was not rerun.

**Remaining:** Phase 4 step (b), referenced shell script files.

## Shell script files (phase 4, step b)

**Commit:** `f824382` — 2026-10-04T15:53:14+09:00

**Change:** The script operand of a recognized shell (in argv or shell
source) and every `.sh` candidate from an argument, environment value, or
shell source are read once per attempt and inspected like shell code,
breadth first within three nesting levels. The invoking shell or the `#!`
line (directly or through `env`) decides the dialect; a script without one
is read as Bash, and one naming another interpreter is not read.
`artifact.Discover` now takes `Sources` (configuration and script readers);
`artifactsource.Cache.Script` caches script contents per version for the run,
at most 16 MiB in total. `DiscoveryVersion` is 6. Added the contract's "Shell
scripts" table with six fixture files, and updated docs/INSPECT.md,
docs/ARCHITECTURE.md, and the plan (phase 4 done).

**Reason:** Phase 4 step (b), completing shell inspection.

**Plan impact:** Phase 4 is done.

**Validation:** `TestScriptInspection` (13 cases: operands, extensionless
scripts, `.sh` behind an unparsed launcher, shebangs, nested scripts, a
cycle read once, the nesting limit, missing and unparsable scripts,
PATH-E1, `cd`, scripts in shell code), `TestScriptProvenance`,
`TestScriptsNeedAKnownBase`, and `TestCacheReadsAScriptOncePerVersion`
passed. `TestArtifactCandidateExamples` passed with 87 rows, along with the
other RUN-9 conformance tests, `TestContractStatus`, and
`TestConformanceLayout`. A full `scripts/check.sh` (vet, test, race) started
after this commit passed with "all checks passed".

**Remaining:** Phase 6, Python source.
