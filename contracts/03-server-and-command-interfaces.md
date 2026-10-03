# Server and command interfaces

This file covers the server control surface, read projections, client lifecycle,
and CLI presentation. Web-specific UI and asset behavior live in
[05-web-assets-and-static-export.md](05-web-assets-and-static-export.md).

Representative implementation and tests:

- [internal/server/serve.go](../internal/server/serve.go),
  [internal/server/client.go](../internal/server/client.go), and
  [internal/server/serve_test.go](../internal/server/serve_test.go) for the
  server lease, the pipe connection to the run client, request dispatch,
  active-run tracking, and the detach and disconnect protocol.
- [cmd/rotari/server.go](../cmd/rotari/server.go) and
  [cmd/rotari/server_test.go](../cmd/rotari/server_test.go) for server
  commands and the `Operations` that perform run work. The request handlers
  live in [internal/supervisor](../internal/supervisor/operations.go) and
  [cmd/rotari/run_command.go](../cmd/rotari/run_command.go). `cancel`,
  `suspend`, and `resume` do not use the server: the CLI
  ([cmd/rotari/job_control.go](../cmd/rotari/job_control.go)) and the Web UI
  call [internal/jobcontrol](../internal/jobcontrol/jobcontrol.go) in their
  own process.
- [cmd/rotari/show.go](../cmd/rotari/show.go) and
  [cmd/rotari/show_test.go](../cmd/rotari/show_test.go) for CLI projections.
- [cmd/rotari/wait.go](../cmd/rotari/wait.go) and
  [cmd/rotari/wait_test.go](../cmd/rotari/wait_test.go) for wait output and
  run selection.

## Server and read projections

