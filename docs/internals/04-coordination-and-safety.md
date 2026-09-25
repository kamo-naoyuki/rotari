# Coordination, durability, and safety

Representative implementation and tests:

- [internal/state/lock.go](../../internal/state/lock.go) and
  [internal/state/lock_test.go](../../internal/state/lock_test.go) for lock
  inspection, with project-state coverage in
  [cmd/rotari/state_test.go](../../cmd/rotari/state_test.go).
- [internal/state/store.go](../../internal/state/store.go) and
  [internal/state/store_test.go](../../internal/state/store_test.go) for
  persisted-state load and write contracts.
- [cmd/rotari/job_executor.go](../../cmd/rotari/job_executor.go) and
  [cmd/rotari/job_executor_test.go](../../cmd/rotari/job_executor_test.go) for
  executor status and control integration.

## Job execution durability

- Every executor runs the command through a self-reporting wrapper that writes
  `<job-id>/status.json` with phase, exit code, and hosts. See
  [`cmd/rotari/job_executor.go`](../../cmd/rotari/job_executor.go) and
  [`cmd/rotari/job_executor_test.go`](../../cmd/rotari/job_executor_test.go).
- The wrapper records status independently of the process that launched it, so
  scheduler accounting lag cannot hide the result.
- The local executor uses the same wrapper. If the coordinating server or async
  worker is killed, the orphaned local job can finish and record its own status
  instead of leaving no result.
- Detached supervisors are not automatically restarted. Crash detection is
  file-backed: the run lock records the supervisor PID and host, and readers
  inspect per-job status files and missing summaries to report an active or
  interrupted run. Recovery remains an explicit operator action.
- The existing `show`/`web.go` fallback chain (`status` -> `status.json` ->
  `summary.json`) consumes this state without reader changes. See
  [`cmd/rotari/show.go`](../../cmd/rotari/show.go),
  [`cmd/rotari/web.go`](../../cmd/rotari/web.go), and
  [`cmd/rotari/show_test.go`](../../cmd/rotari/show_test.go).
- This does not kill or reconcile leftover jobs during recovery; `reset
  --recover` and `unlock` still require the operator to confirm that jobs have
  stopped.

## Shared-state coordination

- Shared-base operation relies on exclusive file creation, atomic rename, and
  advisory `flock` semantics from the shared filesystem. See
  [`internal/state/lock.go`](../../internal/state/lock.go) and
  [`internal/state/lock_test.go`](../../internal/state/lock_test.go).
- The state lock serializes queue mutations and `running.lock` prevents a
  second runner from starting the same project. See
  [`cmd/rotari/state_lock.go`](../../cmd/rotari/state_lock.go) and
  [`cmd/rotari/state_test.go`](../../cmd/rotari/state_test.go).
- This is coordination, not distributed locking: it cannot fence a host after a
  network partition or determine whether a remote PID is alive. A remote run
  lock remains active until an operator confirms the run stopped and uses
  `unlock`.
- Project locks are scoped to project directories, so different projects largely
  isolate queue and run state. Server, registry, and filesystem state remain
  common base-level dependencies.
- `controlQueueJobs` and `cancelJobs` signal local jobs by process group. The
  wrapper's PID is also its process-group ID via `Setpgid`, so a negative PID
  reaches both the wrapper and its command.
- `localExecutorHostMismatch` compares the current host with `context.json`
  before signaling. A cross-host local control request gets an explicit host
  error instead of a misleading "job is not running" or a PID reuse hazard.
- Scheduler executors and SSH are expected to work from any host with the
  required access. Their control CLIs must still be installed on the host
  issuing the request.
- `schedulerCommandHint` turns missing scheduler binaries into explicit errors
  and preserves scheduler stdout/stderr, including explanations for rejected
  operations on queued jobs.
- Whole-run cancel has the same PID locality issue. `runningWorkerHostMismatch`
  checks `running.lock`'s host before signaling; a cross-host request fails
  instead of reporting success while leaving the real runner untouched.
- Finalization rechecks that `running.lock` belongs to the finishing run while
  holding the state lock. New locks are written to a temporary file and
  published without replacing an existing lock, preventing partial JSON.
- State and registry trees use centralized permission helpers. The default
  modes are `0755`/`0644`; `ROTARI_PRIVATE_STATE=true` switches new paths to
  owner-only `0700`/`0600`/`0700`.
- Permission settings apply only to newly created paths. Existing paths are not
  rechmoded, so changing the setting can produce mixed permissions.
