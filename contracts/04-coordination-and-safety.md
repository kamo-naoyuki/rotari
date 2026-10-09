# Coordination, durability, and safety

Representative implementation and tests:

- [internal/state/lock.go](../internal/state/lock.go) and
  [internal/state/lock_test.go](../internal/state/lock_test.go) for lock
  inspection, with project-state coverage in
  [internal/project/inspect_test.go](../internal/project/inspect_test.go).
- [internal/state/store.go](../internal/state/store.go) and
  [internal/state/store_test.go](../internal/state/store_test.go) for
  persisted-state load and write contracts.
- [internal/executor/registry.go](../internal/executor/registry.go) and
  [internal/executor/registry_test.go](../internal/executor/registry_test.go)
  for the executor registry, job ownership, and local host checks.
- [internal/jobcontrol/jobcontrol.go](../internal/jobcontrol/jobcontrol.go)
  and [internal/jobcontrol/jobcontrol_test.go](../internal/jobcontrol/jobcontrol_test.go)
  for cancel, suspend, and resume.

## Job execution durability

- **DUR-1** Every executor runs the command through a self-reporting wrapper that writes
  the attempt's `status.json` with phase, exit code, and hosts.
- **DUR-2** The wrapper records status independently of the process that launched it, so
  scheduler accounting lag cannot hide the result.
- **DUR-3** The local executor uses the same wrapper. If the run's supervisor is
  killed, the orphaned local job still finishes and records its own status,
  which `show` and `jobs` then report, instead of leaving no result.
- **DUR-4** Detached supervisors are not automatically restarted. Crash detection is
  file-backed: the run lock records the supervisor PID and host, and readers
  inspect per-job status files and missing summaries to report an active or
  interrupted run. Recovery remains an explicit operator action.
- **DUR-5** Readers consume this state through one fallback chain: the attempt's
  `status`, then a terminal `status.json`, then a terminal
  `scheduler_status.json`, and finally the run's recorded result, which
  alone decides a job that never ran, such as one blocked by a failed
  dependency. The recorded result is the run's `summary.json` result or,
  before the run writes its summary, its carried result (DUR-7). A summary result still supplies acceptance, blocked state,
  hosts, and diagnoses when an attempt file decides the exit code. `show` of a
  run and of a job, `jobs`, reports, and the Web UI all use it, so they never
  disagree about a job; each labels a job as `show` does, through
  `jobstatus.DisplayLabel`, including `success (accepted)` and `(carried)`. The summary result belongs to the latest attempt, so
  a selected older attempt (`show ATTEMPT_ID` or the Web UI attempt selector)
  resolves from its own files only and shows its own timestamps.
- **DUR-6** This does not kill or reconcile leftover jobs during recovery;
  `unlock` requires the operator to confirm that jobs have stopped, and a job
  that was still running keeps running and records its result.
- **DUR-7** Before it dispatches any job, a run records the results it carries
  forward from earlier runs in `carried.json` beside `commands.json` (a
  `model.RunSummary` holding only those results). Until the run writes
  `summary.json`, readers take a carried job's result from it, so `show`,
  `lineage`, `jobs`, the Web API, and the MCP tools read a carried job as its
  carried result rather than as running. The recent-jobs listing also marks
  that result as `(carried)` and uses the origin attempt's timestamps. Job
  control (`cancel`,
  `suspend`, `resume`, their previews and filters) does not reach it; a
  cancel naming a carried job fails. A job with an origin but no recorded
  result is one the run will still execute. Written in
  [internal/projectrun/execute.go](../internal/projectrun/execute.go), read
  through `jobstatus.RecordedResults` and `runlineage.IsCarried`; recent jobs
  use [internal/joblist/joblist.go](../internal/joblist/joblist.go); checked by
  `TestCarriedJobsReadAsCarriedDuringTheRun` in
  [conformance/03-interfaces/carried_test.go](../conformance/03-interfaces/carried_test.go).
