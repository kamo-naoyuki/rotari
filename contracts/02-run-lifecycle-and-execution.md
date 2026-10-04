# Run lifecycle and execution semantics

## Job logs

- **LOG-1** Each attempt records stdout and stderr according to its
  `log_mode`: `merge` is the default and records one combined `output` log;
  `separate` records `stdout` and `stderr` independently. The mode is
  independent of external output destinations. Covered by
  `TestJobStreamsPersistSeparately` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go).
- **LOG-2** `add --output FILE` and `add --error FILE` may each be repeated.
  They add external stdout/stderr destinations without replacing the attempt
  log. If `--output` is supplied without `--error`, stderr is routed to the
  `--output` destinations too; specifying `--error` routes stderr only to the
  `--error` destinations. This is independent of internal `log_mode`.
  Duplicates within either option are rejected; the same destination may be
  used in both lists to combine them externally.
- **LOG-3** Missing parent directories for external output destinations are
  created on the execution host before the command starts. A destination open
  or directory creation failure fails the job before its command starts;
  scheduler-native submission failure remains distinct from a failure on the
  execution host.
- **LOG-4** External destinations append by default; `--open-mode truncate`
  truncates each unique destination once before execution. When stdout and
  stderr are directed to the same destination, their combined ordering is not
  guaranteed.

## Run lifecycle

- **RUN-1** A filtered rerun executes the selected jobs, carries completed
  results outside the selection into the new run, and leaves the source run
  unchanged.
- **RUN-2** A run-level retry limit retries a failed job within the same run
  until it succeeds or the limit is exhausted; a successful retry makes the
  run successful.
- **RUN-3** At run start, rotari captures the caller's absolute working
  directory. On every executor, that directory is the default job working
  directory; a job's `--working-directory` overrides it, and a relative value
  is resolved against the caller's directory. The resulting path is interpreted
  on the execution host. If it is missing, inaccessible, or not a directory
  there, the job fails before its command starts; SSH and scheduler executors
  must not silently fall back to a login or scheduler default directory. The
  caller is responsible for making the path available on remote hosts (for
  example, through a shared mount); host-specific path mapping is not implied.
  `run` and `retry` use the caller's environment by default (`--env=ALL`).
  `--env=NONE` suppresses ordinary caller variables. Both modes preserve
  rotari-managed job metadata and the job's own `--env` values; job values
  override caller values, and rotari metadata is authoritative on name
  collisions. Rotari uses each executor's native environment transfer option
  where available and compensates for documented executor differences. The
  Scheduler-generated variables may be present when a scheduler supplies them;
  rotari guarantees propagation of caller values, not an identical set of
  scheduler metadata variables across executors.
  `PWD` is set to the effective job working directory, not copied from the caller; relative job
  `--working-directory` values are resolved against the caller's directory.
  `NONE` retains
  a standard executable `PATH`, job `--env`, and rotari-managed metadata while
  suppressing other caller variables. `ALL` forwards
  values, not just variable names, and is not a secret-management mechanism:
  sensitive values can reach remote jobs and scheduler records. The run's
  `context.json` and `ROTARI_CWD` record the caller's directory; they do not
  assert that the path exists on a remote host.

- **RUN-4** When a new run uses the `id-and-fingerprint` matching mode, Job
  ID/Origin matching is resolved first. Expanded execution units already
  matched that way are removed from the fingerprint candidates. The remaining
  normal jobs, array tasks, and matrix leaves form one candidate set. A
  fingerprint is computed from the command/argv, final explicitly stored
  `add --env` values, explicitly stored `add --working-directory` after `.` and
  `..` normalization, expanded matrix parameters, and array task number. It
  does not include job ID, name, executor settings, timeout, retry settings,
  DAG, or the array/matrix range as a whole. Fingerprints are recalculated
  from the current queue and historical `commands.json`; they are not
  persisted. Matching uses `(fingerprint, queue occurrence)` after ID matches
  have been removed. If counts differ for a fingerprint, all current units
  with that fingerprint are new work. A missing historical input makes that
  historical unit ineligible rather than using a weaker fallback.

- **RUN-5** Importing a manifest job without source provenance creates new
  work. The next `run` executes that job without looking up a previous-run
  result. Implemented in [internal/run/rerun.go](../internal/run/rerun.go) and
  covered by `TestImportedWorkflowRunsFreshJobs` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go).
- **RUN-6** `wait --until-failure` returns, with status 1 and the failures
  grouped by cause, as soon as a job of the waited run has failed with no
  retry left; a failed attempt that the run will retry does not count, and a
  run that finishes first is reported as `wait` reports any run. A job's
  result is final once the run records `final_result.json` for its attempt;
  [internal/runview/run.go](../internal/runview/run.go) marks it `Final`, and
  [cmd/rotari/wait.go](../cmd/rotari/wait.go) waits on it. Covered by
  `TestWaitUntilFailureReturnsAtAFinalFailure` and
  `TestWaitUntilFailureIgnoresAFailureTheRunRetries` in
  [conformance/02-lifecycle/wait_failure_test.go](../conformance/02-lifecycle/wait_failure_test.go).
- **RUN-7** In `--depends-on` and `--depends-on-finished`, a named array
  job's name stands for all of its tasks, as a stage name stands for its jobs
  and a matrix name for its members: `--depends-on train` waits for every
  task of the array `train` to succeed, and `--depends-on-finished train` for
  every task to finish. The expansion is in `QueueToJobs` in
  [internal/model/model.go](../internal/model/model.go); covered by
  `TestArrayNameDependsOnEveryTask` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go).
- **RUN-8** A workflow manifest's `matrix_exclude` and `rotari add`'s
  `--matrix-exclude` omit each Cartesian combination matching all assignments
  in any exclusion rule. The CLI option requires `--matrix`; supplying it
  without a matrix is an error, as is a rule that names one dimension twice,
  in the CLI and in every manifest format. Normalized exclusions are stored
  with matrix provenance, and queue/run export followed by import preserves
  the same effective combinations. Covered by
  `TestWorkflowMatrixExclusionExportImport` and
  `TestMatrixExclusionRejectsRepeatedDimension` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go).
