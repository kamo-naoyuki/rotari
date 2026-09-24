# Overview and system model

This is the entry point for Rotari's internal design notes. User-facing behavior
belongs in [README.md](../../README.md); local implementation details belong in
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
├── server.sock          # while the background server is running
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
  run. Queue edits change that queue; starting a run snapshots it, and normal
  completion clears the consumed queue. An interrupted run retains the queue
  until it is recovered or reset. See [`cmd/rotari/mixed_run.go`](../../cmd/rotari/mixed_run.go),
  [`cmd/rotari/reset.go`](../../cmd/rotari/reset.go), and
  [`cmd/rotari/state_test.go`](../../cmd/rotari/state_test.go).
- A project has at most one active run and runner at a time. That runner may
  execute multiple jobs concurrently, while different projects can run
  independently. See [`cmd/rotari/project_state.go`](../../cmd/rotari/project_state.go)
  and [`cmd/rotari/state_test.go`](../../cmd/rotari/state_test.go).
- Completed runs are immutable history. Retries, filtered runs, and
  carry-forward create or modify only a new destination run, never their source
  run. See [`cmd/rotari/run_selection.go`](../../cmd/rotari/run_selection.go),
  [`cmd/rotari/copy.go`](../../cmd/rotari/copy.go), and
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
  ├── internal/diagnose  # rule-based log diagnosis
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