- **DUR-8** Run lifecycle, best-effort per-job execution state, and the initiating
  client's connection history are separate display dimensions. Job readers
  distinguish recorded nonterminal phases from terminal results and unknown
  state; `jobs` keeps attempts from interrupted runs visible, and a recorded
  `running` phase does not prove the executor is still alive.
  The client record distinguishes `detached (async)` from `detached (Ctrl-D)`
  with detached-first labels and is reported as unknown when the supervisor
  cannot be verified. The Web UI prefers the server's `client_label`, falling
  back to the same labels when absent. `runs`, `show`, and the Web
  API use the shared projections in
  [internal/runview](../internal/runview/client.go) and
  [internal/jobstatus](../internal/jobstatus/job.go); checked by
  `TestCLIAndWebAgreeOnJobResults` and `TestJobOutlivesKilledSupervisor` in
  [conformance/04-coordination](../conformance/04-coordination/), and detached
  labels in `TestAsyncStartHintsWork` in
  [conformance/02-lifecycle/cancel_test.go](../conformance/02-lifecycle/cancel_test.go).
- **DUR-9** `delete` of one run or of every run also removes the deleted
  runs' client-attachment state from the project directory: the run's
  session-attachment marker and session records no live client holds. It
  leaves other runs' attachment state alone. Implemented by
  `attachment.ForgetRun` in [internal/attachment](../internal/attachment/session.go),
  called from [internal/queueops/delete.go](../internal/queueops/delete.go);
  checked by `TestDeleteRemovesRunAttachmentState` in
  [conformance/04-coordination](../conformance/04-coordination/).

Implementation and tests: the wrapper is built in
[`internal/executor/wrapper.go`](../internal/executor/wrapper.go), with
[`cmd/rotari/job_executor_test.go`](../cmd/rotari/job_executor_test.go).
DUR-3, DUR-4, and DUR-6 are checked through the binary by
[`conformance/04-coordination/durability_test.go`](../conformance/04-coordination/durability_test.go), which
kills a run's supervisor with SIGKILL. For DUR-5, `jobstatus.ReadAttempt` and
`jobstatus.ResolveAttempt` in [`internal/jobstatus`](../internal/jobstatus/),
with [`internal/jobstatus/attempt_test.go`](../internal/jobstatus/attempt_test.go),
[`internal/jobstatus/job_test.go`](../internal/jobstatus/job_test.go),
`TestLoadJobsSelectedOlderAttemptIgnoresSummary` in
[`internal/web/loader_test.go`](../internal/web/loader_test.go), and
`TestShowJobOlderAttemptIgnoresLatestSummary` in
[`cmd/rotari/show_test.go`](../cmd/rotari/show_test.go). Every step of the
chain is checked across `show`, `jobs`, reports, and the Web API by
`TestStatusFallbackChainAgreesAcrossViews` in
[`conformance/04-coordination/status_views_test.go`](../conformance/04-coordination/status_views_test.go).

## Shared-state coordination

- **COORD-1** Controlling a local job (`cancel`, `suspend`, or `resume`, from
  the CLI or the Web UI) from a host other than the one it runs on fails with
  an error naming that host, instead of reporting that the job is not running
  or signalling an unrelated process with the same PID.
- **COORD-2** A whole-run cancel of a run whose coordinator runs on another
  host fails the same way, naming that host, instead of reporting success
  while the run goes on.
- **COORD-3** A run lock from another host counts as running: `check`
  reports the project `locked`, and the commands of SAFE-2 are rejected. Its
  coordinator cannot be checked, so the lock stays until the operator
  confirms that the run stopped and uses `unlock` (SAFE-4). This is
  coordination, not distributed locking: it cannot fence a host after a
  network partition.
- **COORD-4** A scheduler job whose scheduler command is not installed fails
  with an error naming the command, which `show` of the job reports.
  Scheduler executors and SSH work from any host with the required access,
  but their commands must be installed on the host issuing the request.