- **RUN-9** Each attempt that a run submits records its artifact candidates
  in the attempt's `artifacts.json`: file and directory references found
  statically in the job's command arguments, the shell code and shell
  scripts it runs, the Python files it names, its own `--env` and matrix
  values, its `--output`/`--error` destinations, the paths declared with
  `add --artifact`, and the configuration files those reference, each with the
  accepting rule and where it was found. Relative references are resolved on
  the attempt's working directory. A candidate is not a claim that the path
  exists or was written by the job, and discovery never changes the job's
  execution or result. Declared paths are kept by `change` (`--artifact`
  replaces them, `--clear-artifacts` clears them), copy, export, and import,
  and are not part of the job's fingerprint. An attempt without the file has
  no discovery
  information, not no associated files. The rules are in
  [internal/artifact](../internal/artifact/) and the recording in
  [internal/projectrun/artifacts.go](../internal/projectrun/artifacts.go);
  covered by `TestStartedAttemptRecordsArtifactCandidates` in
  [conformance/02-lifecycle/artifacts_test.go](../conformance/02-lifecycle/artifacts_test.go).
  [Artifact candidate rules](#artifact-candidate-rules) lists every rule
  with examples of what is recorded and what is not.
- **RUN-10** An edited `status` in an imported manifest applies to what it is
  written on. A plain job's `status` is the job's own. An `instances` entry's
  `status` applies to that matrix combination or array task. A matrix or array
  job has no status of its own: export writes none, a leaf missing from
  `instances` keeps its source result, even when `instances` is empty, and
  import rejects a job-level `status` rather than ignoring it. The one value accepted is the
  aggregate of the linked source results that earlier exports wrote, so those
  manifests still import. A leaf the source never ran, such as a task added
  by widening an array, has no result to keep and stays unfinished rather
  than accepted. The rules are `desiredStatus` and `checkGroupStatus` in
  [internal/workflow/reconcile.go](../internal/workflow/reconcile.go);
  covered by `TestImportedArrayWideningExecutesNewTasks` and
  `TestImportedGroupStatusKeepsUnlistedLeafResults` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go),
  and by `TestReconcileGroupStatusComesOnlyFromInstances`,
  `TestReconcileLeavesNewArrayTasksUnfinished`, and
  `TestFromRunGivesGroupStatusesOnlyToInstances` in
  [internal/workflow](../internal/workflow/).
- **RUN-11** Export and import select the latest command snapshot by the
  latest leaf finish time (or submission time for unfinished leaves), falling
  back to run finish time. If timestamps tie, the last listed run wins; random
  run ID suffixes do not define ordering. Selection is implemented in
  [internal/workflow/source.go](../internal/workflow/source.go) and
  [internal/workflow/reconcile.go](../internal/workflow/reconcile.go), covered
  by `TestMergeRunsUsesListedOrderForSameTimestamp`,
  `TestListedCommandRunUsesListedOrderForSameTimestamp`, and
  `TestLatestMatrixMemberUsesListedOrderForSameTimestamp` in
  [internal/workflow/reconcile_test.go](../internal/workflow/reconcile_test.go),
  and through the CLI by
  `TestWorkflowExportUsesListedRunOrderForTimestampTies` in
  [conformance/02-lifecycle/workflow_snapshots_test.go](../conformance/02-lifecycle/workflow_snapshots_test.go).

- A queue, a run's command snapshot, and an exported workflow hold the command
  layer only: each job's command, its own `--env` and `--working-directory`,
  and its scheduling fields. The working directory and environment a run uses
  otherwise come from its caller at run time and are not part of the
  workflow, so the same queue or workflow can be run again from another
  directory or shell. The caller's working directory is recorded in
  `context.json`; its environment is not part of that context record.
- The caller context is captured per run, not taken from a reusable supervisor:
  `run` starts a supervisor child (`startSupervisorProcess` in
  [cmd/rotari/server.go](../cmd/rotari/server.go) and `server.Start` in
  [internal/server/client.go](../internal/server/client.go)). The existing
  `TestRunUsesCallersDirectoryAndEnvironment` in
  [conformance/02-lifecycle/lifecycle_test.go](../conformance/02-lifecycle/lifecycle_test.go)
  checks local execution only; RUN-3 remains partial until executor-specific
  tests cover the common directory and explicit `--env` guarantees, including
  failure on an unavailable remote directory.

- Queue-editing commands mutate `queue.json`. Starting a run assigns a new ID,
  snapshots the queue, the config actually loaded for the run, and any active
  notification config, records context, and marks
  it active. Completion writes results and summary, updates metadata, clears the
  consumed queue, and removes the active lock. Completed run snapshots, results,
  logs, and config copies remain immutable until the run is explicitly deleted.
