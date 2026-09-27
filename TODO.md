# TODO

## Near-term candidates

These are smaller steps that can come before the AI roadmap in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md). Choose them by whether
they make it cheaper to run a batch from a script, see what happened, and
retry what failed, not by whether another workflow engine has them. Prefer
features that serve the script over features that turn the queue into a
workflow definition.

### Orchestration

- Consider run-only overrides of stored job settings (`--retry` and its
  delays, `--timeout`, `--executor-option`, `--env`) that take precedence for
  one run without writing them back to the queue. Run-level options such as
  `run --retry` only act as defaults for jobs without their own value, and
  `change --stage/--matrix/--all` writes the queue.
- Consider making the grace period between a timeout's SIGTERM and SIGKILL
  configurable (fixed at 30 seconds), together with the stop signal below.
- Consider run-level retry delay defaults (`run --retry-delay` and friends)
  for jobs without their own, like `run --retry` for the limit.
- Consider deciding retries from the rule-based diagnosis of a failure rather
  than its exit code, which rarely tells causes apart (most Python errors
  exit with 1). A user-written rule would map a diagnosis to an action:
  retry as is for transient causes such as a stale NFS handle or a network
  timeout; retry with changed settings, such as halving a `BATCH_SIZE`
  environment variable or requesting a larger GPU through executor options
  after `CUDA out of memory`; or do not retry deterministic failures such as
  `ImportError`. rotari should not guess the changed settings. Each attempt's
  settings must be recorded so `show`, `diff`, and the Web UI's per-job
  attempt history can show what changed between attempts.
- Consider concurrency limits for a named group of jobs (for example "at most
  four GPU jobs at once"). Limits are per executor today.

- Consider a per-job stop signal (for example `add --stop-signal USR1`) so
  cancellation can let training scripts save a checkpoint before exiting.
  Slurm has a matching `--signal` option.

- Decide how the working directory and environment of `run` reach jobs on
  each executor. The design is that a queue holds commands only and jobs
  take the caller's directory and environment (see "Workflow and execution
  environment" in [docs/CONCEPTS.md](docs/CONCEPTS.md)), but only the local
  executor guarantees it; the others keep their own defaults. The SSH
  wrapper applies only the job's own `--env` and `--working-directory`, so
  without them a job starts in the remote login directory with the remote
  login environment. Slurm, PBS, and LSF submit with the scheduler's
  defaults: `sbatch` and `bsub` usually carry the submitting directory and
  environment, while `qsub` starts in `$HOME` without `-V`, and sites can
  change all of these. Decide per executor whether to carry the caller's
  directory (and which variables, if any) or to document the difference,
  then make CONCEPTS.md and the README state it per executor.

For field design, Dagu's step options are a useful reference
(`dagu/internal/spec/step.go`), for example `signal_on_stop`.

Each of these must define its behavior for every executor (local, SSH, Slurm,
PBS, LSF) and be tested per executor. Add them one at a time.

Out of scope: output passing between jobs, conditional branches, loops, cron
scheduling, and event triggers. They turn the queue into a workflow language or
belong to operating workflows that are already settled. Resource allocation
(GPUs, memory, nodes) is out of scope too: it belongs to the scheduler or other
middleware, and rotari only limits how many jobs it runs or submits at once.

### Runs as experiment versions

A run snapshots the queue the script built, so the run sequence records what
the experiment actually ran. See "Runs as history" in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md).

What is missing today: `show RUN` describes a run on its own, so whether the
last fix worked needs a separate `diff`; rule-based diagnoses are per attempt,
so the causes of a run's failures cannot be seen together; one job cannot be
followed across runs; and runs are listed twice, by `show -p` (status and
times) and by `show --lineage` (counts and changes). The first two matter
most.

- Consider one command for runs as generations, replacing `show --lineage`
  and absorbing `diff`, which share `internal/rundiff`: with a project it
  lists the generations, one summary line per run; with one run it prints
  that run's summary; with two runs it compares them as `diff` does. `show`
  stays the view of current state. Avoid the name `log`, which clashes with
  `show --logs`.
