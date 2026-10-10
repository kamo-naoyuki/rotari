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

The startup pipe carries the run request and one acceptance or rejection. For
a synchronous `run` / `retry`, acceptance includes the committed run ID and
transfers client-liveness responsibility to the per-client attachment session;
the pipe is then closed normally. The pipe does not carry steady-state
progress or controls. `run --async` returns after the same startup acceptance
without creating an attached session. `wait` resolves an existing run and
joins the same follower/session lifecycle without starting or re-executing it.

Each attached client has an independent session record and process-held lock
under the project's `.rotari-attachments/` directory. Current attachment is
derived from live sessions, not a mutable count or a single boolean in
`running.lock`; old locks without the session marker retain their historical
fallback. The supervisor treats a session as live while its lock is held,
including while its process is suspended. After a same-host process dies, the
released lock and recorded process-start identity distinguish death from PID
reuse. A remote host cannot be verified locally and is treated as unknown/live,
not proved dead. The supervisor checks for stale sessions while the run is
executing; the detection interval is 100 ms plus filesystem and cancellation
latency. If liveness cannot be read or verified, it does not infer a
disconnect. Thus `--disconnect-action cancel` detects same-host `SIGKILL`, but
does not promise to detect a lost remote client.

- Ctrl-C requests cancellation of the fixed run ID (all selected active run
  IDs for multi-run `wait`) and exits with status 130. It is independent of
  the unexpected-disconnect policy; execution finalizes after jobs stop.
- Ctrl-D releases only the caller's session and leaves execution uncancelled.
  Other live clients remain attached. A sync `run` returns to the background;
  `wait` merely stops following.
- Unexpected disconnect defaults to detach. `--disconnect-action cancel` or
  `ROTARI_DISCONNECT_ACTION=cancel` asks the supervisor to cancel after it
  proves a local client session dead; explicit CLI options take precedence.
- Ctrl-Z suspends the client while its process-held lock remains held, so it
  is not mistaken for a disconnect. If it is later terminated, the selected
  disconnect policy applies.
- An async run has no attached client until a later client enters via `wait`.
  Web and MCP reads are not attachments. Completed runs use the saved summary
  and do not create a live session. Supervisor death still means interruption,
  not successful completion.
- The startup request reserves the initiating session before launch and binds
  it to the run ID before acceptance. If the acknowledgement is lost after
  binding, the client does not retry; its released lock leaves a record the
  supervisor can inspect and apply the configured policy to.

## CLI presentation

- **CLI-21** `info` reports the resolved master directory, base directory,
  selected project (marking it when not yet created) or available choices,
  visible config files, supervisors, run locks, and active or
  interrupted runs. For unfinished attempts, it best-effort checks the local
  process group for local-executor jobs; remote and other executor processes
  are unverified, and this is not proof of process identity. Each run's inline
  job counts include jobs with a recorded final result and carried results as
  `finished`, and those of them that failed also as `failed`; terminal
  attempts awaiting a retry and `pending` jobs, not yet dispatched, are counted as
  `pending`. It is read-only
  and does not remove stale locks. Coordinator liveness is reported only for
  local locks. Human-readable output uses the standard CLI colors only on a
  terminal; JSON and redirected output remain plain. The implementation is in
  [`cmd/rotari/info.go`](../cmd/rotari/info.go), with CLI and non-mutation
  coverage in [`cmd/rotari/info_test.go`](../cmd/rotari/info_test.go) and
  [`conformance/03-interfaces/info_test.go`](../conformance/03-interfaces/info_test.go).
- **CLI-22** A command that rotari prints for the user to run next, such as
  a `retry:` line of `lineage` or the failure summary of `show`, names
  `--basedir` only when the project's state directory is not the one a
  command started in the same place would use: `ROTARI_BASEDIR`, then the
  configured `basedir`, then the default. A printed command that names a run
  then needs no location at all. Every printed command works as printed. The
  same holds for commands built outside `cmd/rotari`: the recovery commands
  of a command that refuses an interrupted project (`project.EnsureIdle`,
  `project.RerunCommand`), which use the renderer a CLI process installs with
  `project.SetCommandLocation`, and the supervisor's retry source notice,
  which uses the location the client sends in its request. The Web UI, MCP,
  and the supervisor itself keep naming `--basedir`, since their readers'
  environment is unknown. The rule is implemented once, in
  [`cmd/rotari/hint_location.go`](../cmd/rotari/hint_location.go)
  (`hintLocation`, `runHintLocation`), with coverage in
  [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go)
  (`TestHintLocationOmitsImplicitBaseDir`),
  [`conformance/03-interfaces/failure_groups_test.go`](../conformance/03-interfaces/failure_groups_test.go),
  and `TestRefusalAndSourceHintsNameOnlyANonImplicitBaseDir` in
  [`conformance/03-interfaces/refusal_hints_test.go`](../conformance/03-interfaces/refusal_hints_test.go).
