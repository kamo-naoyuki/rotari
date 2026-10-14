# Plan: Bug Report Bundle

**Created:** 2026-10-14
**Status:** Phase 1 done. Phase 2 in progress; Phases 3–4 not started.

## Purpose

When rotari misbehaves for a user, the developer needs the same facts every
time: the version, the resolved paths and configuration, the scheduler and
filesystem environment, and the state files of the run that went wrong. Today
these are spread over `version`, `info`, `check --deep`, `env`, `server.log`,
`show --report`, and files under the run directory. Users collect them by
hand, which is slow and leaves gaps.

This plan adds `rotari bug-report`, which collects those facts into a
Markdown summary on stdout or a `.tar.gz` bundle, with secrets removed.

A Slurm bug reported on 2026-10-14 (a running job recorded as finished with
error `running`; see the Resolved entry in [ISSUES.md](../ISSUES.md)) showed
that a bundle alone is not enough. The bundle would show the symptom and the
Slurm job ID, but rotari discarded the squeue and sacct errors and did not
record when the queue query failed or recovered. That timeline was needed to
find the second half of the bug, the accounting deadline carried over from an
earlier failure. Phase 1 records it.

## Non-goals

- Uploading the bundle or opening a GitHub issue. The user reviews and
  attaches the file.
- A `doctor` command that suggests fixes. `check --deep` and the rule-based
  job diagnosis (`internal/diagnose`) cover user-side problems; this command
  gathers evidence for rotari's own bugs.
- Web UI, Web API, MCP, and Python client entry points beyond what the
  generated Python CLI wrapper provides. The bundle reads local files and
  runs scheduler commands on the user's host, so the CLI is the interface.
- Recording every scheduler query. Only changes in outcome are logged
  (Phase 1, D2).
- Moving `info` and `check` out of `cmd/rotari`. The command passes their
  output to the new package as facts (D6); moving them is a separate
  refactoring.

## Current implementation

- `rotari version` prints the release or the VCS revision
  ([cmd/rotari/version.go](../../cmd/rotari/version.go)).
- `rotari info` reports basedir, masterdir, project, visible configuration
  files, supervisors, run locks, and active runs
  ([cmd/rotari/info.go](../../cmd/rotari/info.go)); `rotari check --deep`
  validates project state ([cmd/rotari/check.go](../../cmd/rotari/check.go));
  `rotari env` lists `ROTARI_*` variables
  ([cmd/rotari/environment.go](../../cmd/rotari/environment.go)).
- The supervisor writes `<basedir>/server.log`.
- A run directory holds `summary.json`, `commands.json`, `context.json`,
  `progress.jsonl`, `load_samples.jsonl` (load average over the run),
  `notes.jsonl`, `carried.json`, `sources.json`, and `client_status.json`.
  A job attempt directory holds `job.json`, `status.json`,
  `scheduler_status.json`, `submitted_at`, `finished_at`, `pid`,
  `cancelled`, and `stdout` / `stderr` / `output`.
- `waitForSchedulerResult`
  ([internal/executor/scheduler_shared.go](../../internal/executor/scheduler_shared.go))
  polls the queue (`squeue`, `qstat`, `bjobs`) and then accounting (`sacct`,
  `qstat -x`, `qacct`, `bjobs -a` / `bhist`) for Slurm, PBS, SGE, and LSF.
  Before Phase 1 a failed query became `Failed: true` and its error and
  command output were dropped; only the last state reached
  `scheduler_status.json`.
- `internal/report` builds `show --report` with path and hostname redaction
  on by default (WEB-8); `RedactPatterns` is the generic part. MCP-2
  replaces environment values and executor options with `[REDACTED]`.
- Secrets live in configuration: `notifications.toml` (webhook URLs and
  headers), the `web` auth token (`ROTARI_WEB_AUTH_TOKEN` or configuration),
  and job environment values in `job.json` and `commands.json`.

## Decisions

- **D1 Event log file.** Each scheduler job attempt directory gets
  `scheduler_events.jsonl` (`state.SchedulerEventsFileName`). Each line has
  `at`, `query` (`queue`, `accounting`, `result`), `state`, `error`, and,
  for `result`, `exit_code`. `result.state` names the source: `wrapper`,
  `accounting`, or `unavailable`.
- **D2 Log only changes.** A queue or accounting line is written when that
  query's outcome (state, or error text) differs from its previous outcome
  in the same wait. A query that succeeded with no state is `missing`. A
  long outage therefore adds a few lines rather than one per poll. The file
  stops growing at 256 KiB; error text is cut at 1000 bytes. A supervisor
  restart starts a new wait, so the current outcome is logged again, which
  also marks the restart.
- **D3 Errors keep command output.** The queue and accounting functions wrap
  their errors with `SchedulerCommandHint`, so a failure reads
  `exit status 1: slurm_load_jobs error: Socket timed out ...` instead of
  `exit status 1`. `schedulerQuery` and `schedulerAccounting` carry `Err`
  instead of `Failed`.
- **D4 Command shape.** `rotari bug-report [-p PROJECT] [--run-id RUN]
  [--job JOB]... [--output FILE.tar.gz] [--full-output | --no-output]
  [--no-redact]`.
  - Without `--output`, a Markdown summary goes to stdout, ready to paste
    into an issue. With `--output`, the bundle is written as a `.tar.gz`
    holding the same summary as `README.md` plus the collected files, and
    stdout names the file and asks the user to review it.
  - The run is `--run-id`, or the project's latest run, resolved as other
    commands resolve it. Without a project or a run, the bundle holds only
    the environment sections.
  - Jobs are those named by `--job`, or else the run's failed and unfinished
    jobs, up to 50; the summary says when the list was cut.