- The supervisor resolves the reference run and checks selection planning under
  the project state lock before `Begin` creates a run, for both synchronous and
  asynchronous starts. A planning error leaves the queue and metadata unchanged,
  with no new run or lock. Execution plans the selection again against the run's
  queue snapshot. If a source becomes unreadable after preflight, execution
  has already written `commands.json` before replanning, so the run is still
  inspectable. See [run preparation](../internal/supervisor/run.go),
  [execution](../internal/projectrun/execute.go),
  [preflight tests](../internal/supervisor/supervisor_test.go), and
  [snapshot tests](../internal/projectrun/lifecycle_test.go).
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
- `retry` is `run` with `--failed --unfinished` as its default selection,
  used only when neither a result filter nor a job selector is given; with
  `--job-id` it runs those jobs, like `run`. It is not an alias of
  `run --failed --unfinished`, which would reject `--job-id` (see "Selector
  combinations" in [06-selectors.md](06-selectors.md)). It does not have a
  separate `ROTARI_RETRY_*` environment-variable namespace. It shares the
  corresponding `ROTARI_RUN_*` defaults, including `ROTARI_RUN_RETRY`,
  `ROTARI_RUN_ASYNC`, and `ROTARI_RUN_QUIET`.
- A queued job that `change` edits keeps its recorded result, whether the
  edit is its command, environment, working directory, or a scheduling field
  such as the timeout. Only a status mark (`change --status`) replaces it:
  `unfinished` discards the result, so a rerun treats the job as unfinished,
  never matches it by `--failed` or `--success`, and never carries that
  result. Implemented by `applyMutation` in
  [internal/queueops/change.go](../internal/queueops/change.go) and
  `jobResult` in [internal/run/rerun.go](../internal/run/rerun.go); covered
  by the "changed" rows of `TestSelectorTable` in
  [conformance/06-selectors/selector_test.go](../conformance/06-selectors/selector_test.go) and
  `TestPlanRerunMarkedStatuses`.
- In a filtered run, selected jobs execute. Completed jobs outside the
  selection carry forward their result and an origin pointing to the original
  attempt's stdout and stderr; jobs without a completed result remain unfinished.
  Carry-forward writes reused results only to the destination run and records
  the source run and job so both streams remain traceable.
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
  See [stage expansion](../internal/model/model.go), [queue dependency
  validation](../internal/model/dependencies.go), [stage barrier test](../cmd/rotari/mixed_run_test.go),
  and [copy preservation test](../internal/queueops/copy_test.go).
- An array queue command has an inclusive `first-last` range or an explicit
  comma-separated task list. Runtime expansion creates one `JobSpec` and
  persisted job directory per selected task. Local executors run those tasks as
  independent processes. Slurm, PBS, and LSF may submit a complete contiguous
  range as one native array; SGE uses independent jobs. Sparse selections fall
  back to independent submissions so scheduler support for sparse native
  arrays is not required. See the [SGE executor](../internal/executor/sge.go)
  and its [tests](../internal/executor/sge_test.go).
- A matrix `add` expands its repeated `KEY=VALUE[,VALUE...]` dimensions before
  persistence. Every Cartesian-product combination is stored as an independent
  queue command with its own generated job ID, a derived job name when the
  base command has one, and ordinary `KEY=VALUE` environment entries.
  Matrix and array expansion can be combined; the array is applied to each
  matrix combination. Expanded commands also store matrix group provenance so
  queue and run export can reconstruct the compact declaration. Workflow
  manifests may add `matrix_exclude` partial assignments, and `add` may take
  repeatable `--matrix-exclude KEY=VALUE[,KEY=VALUE...]` rules; a generated
  combination is omitted when it matches every assignment in any rule. The
  normalized rules are part of matrix provenance, and queue/run export retains
  them. Group validation and manifest reconciliation use the effective
  combinations after exclusion, not the full Cartesian product. Partial
  `copy` or `remove`, and a `change` that leaves the group's members
  inconsistent with it, clear provenance for the affected group rather
  than presenting an incomplete group as the original matrix. A `change
  --matrix`, `--stage`, or `--all` that changes every member the same way
  keeps it ([bulk change tests](../cmd/rotari/change_test.go)). A dependency on
  the group's base name resolves to every member only while provenance exists,
  so clearing it rewrites such dependencies to the member names that remain.
  `copy` keeps a base-name dependency when the whole group is copied;
  otherwise every excluded member must have succeeded, and the dependency is
  rewritten to the copied members.
  Legacy snapshots
  without provenance export as independent jobs. See
  [matrix validation](../internal/model/dependencies.go),
  [matrix queue mutation tests](../internal/queueops/carry_state_test.go),
  [manifest compilation](../internal/workflow/manifest.go), and
  [matrix export tests](../internal/workflow/export_test.go). The
  `matrix_exclude` export/import contract is covered by
  [matrix expansion tests](../internal/model/model_test.go),
  [workflow manifest tests](../internal/workflow/manifest_test.go),
  [workflow reconciliation tests](../internal/workflow/reconcile_test.go), and
  [binary conformance](../conformance/02-lifecycle/lifecycle_test.go).
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
  [copy rules](../internal/queueedit/copy.go),
  [copy unit tests](../internal/queueedit/copy_test.go), and
  [partial stage copy tests](../internal/queueops/copy_test.go).
- A filtered run carries or re-executes jobs by their origin's result; a job
  without an origin uses the reference run's result instead. The reference
  run is `--run-id` when given, otherwise the project's last run, and it is
  resolved once, before `Begin` records the new run as the last one:
  `projectrun.ReferenceRun`, called from the supervisor's `prepareRun`
  ([internal/supervisor/run.go](../internal/supervisor/run.go)).
  `run.PlanRerun` never reads the last run itself; a job that needs a
  reference run it was not given fails planning with `ErrNoReferenceRun`. See
  `TestPrepareRunResolvesReferenceRunBeforeBegin`
  ([internal/supervisor/supervisor_test.go](../internal/supervisor/supervisor_test.go))
  and `TestPlanRerunFallsBackToReferenceRun`
  ([internal/run/plan_test.go](../internal/run/plan_test.go)).
- `run`/`retry` default to `--partial-array=true`. For a filtered rerun,
  `run.PlanRerun` evaluates each array task's own result against the
  selection instead of the aggregate, so only the
  matching tasks (for example, the failed ones) re-execute while the rest carry
  their own result forward into the new run's summary. Each carried task's
  `Origin` is recorded in `QueuedCommand.TaskOrigins`, keyed by task ID such as
  `id-1`, separately from the whole-command `Origin` field. This is needed
  because one array command can have some tasks freshly executed and others
  carried in the same run. `model.Queue.OriginOf`, which `state.LoadRunOrigin`
  applies for `show` and `web`, checks both `Origin` and
  `TaskOrigins` when resolving where a job's output lives. `--partial-array=false`
  restores the older whole-array behavior: any match re-executes every task,
  using only the whole-command `Origin`.
- A run-exported workflow manifest records compact status and attempt
  provenance. Import validates source attempts, writes explicit carry, force,
  and manual-acceptance dispositions into the queue, and leaves execution to
  the normal run path. Unchanged successes carry forward; failed, unfinished,
  changed, and downstream jobs execute. Matrix combinations and array tasks
  retain independent dispositions. A leaf without its own manifest attempt is
  recovered from the listed source run that supplied its command (the same
  latest-run rule as export): snapshots are ordered by the latest leaf finish
  or submission time, falling back to run finish time, and ties select the
  last run in the manifest's `run_ids` order. Run ID suffixes are not an
  ordering signal. A result carried into that run resolves to
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
  one of its attempts for a non-matrix job, its matrix member ID or its name
  is still queued, or it is unnamed, has no attempts, and an identical
  definition is still queued. A `matrix_exclude` rule that removes a source
  member reports that member as `remove`, even when other members remain;
  this report never affects reconciliation. See
  [workflow reconciliation](../internal/workflow/reconcile.go) and its
  [unit tests](../internal/workflow/reconcile_test.go),
  [run planning](../internal/run/rerun.go) and its
  [unit tests](../internal/run/plan_test.go),
  [workflow integration tests](../cmd/rotari/import_test.go),
  [workflow reconciliation edge cases](../cmd/rotari/workflow_manifest_errors_test.go),
  [accepted result display tests](../cmd/rotari/workflow_accepted_display_test.go),
  and [import plan tests](../cmd/rotari/workflow_import_plan_test.go).
- An `ATTEMPT_ID` passed to `copy --job-id` identifies one exact execution
  attempt. A normal job ID selects the latest attempt. For an array task
  attempt, copy narrows the source command to a sparse array containing only
  that task; multiple task attempt IDs are grouped into one sparse command
  where possible. `run --job-id ATTEMPT_ID` and
  `retry --job-id ATTEMPT_ID` use the same copy-then-execute path.
- Jobs run event by event: `ExecuteJobs` in
  [internal/run/engine.go](../internal/run/engine.go) re-checks the waiting
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
  [internal/run/lifecycle_test.go](../internal/run/lifecycle_test.go).
- `DependsOnFinished` (`--depends-on-finished`, manifest `depends_on_finished`)
  is Slurm's `afterany`: the dependent starts once each prerequisite succeeded
  or has a final failure. A result is final when the job will not run again:
  a success, a failure without retries left or that `ShouldRetry` rejects
  (for example, an explicit cancellation), or a result carried from an
  earlier run. Blocking a job settles its result, so the engine re-checks the
  waiting jobs and `afterany` dependents of blocked jobs still start. Both lists share validation (unknown names, cycles across
  kinds, and stage and matrix expansion); a name listed in both on one command
  is rejected. Covered by the `TestFinishedDependency*` tests in
  [internal/run/lifecycle_test.go](../internal/run/lifecycle_test.go) and
  `TestExecuteMixedRunStartsFinishedDependentAfterFailure` in
  [cmd/rotari/mixed_run_test.go](../cmd/rotari/mixed_run_test.go).
- A result-filtered rerun also executes every job whose `DependsOnFinished`
  names an executing job, transitively through such edges
  (`expandFinishedDownstream` in [internal/run/rerun.go](../internal/run/rerun.go)),
  because an `afterany` job may have succeeded on a failed prerequisite's
  output. `copy` requires an omitted `DependsOnFinished` prerequisite to have
  finished with any result, rather than to have succeeded.
- The server protocol version is 9 since a supervisor serves one run of one
  project (8 moved `cancel`, `suspend`, and `resume` out of the server, 7 added
  a stage or matrix scope to run requests, 6 the retry delay fields, 5 per-job
  `retry`, 4 `timeout`, 3 `depends_on_finished`). A `run` client talks only
  to the supervisor it started from its own executable, so ping reports the
  version for diagnosis, not negotiation.
- `change`, `remove`, and the `--stage`/`--matrix` scope of `run`, `retry`,
  `copy`, and `show` select queue commands through one rule, `model.SelectCommands` in
  [internal/model/command_selector.go](../internal/model/command_selector.go):
  by job IDs, a job name, a stage, a matrix base name, or all. An array command
  is selected as a whole, and naming one of its tasks is an error that points
  to the array job. A scope narrows a result filter; jobs outside it carry
  their results forward (`PlanRerun` in
  [internal/run/rerun.go](../internal/run/rerun.go), `Copy` in
  [internal/queueedit/copy.go](../internal/queueedit/copy.go)). The server
  checks the scope before it creates the run. See
  [selector tests](../internal/model/command_selector_test.go) and
  [scoped plan tests](../internal/run/plan_test.go).
- A job `Timeout` is enforced inside the job wrappers, not by the supervisor,
  so it counts running time on every executor.
  [internal/executor/wrapper.go](../internal/executor/wrapper.go) builds a
  watchdog shared by the status wrapper (local, Slurm, PBS, LSF), the native
  array wrapper, and the SSH wrapper. After the timeout it marks the attempt
  timed out, sends SIGTERM to the job's process group, waits 30 seconds, and
  sends SIGKILL if the job is still running. The wrapper records exit code 124
  (`TimeoutExitCode`) with `timed out after ...` as the error; the local
  executor and SSH `Wait` apply the same result when the wrapper itself was
  killed or only the exit code is known. Because the watchdog signals process
  group 0, a status wrapper with a timeout first makes itself a process group
  leader: the local executor already starts it that way, and otherwise it
  re-executes itself under `setsid`, which keeps its PID. It reads its process
  group from `/proc/$$/stat`, falling back to `ps -o pgid= -p`, because
  BusyBox `ps` (as on the OpenPBS test image) rejects `-p`. If it still is not a
  leader, it skips the watchdog and logs that the timeout is not enforced
  rather than signal a group it does not own. The SSH wrapper signals the
  command's own `setsid` group. Covered by
  [internal/executor/timeout_test.go](../internal/executor/timeout_test.go)
  and `TestExecuteMixedRunRecordsJobTimeout`, and against real schedulers by
  `TestSchedulerContainerStopsTimedOutJob`.

### Artifact candidate rules

These rules and examples define what RUN-9 records. Each row is a command
typed in a working directory that contains the files shown in this section.
"Recorded" lists the candidates of the job's first attempt, in record order,
written relative to the working directory (absolute paths stay absolute).
Whether a file exists, and whether the job fails, does not matter. Where the
first column names a `PATH-R` or `PATH-D` rule, every recorded candidate was
accepted by that rule. `TestArtifactCandidateExamples` in
[conformance/02-lifecycle/artifacts_test.go](../conformance/02-lifecycle/artifacts_test.go)
runs every row exactly as written, so a change to a row is a change to the
contract. The rule IDs match
[the discovery plan](../development/2026-10-03-artifact-discovery/plan.md).

One value is decided in this order: a value an exclusion matches is not
recorded; otherwise the first positive rule that matches records it;
otherwise it is not recorded.

<!-- artifact-examples:start -->

#### Where values are read

<!-- artifact-example-file: conf/keys.yaml -->
```yaml
log_dir: logs
cache_dir: [c1, c2]
```

| Source | Key context | Command | Recorded |
| --- | --- | --- | --- |
| An argument | none | `rotari add -- train results/a` | `results/a` |
| `--name=value` | `name` | `rotari add -- train --log-dir=logs` | `logs` |
| `--name value` | `name`, for the next word unless it is an option | `rotari add -- train --log-dir logs` | `logs` |
| `key=value` with an identifier key | `key` | `rotari add -- train log_dir=logs` | `logs` |
| `=` after a non-identifier | none: one value | `rotari add -- train out/a=b.csv` | `out/a=b.csv` |
| A word starting with one `-` | skipped whole | `rotari add -- train -oresults/a` | nothing |
| An `--env` or matrix value | the variable name | `rotari add --env LOG_DIR=logs -- true` | `logs` |
| A string in a configuration file named by an argument or environment value | its key; a sequence item takes its sequence's key | `rotari add -- train conf/keys.yaml` | `conf/keys.yaml`, `logs`, `c1`, `c2` |
| `--output` / `--error` | not classified (`PATH-D1`) | `rotari add --output logs/a --error logs/b -- true` | `logs/a`, `logs/b` |

#### Exclusions

<!-- artifact-example-file: conf/interpolated.yaml -->
```yaml
a_dir: "${x}/a"
b_dir: "$HOME/b"
c_dir: "$(pwd)/c"
d_dir: "`pwd`/d"
e_dir: "{{ root }}/e"
f_dir: "%(root)s/f"
g_dir: "out/*.csv"
h_dir: "out/run?.csv"
i_dir: "out/run[0-9].csv"
j_dir: "~/j"
```

| Rule | Condition | Command | Recorded |
| --- | --- | --- | --- |
| `PATH-X1` | Empty or blank | `rotari add -- train --log-dir '' --cache-dir ' '` | nothing |
| `PATH-X1` | A URL or URI (`scheme://`, `mailto:`, `data:`, `urn:`) | `rotari add -- train https://example.org/a.png s3://bucket/a file:///a.csv mailto:me@example.org` | nothing |
| `PATH-X1` | A number or numeric ratio | `rotari add -- train --log-dir 10 0.001 1e-3 inf 1/2 0.5/1.5` | nothing |
| `PATH-X1` | A regular expression (`^…`, `.*`, `.+`, `\.`, `\d`, `\w`, `\s`, `(?`, `[^`) | `rotari add -- train '^results' '.*/a.csv' 'a\.csv' 'a/\d+'` | nothing |
| `PATH-X1` | A control character, such as a newline in code | `rotari add -- train "$(printf 'a/b\nc.csv')"` | nothing |
| `PATH-X1` | An environment search-path list (name ending in `PATH`, value with `:`) | `rotari add --env PYTHONPATH=src:lib --env LD_LIBRARY_PATH=/a:/b -- true` | nothing |
| `PATH-X2` | In a configuration file: `$NAME`, `${…}`, `$(…)`, backquotes, `{{ }}`, `%(name)s`, glob characters, or a leading `~` | `rotari add -- train conf/interpolated.yaml` | `conf/interpolated.yaml` |
| `PATH-X2` | Not in arguments: argv reaches the program literally | `rotari add -- train '${x}/a' 'out/*.csv' '~/data'` | `${x}/a`, `out/*.csv`, `~/data` |
| `PATH-X3` | A special device or descriptor | `rotari add --output /dev/null -- train /dev/stderr /dev/fd/3 /proc/self/fd/1 /dev//null` | nothing |
| `PATH-X5` | The arguments of `echo` or `printf` | `rotari add -- echo a/b.csv` | nothing |
| `PATH-X5` | The same behind a launcher | `rotari add -- timeout 5 printf '%s' a/b.csv` | nothing |
| `PATH-X5` | Not when `echo` is only an argument | `rotari add -- cat echo a/b.csv` | `a/b.csv` |

#### Interpreter code (PATH-X4)

The code operand of an interpreter at the start of the command, or behind
`env`, `timeout`, or `srun` (up to four nested launchers), is never
classified as one value. A shell's code is inspected as shell source (see
the next table); Python, Perl, and Node code is not searched. Options before
the code option are parsed; an option not listed here is taken as a flag
that consumes no word. Words after the code operand are classified as
usual. `srun`'s `--job-name`/`-J` and `--partition`/`-p` values
are skipped when looking for the interpreter; `srun` rows are in the package
tests, because running them would submit jobs.

