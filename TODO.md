# TODO

## Selector and positional contract changes (decided, in progress)

Decided on 2026-09-26 after reviewing
[docs/contracts/06-selectors.md](docs/contracts/06-selectors.md). Update the
contract and `TestSelectorTable` / `TestPositionalArguments` with each item,
and remove the item here once it is done.

- `run --job-id` selects from a non-empty queue instead of replacing it with
  the latest run, as `run --failed` does.
- `delete` without a run deletes nothing; deleting every run needs `--all`.
- An array command's job ID or name selects the whole array in `show`,
  `copy`, `run`, and `retry`; a task ID or name selects that task.
- `change` and `remove` say that attempt IDs are not accepted.
- `latest` is accepted wherever a run can be given, and is a reserved word:
  project, run, job, stage, and matrix names cannot be `latest`.
- Reading and editing commands report a project that does not exist as
  `project "x" does not exist`; commands that create projects are unchanged.
- Without `--project-name`, a job selector searches every project in every
  command, `diagnose` included; an ambiguous match lists the candidates.
- `show NAME` also finds active runs; `wait NAME` and `wait PROJECT` wait for
  the active run, or else return the latest matching run's result.
- Without a run: options that only apply to runs (`--failed`, `--logs`,
  `--report`, ...) use the active, interrupted, or latest run; other views use
  a non-empty queue, else the latest run, and say which. `export` picks its
  source the same way but refuses an active run (pointing to `wait`) and an
  interrupted run (pointing to `unlock`), and reports which source it wrote.
- Rename `diff --all` to `--unchanged` and `jobs --all` to `--all-basedirs`.
- `show` gains `--unfinished` and `--success`; `diagnose` gains `--job-name`;
  `run` and `retry` accept a positional `RUN_ID`.

- Consider extending `cancel`/`suspend`/`resume` selectors to also accept a
  `run_name` and/or a bare `project_name`, alongside the existing job_id,
  `att_` attempt_id, and bare run_id support. Unlike run_id (fixed generated
  format) and attempt_id (`att_` prefix), `run_name` is a free-form label with
  no reserved shape, so mixing it into the positional/`--job-id` list risks
  colliding with a real job_id. `project_name` is safer to detect (an existing
  `projects/<name>` directory), but still not fully unambiguous. If this is
  implemented, prefer resolving `run_name` only through a dedicated
  `--run-name` flag (mirroring `run --run-name`) rather than the mixed
  positional list, and reuse `wait`'s active-run scanning
  (`resolveRunNameTargets` in [cmd/rotari/wait.go](cmd/rotari/wait.go)) for
  lookup semantics and ambiguity errors.

## Near-term candidates

These are smaller steps that can come before the AI roadmap in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md). Choose them by whether
they improve the fix-and-retry loop for experiment batches, not by whether
another workflow engine has them. The experiment is the goal and orchestration
is only a means: prefer features that make the loop cheaper over features that
make users settle structure up front.

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

For field design, Dagu's step options are a useful reference
(`dagu/internal/spec/step.go`), for example `signal_on_stop`.

Each of these must define its behavior for every executor (local, SSH, Slurm,
PBS, LSF) and be tested per executor. Add them one at a time.

Out of scope: output passing between jobs, conditional branches, loops, cron
scheduling, and event triggers. They turn the queue into a workflow language or
belong to operating workflows that are already settled.

### Runs as experiment versions

A run snapshots the edited queue, so the run sequence is the version history of
the experiment. See "Runs as the history of the loop" in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md).

- Consider building `show --lineage` from copy and retry provenance instead
  of start time, so runs of one project that explore separate branches are
  not listed as one sequence.
- Consider showing the `rotari diff` summary (fixed, still failing, newly
  failing, elapsed time) after `run` and `wait` finish, and in the Web UI run
  page.
- Consider recording `state_version` in `meta.json`, `context.json`, and
  per-job files, as `queue.json`, `commands.json`, and `summary.json` already
  do. See "State load and write contracts" in
  [docs/contracts/04-coordination-and-safety.md](docs/contracts/04-coordination-and-safety.md).
- Building blocks for the above: `runs/<run-id>/commands.json`, `JobOrigin` in
  [internal/model/model.go](internal/model/model.go), and the queue-versus-run
  change count in `compareQueueWithRun` in
  [cmd/rotari/show.go](cmd/rotari/show.go).

### Black-box conformance tests

- Consider CLI-level black-box tests derived from the invariants in
  [docs/CONTRACTS.md](docs/CONTRACTS.md), independent of package structure, so
  behavior is protected during package-level refactoring. Dagu keeps
  normative specs in `specs/` with an implementation-status table and tests
  them from `conformance/`.

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
  report.
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