- **COORD-5** With `ROTARI_PRIVATE_STATE=true`, new state and registry paths
  are owner-only (`0700` directories, `0600` files); otherwise they are
  `0755`/`0644`, less the umask. Existing paths keep their mode, so changing
  the setting can leave mixed permissions. The static Web export
  (`web --static-dir`) always writes publishable `0755`/`0644` output.
- **COORD-6** `suspend` and `resume` validate every selected running job's
  host and executor support before signalling any job. If a scheduler command
  itself fails after earlier jobs were acted on, the error names those jobs.
  The shared implementation is
  [`internal/jobcontrol/jobcontrol.go`](../internal/jobcontrol/jobcontrol.go);
  unit coverage is in
  [`internal/jobcontrol/active_run_test.go`](../internal/jobcontrol/active_run_test.go)
  and CLI/Web conformance is in
  [`conformance/04-coordination/private_state_test.go`](../conformance/04-coordination/private_state_test.go).

Design and implementation notes:

- Shared-base operation relies on exclusive file creation, atomic rename, and
  advisory `flock` semantics from the shared filesystem. See
  [`internal/state/lock.go`](../internal/state/lock.go) and
  [`internal/state/lock_test.go`](../internal/state/lock_test.go).
- The state lock serializes queue mutations and `running.lock` prevents a
  second runner from starting the same project. See
  `AcquireStateLock` and `AcquireRunLock` in
  [`internal/state/lock.go`](../internal/state/lock.go), and
  `TestBeginRejectsActiveRun` in
  [`internal/projectrun/lifecycle_test.go`](../internal/projectrun/lifecycle_test.go).
- Project locks are scoped to project directories, so different projects largely
  isolate queue and run state. Server, registry, and filesystem state remain
  common base-level dependencies.
- `jobcontrol.Controller.Control` and `CancelJobs` signal local jobs by process
  group. The wrapper's PID is also its process-group ID via `Setpgid`, so a
  negative PID reaches both the wrapper and its command.
  `executor.LocalHostMismatch` compares the current host with `context.json`
  (COORD-1), and `runnerHostMismatch` in `internal/jobcontrol` compares it
  with `running.lock` (COORD-2), before any signal.
- `schedulerCommandHint` turns missing scheduler binaries into explicit errors
  and preserves scheduler stdout/stderr, including explanations for rejected
  operations on queued jobs (COORD-4).
- Finalization rechecks that `running.lock` belongs to the finishing run while
  holding the state lock. New locks are written to a temporary file and
  published without replacing an existing lock, preventing partial JSON.
- State and registry trees use centralized permission helpers (COORD-5);
  `webui.GenerateStatic` is the intentional exception.

COORD-1 to COORD-5 are checked through the binary by
[`conformance/04-coordination/private_state_test.go`](../conformance/04-coordination/private_state_test.go),
which rewrites the host recorded in `context.json` and `running.lock` to
stand for another host.

## Concurrency and safety

A project is in one of three states, derived from `running.lock` and
`meta.json` only, never from job-level files such as a job's own
`status.json` (see "Job execution durability" above):

| State | `running.lock` | `meta.json` phase | `run`/`retry`/`delete` | `add`/`copy`/`change`/`remove`/`import` | `reset` |
| --- | --- | --- | --- | --- | --- |
| `Idle` | absent, or present but stale (auto-removed) | `collecting`/`finished` | allowed | allowed | allowed |
| `Running` | present; owning coordinator PID is alive, or it runs on another host | `running`/`cancelling` | rejected: "is running; ... is not allowed" | allowed, except `copy` of the running run | allowed; clears only the next queue |
| `Interrupted` | absent, or present but the coordinator PID is dead | `running`/`cancelling` with `last_run_id` set | rejected: "has interrupted run ...", naming how to inspect and recover it | allowed, except `copy` of the interrupted run | allowed; clears only the next queue |

- **SAFE-1** `check` and `show` report a project's state as the table says;
  `check` names an idle project `ready` or `empty` (with or without queued
  jobs), a running one `running`, or `locked` when its lock comes from
  another host (COORD-3), and an interrupted one `interrupted`.
  A run whose coordinator is gone, for example killed with SIGKILL, leaves
  the project interrupted, never idle; a dead local lock is removed, and the
  metadata alone then marks the run.
