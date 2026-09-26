# Overview and system model

This page holds the shared model and core contracts. User-facing behavior
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

The package map, process roles, and per-command walkthroughs are in
[../ARCHITECTURE.md](../ARCHITECTURE.md). The boundary rules that must hold:

- Internal packages must not import `cmd/rotari`. Shared behavior moves
  downward through explicit data and callback contracts instead.
- `internal/model` has no I/O. `internal/state` owns every file and directory
  layout decision and makes no execution-policy decisions. `internal/run` owns
  run orchestration. Executors implement job execution only.
- Renderers do not read status files themselves: `show`, `jobs`, `report`,
  and the Web UI resolve outcomes through `internal/jobstatus` so they cannot
  drift apart.
- New run behavior goes in `internal/run`, not in a CLI or Web path, so no
  interface silently reimplements run semantics.

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
