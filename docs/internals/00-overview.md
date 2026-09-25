# Overview and system model

This is the entry point for Rotari's internal design notes. User-facing behavior
belongs in [README.md](../../README.md) and the user guides it links to; local implementation details belong in
code and tests. Update these notes when a cross-cutting contract changes, and replace
obsolete rules rather than accumulating history.

Keep this file focused on the shared model and invariants; chapter-specific
implementation details belong in the numbered notes. Detailed Web asset, static
export, and routing notes live in
[05-web-assets-and-static-export.md](05-web-assets-and-static-export.md). Read
that file for Web UI changes.

## Terminology

- **Supervisor** means the background process that coordinates runs for one
  base directory over its Unix socket. Existing source names, package names,
  files such as `server.go`, and the `rotari server` management command retain
  the older **server** terminology for now.
- **Web server** means the HTTP process started by `rotari web` that serves the
  browser UI and API.
- In prose and user-facing messages, use **supervisor** or **Web server** when
  the distinction matters rather than referring to either process only as the
  server.

## Technology rationale

- The core implementation uses Go because rotari is primarily a command-line
  and background-server tool that coordinates OS processes, files, locks,
  signals, Unix sockets, and external schedulers.
- A statically linked Go binary keeps installation and deployment simple on
  login nodes, worker nodes, and shared HPC environments. The core does not
  require a language runtime, daemon framework, or database service at runtime.
- Go's standard library provides required filesystem, process, signal,
  networking, JSON, and concurrency primitives directly. This keeps the
  file-backed state model explicit and makes the local and server execution
  paths share the same implementation.
- Goroutines and channels fit the execution model: multiple jobs may run
  concurrently, while locks and a single server coordinate access to each
  project.
- Python is intentionally limited to the optional client interface. It wraps the
  installed CLI rather than reimplementing queue, persistence, or execution
  semantics, so there is one authoritative core implementation.
- This choice does not make Go a requirement for job commands or scheduler
  integrations. Jobs may use any executable, and executor-specific behavior
  remains behind the `JobExecutor` boundary.

## System model

- Rotari is file-backed.
- A base directory contains projects. Each project owns one mutable queue and a
  run history.
- A run snapshots the queue and stores execution state, results, and logs by
  stable job ID. Each job stores every execution attempt separately; the latest
  attempt is selected from the attempt directories.
- The CLI, server, executors, and web UI project the same persisted state model.
- The directory layout below is representative: job status and
  executor-specific files may be added incrementally while a run is active.

The normal state layout is:

```text
<basedir>/
├── server.log
├── server.lock
├── server.sock          # while the server runs; /tmp/rotari-<uid>/<hash>.sock if too long
├── server.pid           # while the background server is running
└── projects/<project>/
    ├── queue.json
    ├── meta.json
    ├── state.lock
    ├── running.lock      # while a project run is active
    └── runs/<run-id>/
        ├── commands.json
        ├── context.json
        ├── summary.json
        └── <job-id>/
            └── attempts/<attempt-id>/
                ├── command.json
                ├── output
                ├── status.json
                └── executor-specific state
```

- Files may appear incrementally while a run is active.
- Readers must tolerate missing optional or not-yet-written run files without
  inventing completed results.
- Run output remains the durable execution record.

## Core design contracts

The following rules govern the current CLI, server, web, and executor design.
An intentional change to one is an architectural change: update this document,
the user-facing documentation, and the affected tests together.

- The filesystem is the source of truth. Registries and in-memory state are
  indexes or coordination aids and must be recoverable from persisted files.
  See [`internal/state/store.go`](../../internal/state/store.go) and
  [`internal/state/store_test.go`](../../internal/state/store_test.go).
- The server coordinates access and execution; it is not persistent authority
  for project or run state. See [`cmd/rotari/server.go`](../../cmd/rotari/server.go)
  and [`cmd/rotari/server_test.go`](../../cmd/rotari/server_test.go).
- Each project owns one mutable current queue as the staging area for the next
  run. Queue edits change that queue only while the project is idle. A run that
  starts snapshots the queue; the queue remains on disk as a preserved snapshot
  while the run is active, and it stays as the retained recovery snapshot after
  an interruption. Normal completion clears the consumed queue, at which point a
  new batch can be prepared again. See [`cmd/rotari/mixed_run.go`](../../cmd/rotari/mixed_run.go),
  [`cmd/rotari/reset.go`](../../cmd/rotari/reset.go), and
  [`cmd/rotari/state_test.go`](../../cmd/rotari/state_test.go).
- The primary user-facing target depends on project state: `idle` projects show
  the queue as the active work target, while a `running` or `interrupted` project
  treats the associated run as the primary subject and the queue as the retained
  snapshot or recovery context. In other words, the contract is: `idle` =
  queue-first, `running`/`interrupted` = run-first. This keeps run state and
  queue semantics consistent across CLI, Web, and recovery flows.
