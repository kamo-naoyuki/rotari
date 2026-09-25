# TODO

- Consider `rotari show --stage NAME` to filter the selected queue or run to
  jobs in that stage. Use the stage name directly rather than introducing a
  generated `--stage-id`.

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

- Consider a per-job timeout (`add --timeout 2h`). Local and SSH jobs that hang
  are never stopped today; decide whether scheduler executors map it to
  walltime or have rotari cancel the job.
- Consider per-job automatic retry (`add --retry N`, optionally with backoff)
  for flaky jobs such as transient NFS or network failures. Only run-level
  `run --retry` exists today.
- Consider a dependency that fires when the prerequisite finishes regardless of
  its result (for example `--depends-on-finished NAME`), so aggregation and
  cleanup jobs still run when part of a sweep fails. `--depends-on` requires
  success.
- Consider concurrency limits for a named group of jobs (for example "at most
  four GPU jobs at once"). Limits are per executor today.

- Consider a per-job stop signal (for example `add --stop-signal USR1`) so
  cancellation can let training scripts save a checkpoint before exiting.
  Slurm has a matching `--signal` option.

For field design, Dagu's step options are a useful reference
(`dagu/internal/spec/step.go`): `retry_policy` has `limit`, `interval_sec`,
`backoff`, `max_interval_sec`, and `exit_code` (retry only on listed exit
codes), plus `timeout_sec`, `signal_on_stop`, and `continue_on`. Dagu puts
`continue_on` on the upstream step; decide whether rotari attaches the
"finished regardless of result" rule to the prerequisite or to the dependent.

Each of these must define its behavior for every executor (local, SSH, Slurm,
PBS, LSF) and be tested per executor. Add them one at a time.

Out of scope: output passing between jobs, conditional branches, loops, cron
scheduling, and event triggers. They turn the queue into a workflow language or
belong to operating workflows that are already settled.

### Runs as experiment versions

A run snapshots the edited queue, so the run sequence is the version history of
the experiment. See "Runs as the history of the loop" in
[docs/DAGU_COMPARISON.md](docs/DAGU_COMPARISON.md).

- Consider a run lineage listing: name, time, success and failure counts, jobs
  added/removed/changed since the previous run, and jobs carried forward.
- Consider `rotari diff RUN_A RUN_B`: per-job changes in command, executor
  options, and environment, with status changes such as failed to succeeded.
- Consider a run summary: fixed, still failing, newly failing, and elapsed
  time.
- Prerequisite: version the run state files. `summary.json`, `commands.json`,
  and `queue.json` carry no schema version today (only workflow manifests do),
  and old formats are converted ad hoc on decode. If runs are the long-lived
  history of an experiment, runs written months ago must stay readable after
  upgrades. Add a version field and a migration policy before building lineage
  and diff features. Dagu's `SCHEMA_MIGRATION.md` is an example of publishing
  a field mapping for a format change.
- Building blocks: `runs/<run-id>/commands.json`, `JobOrigin` in
  [internal/model/model.go](internal/model/model.go), and the queue-versus-run
  change count in `compareQueueWithRun` in
  [cmd/rotari/show.go](cmd/rotari/show.go).

### Agent-facing guide

- Consider shipping an agent-facing usage guide inside the binary and
  generating it from the CLI metadata in
  [cmd/rotari/cli_spec.go](cmd/rotari/cli_spec.go) so it cannot drift from the
  CLI. Dagu embeds `skills/dagu/SKILL.md` with Go `embed` and generates
  `llms.txt` with `internal/tools/llmsgen`.
- The guide should tell agents to use `run --async` with `wait` (synchronous
  `run` is cancelled when the client is killed), to verify with `check` and
  `import --dry-run` instead of guessing, and to use `--json` output. Dagu's
  guide likewise tells agents to use `dagu schema` and `dagu validate` rather
  than guess field names.

### Black-box conformance tests

- Consider CLI-level black-box tests derived from the invariants in
  [docs/INTERNALS.md](docs/INTERNALS.md), independent of package structure, so
  behavior is protected during package-level refactoring. Dagu keeps
  normative specs in `specs/` with an implementation-status table and tests
  them from `conformance/`.

### Web UI

- Polish the overall look; it is the first impression in demos.
- Consider a matrix grid view: parameters on the axes and each cell colored by
  status, so failures in, for example, `lr x seed` are visible at a glance.
- Consider a per-job attempt history that shows how the command or executor
  options changed between attempts.
- Consider grouping failed jobs by rule-based diagnosis result.
- Consider a run timeline built on the run lineage above.

Suggested first steps: the matrix grid view and the
finished-regardless-of-result dependency. Both serve experiment batches
directly and depend little on executor differences.
