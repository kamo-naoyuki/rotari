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
  `<job-id>/status.json` with phase, exit code, and hosts. See
  [`internal/executor/wrapper.go`](../internal/executor/wrapper.go) and
  [`cmd/rotari/job_executor_test.go`](../cmd/rotari/job_executor_test.go).
- **DUR-2** The wrapper records status independently of the process that launched it, so
  scheduler accounting lag cannot hide the result.
- **DUR-3** The local executor uses the same wrapper. If the coordinating server or async
  worker is killed, the orphaned local job can finish and record its own status
  instead of leaving no result.
- **DUR-4** Detached supervisors are not automatically restarted. Crash detection is
  file-backed: the run lock records the supervisor PID and host, and readers
  inspect per-job status files and missing summaries to report an active or
  interrupted run. Recovery remains an explicit operator action.
- **DUR-5** Readers consume this state through one fallback chain: the attempt's
  `status`, then a terminal `status.json`, then a terminal
  `scheduler_status.json`, and finally the run's `summary.json` result, which
  alone decides a job that never ran, such as one blocked by a failed
  dependency. A summary result still supplies acceptance, blocked state,
  hosts, and diagnoses when an attempt file decides the exit code. `show` of a
  run and of a job, `jobs`, reports, and the Web UI all use it, so they never
  disagree about a job. The summary result belongs to the latest attempt, so
  a selected older attempt (`show ATTEMPT_ID` or the Web UI attempt selector)
  resolves from its own files only and shows its own timestamps.
- **DUR-6** This does not kill or reconcile leftover jobs during recovery; `reset
  --recover` and `unlock` still require the operator to confirm that jobs have
  stopped.

Implementation and tests for DUR-5: `jobstatus.ReadAttempt` and
`jobstatus.ResolveAttempt` in [`internal/jobstatus`](../internal/jobstatus/),
with [`internal/jobstatus/attempt_test.go`](../internal/jobstatus/attempt_test.go),
[`internal/jobstatus/job_test.go`](../internal/jobstatus/job_test.go),
`TestLoadJobsSelectedOlderAttemptIgnoresSummary` in
[`internal/web/loader_test.go`](../internal/web/loader_test.go), and
`TestShowJobOlderAttemptIgnoresLatestSummary` in
[`cmd/rotari/show_test.go`](../cmd/rotari/show_test.go). Every step of the
chain is checked across `show`, `jobs`, reports, and the Web API by
[`conformance/fallback_test.go`](../conformance/fallback_test.go).

## Shared-state coordination

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
- This is coordination, not distributed locking: it cannot fence a host after a
  network partition or determine whether a remote PID is alive. A remote run
  lock remains active until an operator confirms the run stopped and uses
  `unlock`.
- Project locks are scoped to project directories, so different projects largely
  isolate queue and run state. Server, registry, and filesystem state remain
  common base-level dependencies.
- `jobcontrol.Controller.Control` and `CancelJobs` signal local jobs by process group. The
  wrapper's PID is also its process-group ID via `Setpgid`, so a negative PID
  reaches both the wrapper and its command.
- `executor.LocalHostMismatch` compares the current host with `context.json`
  before signaling. A cross-host local control request gets an explicit host
  error instead of a misleading "job is not running" or a PID reuse hazard.
- Scheduler executors and SSH are expected to work from any host with the
  required access. Their control CLIs must still be installed on the host
  issuing the request.
- `schedulerCommandHint` turns missing scheduler binaries into explicit errors
  and preserves scheduler stdout/stderr, including explanations for rejected
  operations on queued jobs.
- Whole-run cancel has the same PID locality issue: it signals the run's local
  jobs by PID. `runnerHostMismatch` in `internal/jobcontrol` checks
  `running.lock`'s host first; a cross-host request fails instead of reporting
  success while leaving the run untouched.
- Finalization rechecks that `running.lock` belongs to the finishing run while
  holding the state lock. New locks are written to a temporary file and
  published without replacing an existing lock, preventing partial JSON.
- State and registry trees use centralized permission helpers. The default
  modes are `0755`/`0644`; `ROTARI_PRIVATE_STATE=true` switches new paths to
  owner-only `0700`/`0600`/`0700`.
- Permission settings apply only to newly created paths. Existing paths are not
  rechmoded, so changing the setting can produce mixed permissions.
- `webui.GenerateStatic` is the intentional exception and always emits
  publishable `0755`/`0644` output.

## Concurrency and safety

A project is in one of three states, derived from `running.lock` and
`meta.json` only, never from job-level files such as a job's own
`status.json` (see "Job execution durability" above):

| State | `running.lock` | `meta.json` phase | `run`/`add`/`copy`/`change`/`delete`/`remove`/`import` | `reset` |
| --- | --- | --- | --- | --- |
| `Idle` | absent, or present but stale (auto-removed) | `collecting`/`finished` | allowed | allowed |
| `Running` | present; owning coordinator PID is alive, or it runs on another host | `running`/`cancelling` | rejected: "is running; ... is not allowed" | rejected |
| `Interrupted` | absent, or present but the coordinator PID is dead | `running`/`cancelling` with `last_run_id` set | rejected: "has interrupted run ...", naming how to inspect and recover it | requires confirmation |