- **CLI-23** The job table of `show` for a run, `show -j`, and `jobs` report
  each job's elapsed time: from submission to its finish, or for a running
  job until now, followed by how long ago the job last wrote to its logs
  (`quiet DURATION`) or `no output` when it has written none. `jobs --json`
  gives the same as `elapsed_seconds` and `quiet_seconds`. Logs an executor
  keeps on another host are not seen. The run time is measured once, by
  `jobstatus.MeasureRunTime` in
  [`internal/jobstatus/times.go`](../internal/jobstatus/times.go), and
  formatted by `joblist.FormatRunTime` in
  [`internal/joblist/joblist.go`](../internal/joblist/joblist.go), which the
  Web jobs page also uses. Covered by
  [`conformance/03-interfaces/show_elapsed_test.go`](../conformance/03-interfaces/show_elapsed_test.go).
- **CLI-24** `show --logs` and `--failed-logs` list a run's jobs in the
  order the run defines them, executed and carried alike, so the members of a
  matrix and the tasks of an array stay together; they list only jobs, not
  other directories of the run. `--tail N` prints only the last N lines of
  each log there and in a job's own view, and is rejected with any other
  view. Implemented by `showRunLogs` and `printJobStreams` in
  [`cmd/rotari/show.go`](../cmd/rotari/show.go); covered by
  [`conformance/03-interfaces/show_logs_tail_test.go`](../conformance/03-interfaces/show_logs_tail_test.go).
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
  list the most frequent cause first. In text, each group of the project's last
  finished run also prints a `retry:` command that previews a rerun of only
  that group's jobs, by `--filter-diagnosis` for a rule diagnosis and by
  `--filter-failure-kind` otherwise; it works as printed. `show` groups only the jobs its table
  lists. The rule is implemented once in
  [`internal/runlineage/failures.go`](../internal/runlineage/failures.go)
  (`FailureGroups`); the CLI text is in
  [`cmd/rotari/failure_groups.go`](../cmd/rotari/failure_groups.go), and the
  end-to-end check is
  [`conformance/03-interfaces/failure_groups_test.go`](../conformance/03-interfaces/failure_groups_test.go).
- **CLI-5** `projects` shows each project's
  last run and its result, and the commands it suggests work as printed, with
  their placeholders filled in, for every listed project. A suggestion names
  the basedir when a listed project lies outside the default state
  directory, since `ROTARI_BASEDIR` counts as an explicit `--basedir`. The
  list is in [`cmd/rotari/projects.go`](../cmd/rotari/projects.go)
  (`showProjectsForBaseDirs`), with the end-to-end check in
  [`conformance/03-interfaces/project_list_test.go`](../conformance/03-interfaces/project_list_test.go).
- **CLI-6** The listing window, `jobs --since`, `runs --since`, and the Web jobs page's
  `since`, takes a Go duration such as `24h` or `90m`, or a whole number of
  days such as `7d`; anything else is rejected. The CLI commands default to
  `1d`; running jobs and active or interrupted runs are included regardless of
  age. All three interfaces parse it with
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
  plan by job ID) and a job that executes only because a job it depends on
  executes marked `depends_on_rerun=NAME` (`run.Plan.RerunDependencies`), and `--if-revision` starts the run only at that revision, compared again by
  the supervisor when it begins the run.