| Interpreter | Code option and value options | Command | Recorded |
| --- | --- | --- | --- |
| `sh`, `dash` | `-c` in an option bundle; `-o`/`+o VALUE`; the code is the first operand | `rotari add -- sh -c 'cat a/b' name x/y` | `a/b`, `x/y` |
| `bash` | also `-O`/`+O VALUE`, `--rcfile FILE`, `--init-file FILE`; bundles such as `-lc`, `-xec` | `rotari add -- bash -o pipefail -lc 'cat a/b'` | `a/b` |
| `bash` | the code is the first operand after `-c`, even after other flags or `--` | `rotari add -- bash -c -e -- 'cat a/b'` | `a/b` |
| `zsh` | like `bash`, without `--rcfile`/`--init-file` | `rotari add -- zsh -c 'cat a/b'` | `a/b` |
| `python`, `python2`, `python3`, `python3.N`, `pypy`, `pypy3` | exactly `-c CODE`; `-W`, `-X`, `--check-hash-based-pycs VALUE` (or attached/`=` forms) | `rotari add -- python3 -X dev -c 'print("a/b")'` | nothing |
| `python` | a `-c` after the script belongs to the script | `rotari add -- python3 train.py -c a/b.yaml` | `train.py`, `a/b.yaml` |
| `perl` | `-e`/`-E CODE`; `-I`, `-M`, `-m VALUE`; option values remain candidates | `rotari add -- perl -I ./lib -e 'print "a/b"'` | `lib` |
| `node` | `-e`/`--eval`/`-p`/`--print CODE`; `-r`/`--require`, `--import VALUE` | `rotari add -- node --require ./hook.js -e '"a/b"'` | `hook.js` |
| `node` | `--eval=CODE` / `--print=CODE` carry the code | `rotari add -- node --eval='"a/b"'` | nothing |
| `env`, `timeout` | launchers are scanned for the first interpreter | `rotari add -- env A=1 timeout 5 python3 -c 'print("a/b")'` | nothing |
| other commands | not launchers: the code is one ordinary argument, not shell source (known false positive) | `rotari add -- nice bash -c 'cat a/b'` | `cat a/b` |

