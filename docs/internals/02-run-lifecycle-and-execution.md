# Run lifecycle and execution semantics

## Run lifecycle

- Queue-editing commands mutate `queue.json`. Starting a run assigns a new ID,
  snapshots the queue, records context, and marks it active. Completion writes
  results and summary, updates metadata, clears the consumed queue, and removes
  the active lock. Completed run snapshots, results, and logs remain immutable
  until the run is explicitly deleted.
- Retries and filtered runs always create new history and never modify their
  source run. `--retry N` retries a failed job up to N additional times within
  the same run. A failed job is one whose result has a non-zero exit code and is
  not explicitly cancelled.
- An explicit cancellation is terminal for the current run. A job marked
  cancelled, or whose recorded execution state is `cancelled`, is not
  automatically retried by that run's `--retry` loop, even if its exit code is
  non-zero.
- A later explicit `rotari retry` may select a cancelled job through the normal
  `failed`/`unfinished` result filters. This is a new run, so it is a new user
  decision to execute the job again.
- `retry` is a shorthand for `run --failed --unfinished` and does not have a
  separate `ROTARI_RETRY_*` environment-variable namespace. It shares the
  corresponding `ROTARI_RUN_*` defaults, including `ROTARI_RUN_RETRY`,
  `ROTARI_RUN_ASYNC`, and `ROTARI_RUN_QUIET`.
- In a filtered run, selected jobs execute. Completed jobs outside the
  selection carry forward their result and an origin pointing to the original
  output; jobs without a completed result remain unfinished. Carry-forward writes
  reused results only to the destination run and records the source run and job
  so output remains traceable.
- A filtered `run` is contractually equivalent to copying the same filter from
  its source run into the queue and then running that queue. This applies to
  `--failed`, `--unfinished`, `--success`, their combinations, and explicit
  `--job-id`/`--job-name` selections. The source is the selected `--run-id`, or
  the current project's latest run when it is omitted.
- Dependencies use unique job names within a queue. Unknown names, duplicates,
  and cycles are rejected before execution. `add` also rejects a duplicate job
  name immediately, without writing the queue, so that mistake is never deferred
  to execution time. A `--depends-on` name may still refer to a job added later
  in the same queue, so unknown-name and cycle checks remain deferred to the
  execution boundary.
- A queued command may belong to a named stage. A dependency name that matches
  a stage expands to every job in that stage before dependency validation and
  execution. Stage members run concurrently; all must succeed before a
  dependent job is ready. Stage names and explicit job names share a namespace
  and therefore cannot collide. Commands without an explicit job name receive
  a runtime-only name derived from their job ID when they are stage members.
  See [stage expansion](../../internal/model/model.go), [queue dependency
  validation](../../internal/model/dependencies.go), [stage barrier test](../../cmd/rotari/mixed_run_test.go),
  and [copy preservation test](../../cmd/rotari/copy_test.go).
- An array queue command has an inclusive `first-last` range or an explicit
  comma-separated task list. Runtime expansion creates one `JobSpec` and
  persisted job directory per selected task. Local executors run those tasks as
  independent processes. Slurm, PBS, and LSF may submit a complete contiguous
  range as one native array; sparse selections fall back to independent
  submissions so scheduler support for sparse native arrays is not required.
- Result-based selection (`--failed`/`--unfinished`/`--success` in `copy`, and
  in rerun when `--partial-array=false`) and copied-job origin status operate on
  the unexpanded `QueuedCommand`, but results are recorded per expanded task ID.
  Matching an array command therefore aggregates its task results
  (`aggregatedJobResult` in `run_selection.go`): it is "finished" only once
  every task has a result, and any non-zero task exit code marks it failed as a
  whole.
- When `copy` selects a job without one of its prerequisites, it removes that
  dependency. An omitted prerequisite without a successful source result
  rejects the copy before the destination queue is written.
- `run`/`retry` default to `--partial-array=true`. For a filtered rerun,
  `planRerunSelection` evaluates each array task's own result against the
  selection (`planArrayTaskSelection`) instead of the aggregate, so only the
  matching tasks (for example, the failed ones) re-execute while the rest carry
  their own result forward into the new run's summary. Each carried task's
  `Origin` is recorded in `QueuedCommand.TaskOrigins`, keyed by task ID such as
  `id-1`, separately from the whole-command `Origin` field. This is needed
  because one array command can have some tasks freshly executed and others
  carried in the same run. `loadRunOrigin`/`show`/`web` check both `Origin` and
  `TaskOrigins` when resolving where a job's output lives. `--partial-array=false`
  restores the older whole-array behavior: any match re-executes every task,
  using only the whole-command `Origin`.
- An `ATTEMPT_ID` passed to `copy --job-id` identifies one exact execution
  attempt. A normal job ID selects the latest attempt. For an array task
  attempt, copy narrows the source command to a sparse array containing only
  that task; multiple task attempt IDs are grouped into one sparse command
  where possible. `run --job-id ATTEMPT_ID` and
  `retry --job-id ATTEMPT_ID` use the same copy-then-execute path.
- `--depends-on` ordering is resolved entirely by rotari itself, wave by wave,
  inside `executeMixedRun`; it never relies on scheduler-native dependency
  features such as Slurm's `--dependency`. This keeps dependency semantics
  identical across every executor, including mixes of local and remote ones in
  the same run. Jobs in a ready wave may run concurrently. A failed prerequisite
  prevents its dependents from executing; each is persisted with a non-zero
  result and `blocked by failed dependency` error.