- **CLI-11** When a command names a project that its state directory does not
  have, the error lists the available projects in the selected state directory,
  sorted by name, with a `--project-name` hint, or `(none)` when it has no
  projects. If that list cannot be read, it is omitted rather than reported
  as empty. The error also lists the other registered state directories that have a
  project of that name, each with its last run's ID, status, and failure
  count, and says to select one with `--basedir`; cross-basedir `jobs` can list
  the project without extra flags, and `rotari basedirs` shows the registered
  state directories. A project that no registered state directory has gets
  no alternate-directory hint.
  Implemented once in `resolve.RegisteredProjectBaseDirs`, used by
  `resolve.RequireProject` and `rotari jobs`; checked by
  `TestMissingProjectNamesWhereItIs` in
  [`conformance/03-interfaces/project_list_test.go`](../conformance/03-interfaces/project_list_test.go)
  and `TestMissingTargetDiagnostics` in
  [`conformance/01-resolution/missing_project_test.go`](../conformance/01-resolution/missing_project_test.go).
- **CLI-8** `run` and `retry` reject `--async` with `--dry-run`: a preview does
  not start a run, so asynchronous return behavior cannot apply. The rejection
  names both options and changes no project state; see
  [`cmd/rotari/run_command.go`](../cmd/rotari/run_command.go) and
  [`conformance/03-interfaces/pairruns/flag_pair_run_test.go`](../conformance/03-interfaces/pairruns/flag_pair_run_test.go).
- **CLI-9** `run` and `retry` report selector/result-filter conflicts in either
  flag order. Stage or matrix scopes combine with result filters; direct job
  selectors do not combine with result or `--filter-*` selectors. Dry-run
  plans use the same rules: partial arrays select individual matching tasks;
  `--partial-array=false` plans the whole array when any task matches. See
  [`internal/projectrun/plan.go`](../internal/projectrun/plan.go) and
  [`conformance/03-interfaces/pairruns/flag_pair_run_test.go`](../conformance/03-interfaces/pairruns/flag_pair_run_test.go).
- **CLI-10** When `run --dry-run --run-name NAME` is given, the preview includes
  `run_name=NAME` in its summary. A dry run does not persist or reserve that
  name. See [`cmd/rotari/run_command.go`](../cmd/rotari/run_command.go) and
  [`conformance/03-interfaces/pairruns/flag_pair_run_test.go`](../conformance/03-interfaces/pairruns/flag_pair_run_test.go).
- **CLI-12** `wait --json` emits one JSON object for a completed run; with
  `--until-failure`, it emits the running run ID and its final-failure groups
  when it returns before completion. The wait is bounded by the selected
  `--timeout`, whose explicit CLI value takes precedence over environment and
  configuration. Checked by
  [`conformance/03-interfaces/pairruns/flag_pair_wait_test.go`](../conformance/03-interfaces/pairruns/flag_pair_wait_test.go).
- **CLI-14** For a completed run, text `wait` prints the same completion
  message as `run`, including each summary, carried, and origin line exactly
  once and ending with a newline. For a failed run, the message groups the
  failed and blocked jobs by cause as `lineage RUN` does, marks carried
  failures, and does not list each failed job, so it stays short however many
  jobs fail. Both use
  [`cmd/rotari/run_completion.go`](../cmd/rotari/run_completion.go);
  checked by [`conformance/03-interfaces/wait_test.go`](../conformance/03-interfaces/wait_test.go).