- **SAFE-2** While a project is running, `run`, `retry`, and `delete` are
  rejected and change nothing, so no second run or in-run retry starts.
  Queue edits, including `reset`, still apply only to the next run. Other
  projects are unaffected.
- **SAFE-3** While a project is interrupted, `run` and `delete` fail with a
  message that names the interrupted run, the `show` and `unlock`
  commands to inspect and recover it, and the `retry --run-id` command that
  reruns its failed and unfinished jobs afterwards.
- **SAFE-4** `unlock` recovers an interrupted run: it leaves the queue as it
  is, returns the project to idle, and names the `retry --run-id` command that
  reruns the run's failed and unfinished jobs, since the run took them from
  the queue when it started (CORE-3, RUN-12). `rotari unlock` and the MCP
  unlock tools share `project.Unlock` in
  [internal/project/unlock.go](../internal/project/unlock.go), covered by
  `TestUnlockByProjectState`. A `--run-id` must name that run. It
  refuses a run whose coordinator is alive on this host. A lock from another
  host, whose coordinator cannot be checked, is never removed automatically;
  `unlock` removes it once the operator has confirmed that the run stopped.
  Without `--run-id`, `unlock` of an idle project with no lock, or a project
  that does not exist, succeeds as a no-op and does not modify or create state.
  `TestCLIFlagPairUnlock` and `TestCLIFlagPairUnlockSafety` in
  [conformance/03-interfaces/pairruns/flag_pair_unlock_test.go](../conformance/03-interfaces/pairruns/flag_pair_unlock_test.go)
  check flag-order parity, retained state, and local-live, mismatched-run,
  remote, and lockless recovery using synthetic locks after fixture execution
  has stopped.
- **SAFE-5** `reset` clears only the queue for the next run and keeps run
  history. It is allowed while a run is running or interrupted, and never
  changes that run's state. Recovery is done separately with `unlock`.
- **SAFE-6** A command asks for confirmation only when stdin is a terminal.
  Otherwise `copy` into a non-empty queue fails with a message naming
  `--append`/`--overwrite`.
- **SAFE-7** Before recovering an interrupted run, the MCP unlock preview
  reports how many of its jobs' latest attempts have not recorded a final
  status; `unlock` prints the same warning when it recovers such a run. The
  scan reads files only, so a job killed without writing its status still
  appears to be running. Implemented in `scanInterruptedRunJobs` in
  [internal/project/inspect.go](../internal/project/inspect.go) over
  `state.LatestAttemptDirs`; checked by
  `TestUnlockWarnsAboutRunningJobs` and `TestMCPUnlockRecoversAnInterruptedRun`.
- **SAFE-8** While a project is running or interrupted, `add`, `copy`,
  `change`, `remove`, `import`, and `reset` edit the queue for the next run,
  since the run took its own jobs when it started (CORE-3). They leave the
  project's `meta.json` phase, which belongs to the run, unchanged. `copy`
  rejects the running run, whose results are not final, and the interrupted
  run until `unlock`. Implemented by `project.EditQueueGuarded` in
  [internal/project/edit.go](../internal/project/edit.go) and
  `project.EnsureRunSettled` in
  [internal/project/inspect.go](../internal/project/inspect.go), covered by
  `TestEditQueueKeepsTheRunsPhase` and through the binary by
  `TestQueueEditsBesideAnActiveRun` and `TestResetClearsQueueBesideActiveRun` in
  [conformance/04-coordination/private_state_test.go](../conformance/04-coordination/private_state_test.go).
- **SAFE-9** `reset --recover` and `ROTARI_RESET_RECOVER` are rejected with an
  instruction to recover an interrupted run with `unlock`; neither is silently
  accepted. Checked through the binary by `TestResetRejectsRemovedRecoveryOptions`.