#### Shell source (PATH-R1, PATH-E1)

The code of a recognized shell (`sh`, `dash`, `bash`, `zsh`) is parsed, never
run. Each simple command's words are classified like argv, with the same
interpreter and text-command rules, and the literal target of a
file-opening redirection is a `PATH-R1` reference whatever it looks like.
A word counts only when its value is fixed: quotes are removed, and a plain
`$NAME` or `${NAME}` expands only for a value rotari fixes for the attempt
(`PATH-E1`): `ROTARI_ARRAY_TASK_ID`, `ROTARI_JOB_DIR`, and the job's own
`--env` and matrix values, unless the source assigns the name itself. After
the first `cd`, `pushd`, or `popd`, relative references in that source are
skipped. Shell source inside shell source (`bash -c`, or a quoted-delimiter
heredoc read by a shell) is inspected up to three levels deep.

| Rule | Condition | Command | Recorded |
| --- | --- | --- | --- |
| `PATH-R1` | `>`, `>>`, `<`, `2>`, `<>`, `>|` | `rotari add -- bash -c 'a > r1; b >> r2; c < r3; d 2> r4; e <> r5; f >\| r6'` | `r1`, `r2`, `r3`, `r4`, `r5`, `r6` |
| `PATH-R1` | `&>` and `&>>` in `bash` and `zsh` | `rotari add -- bash -c 'a &> r7; b &>> r8'` | `r7`, `r8` |
| `PATH-R1` | Any literal target, with quoted spaces, or a number | `rotari add -- sh -c 'a > "result table"; b > 123'` | `result table`, `123` |
| `PATH-R1` | Not descriptor duplication or closing | `rotari add -- bash -c 'a 2>&1; b <&0; c >&-'` | nothing |
| `PATH-R1` | Not a here-document or here-string read as data | `rotari add -- bash -c "$(printf 'cat <<EOF\nr.csv\nEOF\ncat <<< a/b.csv')"` | nothing |
| `PATH-R1` | Not a target with an unresolved expansion or a process substitution | `rotari add -- bash -c 'a > "$OUTPUT"; b > "$(date)"; c > >(cat)'` | nothing |
| `PATH-R1` | Not a special device | `rotari add -- bash -c 'a > /dev/null 2> /dev/stderr'` | nothing |
| Commands | Words classified like argv; `echo` text is not | `rotari add -- bash -c 'python train.py --out results/x; echo a/b.csv > log.txt'` | `train.py`, `results/x`, `log.txt` |
| Commands | Globs, brace expansion, and a leading `~` are not fixed | `rotari add -- bash -c 'cat *.csv a/{x,y}.csv ~/x.csv'` | nothing |
| Commands | After `cd`, relative references are skipped | `rotari add -- bash -c 'cat a.csv; cd sub; cat b.csv > /nonexistent/out.txt'` | `a.csv`, `/nonexistent/out.txt` |
| Nesting | A shell inside shell source | `rotari add -- bash -c "bash -c 'cat a/b.csv'"` | `a/b.csv` |
| Nesting | A heredoc with a quoted delimiter read by a shell | `rotari add -- bash -c "$(printf "bash <<'SH'\npython train.py > result.csv\nSH")"` | `train.py`, `result.csv` |
| Nesting | Not an unquoted delimiter, which the outer shell expands first | `rotari add -- bash -c "$(printf 'bash <<SH\ncat a/b.csv\nSH')"` | nothing |
| Nesting | Not a heredoc read by another program | `rotari add -- bash -c "$(printf "python3 <<'PY'\nprint('a/b.csv')\nPY")"` | nothing |
| `PATH-R1` | `PATH-E1`: a job's own variable expands | `rotari add --env LR=0.1 -- bash -c 'true > "res/$LR.csv"'` | `res/0.1.csv` |
| `PATH-R1` | `PATH-E1`: not an operator form, an inherited variable, or a name the source assigns | `rotari add --env LR=0.1 -- bash -c 'a > "${LR:-x}.csv"; b > "$HOME/x.csv"; LR=5; c > "out/$LR.csv"'` | nothing |
| Arguments | argv is not shell source: `$` stays literal | `rotari add --env LR=0.1 -- train 'res/$LR.csv'` | `res/$LR.csv` |

Each array task and the attempt directory expand to their own values; that
is checked by `TestShellVariablesDifferPerArrayTask`, because an array job has
one row per task.

#### Shell scripts

A shell script the job runs is read and inspected like shell code: the
script operand of a recognized shell, in argv or in shell source, and every
`.sh` file an argument, environment value, or shell source names. The
invoking shell decides the dialect; otherwise the script's `#!` line does,
directly or through `env`, and a script without one is read as Bash. A
script whose `#!` names another interpreter is not read. Each script is read
once per attempt, within the same size limit as configuration files and the
same three levels of nesting. Relative references in a script resolve on the
job's working directory, not on the script's directory.