- `generateStaticWeb` is the intentional exception and always emits
  publishable `0755`/`0644` output.

## Concurrency and safety

- Project mutations hold the advisory `state.lock`.
- `running.lock` represents an active run and includes host information, because
  local PID checks cannot prove remote process liveness.

`inspectProjectRunState` derives one of three states from just `running.lock` and
`meta.json` -- never from job-level files like a job's own self-reported
`status.json` (see "Job execution durability" above), which only feeds
`show`/the web UI, not this state machine:

| State | `running.lock` | `meta.json` phase | `run`/`add`/`copy`/`change`/`delete`/`remove` | `reset` |
| --- | --- | --- | --- | --- |
| `projectIdle` | absent, or present but stale (auto-removed) | `collecting`/`finished` | allowed | allowed |
| `projectRunning` | present; owning coordinator PID is alive | `running`/`cancelling` | rejected: "is running; ... is not allowed" | rejected: same message |
| `projectInterrupted` | absent, or present but the coordinator PID is dead | `running`/`cancelling` with `last_run_id` set | rejected: "has interrupted run ...; recover with unlock" | `--recover` proceeds |

- A dead local run lock is removed automatically. `meta.json` remaining in
  `running` or `cancelling` with `last_run_id` marks an interrupted run.
- `ensureProjectIdleForPaths` is the shared check for `run`, `add`, `copy`,
  `change`, `delete`, and `remove`. It rejects both active and interrupted
  projects with the same message, preventing accidental queue mutation.
- `interruptedRunStatusDetail` scans job directories rather than
  `summary.json` and reports jobs whose `status` or `status.json` is still
  non-terminal, along with phase and last-update time.
- Missing or unparseable job status counts as still running. The detail only
  improves rejection and confirmation messages; it does not change what
  `--recover` or `unlock` may do.
- Before `check` reports an active or interrupted run, and before `run` or
  `reset` acts on that project state, rotari verifies that lock and metadata run
  IDs agree, the run ID is a safe path element, and the run directory and
  initial `context.json` exist. An interrupted run must also have its
  `commands.json` snapshot. Active runs may temporarily lack `commands.json`
  while the worker starts. Stale locks are removed only after these checks
  succeed.
- `unlock` derives the run ID from the selected project's lock, or from
  interrupted metadata when no lock remains. An optional `--run-id` verifies
  the expected ID before recovery; it keeps the retained queue and returns the
  phase to `collecting`.
- `reset` discards the current queue while keeping defaults and history. It
  confirms that jobs stopped before recovering an interrupted run, unless
  `reset --recover` supplies that confirmation. It rejects an active run.
- Server management is separate (`server status`, `server shutdown`); project
  commands do not stop or query the server as a side effect.
- Never silently remove a possibly active remote lock. Destructive commands
  reject ambiguous targets, and exact IDs never degrade into latest-item
  selection.
- JSON writes use the common atomic helper. Optional fields must retain
  backward-compatible reads, and unrelated history must not be rewritten. See
  [`internal/state/store.go`](../../internal/state/store.go) and
  [`internal/state/store_test.go`](../../internal/state/store_test.go).

## State load and write contracts

The `internal/state` package is the shared boundary for persisted project and
run data:

- `LoadMeta` treats a missing `meta.json` as a new project and returns the
  collecting default with an RFC3339 `updated_at`. Existing metadata with an
  empty phase or timestamp receives the same defaults. Other read errors,
  including invalid JSON, are returned to the caller.
- `LoadQueue` treats a missing `queue.json` as an empty queue. Other read and
  decode errors are returned.
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
  `reset`, `import`) go through `writeIdleQueue`, which writes the collecting
  metadata first and the queue second: the metadata change is harmless for an
  idle project, so a failed queue write leaves the previous queue in place.
  Run finalization and interrupted-run recovery keep their own order (queue
  first), so a failed metadata write leaves the project interrupted and
  recoverable instead of idle with a stale queue. See
  [`cmd/rotari/project_state.go`](../../cmd/rotari/project_state.go) and
  [`cmd/rotari/idle_queue_write_test.go`](../../cmd/rotari/idle_queue_write_test.go).
- `AppendLoadSample` creates the sample file as needed and appends one JSONL
  record. `ReadLoadSamples` ignores missing files, blank lines, and malformed
  records because load sampling is observational metadata, not run state.

When behavior crosses these boundaries, add a focused test at the public
command or persisted-state boundary. Keep CLI metadata, completion, README
usage, and this document synchronized only where their contracts actually
change.