- **CLI-19** Synchronous `run`/`retry` and `wait` enter one post-start follower
  and per-client attachment lifecycle. The startup pipe carries only request
  and acceptance/rejection (with the committed run ID); progress and control
  after acceptance use the run's files and shared job-control operation.
  Each client owns a separate process-held session lock and record under
  `.rotari-attachments/`. Aggregate attachment is derived from live sessions;
  ending one client never detaches another, and implicit waiters register
  their own session for each still-running run under the project state lock,
  so several waiters can follow one run. An async
  run creates no session until `wait` attaches. Web and MCP reads are not
  attachments.

  Text followers render job-start, retry, final-failure, and progress-count
  events from `progress.jsonl`, draining final events before completion. Every
  progress count, including the first, is out of the jobs the run executes, not
  the results it carries. `run`
  reads from the run's beginning; `wait` prints `=== Run attached ===` and the
  latest progress snapshot, skips older event history, and does not replay
  progress for an already finished run. An absent journal remains compatible
  with old runs. Partial lines are deferred; malformed complete lines are
  reported and skipped without losing later events. Quiet suppresses normal
  progress and completion but keeps diagnostics, errors, and timeouts. JSON
  never emits text progress, still holds a session while following an active
  run, and emits results in selector order. Ctrl-D releases only the caller's
  session; timeout and `--until-failure` also release only that session without
  cancelling work. Ctrl-C cancels every still-active selected run and exits
  130. Explicitly selected active runs may have multiple followers and warn
  when another session is attached; implicit selection skips any run with a
  valid session.

  Unexpected disconnect defaults to detach. With
  `--disconnect-action cancel` / `ROTARI_DISCONNECT_ACTION=cancel`, the
  supervisor detects a same-host dead process by its released session lock and
  recorded process-start identity, then requests cancellation of that fixed
  run ID. A suspended client retains its lock. A remote client's liveness is
  unknown and is treated as live; cancel-on-disconnect cannot promise to detect
  a lost remote client. Detection polls every 100 ms plus filesystem and
  cancellation latency. Startup sessions are reserved before launch and bound
  before acceptance; a lost acknowledgment is not retried, and a bound stale
  session remains observable to the supervisor. Implemented in
  [`cmd/rotari/run_command.go`](../cmd/rotari/run_command.go),
  [`cmd/rotari/wait.go`](../cmd/rotari/wait.go),
  [`internal/attachment/session.go`](../internal/attachment/session.go),
  [`internal/server/client.go`](../internal/server/client.go),
  [`internal/server/serve.go`](../internal/server/serve.go), and
  [`internal/supervisor/run.go`](../internal/supervisor/run.go). Package and
  binary checks include `TestWaitAttachmentIsSharedAndImplicitWaitFollowsIt`,
  `TestConcurrentImplicitWaitReservationsAreIndependent`,
  `TestWaitWithoutSelectorWaitsForRunsThisProcessStarted`,
  `TestSynchronousRunInterruptCancelsAcceptedRun`,
  `TestSessionLivenessSurvivesSuspendAndDetectsSIGKILL`,
  `TestRunClientDisconnectDetachesByDefaultAndCanCancel`, and
  `TestWaitClientDisconnectDetachesByDefaultAndCanCancel`.
- **CLI-15** A single-value CLI option may be specified only once in an
  invocation; a second occurrence is rejected before command effects. Options
  declared repeatable remain repeatable, including short and long aliases of
  the same option. The shared checks are in
  [`cmd/rotari/cli_spec.go`](../cmd/rotari/cli_spec.go), with parser tests in
  [`cmd/rotari/cli_spec_test.go`](../cmd/rotari/cli_spec_test.go) and an
  executable-level check in
  [`conformance/03-interfaces/flag_pair_projections_test.go`](../conformance/03-interfaces/flag_pair_projections_test.go).
- **CLI-16** `show` of one executed job lists the artifact candidates its
  attempt recorded (RUN-9) after the command: what the showing host finds at
  each path now (`file`, `directory`, `other`, `missing`, or `unknown` for a
  path whose base was never known), the path relative to the job's working
  directory when under it, and where it was found. It lists at most 20 and
  names `show -j ATTEMPT --artifacts` for the rest; an attempt without a
  record reads `(not recorded)`, unlike `none found`. `--artifacts` lists
  every candidate and the discovery notes instead of the logs, requires one
  job, and is rejected with log, follow, stream, JSON, report, queue, list,
  and result-filter options. `show -j JOB --json` carries the same listing per
  job. A carried job shows the candidates of the attempt that produced its
  result. The listing is built once in `jobstatus.ListArtifacts`
  ([internal/jobstatus/artifacts.go](../internal/jobstatus/artifacts.go)) and
  printed by [`cmd/rotari/show_artifacts.go`](../cmd/rotari/show_artifacts.go);
  covered by `TestShowListsArtifactCandidates` in
  [conformance/03-interfaces/artifacts_test.go](../conformance/03-interfaces/artifacts_test.go).

- **CLI-17** Configuration-loading commands remain able to show their help
  when configuration loading fails: they warn on stderr, show help using
  built-in and environment defaults on stdout, and exit zero. Successfully
  loaded configuration still supplies help defaults. Normal execution fails
  on configuration errors; help tokens used as option values, after `--`, or
  within `add`/`change` job commands do not bypass that failure. Implemented in
  [`cmd/rotari/config_help.go`](../cmd/rotari/config_help.go), with parser tests
  in [`cmd/rotari/config_help_test.go`](../cmd/rotari/config_help_test.go) and
  binary coverage in
  [`conformance/03-interfaces/config_help_test.go`](../conformance/03-interfaces/config_help_test.go).