- The run summary should answer "did this generation improve on the last?"
  and "why did the remaining jobs fail?" together: result counts with their
  change, fixed / still failing / newly failing / changed / carried forward,
  elapsed time, and failures grouped by diagnosis rule and crossed with that
  classification (for example "OOM 8: still 6, new 2"), with no-match and
  unavailable as their own rows. Show the short form after `run` and `wait`
  finish, and the same summary on the Web UI run page. Job lists stay in
  `show RUN --failed` and the comparison view.
- A run has no unique parent. `JobOrigin` is per job and is set by `copy` and
  `retry`, not `add`; `copy --append` from two runs mixes origins, and
  `SourceRunID` is not saved. `diff` and `show --lineage` compare with the run
  that started just before, matching jobs by name. Prefer comparing each job
  with its origin job and treating jobs without one as new, so the summary is
  consistent without a run-level parent; show the origin breakdown (`from r4:
  180, r3: 20, new: 12`) and print `parent: RUN` only when it is unique.
  Decide separately whether the generation list is a provenance tree or one
  time-ordered sequence, so runs exploring separate branches are not listed
  as one sequence.
- Consider comparing three or more runs as a job-by-run grid of results,
  marking where a job's definition changed. It separates flaky jobs from
  persistently failing ones, which two runs cannot, and shows where each job
  broke. The generation list is the same data aggregated per run instead of
  per job. Decide whether marks are relative to the previous column or to a
  fixed baseline, how runs are chosen (a list, a range, the last N, or N
  generations up the origin chain), and how many columns the CLI shows before
  leaving the rest to the Web UI.
- Consider recording `state_version` in `meta.json`, `context.json`, and
  per-job files, as `queue.json`, `commands.json`, and `summary.json` already
  do. See "State load and write contracts" in
  [contracts/04-coordination-and-safety.md](contracts/04-coordination-and-safety.md).
- Building blocks for the above: `runs/<run-id>/commands.json`, `JobOrigin` in
  [internal/model/model.go](internal/model/model.go), and the queue-versus-run
  change count in `compareQueueWithRun` in
  [cmd/rotari/show.go](cmd/rotari/show.go).

### Files as the control path, the socket as a fast path

Today `run`, `cancel`, `suspend`, and `resume` work only through the server
socket, so they fail wherever the socket cannot be created, such as sandboxes
that block Unix sockets. Follow Dagu's design instead: files hold the
authoritative state and requests, and the socket only makes them take effect
faster. It also fits rotari's rule that durable behavior belongs in files. See
"No execution path when Unix sockets are unavailable" in
[ISSUES.md](ISSUES.md) for the Dagu reference and details.

- Consider letting `cancel` and `suspend` write a durable request file that the
  runner picks up, then notifying the server over the socket for immediate
  effect when it is reachable.
- Consider running a synchronous `run` in process when the socket cannot be
  created. This needs a design for coordinating concurrent runs on one basedir
  with file locks alone, and the unsupported-socket check should cover `EPERM`
  from seccomp sandboxes as well as `EAFNOSUPPORT`.

The short socket location for long base directories, the first step of this
plan, is done (`internal/server/socket.go`).

### Registry indexing without SQLite

Do not introduce SQLite for registry metadata. Keep the filesystem as the
authoritative store and add a separate basedir registry for discovery. The
current run registry is keyed by run ID (`master/runs/<run-id>.json`), which
keeps direct run lookup cheap but makes `show` scan every historical run just
to discover distinct basedirs.

The basedir registry should be a one-entry-per-basedir filesystem index under
the master directory. Register a basedir when a command creates or adopts
state there, including queue/project creation, run creation, import, and
copy. Registration must be idempotent. Read-only commands such as `show` and
`jobs` must not create registry entries as a side effect.

Use the basedir registry for basedir discovery, especially bare `show` and
`show --basedirs`. Keep the run registry as the authoritative run-ID lookup
for `show --run-id`, `wait`, retry, and other commands that resolve a
specific run. Registering a run should also register its basedir; deleting a
run should remove only its run-registry entry and must not remove the
basedir entry, because queues, projects, or other runs may still use it.