- **SAFE-10** When the selected run is the project's active or interrupted
  run and the next queue is non-empty, `show` displays the run snapshot and the
  next queue separately. JSON keeps the run's `commands` snapshot and adds
  `next_queue`; it never substitutes the next queue for the run's jobs.
  `check` continues to report the next queue's count. Checked through the
  binary by `TestShowActiveRunIncludesNextQueue`.

Further rules:

- Before `check` reports an active or interrupted run, and before `run` or
  `reset` acts on that project state, rotari verifies that lock and metadata run
  IDs agree, the run ID is a safe path element, and the run directory and
  initial `context.json` exist. An interrupted run must also have its
  `commands.json` snapshot. A run writes `commands.json` before it takes the
  run lock, so an active run has it as well; the check still tolerates an
  active run without it, as an earlier rotari could leave one. Stale locks are removed only after these checks
  succeed.
- The message for an interrupted run lists the jobs whose `status` or
  `status.json` is still non-terminal, with phase and last-update time;
  missing or unparseable job status counts as still running. The detail warns
  before `unlock`; it does not change what recovery may do.
- Server management is separate (`server status`, `server shutdown`); project
  commands do not stop or query a supervisor as a side effect. Only `run` and
  `retry` start one, for their own run.
- Destructive commands reject ambiguous targets, and exact IDs never degrade
  into latest-item selection.
- JSON writes use the common atomic helper. Optional fields must retain
  backward-compatible reads, and unrelated history must not be rewritten. See
  [`internal/state/store.go`](../internal/state/store.go) and
  [`internal/state/store_test.go`](../internal/state/store_test.go).

Implementation and tests: `project.Inspect` in
[`internal/project/inspect.go`](../internal/project/inspect.go) derives the
state, and `project.EnsureIdle` is the shared check for `run` and `delete`,
which reaches it through `project.Edit`. Queue edits go through
`project.EditQueue`, which takes the state lock and checks only that the
project state is consistent. `project.InterruptedRunDetail` builds the job detail.
`unlock` is [`cmd/rotari/unlock.go`](../cmd/rotari/unlock.go). Prompts go
through `isTerminal`, which asks for termios settings, so `/dev/null`, pipes,
and files are not terminals; see
[`cmd/rotari/terminal.go`](../cmd/rotari/terminal.go) and
[`cmd/rotari/terminal_test.go`](../cmd/rotari/terminal_test.go). SAFE-1 to
SAFE-6 are checked through the binary by
[`conformance/04-coordination/private_state_test.go`](../conformance/04-coordination/private_state_test.go).

## State load and write contracts

- **STATE-1** A command that reads `queue.json`, `commands.json`, or
  `summary.json` written by a newer rotari, with a greater `state_version`,
  fails with a message to upgrade rotari and leaves the file as it was. It
  never reads such a file by dropping the fields it does not know. The Web UI
  lists such a run as `unreadable`, showing the upgrade message on its page,
  and still shows the project's other runs.
- **STATE-2** These files without `state_version`, written before versioning,
  are read as version 1 and keep working.
- **STATE-3** Reading history never rewrites it: the commands and Web views
  that read a finished run leave its files as they were, whatever version
  they carry.
- **STATE-4** Load samples are observational: blank or malformed lines in
  `load_samples.jsonl` are skipped, and `show` and the Web UI still work.

STATE-1 to STATE-4 are checked through the binary by
[`conformance/04-coordination/private_state_test.go`](../conformance/04-coordination/private_state_test.go).

The `internal/state` package is the shared boundary for persisted project and
run data. How it implements these rules and the rest of the state layout:

- `LoadMeta` treats a missing `meta.json` as a new project and returns the
  collecting default with an RFC3339 `updated_at`. Existing metadata with an
  empty phase or timestamp receives the same defaults. Other read errors,
  including invalid JSON, are returned to the caller.