- A project has at most one active run and runner at a time. That runner may
  execute multiple jobs concurrently, while different projects can run
  independently. See [`cmd/rotari/project_state.go`](../../cmd/rotari/project_state.go)
  and [`cmd/rotari/state_test.go`](../../cmd/rotari/state_test.go).
- Completed runs are immutable history. Retries, filtered runs, and
  carry-forward create or modify only a new destination run, never their source
  run. See [`internal/run/rerun.go`](../../internal/run/rerun.go),
  [`internal/queueedit/copy.go`](../../internal/queueedit/copy.go), and
  [`cmd/rotari/run_selection_test.go`](../../cmd/rotari/run_selection_test.go).
- Executors implement job execution and scheduler integration, not run
  semantics. Run planning, dependency handling, carry-forward, and summary
  finalization belong to rotari's shared execution path. See
  [`cmd/rotari/mixed_run.go`](../../cmd/rotari/mixed_run.go),
  [`internal/executor/contracts.go`](../../internal/executor/contracts.go), and
  [`cmd/rotari/mixed_run_test.go`](../../cmd/rotari/mixed_run_test.go).

## Go package boundaries

The Go implementation is layered so command adapters do not own domain
semantics:

```text
cmd/rotari
  ├── internal/model     # queue, job, run, array, selection, validation
  ├── internal/state     # paths, JSON persistence, locks, attempts
  ├── internal/executor  # executor contracts and local execution primitives
  ├── internal/jobstatus # read-side job result and timestamp resolution
  ├── internal/diagnose  # rule-based log diagnosis
  ├── internal/server    # server protocol, transport, and lifetime
  ├── internal/jobcontrol # cancel, suspend, and resume of running jobs
  ├── internal/queueedit # queue edits such as copying jobs from a run
  ├── internal/workflow  # workflow manifests, export merge, import reconcile
  ├── internal/rundiff   # comparison of two loaded runs for `diff`
  └── internal/run       # run planning, worker lifecycle, lanes, orchestration
```

The three packages that most often look similar are split by responsibility:

- `internal/model` owns Rotari's domain data and pure domain rules. It defines
  what a queue, queued command, job, run summary, job result, selection, and
  array job mean without knowing where they are stored or how they are run.
- `internal/state` owns the filesystem boundary. It turns model values into
  durable JSON files, validates path elements, lists run and attempt
  directories, and manages locks and persistence details. It should not decide
  execution policy.
- `internal/run` owns run orchestration. It uses model values and caller-provided
  state to decide what should execute, what can be carried forward, how
  dependencies unblock work, when retries happen, and how workers and lanes
  advance a run.

`internal/jobstatus` owns the read-side fallback chain that turns an attempt's
state files and the run summary into a job's displayed outcome and timestamps.
CLI and Web projections render its resolution instead of reading status files
themselves, so `show`, `jobs`, `report`, and the Web UI cannot drift apart.

`internal/rundiff` compares two runs that `cmd/rotari/diff.go` has loaded, with
each job's status already resolved through `internal/jobstatus`. It matches
jobs by name, or by job ID for unnamed jobs, because a job changed through an
imported manifest gets a new ID but keeps its name. It classifies result moves
as fixed, still failing, or newly failing and lists changed definition fields;
it never reads state files. `previousRunID` orders a project's runs by their
first load sample, then by the summary's start time, then by run ID, because
run IDs only have one-second resolution. `show --lineage` lists runs in the
same order and uses `rundiff.Lineage` for each run's counts and its changes
since the previous run. Covered by
[`internal/rundiff/rundiff_test.go`](../../internal/rundiff/rundiff_test.go)
and [`cmd/rotari/diff_test.go`](../../cmd/rotari/diff_test.go).

`internal/queueedit` and `internal/workflow` hold queue-shaping rules that
`copy`, `retry`, `export`, and `import` share: which jobs a selection copies,
which omitted prerequisites must have succeeded, which run snapshot describes a
command, and which disposition an imported job gets. They read runs through
small caller-provided interfaces, so `cmd/rotari` keeps only locking, file
access, and output.

A useful placement test is: if the code can be explained without mentioning
paths, files, locks, or directories, it probably does not belong in
`internal/state`; if it decides the next execution step for a run, it belongs in
`internal/run`; if it only defines or validates Rotari concepts, it belongs in
`internal/model`.

`cmd/rotari` remains the CLI, server, Web, and filesystem adapter layer. It
parses flags, resolves concrete state paths, connects callbacks, and formats
user-facing output. The internal packages must not import `cmd/rotari`; shared
behavior moves downward through explicit data and callback contracts instead.

When adding run behavior, prefer `internal/run` for orchestration, keeping
filesystem access in `internal/state` and scheduler/process details in
`internal/executor`. This prevents a new CLI or Web path from silently
reimplementing run semantics.

Representative implementation and tests:

- [internal/model/model.go](../../internal/model/model.go) and
  [internal/model/queue_test.go](../../internal/model/queue_test.go) for queue
  and job modeling.
- [internal/run/plan.go](../../internal/run/plan.go) for run planning.
- [internal/executor/contracts.go](../../internal/executor/contracts.go) for
  the executor boundary.