Do not add basedir-registry cleanup to the first implementation. A stale
basedir entry is harmless: discovery can skip it after a failed existence
check. The existing run-registry GC remains responsible for orphan run
entries, whose stale records can interfere with run-ID resolution. A separate
basedir GC can be considered later if stale discovery entries become a
measured problem, but it must not delete state directories.

Keep the run files, project state, and logs as the source of truth. The
basedir registry is only a discovery index and must be rebuildable or safely
discardable without losing job or run data. Add migration/backfill for
existing basedirs before relying on the new index, and retain a deliberate
fallback or explicit repair path for installations created by older
versions.

### Machine-checked guardrails for coding agents

Rules that live only in prose (AGENTS.md, the contracts) are easy for a coding
agent to break without noticing, and most `cmd/rotari` tests import
`internal/` packages, so a package-level refactoring has to edit the tests
that should be protecting it. Turn the rules into checks, cheapest and most
immediately useful first. Each step stands alone and can stop there.

1. **Widen conformance coverage.** Move `partial` and `pending` rows in the
   "Contract status" table of [contracts/README.md](contracts/README.md)
  toward `conformance`. The first run-lifecycle slice is now covered by
  `RUN-1` and `RUN-2` (failure then filtered rerun, retries within a run).
  Next, in order: the remaining run lifecycle rules, then server and command interfaces in
   [contracts/03-server-and-command-interfaces.md](contracts/03-server-and-command-interfaces.md);
   the `pending` RES rows; and the `partial` rows, whose gaps the table and
   the test comments name (COORD-4 covers only Slurm, COORD-5 not the
   registry, SEL-1 and SEL-2 not every command, DUR-6 not `reset --recover`).
2. **Golden output files.** Golden files with an `-update` flag for `--help`,
   `schema --json`, and representative `show --json` output, so an
   unintended output change shows up as a diff. These now live under
   `conformance/testdata/golden/`; schema and show output are normalized to
   keep the files maintainable.
3. **Mirror the contract documents in conformance.** Organize tests under
  directories matching the contract Markdown structure, rather than making
  one directory per contract ID. Keep the existing `00` through `06` topics
  as the top-level groups, and split a large Markdown file into responsibility
  files when its sections are unrelated (for example, lifecycle, cancellation,
  validation, orchestration, and integrations under `02`). A test may cover
  several IDs from one document, and each directory should link back to its
  Markdown section. Extract the shared harness first, then move one document
  or subsection at a time; keep `go test ./conformance/...` and the contract
  status scan working throughout the migration. Extend `TestContractStatus`
  to verify the prefix-to-Markdown mapping and recursively discover tests in
  the new directories; do not rely on directory names being correct by
  convention alone.

Done so far: the package boundary test
([internal/archtest](internal/archtest/boundaries_test.go)), the
documentation link test
([internal/doclinks](internal/doclinks/links_test.go)), one local check
command ([scripts/check.sh](scripts/check.sh)), the conformance harness
([conformance/](conformance/)), and contract IDs with a status table that
`TestContractStatus` keeps in agreement with the tests' `covers` calls.
Conformance covers path rules, cancellation, the resolution rules, the
status fallback chain, the selector tables, and all of
[contracts/04-coordination-and-safety.md](contracts/04-coordination-and-safety.md).
The fallback and selector tests were moved from in-process `cmd/rotari`
tests. The harness is hand-written rather than `testscript`: the Web API
checks and JSON comparisons need Go code either way, and it adds no
dependency.
The current migration has moved resolution and display-time tests under
`01-resolution`, run lifecycle and cancellation under `02-lifecycle`, command
checks under `03-interfaces`, coordination, state, and projection checks under
`04-coordination`, and selector/path tests under `06-selectors`. SAFE recovery
and orphan-process durability tests are also under `04-coordination`.
Root `harness_test.go`, `contracts_test.go`, and `golden_test.go` are shared
test infrastructure and may remain at the package root.

#### Handoff status