## Validation and readiness

- `run`, `reset`, and `check` share the same project-state inspector. `run` and
  `reset` remove a stale local lock only after consistency checks pass; read-only
  `check` never removes it.
- `run` and `check` share queue loading and preflight validation: dependency
  resolution, executor names and option tokenizing, SSH target requirements,
  reserved native-array options, array range/task consistency, expanded job ID
  uniqueness, non-empty commands, environment assignments, and process-compatible
  working-directory values.
- Preflight does not execute or contact an executor and does not require local
  or remote working directories to exist.
- `check` reports a remote-host lock as unverifiable rather than probing its PID.
  It succeeds only for an idle project with a valid, non-empty queue.
- `check --deep` extends preflight with current-host checks. It resolves
  `/bin/sh`, the configured SSH executable, or the scheduler submit command for
  each effective executor. Local job commands are resolved through `/bin/sh`
  using the job's environment and working directory. Only local working
  directories are checked; no SSH or scheduler connection is made.

## Run orchestration and executor responsibilities

- `executeMixedRun` (`mixed_run.go`) is the single execution engine for every
  run, regardless of executor mix.
- Both the synchronous path (`runServerSync`) and async worker path
  (`cmdWorkerRun`) drive it. It expands array plans, dispatches to executors,
  and writes the run summary. Commands are enqueued by `add` and started by
  `run`; `run --async` selects the async worker path.
- Per-executor full-run orchestrators must not be added outside this path.
  Extend `JobExecutor` methods or `executeMixedRun` instead.
- Run dispatch has a local concurrency lane and one independent lane per
  non-local executor. `local-concurrency` and `batch-concurrency` are common
  defaults used when no executor-specific setting is supplied; executor
  settings override those defaults, and job-specific executor options remain
  highest priority.
- The former Slurm-only orchestration path was removed because it duplicated this
  responsibility and became dead after `executeMixedRun` replaced it.
- `JobExecutor` is the scheduler boundary. Implementations share lifecycle and
  result semantics where supported; scheduler metadata belongs in the job's run
  directory.
- Polling executors persist normalized scheduler state in
  `scheduler_status.json`. Read projections use it without querying schedulers
  directly.
- Scheduler display names may offer inspection commands, but must not be the
  only way to locate state. `controlQueueJobs` also writes
  `scheduler_status.json` immediately after successful suspend/resume calls,
  including for executors whose `Wait` loop does not poll that file.
- Task wrappers normalize scheduler-specific task indexes into
  `ROTARI_ARRAY_TASK_ID` and related `ROTARI_ARRAY_*` variables. Job wrappers
  expose stable run, project, job, directory, working-directory, and
  executable-path variables prefixed with `ROTARI_`.
- `QueuedCommand.Environment` stores user-supplied `KEY=VALUE` entries from
  `--env`. Values are passed to every executor and copied into run snapshots.
  Generated `ROTARI_*` variables override user values, and invalid names are
  rejected at every queue mutation boundary.
- `QueuedCommand.WorkingDirectory` is copied into each expanded `JobSpec` and
  applied before commands start by local, SSH, Slurm, PBS, and LSF executors.
  An SSH path is resolved on the remote host, not from local `context.json`.
- The SSH executor treats its first executor option as the target host and the
  rest as `ssh` options. It records output and final status locally and does not
  require the remote host to mount the run directory. It starts each command in a
  remote process group and records the PID plus Linux `/proc` start time under
  an owner-only runtime directory keyed by a random token. Cancellation opens
  another SSH connection and signals the group only when the current process
  start time and group ID match the recorded values. Its native ID remains the
  local SSH process ID for in-process waiting; legacy jobs without remote
  metadata can only cancel that local SSH process while the supervising rotari
  process remains alive.

## External integrations

- When `webhook.url` is configured, or `ROTARI_WEBHOOK_URL` is set, finalized
  runs send one `POST` summary to that endpoint. The default format is the
  generic rotari JSON payload; `webhook.format` or `ROTARI_WEBHOOK_FORMAT`
  selects a supported service-specific payload. Event filters apply after
  environment-over-config precedence. Delivery errors are warnings and do not
  change run status; successful delivery is marked by `webhook.sent` in the run
  directory.
- `diagnose` is an explicitly invoked, stateless integration. It sends one job's
  command, recorded result, and at most the last 12,000 characters of output to
  the configured LLM endpoint. API keys and diagnoses are never persisted or
  injected into job environments. An explicit BCP 47 response language is
  included when configured.
- `diagnose --rules` is a local, read-only alternative. It evaluates the same
  recorded scheduler error and output against a fixed set of documented
  signatures after case, ANSI-escape, and whitespace normalization. It makes no
  network request and reports only matched signatures with their evidence. When
  a failed run is finalized, annotations are saved on its `summary.json` result
  as informational snapshots; they never affect run status, retry planning,
  dependency resolution, or scheduler control. Every finalized failed job
  records a recognized diagnosis, an explicit no-match annotation, or an
  analysis-unavailable annotation when output cannot be read. `showJob` reads
  those saved annotations for CLI job detail, while the Web UI always shows a
  `Diagnosis` control and enables it for finalized failed jobs with saved
  annotations; it is disabled for live fallback results that have no finalized
  summary analysis.
