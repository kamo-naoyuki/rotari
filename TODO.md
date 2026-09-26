# TODO

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
another workflow engine has them.

### Orchestration

- Job settings stored on queued commands (`--retry` and its delays,
  `--timeout`, `--executor-option` such as Slurm options, `--env`) are copied
  into later runs by `copy` and `retry`, but changing them afterwards means
  one `change` per job. Run-level options such as `run --retry` and
  `--slurm-options` only act as defaults for jobs without their own value.
  Consider an easier way to change them for many jobs or for one run, for
  example `change` selecting a stage, matrix group, or every job
  (`change --stage sweep --retry 0`), or run-only overrides that take
  precedence over the stored job settings for that run without writing them
  back to the queue.

- Per-job timeouts (`add --timeout 2h`) are enforced by the job wrappers. The
  grace period before SIGKILL is fixed at 30 seconds; consider making it
  configurable together with the stop signal below.
- Per-job retry limits and delays exist (`add --retry N --retry-delay 30s
  --retry-backoff 2 --retry-max-delay 5m`), and failed jobs are retried
  immediately rather than after the whole run's attempt. Consider retrying
  only on listed exit codes, as Dagu's `retry_policy.exit_code` does, so
  deterministic failures are not retried, and run-level delay defaults.
- Consider concurrency limits for a named group of jobs (for example "at most
  four GPU jobs at once"). Limits are per executor today.

- Consider a per-job stop signal (for example `add --stop-signal USR1`) so
  cancellation can let training scripts save a checkpoint before exiting.
  Slurm has a matching `--signal` option.

For field design, Dagu's step options are a useful reference
(`dagu/internal/spec/step.go`): `retry_policy` has `limit`, `interval_sec`,
`backoff`, `max_interval_sec`, and `exit_code` (retry only on listed exit
codes), plus `timeout_sec` and `signal_on_stop`.

Each of these must define its behavior for every executor (local, SSH, Slurm,
PBS, LSF) and be tested per executor. Add them one at a time.

Out of scope: output passing between jobs, conditional branches, loops, cron
scheduling, and event triggers. They turn the queue into a workflow language or
belong to operating workflows that are already settled.

### Runs as experiment versions

A run snapshots the edited queue, so the run sequence is the version history of
the experiment. See "Runs as the history of the loop" in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md).

- `rotari show --lineage` lists a project's runs with counts and changes since
  the previous run. Consider following copy and retry provenance instead of
  start time when runs of one project explore separate branches.
- `rotari diff` exists and covers the run summary (fixed, still failing,
  newly failing, elapsed time). Consider showing the same summary after
  `run` and `wait` finish, and in the Web UI run page.
- Run state versioning is in place: `queue.json`, `commands.json`, and
  `summary.json` record `state_version`, older files are read as version 1,
  and newer ones are rejected. See "State load and write contracts" in
  [docs/internals/04-coordination-and-safety.md](docs/internals/04-coordination-and-safety.md)
  for the migration policy. `meta.json`, `context.json`, and per-job files
  are not versioned yet.
- Building blocks: `runs/<run-id>/commands.json`, `JobOrigin` in
  [internal/model/model.go](internal/model/model.go), and the queue-versus-run
  change count in `compareQueueWithRun` in
  [cmd/rotari/show.go](cmd/rotari/show.go).

### Black-box conformance tests

- Consider CLI-level black-box tests derived from the invariants in
  [docs/INTERNALS.md](docs/INTERNALS.md), independent of package structure, so
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