- Completed and committed: document-to-directory layout, recursive contract
  checking, all `01` through `06` conformance migrations, golden outputs,
  and the wait/copy/show selector deduplication in `cmd/rotari`.
- The moved durability tests (`DUR-3`, `DUR-4`, and `DUR-6`) use the
  package-local support fixture under `04-coordination`.
- All pending contract rows are covered by at least one conformance test.
  Remaining coverage is the `partial` rows in `contracts/README.md`, notably
  CORE/DUR, RES, CLI-1, and SEL-1/2.
- Golden test operation is documented in
  [`conformance/README.md`](conformance/README.md); `-update` refreshes
  `conformance/testdata/golden/`.
- Recent cleanup commits: `857de4c` (wait), `333d51d` and `41f99fa` (copy),
  `bc87a21` and `c8d2e2f` (show), and `d126dde` (mixed-run retry).
- `scripts/check.sh --short` still cannot complete in this sandbox because
  Unix sockets return `operation not permitted` in internal/server tests.
- Known flaky tests are recorded in [ISSUES.md](ISSUES.md), notably
  `TestControlFromAnotherHost` and `TestResetOfInterruptedProject`; do not
  treat those failures as migration regressions without reproducing them in
  isolation.

#### Working notes for the next agent

How one contract section has been done, one commit per step:

1. Read the section and sort each rule into an observable contract (what the
   CLI, the Web API, or the files on disk show), a design rule, or an
   implementation note. Rewrite the observable ones so they name no Go
   function, give them IDs (`PREFIX-N` at the start of the rule; add the
   prefix to the table in [contracts/README.md](contracts/README.md)), and
  move function names and file links to an "Implementation and tests" note
  after the rules. When contract rules and internal implementation/spec
  notes would otherwise be mixed, put them in separate subsections rather
  than interleaving them in one list.
2. Before writing expectations, probe the real behavior with the built
   binary: a throwaway `conformance/zz_probe_test.go` that logs output, run
   with `-v`, then deleted. Every section so far turned up at least one bug
   this way (async and Web cancel, `wait` ignoring the registry, `show` of a
   job that never ran, `unlock` of a live run, readers of newer state).
3. Write the conformance test with `covers(t, "ID")`. If the current
   behavior breaks the rule, skip that case with `knownDeviation(t, "ID")`,
   set the row to `deviation`, add an ISSUES.md entry naming the ID, and
   commit. Then fix the code, remove the skip, move the row to `partial` or
   `conformance`, move the ISSUES.md entry to Resolved, and commit again.
4. When a conformance test covers what an existing in-process test under
  `cmd/rotari` or `internal/` checks from the outside, move that observable
  case to `conformance/` and delete the old test and its now-unused helpers.
  Search both trees before adding a new case. Keep unit tests of internal
  branches and implementation details in their original package.
5. Run `scripts/check.sh` (with `-race`) before each commit.
6. When an unrelated bug or design concern is discovered during the work,
   record it in [ISSUES.md](ISSUES.md) with enough context to act on it later;
   do not broaden the current fix just to resolve it.

Pitfalls met so far:

- `ROTARI_BASEDIR` counts as an explicit location, so a run ID that the
  registry places elsewhere is rejected. Tests of registry lookups use an env
  without it (`e.without("ROTARI_BASEDIR")`, as the selector fixture does).
- In a subtest, use `e.in(t)` so failures report to the subtest. Parallel
  subtests need their own fixture; building one takes well under a second.
- A second synchronous `run` of a project whose first run is live blocks on
  its jobs if it is wrongly accepted; probe with `--async` or a timeout.
- After SIGKILL, a supervisor stays a zombie, which counts as alive, until
  its parent reaps it. Wait for `check` to report `interrupted`
  (`waitForInterrupted`) instead of checking at once.
- Job wrappers trap SIGTERM; clean up with `killStrays` (SIGKILL to the
  process group). Never use `pkill -f` with a pattern that also appears in
  your own shell command: it kills the shell. Kill by PID.
- State files are indented JSON; edit them by decoding, as `setJSONField`
  and `setStateVersion` do, not by string replacement.