- A server (the supervisor) serves one run of one project. `run` and `retry`
  start a new one for every run, as a child process that inherits their
  working directory and environment (RUN-3 in
  [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md#run-lifecycle)),
  and never reuse a running one: a supervisor that finds the project's lease
  held reports it and exits, so `server.Start` fails, and a supervisor
  rejects a second run request. It stops when its run ends, when its client
  leaves without a run request, or after a minute without one. Its lease
  (`server.lock`) and PID file (`server.pid`) live in the project directory,
  so the runs of different projects have different supervisors. Durable
  behavior belongs in files, not memory. Covered by
  [internal/server/serve_test.go](../internal/server/serve_test.go) and
  [internal/server/client_test.go](../internal/server/client_test.go).
- The run client and its supervisor talk over two pipes the supervisor
  inherits as descriptors 3 and 4, not over a socket, so `run` needs no Unix
  socket and works in sandboxes that block them, whatever the length of the
  base directory. Only that client can reach the supervisor. The supervisor
  marks the descriptors close-on-exec, so jobs do not hold them, and its
  first message says whether it took the lease. Covered by
  [internal/server/client_test.go](../internal/server/client_test.go).
- A supervisor is running while its lease is locked. `server status`, `show`,
  and `server list` check the lock and read the PID file without contacting
  the supervisor; the check never creates the lease file, and a starting
  supervisor retries the lease briefly so a check does not make it fail.
  `server status` and `server shutdown` act on the supervisor of every
  project in the base directory that has one, and report `server is not
  running` when none does. `server shutdown` sends it `SIGTERM` and waits for
  the lease to be released. Shutting a supervisor down leaves its run
  interrupted with its jobs still running; `cancel` is how to stop a run.
  `server list` shows the supervisors registered in the master directory,
  one per project.
- A detached server discards stderr, so `runServer` writes a `start failed`
  event to `server.log` when it cannot start for any reason other than
  another server holding the lease, and sends the reason to its client;
  `server.Start` reports it with the log's path, as it does when the server
  exits or does not become ready.
- Every supervisor of a base directory writes lifecycle, request, and error
  events, each prefixed with its project, to `<basedir>/server.log`. Before an
  event would make the regular file exceed 1 MiB, it is truncated and the new
  event is written.
- The optional Python interface invokes the installed CLI with `subprocess` and
  must not implement queue or execution semantics itself.
- `wait --json` emits one `RunSummary` object per requested run, using NDJSON
  when multiple IDs are supplied.
- `wait` polls for the run's `summary.json`. While it is missing, `wait` also
  inspects the project state without removing a stale lock; when the run is no
  longer active (for example, interrupted because its supervisor exited), it
  prints how to inspect and recover the run and exits 1 instead of waiting
  forever. A malformed summary is treated as still being written while its
  run is active; if the run has ended without a valid summary, `wait` reports
  that failure. A summary from a newer state version fails with an upgrade
  message. A readable summary is returned only after the project has stopped
  running that run; if the run was interrupted after writing it, `wait` reports
  the interruption rather than treating it as finalized. Selector resolution
  must also leave stale run locks in place for recovery.
- `show --json` emits one object with the resolved location, available run
  summary, and saved commands. With `--run-id` and `--job-id`, it emits a job
  view with the selected command's expanded jobs, whether each has finished,
  and a resolved result when available, using the same fallback chain as
  human-readable show. Array command IDs include all tasks. See
  [implementation](../cmd/rotari/show_json_job.go) and
  [tests](../cmd/rotari/show_json_job_test.go). JSON modes are additive;
  default CLI output remains human-facing.
 - `show RUN` compares the current queue with that run's saved commands by job
  ID. Its changed count uses the same definition-field comparison as `lineage`,
  including environment, stage, timeout, and retry settings; see
  [`cmd/rotari/show.go`](../cmd/rotari/show.go),
  [`internal/runlineage/runlineage.go`](../internal/runlineage/runlineage.go), and
  [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go).

## Client connection lifecycle

The server distinguishes client cancellation, detach, and run completion as
follows:

- A synchronous client disconnect, including Ctrl-C, requests cancellation and
  returns immediately with exit code 130. The server-side run continues in the
  background and then finalizes normally.
- Ctrl-D sends an explicit detach before disconnecting. The run is left
  uncancelled and completion cleanup moves to the background waiter.
- Ctrl-Z sends no rotari protocol message. The terminal suspends the client
  while the server-side run continues; a later EOF follows the normal
  disconnect path and requests cancellation.
- An async run executes inside its supervisor, like a sync run whose client
  detached at once: `run --async` returns after `=== Run started ===`, and the
  supervisor finishes the run in the background. Interrupted-run recovery is
  reserved for failures that bypass finalization, such as a killed
  supervisor.
- Completed sync and async runs decrement the active-run count immediately
  through `Server.BeginRun`/`Server.EndRun`. When it reaches zero, the server stops without
  waiting for the idle timeout.

## CLI presentation

- **CLI-1** `check --json` reports the same project state, run identifier,
  queue count, lock, and runnable result as the human-readable `check`
  output.
- **CLI-2** The human-readable `jobs` table keeps every column's visible start
  position aligned across rows, including when status values are colorized.
  ANSI escape sequences are presentation only and do not count toward a
  column's width. The implementation is in
  [`cmd/rotari/jobs.go`](../cmd/rotari/jobs.go), with the end-to-end check in
  [`conformance/03-interfaces/jobs_presentation_test.go`](../conformance/03-interfaces/jobs_presentation_test.go).
- **CLI-3** For every CLI option that can be supplied by the command line, its
  matching environment variable, and configuration, precedence is explicit
  command-line value, then environment, then configuration, then the built-in
  default. Within configuration, the command-specific section takes precedence
  over the root value. For `quiet`, a command-specific environment variable
  takes precedence over `ROTARI_QUIET`. `--config FILE` selects the config
  source; it does not change this value precedence. This applies to every
  command exposing such options. The shared loaders are in
  [`cmd/rotari/cli_spec.go`](../cmd/rotari/cli_spec.go) and
  [`cmd/rotari/config.go`](../cmd/rotari/config.go); the external check
  enumerates each command exposing `--config` in the CLI schema in
  [`conformance/03-interfaces/options_test.go`](../conformance/03-interfaces/options_test.go).
- **CLI-4** `show` for a run, `lineage RUN`, and the Web API's run
  `lineage_summary` group the run's failed and blocked jobs by the same
  causes, in text and in their `failures` JSON. Each job is classified on its
  own result, so array tasks and matrix members fall into their own causes: a
  block, cancellation, or timeout that rotari recorded comes first, then the
  job's latest saved rule diagnosis, then its remaining failure kind. Groups
  list the most frequent cause first. `show` groups only the jobs its table
  lists. The rule is implemented once in
  [`internal/runlineage/failures.go`](../internal/runlineage/failures.go)
  (`FailureGroups`); the CLI text is in
  [`cmd/rotari/failure_groups.go`](../cmd/rotari/failure_groups.go), and the
  end-to-end check is
  [`conformance/03-interfaces/failure_groups_test.go`](../conformance/03-interfaces/failure_groups_test.go).
- **CLI-5** The project list (`show` without a project) shows each project's
  last run and its result, and the commands it suggests work as printed, with
  their placeholders filled in, for every listed project. A suggestion names
  the basedir when a listed project lies outside the default state
  directory, since `ROTARI_BASEDIR` counts as an explicit `--basedir`. The
  list is in [`cmd/rotari/show.go`](../cmd/rotari/show.go)
  (`showProjectsForBaseDirs`), with the end-to-end check in
  [`conformance/03-interfaces/project_list_test.go`](../conformance/03-interfaces/project_list_test.go).
- **CLI-6** The job listing window, `jobs --since` and the Web jobs page's
  `since`, takes a Go duration such as `24h` or `90m`, or a whole number of
  days such as `7d`; anything else is rejected. Both parse it with
  `joblist.ParseSince` in [`internal/joblist/joblist.go`](../internal/joblist/joblist.go),
  with the end-to-end check in
  [`conformance/03-interfaces/jobs_presentation_test.go`](../conformance/03-interfaces/jobs_presentation_test.go).
- **CLI-7** The commands that change a project's queue or run history
  (`add`, `change`, `copy`, `delete`, `import`, `remove`, `reset`) take
  `--dry-run`, which writes nothing and prints the change and the project
  revision that `check` also reports, and `--if-revision REVISION`, which applies the
  change only while the project is still at that revision, compared under the
  state lock, and prints the new revision. A stale revision fails and changes
  nothing. The rule is implemented once in `project.EditGuarded` and
  `project.EditQueueGuarded` in
  [`internal/project/edit.go`](../internal/project/edit.go); the end-to-end
  check is
  [`conformance/03-interfaces/guard_test.go`](../conformance/03-interfaces/guard_test.go).
  `run` and `retry` take the same options: `--dry-run` lists the jobs the run
  would execute, planned by `projectrun.Runner.PlanRun` as the run itself is,
  with each task of an array that runs whole listed (`run.PlanRerun` keys the
  plan by job ID), and `--if-revision` starts the run only at that revision, compared again by
  the supervisor when it begins the run.
- **CLI-11** When a command names a project that its state directory does not
  have, the error lists the other registered state directories that have a
  project of that name and says to select one with `--basedir`; `jobs`,
  which lists state directories itself, points to `--all-basedirs`. A
  project that no registered state directory has gets the plain error.
  Implemented once in `resolve.RegisteredProjectBaseDirs`, used by
  `resolve.RequireProject` and `rotari jobs`; checked by
  `TestMissingProjectNamesWhereItIs` in
  [`conformance/03-interfaces/project_list_test.go`](../conformance/03-interfaces/project_list_test.go).

- CLI colors are semantic presentation, not machine-readable output. They are
  emitted only on TTY streams; redirected and piped output remains plain text.
- Red denotes errors and failed results; green denotes success; yellow denotes
  warnings, running/blocked state, retries, and recovery/cancellation; cyan
  denotes informational labels and suggested actions; white denotes values.
- Parsers and tests must rely on text, not ANSI sequences or color choice.
- `check --json` emits the same result as its text view. `queued` is a JSON
  number when known and `null` when an active or interrupted project's queue
  cannot be read; `run_id` is omitted when no run is associated with the state.
  JSON output is emitted even if quiet is configured, including for a runnable
  project; see [implementation](../cmd/rotari/check.go) and
  [tests](../cmd/rotari/check_test.go).
- `show` uses a lazy pager. `--no-pager` and non-TTY output go directly to
  stdout. On a TTY, output of at most 24 lines is direct; longer output uses
  `$PAGER`, defaulting to `less -R`. Pager failure falls back to stdout.

### Quiet output contract

- `--quiet` and `ROTARI_QUIET=true` are equivalent global defaults for all
  commands that support quiet output. Each command also accepts its own
  environment default (`ROTARI_ADD_QUIET`, `ROTARI_COPY_QUIET`,
  `ROTARI_CHANGE_QUIET`, `ROTARI_REMOVE_QUIET`, `ROTARI_RESET_QUIET`,
  `ROTARI_CHECK_QUIET` or `ROTARI_RUN_QUIET`), which overrides the global
  value. `retry` shares `ROTARI_RUN_QUIET`. Config files support
  root `quiet` and command-specific values such as `add.quiet` and `run.quiet`
  with the same precedence.
  The shared environment-variable mapping is defined in
  [`cmd/rotari/environment.go`](../cmd/rotari/environment.go) and
  [`cmd/rotari/cli_spec.go`](../cmd/rotari/cli_spec.go).
- For `run`, quiet is carried in the server request as `Request.Quiet`; the
  client suppresses successful progress and completion output. Job failures,
  startup errors, and other command errors remain visible, and synchronous
  `run` still returns a non-zero status when a job fails.
- The same success-silent/error-visible rule applies to queue-editing commands
  (`add`, `copy`, `change`, `remove`, `reset`, and `check`). The protocol field
  is defined in [`internal/server/protocol.go`](../internal/server/protocol.go),
  with client behavior covered by
  [`cmd/rotari/server_test.go`](../cmd/rotari/server_test.go).
  The machine-readable `check --json` output is not suppressed by quiet.

## MCP tools

`rotari mcp` serves MCP tools over stdio for one master directory; see
[docs/MCP.md](../docs/MCP.md). The tools present the same shared functions
as the CLI and hold no rules of their own.

- **MCP-1** A tool that changes a project comes as a read-only preview and a
  write. The preview (`rotari_preview_import`, `rotari_preview_run`) changes
  nothing and returns the project revision that `check` reports, planned as
  the CLI's `--dry-run` plans it. The write (`rotari_import`,
  `rotari_start_run`) requires that revision and applies only while the
  project is still at it; otherwise it fails and changes nothing. A run the
  write starts executes the jobs its preview listed. The tools are in
  [`internal/mcp/write.go`](../internal/mcp/write.go), with the end-to-end
  check in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
- **MCP-2** `rotari_export_run` returns a run's workflow manifest as a
  view for reading: environment values and executor options are replaced
  with `[REDACTED]` and paths are redacted where detected, while
  `rotari export` keeps them. The MCP import tools refuse a manifest that
  still holds such a placeholder. Implemented in
  [`internal/mcp/export.go`](../internal/mcp/export.go); checked by
  `TestMCPExportIsARedactedViewThatImportRefuses` in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
- **MCP-3** `rotari_run_summary` reports a run's `state` as `rotari wait`
  decides it, with `project.RunPhaseOf`: `running` while the run holds the
  project's run lock, from the moment `rotari_start_run` returns its ID,
  then `finished`, `interrupted`, or `ended`. A run that has not written its
  jobs yet is `running` with no jobs, not an error. No MCP tool's error
  names a state directory: each tool is added through one wrapper that
  replaces registered basedirs with `BASEDIR`. `rotari_wait_run` waits for
  at most `timeout_seconds` (1 to 300, default 30) until the run is no
  longer running (`settled`), or with `until_failure` until a job has failed
  with no retry left (`failure`, from `runview.FinalFailureGroups`, which
  `rotari wait --until-failure` also uses), and otherwise returns `timeout`.
  Implemented in
  [`internal/project/run_phase.go`](../internal/project/run_phase.go),
  [`internal/mcp/server.go`](../internal/mcp/server.go), and
  [`internal/mcp/wait.go`](../internal/mcp/wait.go); checked by
  `TestMCPWritesApplyOnlyAtThePreviewedRevision`, which follows a started
  run through MCP alone, and `TestMCPWaitReturnsOnTheFirstFinalFailure`.
- **MCP-4** `rotari_preview_job_control` lists, without changing anything,
  the unfinished jobs of a running run that an operation reaches, chosen as
  `rotari cancel`, `suspend`, and `resume` choose them
  (`jobcontrol.Controller.Select` with `jobcontrol.States`). `rotari_cancel`,
  `rotari_suspend`, and `rotari_resume` take the run ID and optional job IDs
  and act as the CLI does with `--run-id`: only while that run is still the
  project's active run, otherwise failing without effect. Without job IDs,
  cancel stops the whole run. Implemented in
  [`internal/mcp/control.go`](../internal/mcp/control.go); checked by
  `TestMCPJobControlActsOnlyOnThePreviewedRunningRun` in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
- **MCP-5** `rotari_preview_reset` reports, without changing anything, how
  many queued jobs a reset removes and the interrupted run it would recover,
  with what that run's jobs last reported, and the revision. `rotari_reset`
  applies only at that revision, recovers an interrupted run only with
  `recover_interrupted`, refuses a running project, and keeps run history,
  as `rotari reset` does through `project.Reset`. Implemented in
  [`internal/mcp/reset.go`](../internal/mcp/reset.go); checked by
  `TestMCPResetRecoversOnlyAConfirmedInterruptedRun` in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