<!-- artifact-example-file: scripts/run.sh -->
```sh
python train.py --config conf/train.yaml > logs/train.log
```

<!-- artifact-example-file: scripts/job -->
```sh
cat data/in.csv
```

<!-- artifact-example-file: scripts/outer.sh -->
```sh
#!/bin/sh
bash scripts/inner.sh
cat data/outer.csv
```

<!-- artifact-example-file: scripts/inner.sh -->
```sh
cat data/inner.csv
```

<!-- artifact-example-file: scripts/tool.sh -->
```sh
#!/usr/bin/env python3
open("data/py.csv")
```

<!-- artifact-example-file: scripts/e1.sh -->
```sh
true > "res/$LR.csv"
```

| Script | Condition | Command | Recorded |
| --- | --- | --- | --- |
| Operand | The script a shell runs, and a configuration file it names | `rotari add -- bash scripts/run.sh` | `scripts/run.sh`, `train.py`, `conf/train.yaml`, `logs/train.log`, `results` |
| Operand | Without a `.sh` extension | `rotari add -- sh scripts/job` | `scripts/job`, `data/in.csv` |
| `.sh` | A `.sh` argument, and a script it runs | `rotari add -- timeout 5 scripts/outer.sh` | `scripts/outer.sh`, `scripts/inner.sh`, `data/outer.csv`, `data/inner.csv` |
| `#!` | Not a script whose `#!` names another interpreter | `rotari add -- scripts/tool.sh` | `scripts/tool.sh` |
| Missing | A script that cannot be read is still a candidate | `rotari add -- bash scripts/missing.sh` | `scripts/missing.sh` |
| `PATH-E1` | The job's own variables expand in a script | `rotari add --env LR=0.1 -- bash scripts/e1.sh` | `scripts/e1.sh`, `res/0.1.csv` |

#### Python files

A `.py` file named by an argument, an environment value, or shell source is
read, never run or imported, and split into tokens only. The `default` of
each `add_argument` call is classified with the call's option name as key;
every other string literal is classified with the name it is assigned or
passed to (`save_dir = "ckpt"`, `f(log_dir="logs")`) or the dictionary key
before it as key. f-strings, bytes, strings with `{}` or `%` placeholders,
globs, and `~` are skipped, and paths built in code are not reconstructed.
Modules run with `-m`, imported files, and `-c` code are not read. A
configuration file named in Python is read.

<!-- artifact-example-file: py/train.py -->
```python
import argparse
parser = argparse.ArgumentParser()
parser.add_argument("--output-dir", default="results")
parser.add_argument("--config", default="conf/train.yaml")
parser.add_argument("--format", default="png")
args = parser.parse_args()
SAVE_DIR = "checkpoints"
# open("commented/out.csv")
open("data/train.csv")
open(f"results/{args.format}.pt")
open("results/{}.pt".format(1))
open("results/%d.pt" % 1)
```

| Python | Condition | Command | Recorded |
| --- | --- | --- | --- |
| File | argparse defaults, then other literals; placeholders and comments are not | `rotari add -- python3 py/train.py` | `py/train.py`, `results`, `conf/train.yaml`, `checkpoints`, `data/train.csv` |
| File | The same through shell code | `rotari add -- bash -c 'python3 py/train.py'` | `py/train.py`, `results`, `conf/train.yaml`, `checkpoints`, `data/train.csv` |
| Module | Not a module run with `-m` | `rotari add -- python3 -m py.train` | nothing |

#### Positive rules