- **SAFE-1** `check` and `show` report a project's state as the table says.
  A run whose coordinator is gone, for example killed with SIGKILL, leaves
  the project interrupted, never idle; a dead local lock is removed, and the
  metadata alone then marks the run.
- **SAFE-2** While a project is running, `run`, `add`, `copy`, `change`,
  `delete`, `remove`, `import`, and `reset` fail and change nothing, so a
  second `run` of the project never starts a second runner. Other projects
  are unaffected.
- **SAFE-3** While a project is interrupted, the same commands except `reset`
  fail with a message that names the interrupted run and the `show` and
  `unlock` commands to inspect and recover it.
- **SAFE-4** `unlock` recovers an interrupted run: it keeps the retained
  queue and returns the project to idle. A `--run-id` must name that run. It
  refuses a run whose coordinator is alive on this host. A lock from another
  host, whose coordinator cannot be checked, is never removed automatically;
  `unlock` removes it once the operator has confirmed that the run stopped.
- **SAFE-5** `reset` discards the queue and keeps the run history. It
  rejects a running project. For an interrupted project it needs the
  operator's confirmation that jobs stopped, then also recovers the run.
- **SAFE-6** A command asks for confirmation only when stdin is a terminal.
  Otherwise `reset` of an interrupted project and `copy` into a non-empty
  queue fail with a message naming `--recover` or `--append`/`--overwrite`.

Further rules:

- Before `check` reports an active or interrupted run, and before `run` or
  `reset` acts on that project state, rotari verifies that lock and metadata run
  IDs agree, the run ID is a safe path element, and the run directory and
  initial `context.json` exist. An interrupted run must also have its
  `commands.json` snapshot. Active runs may temporarily lack `commands.json`
  while the worker starts. Stale locks are removed only after these checks
  succeed.
- The message for an interrupted run lists the jobs whose `status` or
  `status.json` is still non-terminal, with phase and last-update time;
  missing or unparseable job status counts as still running. The detail only
  improves the message; it does not change what `reset --recover` or `unlock`
  may do.
- Server management is separate (`server status`, `server shutdown`); project
  commands do not stop or query the server as a side effect.
- Destructive commands reject ambiguous targets, and exact IDs never degrade
  into latest-item selection.
- JSON writes use the common atomic helper. Optional fields must retain
  backward-compatible reads, and unrelated history must not be rewritten. See
  [`internal/state/store.go`](../internal/state/store.go) and
  [`internal/state/store_test.go`](../internal/state/store_test.go).

Implementation and tests: `project.Inspect` in
[`internal/project/inspect.go`](../internal/project/inspect.go) derives the
state, and `project.EnsureIdle` is the shared check for `run`, `add`, `copy`,
`change`, `delete`, `remove`, and `import`; every idle edit except `run`
reaches it through `project.Edit` or `project.EditQueue`, which take the
state lock first. `project.InterruptedRunDetail` builds the job detail.
`unlock` is [`cmd/rotari/unlock.go`](../cmd/rotari/unlock.go). Prompts go
through `isTerminal`, which asks for termios settings, so `/dev/null`, pipes,
and files are not terminals; see
[`cmd/rotari/terminal.go`](../cmd/rotari/terminal.go) and
[`cmd/rotari/terminal_test.go`](../cmd/rotari/terminal_test.go). SAFE-1 to
SAFE-6 are checked through the binary by
[`conformance/safety_test.go`](../conformance/safety_test.go).

## State load and write contracts

The `internal/state` package is the shared boundary for persisted project and
run data:

- `LoadMeta` treats a missing `meta.json` as a new project and returns the
  collecting default with an RFC3339 `updated_at`. Existing metadata with an
  empty phase or timestamp receives the same defaults. Other read errors,
  including invalid JSON, are returned to the caller.
- `LoadQueue` treats a missing `queue.json` as an empty queue. Other read and
  decode errors are returned. `ReadQueueFile` reads the same files but returns
  `os.ErrNotExist` for a missing one, for callers such as restoring a run's
  `commands.json`, where a missing snapshot is an error.
- `queue.json`, `commands.json`, and `summary.json` carry `state_version`
  (`model.StateVersion`). `Store.WriteJSON` stamps the current version on every
  `model.Queue` and `model.RunSummary` it writes, without changing the
  caller's value. `LoadQueue`, `ReadQueueFile`, and `LoadRunSummary` read files
  without the field as version 1 and reject a newer version with
  `ErrNewerStateVersion`, telling the user to upgrade, instead of dropping
  fields this binary does not know. Read these files only through those
  loaders so the check applies. Covered by `TestWriteJSONStampsStateVersion`
  and `TestLoadStateAcceptsLegacyAndRejectsNewerVersions` in
  [`internal/state/store_test.go`](../internal/state/store_test.go).
- Version policy: an added optional field does not change the version.
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
