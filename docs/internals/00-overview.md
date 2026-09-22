# Overview and system model

This is the entry point for Rotari's internal design notes. User-facing behavior
belongs in [README.md](../README.md); local implementation details belong in code
and tests. Update these notes when a cross-cutting contract changes, and replace
obsolete rules rather than accumulating history.

Keep this file focused on the shared model and invariants; chapter-specific
implementation details belong in the numbered notes. Detailed Web asset, static
export, and routing notes live in
[05-web-assets-and-static-export.md](05-web-assets-and-static-export.md). Read
that file for Web UI changes.

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
- The server coordinates access and execution; it is not persistent authority
  for project or run state.
- Each project owns one mutable current queue as the staging area for the next
  run. Queue edits change that queue; starting a run snapshots it, and normal
  completion clears the consumed queue. An interrupted run retains the queue
  until it is recovered or reset.
- A project has at most one active run and runner at a time. That runner may
  execute multiple jobs concurrently, while different projects can run
  independently.
- Completed runs are immutable history. Retries, filtered runs, and
  carry-forward create or modify only a new destination run, never their source
  run.
- Executors implement job execution and scheduler integration, not run
  semantics. Run planning, dependency handling, carry-forward, and summary
  finalization belong to rotari's shared execution path.