- **D5 Job output.** Some bugs cannot be judged without the job's own
  output, so each included job's `stdout`, `stderr`, or `output` is included
  by default as its last 64 KiB. `--full-output` includes whole files;
  `--no-output` leaves them out.
- **D6 Package boundary.** `internal/bugreport` owns collection, redaction,
  scheduler probing, the Markdown summary, and the tar writer. `cmd/rotari`
  parses options, resolves the project and run, and passes the `version`,
  `info`, and `check --deep` results in as already-encoded facts.
  `internal/bugreport` does not import `cmd/rotari`.
- **D7 Secrets are always removed.** Regardless of `--no-redact`:
  configuration values whose key names a secret (`token`, `password`,
  `secret`, `key`, `auth`, `header`, `url` in `notifications.toml`) become
  `[REDACTED]`; `ROTARI_*` variables are listed with secret values replaced;
  job environment values in `job.json` and `commands.json` become
  `[REDACTED]` while their names stay, as in MCP-2. Job output is not
  scanned for secrets; the summary says so.
- **D8 Paths and hostnames.** Redacted by default with `internal/report`'s
  redaction, matching `show --report` (WEB-8). `--no-redact` keeps them,
  for a report shared within a trusted team or for path-handling bugs.
- **D9 Environment facts.** The summary includes: version and VCS revision,
  Go version, OS and architecture; the `info` report; `check --deep`
  results; the filesystem type of the basedir (`statfs`, so NFS is visible);
  clock skew between this host and the basedir's filesystem (write a probe
  file, compare its mtime with the local clock, remove it); the scheduler
  commands found in `PATH` with their `--version` output; the last 1 MiB of
  `server.log`.
- **D10 Live scheduler state.** For each included scheduler job, the bundle
  runs the scheduler's read-only queue and accounting commands (for Slurm,
  `squeue -j` and `sacct -j` with the fields rotari reads) and records their
  output and exit code. Only read-only commands run; each has the
  executor's command timeout.

## Phases

### Phase 1: Record scheduler query outcomes (done)

The `accounting_wait` start and clear events first considered were dropped:
while squeue fails and sacct reports the job running, the wait starts and
clears on every poll, and the queue and accounting transitions already give
the timeline. `TestWaitForSchedulerResultUsesInjectedTiming` now checks the
timestamps the injected clock produced instead of counting clock calls,
which the event log adds to.

1. `internal/executor/scheduler_events.go`: `SchedulerEvent`,
   `schedulerEventLog` (D1, D2).
2. `waitForSchedulerResult` and `resolveMissingSchedulerState` log queue and
   accounting outcomes and the result; `schedulerQuery.Err` and
   `schedulerAccounting.Err` replace `Failed` (D3).
3. Slurm, PBS, SGE, and LSF queue and accounting functions wrap errors with
   `SchedulerCommandHint`.
4. `state.ValidatedStateFile` accepts `scheduler_events.jsonl`.
5. Tests: the event sequence for a queue failure followed by recovery, for
   an accounting state while the queue fails (the Slurm bug's shape), for a
   result from each source, deduplication of repeated outcomes, and the size
   limit. Existing tests change only from `Failed: true` to an error value.
6. Contract note next to `scheduler_status.json` in
   [contracts/02-run-lifecycle-and-execution.md](../../contracts/02-run-lifecycle-and-execution.md).

### Phase 2: `internal/bugreport` and the command

1. Collector over the inputs in D6, producing named entries and the summary
   (D4, D5, D9, D10).
2. Redaction (D7, D8), with tests that fixtures holding a webhook URL, an
   auth token, and a job environment secret never appear in the summary or
   any bundle entry, with and without `--no-redact`.
3. Tar writer: regular files only, entry names built from fixed names and
   validated path elements (no `/` or `\` from run or job IDs), file mode
   0600 for the output, refusal to overwrite an existing file.
4. `cmd/rotari/bug_report.go`, the `cli_spec.go` entry, and the dispatch
   case. Regenerate `docs/CLI_REFERENCE.md` and
   `python/rotari/generated_cli.py`.

### Phase 3: Documentation and contracts

- A contract rule for the secret guarantee (D7) and the bundle's contents,
  with a conformance test through the built binary that reads the tarball.
- `docs/FAQ.md` (how to report a bug), `docs/OPERATIONS.md` or
  `docs/RECOVERING.md` where they discuss collecting evidence, and
  `docs/ARCHITECTURE.md` for the new package and command.

### Phase 4: Check the bundle against the Slurm bug locally

Reproduce the 2026-10-14 bug with fake `squeue` (failing) and `sacct`
(`RUNNING|0:0`) scripts on `PATH`:

1. In a temporary worktree at `939ceeb14` (before the fix), apply Phase 1 and
   build; run a Slurm job against the fakes until the run ends with error
   `running`.
2. Run `bug-report --output` from the current build on that state.
3. Judge whether the bundle alone shows both causes: the sacct `running`
   state taken as a result, and the deadline set by an earlier queue
   failure. Record gaps and adjust the collected facts.
4. Repeat for one more reported or constructed bug, outside the scheduler
   path, to check the environment sections.

## Validation

- `go test ./internal/executor ./internal/state` for Phase 1;
  `go test ./internal/bugreport ./cmd/rotari` for Phase 2;
  `go test ./conformance` for Phase 3.
- `scripts/check.sh --short`, then `scripts/check.sh` before each commit.
