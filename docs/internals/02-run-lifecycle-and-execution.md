# Run lifecycle and execution semantics

## Run lifecycle

- Queue-editing commands mutate `queue.json`. Starting a run assigns a new ID,
  snapshots the queue and every active config file, records context, and marks
  it active. Completion writes results and summary, updates metadata, clears the
  consumed queue, and removes the active lock. Completed run snapshots, results,
  logs, and config copies remain immutable until the run is explicitly deleted.
- Retries and filtered runs always create new history and never modify their
  source run. `--retry N` retries a failed job up to N additional times within
  the same run. A job's own `Retry` (`add --retry`, manifest `retry`) replaces
  that limit for the job (`JobSpec.RetryLimit`). A failed job with retries
  left is started again as soon as its result arrives, or after
  `JobSpec.RetryDelayFor` when it has `retry_delay` (multiplied by
  `retry_backoff` per further retry and capped by `retry_max_delay`); other
  jobs never hold it back. Attempt numbers count per job. A failed job is one
  whose result has a non-zero exit code and is not explicitly cancelled.
  Covered by `TestPerJobRetryOverridesRunRetry`,
  `TestExecuteJobsSpacesRetriesWithBackoff`, and
  `TestExecuteMixedRunHonorsPerJobRetry`.
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
- `copy` without a selection restores every command from the current project's
  latest run. Result filters belong on the following `run`, allowing the full
  restored queue to be inspected or edited before execution. A filtered `run`
  selects work from each queued command's origin result and carries forward
  completed non-matching results. If the queue is empty, `run --failed` and
  other result-filtered runs first restore the selected `--run-id`, or the
  project's latest run when it is omitted. Copy-side result filters remain
  available for intentionally restoring only a subset.
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
- A matrix `add` expands its repeated `KEY=VALUE[,VALUE...]` dimensions before
  persistence. Every Cartesian-product combination is stored as an independent
  queue command with its own generated job ID, a derived job name when the
  base command has one, and ordinary `KEY=VALUE` environment entries.
  Matrix and array expansion can be combined; the array is applied to each
  matrix combination. Expanded commands also store matrix group provenance so
  queue and run export can reconstruct the compact declaration. Partial
  `copy`, `remove`, or `change` clears provenance for the affected group rather
  than presenting an incomplete group as the original matrix. A dependency on
  the group's base name resolves to every member only while provenance exists,
  so clearing it rewrites such dependencies to the member names that remain.
  `copy` keeps a base-name dependency when the whole group is copied;
  otherwise every excluded member must have succeeded, and the dependency is
  rewritten to the copied members.
  Legacy snapshots
  without provenance export as independent jobs. See
  [matrix validation](../../internal/model/dependencies.go),
  [matrix queue mutation tests](../../cmd/rotari/queue_carry_state_test.go),
  [manifest compilation](../../internal/workflow/manifest.go), and
  [matrix export tests](../../internal/workflow/export_test.go).
- Result-based selection (`--failed`/`--unfinished`/`--success` in `copy`, and
  in rerun when `--partial-array=false`) and copied-job origin status operate on
  the unexpanded `QueuedCommand`, but results are recorded per expanded task ID.
  Matching an array command therefore aggregates its task results
  (`model.AggregatedJobResult`): it is "finished" only once
  every task has a result, and any non-zero task exit code marks it failed as a
  whole.
- When `copy` selects a job without one of its prerequisites, it removes that
  dependency. An omitted prerequisite without a successful source result
  rejects the copy before the destination queue is written. For a stage
  dependency, only the omitted stage members must have succeeded; the
  dependency stays while any member is copied, because copied members keep
  their stage and the name resolves to them. A retry that re-executes one
  failed stage member therefore keeps its dependents waiting for it. See
  [copy rules](../../internal/queueedit/copy.go),
  [copy unit tests](../../internal/queueedit/copy_test.go), and
  [partial stage copy tests](../../cmd/rotari/copy_test.go).