- `LoadQueue` treats a missing `queue.json` as an empty queue. Other read and
  decode errors are returned. `ReadQueueFile` reads the same files but returns
  `os.ErrNotExist` for a missing one, for callers such as restoring a run's
  `commands.json`, where a missing snapshot is an error.
  Job lookup through `resolve.JobInRun` likewise treats a missing snapshot as
  no jobs (including a run still starting), but returns other read, decode,
  and version errors rather than reporting a missing job. `run`, `retry`,
  `copy`, and `show` preserve those errors for job-name lookup. Covered by
  [`internal/resolve/job_snapshot_test.go`](../internal/resolve/job_snapshot_test.go)
  and [`conformance/04-coordination/job_lookup_test.go`](../conformance/04-coordination/job_lookup_test.go).
- `queue.json`, `commands.json`, `summary.json`, and `carried.json` carry `state_version`
  (`model.StateVersion`). `Store.WriteJSON` stamps the current version on every
  `model.Queue` and `model.RunSummary` it writes, without changing the
  caller's value. `LoadQueue`, `ReadQueueFile`, and `LoadRunSummary` read files
  without the field as version 1 and reject a newer version with
  `ErrNewerStateVersion`, telling the user to upgrade, instead of dropping
  fields this binary does not know. Read these files only through those
  loaders so the check applies. A reader that treats a run's `commands.json`
  or `summary.json` as optional calls `state.CheckRunVersions` first, so a
  newer file is refused rather than read as missing. Covered by `TestWriteJSONStampsStateVersion`
  and `TestLoadStateAcceptsLegacyAndRejectsNewerVersions` in
  [`internal/state/store_test.go`](../internal/state/store_test.go).
- Version policy: an added optional field does not change the version.
  For example, each result of a run summary carries the job's `name`, such
  as `train[3]`, so that `wait --json`, `show --json`, and the MCP tools can
  tell array tasks apart; summaries written before it have no `name`, and
  readers must not require it.
  Renaming, removing, or reinterpreting a field bumps `model.StateVersion`;
  the loaders then convert every older version in memory after decoding, and
  the conversion is recorded here with the old and new field mapping.
  Historical run files are never rewritten just to upgrade them. `meta.json`,
  `context.json`, and per-job files are not versioned yet.
- `LoadRunSummary` and `LoadContext` do not invent missing state. A missing
  or invalid file is returned as an error so callers can distinguish a run in
  progress from a completed run.
- `ReadJobTimestamp` resolves the latest valid attempt and returns an empty
  string for an unsafe path element, unsupported state filename, or missing
  file. `ReadAttemptTimestamp` applies the same file-name boundary to an
  already resolved attempt directory.
- `LoadLocalJobResult` requires the `finished_at` marker and a numeric
  `status` file. It returns the saved command and exit code, and treats
  missing, unsafe, or malformed state as no local result rather than
  fabricating one.
- `WriteJSON` and `Store.WriteJSON` create parent directories, write through a
  temporary file, apply the configured file mode, and publish with rename.
  Directory, encoding, permission, and rename failures are returned.
- Each file is replaced atomically, but `queue.json` and `meta.json` are not
  replaced together. Idle queue edits (`add`, `copy`, `change`, `remove`,
  `reset`, `import`) go through `project.WriteIdleQueue`, which writes the collecting
  metadata first and the queue second: the metadata change is harmless for an
  idle project, so a failed queue write leaves the previous queue in place.
  Run finalization and interrupted-run recovery keep their own order (queue
  first), so a failed metadata write leaves the project interrupted and
  recoverable instead of idle with a stale queue. See
  [`internal/project/edit.go`](../internal/project/edit.go) and
  [`internal/project/edit_test.go`](../internal/project/edit_test.go).
- `AppendLoadSample` creates the sample file as needed and appends one JSONL
  record. `ReadLoadSamples` ignores missing files, blank lines, and malformed
  records because load sampling is observational metadata, not run state.

When behavior crosses these boundaries, add a focused test at the public
command or persisted-state boundary. Keep CLI metadata, completion, user-facing
usage docs, and this document synchronized only where their contracts actually
change.