- `curl` to the Web server may go through an HTTP proxy from the
  environment; use `--noproxy '*'`. Go's client never proxies loopback.
- When removing an ISSUES.md entry, keep the blank line before
  `## Resolved`; one edit lost it once.
- Other agents may be working in the same tree. Stage only your own files,
  check `git status` before committing, and re-stage a `git mv` if another
  commit reset the index.

Not planned: decision records beyond the existing rationale in the contracts,
and tool-specific agent hooks or skills; AGENTS.md stays the tool-neutral
entry point.

### Web UI

- Polish the overall look; it is the first impression in demos and still looks
  plain next to Dagu's UI. Consider tabs for run page sections and a sidebar
  for navigation between projects, runs, and the info pages.
- The job table is far wider than the page and overflows to the right.
  Reconsider which columns are shown by default, wrapping, and a horizontal
  scroll container limited to the table.
- Info pages such as CLI docs, Environment variables, and Job activity link
  back only to the project list. Make it easy to return to the project or run
  page the user came from.
- The Load average section is the only run section without summary text on
  the right of its header; show, for example, the peak or latest load there.
- A report for several selected jobs repeats the redaction notice ("Paths and
  hostnames are redacted where detected. ...") once per job. Show it once per
  report. Only the static export repeats it: `rotari web` builds one report
  with `buildAIReportForJobs`, but the static bootstrap joins the per-job
  reports, each already redacted
  ([web_static_bootstrap.js](internal/webui/assets/web_static_bootstrap.js),
  `/api/report`).
- Add an option to generate reports without redaction, for sharing within a
  trusted team. Decide whether it is a CLI flag (`show --report`), a Web
  toggle, or both, and keep redaction the default.
- Matrix grid: consider clicking a row or column heading to filter the job
  table to that parameter value.
- Consider a per-job attempt history that shows how the command or executor
  options changed between attempts.
- Consider grouping failed jobs by rule-based diagnosis result.
- Consider a run timeline built on the run lineage above.

Suggested first steps: the report redaction notice and option, which are
small, then the table width and navigation, which most affect everyday use.

## Ideas, not decided

Thoughts from reviewing rotari's positioning against Dagu and the tools in
[docs/TOOL_COMPARISON.md](docs/TOOL_COMPARISON.md). Each may be worth doing,
or may turn out not to be; decide before starting any of them.

- **Record more of the environment a run used.** A queue leaves the
  environment to the caller, so the record is the only way to tell later what
  a result ran with, yet only the caller's directory is kept (`context.json`,
  `ROTARI_CWD`). Candidates: the git commit and dirty state of that
  directory, the active conda or virtualenv, the host, and an allowlist of
  variables (for example `CUDA_VISIBLE_DEVICES`, `PYTHONPATH`). A hash of the
  whole environment would let `diff` say that it changed without storing
  secrets. Show differences between runs in `diff` and the run summary.
- **Keep the script the center.** The normal use is to run a script of
  `rotari add` lines and read the history and logs afterwards; the queue is
  emptied after each run, so rerunning the script starts the next run.
  Editing inside rotari (`change`, `copy`, manifests) is an occasional
  extra. Keep it out of the first-contact path, and do not design new
  features that assume users edit the queue rather than the script.
- **Keep the first-contact surface small.** The README lists 27 commands and
  the concepts include projects, queues, runs, jobs, attempts, stages,
  matrices, arrays, and manifests. Consider `rotari --help` listing the core
  commands (`add`, `run`, `show`, `retry`) first and the rest under an
  advanced heading, and check each new feature against whether a first-time
  user has to learn it.
- **Treat coding agents as a primary user.** A stateful CLI with `diff`,
  `guide`, and `--json` output fits an agent running and retrying batches.
  Consider a worked example of an agent-driven sweep in the docs, and a
  stable, documented JSON schema for the outputs an agent reads.
- **A FAQ entry for sharing GPUs on a workstation.** Since rotari will not
  allocate them, show the options: a per-job `--env CUDA_VISIBLE_DEVICES=...`
  with `--local-concurrency`, a single-node Slurm, or task-spooler behind the
  local executor.