- `run`/`retry` default to `--partial-array=true`. For a filtered rerun,
  `run.PlanRerun` evaluates each array task's own result against the
  selection instead of the aggregate, so only the
  matching tasks (for example, the failed ones) re-execute while the rest carry
  their own result forward into the new run's summary. Each carried task's
  `Origin` is recorded in `QueuedCommand.TaskOrigins`, keyed by task ID such as
  `id-1`, separately from the whole-command `Origin` field. This is needed
  because one array command can have some tasks freshly executed and others
  carried in the same run. `loadRunOrigin`/`show`/`web` check both `Origin` and
  `TaskOrigins` when resolving where a job's output lives. `--partial-array=false`
  restores the older whole-array behavior: any match re-executes every task,
  using only the whole-command `Origin`.
- A run-exported workflow manifest records compact status and attempt
  provenance. Import validates source attempts, writes explicit carry, force,
  and manual-acceptance dispositions into the queue, and leaves execution to
  the normal run path. Unchanged successes carry forward; failed, unfinished,
  changed, and downstream jobs execute. Imported jobs without an origin are
  planned as new work and never consult the project's last run, which the run
  server has already replaced with the run being planned. Matrix combinations and array tasks
  retain independent dispositions. A leaf without its own manifest attempt is
  recovered from the listed source run that supplied its command (the same
  latest-run rule as export), and a result carried into that run resolves to
  the attempt's original run, so a task re-executed by a filtered retry is
  reused rather than replaced by its earlier failure. Manual acceptance creates a destination
  result with exit code zero and `accepted: true`, while `Origin` continues to
  reference the immutable failed source result and output. `show` run and job
  views and the Web job table all read the destination result first, so they
  display `success (accepted)`; `show` job then shows the accepted source
  attempt named by `Origin`, not merely the source's latest attempt. The
  import plan reports each job's decoded source run, job, attempt, and status
  (per task for arrays; the human view shows only the attempt ID, which
  encodes the run and job IDs, and the status) and lists exported source jobs that the manifest no
  longer describes as `remove`; each job line ends with `command=`, whose value
  runs to the end of the line. On a TTY, `execute` lines are green like other
  successful queue changes such as `add`, `reuse` lines are cyan because they
  only report a carried result, and `accept` and `remove` lines are yellow, following the CLI color rules in
  [03](03-server-and-command-interfaces.md#cli-presentation). A source job is kept when the manifest names
  one of its attempts, another member of its matrix group is kept, its name is
  still queued, or it is unnamed, has no attempts, and an identical definition
  is still queued; this report never affects reconciliation. See
  [workflow reconciliation](../../internal/workflow/reconcile.go) and its
  [unit tests](../../internal/workflow/reconcile_test.go),
  [run planning](../../internal/run/rerun.go) and its
  [unit tests](../../internal/run/plan_test.go),
  [workflow integration tests](../../cmd/rotari/import_test.go),
  [workflow reconciliation edge cases](../../cmd/rotari/workflow_manifest_errors_test.go),
  [accepted result display tests](../../cmd/rotari/workflow_accepted_display_test.go),
  and [import plan tests](../../cmd/rotari/workflow_import_plan_test.go).
- An `ATTEMPT_ID` passed to `copy --job-id` identifies one exact execution
  attempt. A normal job ID selects the latest attempt. For an array task
  attempt, copy narrows the source command to a sparse array containing only
  that task; multiple task attempt IDs are grouped into one sparse command
  where possible. `run --job-id ATTEMPT_ID` and
  `retry --job-id ATTEMPT_ID` use the same copy-then-execute path.
- Jobs run event by event: `ExecuteJobs` in
  [internal/run/engine.go](../../internal/run/engine.go) re-checks the waiting
  jobs whenever a result arrives or a retry delay passes, and starts each job
  as soon as its prerequisites allow, without waiting for unrelated jobs. It
  never relies on scheduler-native dependency features such as Slurm's
  `--dependency`, which keeps dependency semantics identical across every
  executor, including mixes of local and remote ones in the same run. A
  prerequisite whose failure is final prevents its `--depends-on` dependents
  from executing; each is persisted with a non-zero result and `blocked by
  failed dependency` error. When `run cancel` marks the project `cancelling`,
  the engine starts no more retries and records jobs that never started as
  `cancelled before start`. Covered by
  `TestExecuteJobsRetriesAndUnblocksWithoutWaitingForOtherJobs` and
  `TestExecuteJobsStopsRetryingWhenStopped` in
  [internal/run/lifecycle_test.go](../../internal/run/lifecycle_test.go).
- `DependsOnFinished` (`--depends-on-finished`, manifest `depends_on_finished`)
  is Slurm's `afterany`: the dependent starts once each prerequisite succeeded
  or has a final failure. A result is final when the job will not run again:
  a success, a failure without retries left or that `ShouldRetry` rejects
  (for example, an explicit cancellation), or a result carried from an
  earlier run. Blocking a job settles its result, so the engine re-checks the
  waiting jobs and `afterany` dependents of blocked jobs still start. Both lists share validation (unknown names, cycles across
  kinds, and stage and matrix expansion); a name listed in both on one command
  is rejected. Covered by the `TestFinishedDependency*` tests in
  [internal/run/lifecycle_test.go](../../internal/run/lifecycle_test.go) and
  `TestExecuteMixedRunStartsFinishedDependentAfterFailure` in
  [cmd/rotari/mixed_run_test.go](../../cmd/rotari/mixed_run_test.go).
- A result-filtered rerun also executes every job whose `DependsOnFinished`
  names an executing job, transitively through such edges
  (`expandFinishedDownstream` in [internal/run/rerun.go](../../internal/run/rerun.go)),
  because an `afterany` job may have succeeded on a failed prerequisite's
  output. `copy` requires an omitted `DependsOnFinished` prerequisite to have
  finished with any result, rather than to have succeeded.
- The server protocol version is 6 since the retry delay fields were added (5
  added per-job `retry`, 4 `timeout`, 3 `depends_on_finished`), so a client
  replaces an older server that would drop new queue fields when it loads the
  queue.
- A job `Timeout` is enforced inside the job wrappers, not by the supervisor,
  so it counts running time on every executor.
  [internal/executor/wrapper.go](../../internal/executor/wrapper.go) builds a
  watchdog shared by the status wrapper (local, Slurm, PBS, LSF), the native
  array wrapper, and the SSH wrapper. After the timeout it marks the attempt
  timed out, sends SIGTERM to the job's process group, waits 30 seconds, and
  sends SIGKILL if the job is still running. The wrapper records exit code 124
  (`TimeoutExitCode`) with `timed out after ...` as the error; the local
  executor and SSH `Wait` apply the same result when the wrapper itself was
  killed or only the exit code is known. Because the watchdog signals process
  group 0, a status wrapper with a timeout first makes itself a process group
  leader: the local executor already starts it that way, and otherwise it
  re-executes itself under `setsid`, which keeps its PID. If it still is not a
  leader, it skips the watchdog and logs that the timeout is not enforced
  rather than signal a group it does not own. The SSH wrapper signals the
  command's own `setsid` group. Covered by
  [internal/executor/timeout_test.go](../../internal/executor/timeout_test.go)
  and `TestExecuteMixedRunRecordsJobTimeout`.

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
- The server launches the async worker as `__worker-run` with arguments built
  by `WorkerArgs` in [internal/run/worker_args.go](../../internal/run/worker_args.go)
  and parsed by `parseWorkerRunArgs` in [cmd/rotari/main.go](../../cmd/rotari/main.go).
  Bool flags must be written as `--name=BOOL`, because a separate value is
  parsed as the first positional argument. Worker-only flags such as
  `--selection` have no CLI metadata and are registered directly on the
  FlagSet. `TestWorkerArgsParseBackToOptions` in
  [cmd/rotari/worker_args_test.go](../../cmd/rotari/worker_args_test.go)
  round-trips the arguments through the worker's parser.
- Per-executor full-run orchestrators must not be added outside this path.
  Extend `JobExecutor` methods or `executeMixedRun` instead.
- Run dispatch (`Dispatcher` in [internal/run/dispatch.go](../../internal/run/dispatch.go))
  keeps one lane per executor for the whole run: a local concurrency lane and
  one independent lane per non-local executor. Each lane is a FIFO queue: jobs
  start in the order they became ready, which is queue order for jobs ready
  together, and a job holds a slot only while it is submitted or running, so
  a finished job's slot goes to the next queued job at once instead of after a
  whole batch. Covered by `TestDispatcherStartsQueuedJobsInOrder`. Array tasks that become
  ready together are still submitted as one native array without taking
  slots; retried tasks are submitted individually or as a sparse array. Jobs
  on a scheduler lane are submitted and waited on concurrently, so
  `schedulerQueryGate` in
  [internal/executor/scheduler_shared.go](../../internal/executor/scheduler_shared.go)
  spaces scheduler state and accounting queries 200ms apart per process;
  reading a job's wrapper `status.json` is not gated. Covered by
  `TestDispatcherRefillsSchedulerSlotsAsJobsFinish`.
- Against real schedulers, the `TestSchedulerContainer*` tests in
  [cmd/rotari/scheduler_container_run_test.go](../../cmd/rotari/scheduler_container_run_test.go)
  build rotari into the state directory mounted at `/state` and run it inside
  the Slurm or PBS container of the scheduler integration workflow: an
  immediate retry, refilled concurrency slots, a timeout, `--depends-on` and
  `--depends-on-finished` after a failure, and a retry of one array task.
  They skip unless `ROTARI_SCHEDULER_CONTAINER_TEST=1`; running them also
  needs `ROTARI_SCHEDULER_EXECUTOR`, `SCHEDULER_CONTAINER`, `SCHEDULER_USER`,
  and `SCHEDULER_STATE_DIR` (the host directory mounted at `/state`), which
  [.github/workflows/scheduler-integration.yml](../../.github/workflows/scheduler-integration.yml)
  sets.
- `local-concurrency` and `batch-concurrency` are common
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
- Repeated scheduler queue or accounting query failures use a shared
  per-process exponential polling backoff, capped at 30 seconds. A successful
  query restores the normal polling interval; wrapper `status.json` remains an
  independent terminal-result source throughout the backoff.
- Scheduler submission uses the same per-process timing boundary. Explicit
  controller or transport failures retry at most twice after 1 and 2 seconds;
  permanent configuration or authorization errors do not retry. Timeouts and
  submit responses without a usable native job ID are ambiguous and must not be
  retried automatically because the scheduler may have accepted the job.
- Each scheduler has an independent per-process submit gate with a 100ms
  minimum interval. The gate applies to ordinary submissions, native arrays,
  and retry attempts, but does not coordinate across rotari processes.
- Per-run executor settings can raise the scheduler-specific submit interval
  and change the transient-submit retry limit. Settings are copied into an
  immutable executor instance for the run, so one server process cannot mutate
  the registered executor policy of another run.
- Scheduler display names may offer inspection commands, but must not be the
  only way to locate state. `jobcontrol.Controller.Control` also writes
  `scheduler_status.json` immediately after successful suspend/resume calls,
  including for executors whose `Wait` loop does not poll that file.
- Task wrappers normalize scheduler-specific task indexes into
  `ROTARI_ARRAY_TASK_ID` and related `ROTARI_ARRAY_*` variables. Job wrappers
  expose stable run, project, job, directory, working-directory, and
  executable-path variables prefixed with `ROTARI_`.
- Slurm supports native sparse arrays, so a selected task list such as
  `1,3,4` is submitted as `sbatch --array=1,3,4`. Other scheduler executors
  use native arrays only for complete ranges and submit sparse selections as
  independent jobs.
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
  network request and reports only matched signatures, each citing its latest
  matching line, ordered from the latest evidence to the earliest with the
  scheduler error treated as later than the log; the generic Python-exception entry is
  dropped when a specific rule explains the same line
  ([rules.go](../../internal/diagnose/rules.go),
  [rules_test.go](../../internal/diagnose/rules_test.go)). When
  a failed run is finalized, the analysis is saved on its `summary.json` result
  as an informational snapshot; it never affects run status, retry planning,
  dependency resolution, or scheduler control. `diagnosis_status` records
  `matched`, `no_match`, or `unavailable` (with the reason in
  `diagnosis_note`), and `diagnoses` lists only recognized diagnoses.
  `diagnosis_rules` is a hash of the rule definitions and matcher revision;
  saved analyses are never recomputed, and `show`, `report`, and the Web UI
  mark a matched or no-match analysis whose hash differs from the current
  rules as produced by earlier rules. Results saved before these fields existed
  are converted when decoded: a lone no-match or unavailable entry becomes the
  status, and they carry no rules hash
  ([analysis.go](../../internal/diagnose/analysis.go),
  [analysis_test.go](../../internal/diagnose/analysis_test.go),
  [job_result_test.go](../../internal/model/job_result_test.go)). `showJob`
  reads the saved analysis for CLI job detail, while the Web UI always shows a
  `Diagnosis` control and enables it for finalized failed jobs with a saved
  analysis; it is disabled for live fallback results that have no finalized
  summary analysis.