| Rule | Condition | Command | Recorded |
| --- | --- | --- | --- |
| `PATH-R2` | Starts with `/`, `./`, or `../` | `rotari add -- train /data/in ./x ../y` | `/data/in`, `x`, `../y` |
| `PATH-R3` | Contains `/` | `rotari add -- train results/a results/` | `results/a`, `results` |
| `PATH-R3` | Known false positives | `rotari add -- train meta-llama/Llama-3-8B origin/main 2026/10/03` | `meta-llama/Llama-3-8B`, `origin/main`, `2026/10/03` |
| `PATH-R4` | A basename with one of these extensions, in any case | `rotari add -- train a.yaml a.yml a.json a.toml a.csv a.tsv a.jsonl a.txt a.png a.jpg a.jpeg a.svg a.pdf a.npy a.npz a.h5 a.hdf5 a.pt a.pth a.sh a.py B.PNG` | `a.yaml`, `a.yml`, `a.json`, `a.toml`, `a.csv`, `a.tsv`, `a.jsonl`, `a.txt`, `a.png`, `a.jpg`, `a.jpeg`, `a.svg`, `a.pdf`, `a.npy`, `a.npz`, `a.h5`, `a.hdf5`, `a.pt`, `a.pth`, `a.sh`, `a.py`, `B.PNG` |
| `PATH-R4` | Any other suffix, a version, or an extension alone | `rotari add -- train a.ckpt a.log v1.2.3 .json results` | nothing |
| `PATH-R5` | Key `path`, `file`, or `dir` | `rotari add -- train --path p1 --file f1 --dir d1` | `p1`, `f1`, `d1` |
| `PATH-R5` | Key ending in `_path`, `_file`, or `_dir` | `rotari add -- train --data-path p2 --log-file f2 --save-dir d2` | `p2`, `f2`, `d2` |
| `PATH-R5` | Key `output`, `out`, or `input`, value not a format name | `rotari add -- train --output o1 --out o2 --input i1` | `o1`, `o2`, `i1` |
| `PATH-R5` | Key ending in `_output`, `_out`, or `_input`, value not a format name | `rotari add -- train --eval-output o3 --save-out o4 --train-input i2` | `o3`, `o4`, `i2` |
| `PATH-R5` | Format names under an output or input key: every `PATH-R4` extension without its dot, `text`, `html`, `xml`, `md`, `markdown`, `table`, `stdout`, `stderr`, `stdin`, in any case | `rotari add -- train --output yaml --output yml --output json --output toml --output csv --output tsv --output jsonl --output txt --output png --output jpg --output jpeg --output svg --output pdf --output npy --output npz --output h5 --output hdf5 --output pt --output pth --output sh --output py --output text --output html --output xml --output md --output markdown --output table --output stdout --output stderr --input stdin --out JSON` | nothing |
| `PATH-R5` | Other keys | `rotari add -- train --name n --format f --dir-name d --outdir o --output-format t --dropout r --filename x --paths y` | nothing |
| `PATH-R5` | Keys compare case-insensitively, `-` as `_`, by the last dotted part, ignoring `--`, `+`, `++`, `~` | `rotari add -- train --LOG-DIR k1 trainer.log_dir=k2 +x.save_dir=k3 ++y.out_dir=k4 '~cache_dir=k5'` | `k1`, `k2`, `k3`, `k4`, `k5` |
| `PATH-R5` | Environment variable names are keys | `rotari add --env OUTPUT=e1 --env CACHE_DIR=e2 --env MODEL=e3 -- true` | `e1`, `e2` |
| `PATH-D1` | The job's log destinations; stderr follows `--output` without `--error` | `rotari add --output logs/out.log --output logs/copy.log -- true` | `logs/out.log`, `logs/copy.log` |
| `PATH-D2` | Paths declared with `add --artifact`, not classified; a plain `$NAME` expands with `ROTARI_ARRAY_TASK_ID`, `ROTARI_JOB_DIR`, and the job's own variables; globs and other `$` text stay literal | `rotari add --env LR=0.1 --artifact 'sweep/$LR.csv' --artifact results/ --artifact 'logs/*.txt' -- true` | `sweep/0.1.csv`, `results`, `logs/*.txt` |

#### Examples

<!-- artifact-example-file: conf/train.yaml -->
```yaml
out_dir: results
lr: 0.1
format: png
plot: ${out_dir}/plot.png
data: https://example.org/data.csv
```

| Command | Recorded | Why |
| --- | --- | --- |
| `rotari add -- python train.py --lr 0.1` | `train.py` | The job's own script is a candidate (`.py`) |
| `rotari add -- python train.py --config conf/train.yaml` | `train.py`, `conf/train.yaml`, `results` | The configuration file is read: `out_dir` is a path key; `format`, the interpolated `plot`, and the URL are not paths |
| `rotari add --env CONFIG=conf/train.yaml -- true` | `conf/train.yaml`, `results` | A configuration file named by an environment value is read too |
| `rotari add -- ./run.sh /data/input` | `run.sh`, `/data/input` | A script path and an absolute input |
| `rotari add -- train metrics.csv` | `metrics.csv` | A path need not exist |
| `rotari add -- train '>' out.txt` | `out.txt` | argv is not shell code: `>` is a literal word, and `out.txt` an ordinary argument |
| `rotari add -- bash -c 'python train.py > out/log.txt'` | `train.py`, `out/log.txt` | Shell code is parsed: the script is a command argument, and the redirection target a `PATH-R1` reference |
| `rotari add -- timeout 1h bash -c 'cat conf/train.yaml'` | `conf/train.yaml`, `results` | The same through a launcher; a configuration file named in shell source is read too |

#### Configuration files

A YAML, JSON, or TOML file named by an argument or environment value is
read, and each string value in it is classified like an argument, with its
own key as context. The file itself is a candidate first. Relative values are
resolved on the job's working directory, not on the file's directory.

<!-- artifact-example-file: conf/nested.yaml -->
```yaml
train:
  checkpoint_dir: ckpt
  data:
    - data/train.csv
    - data/valid.csv
  log_file: train.log
eval:
  output: eval_results
  report: report.pdf
```

<!-- artifact-example-file: conf/skipped.yaml -->
```yaml
name: baseline
optimizer: adam
output: png
dir_name: results
lr: 0.001
resume: true
url: https://example.org/model.pt
plot: ${out_dir}/plot.png
home: ~/data
pattern: logs/*.txt
```

<!-- artifact-example-file: conf/combine.yaml -->
```yaml
out_dir: results
filename: plot.png
```

<!-- artifact-example-file: conf/anchors.yaml -->
```yaml
base: &base
  cache_dir: cache
run: *base
extra: !include other.yaml
explicit: !!str kept.csv
```

<!-- artifact-example-file: conf/run.json -->
```json
{"output_dir": "out", "inputs": ["a/1.csv", "a/2.csv"], "seed": 1, "model": {"weights_path": "/models/w.pt"}}
```

<!-- artifact-example-file: conf/run.toml -->
```toml
log_dir = "logs"
[data]
train = "data/train.tsv"
[[stages]]
out = "stage1"
[[stages]]
out = "stage2"
```

<!-- artifact-example-file: conf/parent.yaml -->
```yaml
child: conf/child.yaml
```

<!-- artifact-example-file: conf/child.yaml -->
```yaml
deep_dir: deep
```

<!-- artifact-example-file: conf/broken.json -->
```json
{"out_dir": "results",
```

| Command | Recorded | Why |
| --- | --- | --- |
| `rotari add -- train --config conf/nested.yaml` | `conf/nested.yaml`, `ckpt`, `data/train.csv`, `data/valid.csv`, `train.log`, `eval_results`, `report.pdf` | Nested mappings and sequences in source order; a sequence item takes its sequence's key (`data`); `ckpt` is resolved on the working directory, not on `conf/` |
| `rotari add -- train --config conf/skipped.yaml` | `conf/skipped.yaml` | Bare names under other keys, a format name under `output`, `dir_name` (not a path key), numbers, booleans, URLs, interpolation, `~`, and globs are not recorded |
| `rotari add -- train --config conf/combine.yaml` | `conf/combine.yaml`, `results`, `plot.png` | Separate keys are never joined into `results/plot.png` |
| `rotari add -- train --config conf/anchors.yaml` | `conf/anchors.yaml`, `cache`, `kept.csv` | Aliases are not followed and custom tags such as `!include` are not evaluated; an explicit `!!str` is a string |
| `rotari add -- train --config conf/run.json` | `conf/run.json`, `a/1.csv`, `a/2.csv`, `/models/w.pt`, `out` | JSON keys are read in sorted order |
| `rotari add -- train --config conf/run.toml` | `conf/run.toml`, `data/train.tsv`, `logs`, `stage1`, `stage2` | TOML tables and arrays of tables, keys in sorted order |
| `rotari add -- train --config conf/parent.yaml` | `conf/parent.yaml`, `conf/child.yaml` | A configuration file named inside one is recorded but not read |
| `rotari add -- train --config conf/broken.json` | `conf/broken.json` | A file that does not parse yields no references |
| `rotari add -- train --config missing.yaml` | `missing.yaml` | A file that cannot be read is still a candidate |
| `rotari add --output logs/run.yaml -- true` | `logs/run.yaml` | A log destination is never read as configuration |

<!-- artifact-examples:end -->

## Cancellation

These rules describe what a caller observes; they hold for synchronous and
asynchronous runs, for the CLI and the Web UI, and whichever process sends
the request, on the host that owns the run.

- **CAN-1** A whole-run cancel of an active run (`rotari cancel` with no job
  selection or only the run ID, or the Web UI's cancel-run) stops every job of
  the run that is running, and no job of the run starts afterwards. A job that
  ends between the request and the signal is already stopped, so it does not
  fail the cancel.
- **CAN-2** A cancelled run still finishes: it writes its summary, every job
  has a recorded result, the running lock is removed, and the project returns
  to idle. Cancellation never leaves the run interrupted, so no `unlock` is
  needed before the next `add` or `run`.
- **CAN-3** `rotari cancel --wait` returns once the cancelled run has finished,
  and exits 0.
- **CAN-4** Cancelling one job (`rotari cancel JOB_ID`, or the Web UI's
  cancel-job) stops only that job. The rest of the run keeps running and
  finishes normally, and the run's `--retry` does not start the cancelled job
  again.
- **CAN-5** A job that a cancel stopped, whether the whole run's or its own,
  records a cancelled result: its error is `cancelled`, keeping an earlier
  error in parentheses. `--filter-failure-kind cancelled` selects it, and
  failure groups list it as `cancelled` rather than by its exit code or
  signal. The result is set once, when the job reaches its final result, in
  [internal/projectrun/execute.go](../internal/projectrun/execute.go); covered
  by `TestCancelledJobsReadAsCancelled` in
  [conformance/02-lifecycle/cancel_test.go](../conformance/02-lifecycle/cancel_test.go).
- **CAN-6** Cancelling a job that has not started yet, such as one waiting
  for a dependency, keeps it from starting: when the run reaches it, the job
  records a cancelled result instead of being submitted. Every path that
  submits jobs checks this first: local jobs, scheduler jobs, and the tasks of
  a native array, which is then submitted without them. Implemented once in
  `Dispatcher.cancelledBeforeStart` in
  [internal/run/dispatch.go](../internal/run/dispatch.go); covered by
  `TestDispatcherNeverSubmitsACancelledJob` and `TestCancelledPendingJobNeverStarts` in
  [conformance/02-lifecycle/cancel_test.go](../conformance/02-lifecycle/cancel_test.go).
- **CAN-7** The message `run --async` and `retry --async` print once the run
  starts ends its last line, and the commands it suggests work as printed:
  `rotari wait ... --run-id RUN_ID` waits for that run, and
  `rotari cancel ... RUN_ID` cancels that run and no other. Written by
  `Operations.StartRun` in
  [internal/supervisor/run.go](../internal/supervisor/run.go); covered by
  `TestAsyncStartHintsWork`.

Whole-run and job cancel go through `jobcontrol.Controller`
([internal/jobcontrol/jobcontrol.go](../internal/jobcontrol/jobcontrol.go)).
Covered by [conformance/02-lifecycle/cancel_test.go](../conformance/02-lifecycle/cancel_test.go).

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

- `projectrun.Runner` ([internal/projectrun](../internal/projectrun/)) is
  the single run lifecycle for every run, regardless of executor mix. The
  run's supervisor calls it for synchronous runs (`Operations.Run` in
  [internal/supervisor/run.go](../internal/supervisor/run.go)) and
  asynchronous ones (`Operations.StartRun`) alike; the two differ only in
  whether the client stays attached:
  - `Begin`, under the state lock of an idle project, writes `context.json`
    first, then takes `running.lock` with the supervisor's PID, registers the
    run, and marks `meta.json` running, so a project that looks running always
    has its run context. A failure after the lock is taken removes the lock; a
    failed registration also removes the new run directory.
  - `Execute` snapshots the queue to `commands.json`, plans the selection,
    expands array plans, dispatches to executors, and writes the run summary.
  - `Finish` records the final load, finalizes the queue and metadata
    (`Finalize`, which checks that the run lock still belongs to the run), and
    removes the run lock unless it belongs to another run. The lock is removed
    even when finalization fails, so the project reads as interrupted rather
    than running.
  Covered by [internal/projectrun/lifecycle_test.go](../internal/projectrun/lifecycle_test.go)
  and [cmd/rotari/mixed_run_test.go](../cmd/rotari/mixed_run_test.go).
  Commands are enqueued by `add` and started by `run`; `run --async` returns
  once the supervisor has begun the run.
- Per-executor full-run orchestrators must not be added outside this path.
  Extend `JobExecutor` methods or `projectrun.Runner.Execute` instead.
- Run dispatch (`Dispatcher` in [internal/run/dispatch.go](../internal/run/dispatch.go))
  keeps one lane per executor for the whole run: a local concurrency lane and
  one independent lane per non-local executor. Each lane is a FIFO queue: jobs
  start in the order they became ready, which is queue order for jobs ready
  together, and a job holds a slot only while it is submitted or running, so
  a finished job's slot goes to the next queued job at once instead of after a
  whole batch. Jobs that start together still submit one after another in
  queue order, because schedulers such as Slurm order pending jobs by
  submission; only waiting for results runs in parallel. Covered by
  `TestDispatcherStartsQueuedJobsInOrder`. Array tasks that become
  ready together are still submitted as one native array without taking
  slots; retried tasks are submitted individually or as a sparse array. Jobs
  on a scheduler lane are waited on concurrently, so
  `schedulerQueryGate` in
  [internal/executor/scheduler_shared.go](../internal/executor/scheduler_shared.go)
  spaces scheduler state and accounting queries 200ms apart per process;
  reading a job's wrapper `status.json` is not gated. Covered by
  `TestDispatcherRefillsSchedulerSlotsAsJobsFinish`.
- Against real schedulers, the `TestSchedulerContainer*` tests in
  [cmd/rotari/scheduler_container_run_test.go](../cmd/rotari/scheduler_container_run_test.go)
  build rotari into the state directory mounted at `/state` and run it inside
  the Slurm or PBS container of the scheduler integration workflow: an
  immediate retry, refilled concurrency slots, a timeout, `--depends-on` and
  `--depends-on-finished` after a failure, and a retry of one array task.
  They skip unless `ROTARI_SCHEDULER_CONTAINER_TEST=1`; running them also
  needs `ROTARI_SCHEDULER_EXECUTOR`, `SCHEDULER_CONTAINER`, `SCHEDULER_USER`,
  and `SCHEDULER_STATE_DIR` (the host directory mounted at `/state`), which
  [.github/workflows/scheduler-integration.yml](../.github/workflows/scheduler-integration.yml)
  sets.
- `local-concurrency` and `batch-concurrency` are common
  defaults used when no executor-specific setting is supplied; executor
  settings override those defaults, and job-specific executor options remain
  highest priority.
- The former Slurm-only orchestration path was removed because it duplicated this
  responsibility and became dead after the shared execution path replaced it.
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

- Notifications are configured only by `notifications.toml`, resolved from the
  first scope that has one: project, then basedir, then the global config
  directory. Scopes are not merged. `ROTARI_WEBHOOK_URL` overrides
  `webhook.url` so the endpoint can stay out of the file. A run resolves its
  webhook settings when it starts and snapshots the file with its other
  configs, so editing the file mid-run does not change that run.
- `webhook.job_failure`, `webhook.job_success`, `webhook.run_failure`, and
  `webhook.run_success` select which events are sent; jobs report only their
  final result, so retried attempts, blocked jobs, and jobs cancelled before
  they start are reported once. Events that occur within ten seconds of the
  first pending event are sent as one `POST`, and a run's completion flushes
  the pending batch immediately. `webhook.fields` selects the reported fields
  from the shared vocabulary ([implementation](../internal/notification/payload.go),
  [tests](../internal/notification/event_test.go)) and `webhook.max_jobs`
  bounds the listed jobs;
  `link` is browser-only. `webhook.format` selects the generic rotari JSON
  payload or a supported service-specific payload. Delivery errors are warnings
  and do not change run status; rotari keeps no delivery ledger and does not
  retry.
- Failure diagnosis is local and rule-based. It evaluates a failed job's
  recorded scheduler error and configured log against a fixed set of documented
  signatures after case, ANSI-escape, and whitespace normalization. It makes no
  network request and reports only matched signatures, each citing its latest
  matching line, ordered from the latest evidence to the earliest with the
  scheduler error treated as later than the log; the generic Python-exception entry is
  dropped when a specific rule explains the same line
  ([rules.go](../internal/diagnose/rules.go),
  [rules_test.go](../internal/diagnose/rules_test.go)). When
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
  ([analysis.go](../internal/diagnose/analysis.go),
  [analysis_test.go](../internal/diagnose/analysis_test.go),
  [job_result_test.go](../internal/model/job_result_test.go)). `showJob`
  reads the saved analysis for CLI job detail, while the Web UI always shows a
  `Diagnosis` control and enables it for finalized failed jobs with a saved
  analysis; it is disabled for live fallback results that have no finalized
  summary analysis.