- **CLI-18** A successful `add` warns on stderr when an added execution unit
  shares its fingerprint with another unit in the resulting queue. It reports
  each affected fingerprint group once, with job IDs and available names,
  asking whether the same command was accidentally added twice rather than
  using fingerprint terminology, without rejecting or deduplicating jobs.
  Unrelated existing duplicates do
  not warn. The warning remains visible with `--quiet` and in `--dry-run`
  previews. Array tasks are separate units and matrix members retain their
  expanded parameters; equality uses the existing fingerprint definition,
  not executor, timeout, or retry settings. Implemented in
  [`internal/queueops/add.go`](../internal/queueops/add.go) and rendered by
  [`cmd/rotari/add.go`](../cmd/rotari/add.go), with CLI tests in
  [`cmd/rotari/add_warning_test.go`](../cmd/rotari/add_warning_test.go) and
  binary coverage in
  [`conformance/03-interfaces/add_warning_test.go`](../conformance/03-interfaces/add_warning_test.go).

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
- **CLI-20** Every command option has a corresponding CLI-default environment
  variable, named by its explicit mapping or `ROTARI_<COMMAND>_<OPTION>`.
  Options marked `CommandLineOnly` in the CLI specification are the only
  exceptions: explicit configuration selection, safety/confirmation and
  revision guards, one-invocation filters, explicit job-definition/path
  settings, and the schema protocol switch must not be supplied implicitly.
  The CLI reference marks these exceptions as `CLI only`.
  The CLI schema and `rotari env` list the mapping; the mapping
  and exception coverage are checked by
  [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go) and
  [`conformance/03-interfaces/environment_contract_test.go`](../conformance/03-interfaces/environment_contract_test.go).

### Quiet output contract

- `--quiet` and `ROTARI_QUIET=true` are equivalent global defaults for all
  commands that support quiet output. Each command also accepts its own
  environment default (`ROTARI_ADD_QUIET`, `ROTARI_COPY_QUIET`,
  `ROTARI_CHANGE_QUIET`, `ROTARI_REMOVE_QUIET`, `ROTARI_RESET_QUIET`,
  `ROTARI_CHECK_QUIET`, `ROTARI_RUN_QUIET`, or `ROTARI_WAIT_QUIET`), which overrides the global
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
  many queued jobs a reset removes and the revision. `rotari_reset` applies
  only at that revision, clears only the queue, and leaves active or
  interrupted runs untouched, as `rotari reset` does through `project.Reset`.
  An old `recover_interrupted` argument is rejected with an instruction to
  use unlock. Implemented in [`internal/mcp/reset.go`](../internal/mcp/reset.go);
  checked by `TestMCPResetOnlyClearsTheQueue` in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
- **MCP-6** Closing stdin ends `rotari mcp` cleanly (exit status 0),
  including when responses are still in flight. Malformed input remains a
  failure reported on stderr. Implemented in
  [`cmd/rotari/mcp.go`](../cmd/rotari/mcp.go), with error classification checks
  in [`cmd/rotari/mcp_test.go`](../cmd/rotari/mcp_test.go); checked through the
  binary by `TestMCPStdinEOF` in
  [`conformance/03-interfaces/flag_pair_mcp_test.go`](../conformance/03-interfaces/flag_pair_mcp_test.go).
- **MCP-7** `rotari_preview_unlock` reports, without changing anything, the
  interrupted run an unlock would recover, with what that run's jobs last
  reported and whether some may still be running, and the revision.
  `rotari_unlock` applies only at that revision, refuses a run whose
  coordinator is alive on the server's host, and keeps the queue, as
  `rotari unlock` does through `project.Unlock`. Implemented in
  [`internal/mcp/unlock.go`](../internal/mcp/unlock.go) and
  [`internal/project/unlock.go`](../internal/project/unlock.go); checked by
  `TestMCPUnlockRecoversAnInterruptedRun` in
  [`conformance/03-interfaces/mcp_test.go`](../conformance/03-interfaces/mcp_test.go).
