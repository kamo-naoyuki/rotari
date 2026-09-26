# Code architecture

This is a map of the code: which process does what, which package owns which
responsibility, and which functions a command passes through. Read it before
changing code you have not touched before.

It describes structure, not rules. The behavior that must be preserved (state
layout, fallback chains, locking, path rules) is in
[CONTRACTS.md](CONTRACTS.md). User-facing behavior is in the
[README](../README.md) and the guides it links to.

## Processes

Rotari is one binary, but at runtime it plays several roles. Knowing which
process a piece of code runs in explains most of the structure.

```mermaid
flowchart LR
  subgraph clients["user shell"]
    runcli(["rotari run / retry<br/>cancel / suspend / resume"])
    direct(["rotari add / copy / change<br/>show / jobs / export ..."])
    webcli(["rotari web<br/>(HTTP)"])
  end
  subgraph sup["one per base directory"]
    supervisor["supervisor<br/>rotari __server<br/>sync run: executes jobs here"]
  end
  worker["rotari __worker-run<br/>(async run, detached)"]
  files[("&lt;basedir&gt;/projects/&lt;project&gt;/<br/>queue.json, meta.json, runs/...")]
  nodes["wrapper scripts on<br/>Slurm / PBS / LSF / SSH nodes"]

  runcli -->|"JSON request over Unix socket"| supervisor
  supervisor -->|progress stream| runcli
  supervisor -->|spawns| worker
  supervisor -->|read / write| files
  worker -->|read / write| files
  direct -->|"read / write under state lock"| files
  webcli -->|"read, edit queue"| files
  nodes -->|write attempt status.json| files

  classDef command fill:#1d4ed8,stroke:#1e3a8a,color:#ffffff
  class runcli,direct,webcli command
```

- **CLI client.** Every `rotari <command>` invocation. Most commands (`add`,
  `copy`, `change`, `show`, `jobs`, `export`, ...) read and write the project
  files directly under the project's state lock. Only `run`, `retry`,
  `cancel`, `suspend`, and `resume` go through the supervisor.
- **Supervisor.** `rotari __server`, started on demand by `ensureServer`
  ([cmd/rotari/server.go](../cmd/rotari/server.go)) and stopped after one
  idle minute. The source still calls it the *server*. It serializes run
  starts and job control for one base directory. It is not an authority for
  state: everything it knows is also on disk.
- **Run execution.** A run executes either inside the supervisor or in a
  separate worker process; see [Sync and async runs](#sync-and-async-runs).
- **Web server.** `rotari web` ([cmd/rotari/web.go](../cmd/rotari/web.go)).
  It reads project files to build JSON for the browser UI, and its control
  endpoints (`/api/copy`, `/api/cancel-job`, ...) call the same `cmd/rotari`
  functions the CLI uses.
- **Jobs and wrappers.** Local jobs are child processes of the run worker.
  Scheduler jobs run elsewhere through a generated wrapper script that writes
  the attempt's `status.json`, which the run worker polls.

### Sync and async runs

`rotari run --async` is the user-facing mode; the *worker* (`rotari
__worker-run`) is the process that executes an async run. Both modes run the
same lifecycle, `projectrun.Runner.Run`; they differ only in which process
calls it.

| | Sync run (`rotari run`) | Async run (`rotari run --async`) |
| --- | --- | --- |
| Client | Stays attached and streams progress | Returns after "Run started" |
| Process that executes the jobs | The supervisor | A worker the supervisor spawns |
| PID in `running.lock` | The supervisor | The worker (rewritten after spawning) |
| Entry point | `runServerSync` ([run_command.go](../cmd/rotari/run_command.go)) | `startServerRun` → `launchAsyncRun` → `cmdWorkerRun` ([main.go](../cmd/rotari/main.go)) |

```text
sync:   client ──▶ supervisor: Begin → Run (Execute → Finish) ──▶ result to client
async:  client ──▶ supervisor: Begin → spawn worker → return at once
                                         └─ worker: Run (Execute → Finish)
```

The worker is started with `setsid`, detached from the supervisor, so an
async run keeps going when the supervisor shuts down or dies: with no client
attached, the run is not tied to the supervisor's lifetime.

Because the filesystem is the shared medium, any process can die and the next
one can reconstruct what happened from the files. That is why so much code
takes a `state.ProjectPaths` and not an in-memory object.

## Packages

Dependencies point one way: `cmd/rotari` imports every `internal` package, and
no `internal` package imports `cmd/rotari`.

```mermaid
flowchart TB
  cmd["cmd/rotari<br/>CLI flags, supervisor operations,<br/>Web handlers, wiring, output"]
  projectrun["projectrun<br/>run lifecycle"]
  project["project<br/>state machine, idle edits"]
  resolve["resolve<br/>selectors to run and job"]
  queueops["queueops<br/>queue and history edits"]
  report["report<br/>diagnosis evidence"]
  joblist["joblist<br/>recent jobs"]
  subgraph l2["orchestration and projections"]
    server
    run
    jobcontrol
    web
    jobstatus
    workflow
    queueedit
    rundiff
    diagnose
  end
  subgraph l1["adapters"]
    executor
    state
    runregistry
  end
  model["model<br/>(imports nothing from rotari)"]

  cmd --> projectrun
  cmd --> project
  cmd --> resolve
  cmd --> queueops
  queueops --> project
  queueops --> resolve
  queueops --> queueedit
  queueops --> executor
  queueops --> state
  cmd --> report
  report --> web
  report --> project
  report --> diagnose
  cmd --> joblist
  joblist --> jobstatus
  joblist --> project
  resolve --> project
  resolve --> runregistry
  resolve --> state
  project --> jobstatus
  project --> executor
  project --> state
  cmd --> l2
  projectrun --> run
  projectrun --> executor
  projectrun --> state
  server --> executor
  run --> executor
  jobcontrol --> executor
  jobcontrol --> state
  web --> jobstatus
  web --> diagnose
  web --> state
  jobstatus --> executor
  jobstatus --> state
  workflow --> state
  executor --> state
  runregistry --> state
  l1 --> model
  l2 --> model
```

Arrows mean "imports". `cmd/rotari` imports every `internal` package, and
every package imports `model`; those edges are drawn once per group.

`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/...` prints
the current graph if this list drifts.

| Package | Owns | Start reading at |
| --- | --- | --- |
| [internal/model](../internal/model/) | Domain types and pure rules: queue, queued command, job spec, result, summary, selections, command selectors, dependencies, arrays. No I/O. | `model.go`, `selection.go`, `command_selector.go`, `dependencies.go` |
| [internal/state](../internal/state/) | The filesystem: path resolution and validation, JSON load/write, locks, run and attempt directory listing. No execution policy. | `paths.go`, `project_paths.go`, `store.go`, `lock.go` |
| [internal/executor](../internal/executor/) | How one job attempt is started, waited for, cancelled, and suspended: local processes, Slurm, PBS, LSF, SSH, wrapper scripts. No run semantics. | `contracts.go` (`JobExecutor`), `local.go`, `slurm.go` |
| [internal/project](../internal/project/) | A project's run state (idle, running, interrupted) from `running.lock` and `meta.json`, consistency checks, recovery, and the idle-edit sequence: state lock, idle check, load, edit, metadata-then-queue write. | `inspect.go` (`Inspect`, `EnsureIdle`), `edit.go` (`EditQueue`) |
| [internal/resolve](../internal/resolve/) | Location rules shared by the commands that read existing state: a run ID through the run registry, an `att_` attempt ID, the latest-run fallback, run names, and job IDs or names looked up in the queue and latest runs. show's and wait's own selector orders build on it. | `resolve.go` (`ExistingRun`, `RunID`, `Jobs`) |
| [internal/runregistry](../internal/runregistry/) | The master directory's run index, `<masterdir>/runs/<run-id>.json`: register, look up, unregister, and find stale entries for `gc`. | `registry.go` |
| [internal/projectrun](../internal/projectrun/) | One project's run against its files: `Begin` (context, run lock, registry, running metadata), `Execute` (snapshot, plan, dispatch, summary), and `Finish` (final context, queue and metadata finalization, lock removal). Shared by the sync run, the async worker, and cancellation. Also checks that a queue can run with the known executors (`ValidateQueue`). | `lifecycle.go`, `execute.go`, `validate.go` |
| [internal/run](../internal/run/) | Run rules without file access: which jobs execute or are carried forward, dependency unblocking, retries, per-executor lanes and concurrency, the summary contents. | `rerun.go` (`PlanRerun`), `engine.go` (`ExecuteJobs`), `dispatch.go` (`Dispatcher`) |
| [internal/jobstatus](../internal/jobstatus/) | Read side: turns attempt files and the summary into one displayed result and timestamps. Shared by CLI and Web. | `job.go`, `attempt.go`, `times.go` |
| [internal/server](../internal/server/) | Supervisor transport: request/response types, socket, lease, peer checks, idle shutdown, attached-run streaming. Work is delegated to an `Operations` interface. | `protocol.go`, `serve.go`, `client.go` |
| [internal/jobcontrol](../internal/jobcontrol/) | Cancel, suspend, resume of running jobs through executors. | `jobcontrol.go` |
| [internal/web](../internal/web/) | JSON projections of runs, jobs, attempts, and timelines for the Web UI. | `loader.go` |
| [internal/queueops](../internal/queueops/) | Queue and run-history edits shared by the CLI, the Web UI, and the supervisor: add, change, remove, copy, and deleting runs. Loads and saves the files around `internal/queueedit` through the `internal/project` idle-edit sequence, and owns `ValidateJobs`. | `editor.go` (`Editor`), `change.go`, `copy.go` |
| [internal/queueedit](../internal/queueedit/) | Pure queue edits, such as building a queue from an earlier run (`copy`, `retry`). | `copy.go` |
| [internal/workflow](../internal/workflow/) | Workflow manifests: `export` merge and `import` reconciliation. | `manifest.go`, `export.go`, `reconcile.go` |
| [internal/joblist](../internal/joblist/) | Recent job attempts across a base directory's projects for `rotari jobs` and the Web UI's jobs page: which attempts are listed, their order, and how their times read. | `joblist.go` (`Collect`) |
| [internal/report](../internal/report/) | The redacted evidence report for AI-assisted diagnosis, shared by `show --report` and the Web UI. Reads jobs through `internal/web`'s projection. | `report.go` (`Build`) |
| [internal/rundiff](../internal/rundiff/) | Comparison of two loaded runs for `diff` and `show --lineage`. | `rundiff.go` |
| [internal/diagnose](../internal/diagnose/) | Rule-based and provider-backed failure diagnosis. | `analysis.go` |

Many `internal` functions take callbacks or hook fields
(`projectrun.Runner`, `run.BatchLaneCallbacks`, `run.OriginResults`, `server.Operations`). This is
how `cmd/rotari` supplies file access and output without the internal package
importing it. When a callback's body is long, the logic probably belongs in an
internal package.

### Where does new code go?

- Can it be explained without paths, files, or locks, and does it only define
  or validate rotari concepts? `internal/model`.
- Does it read or write a file or directory layout? `internal/state`.
- Does it decide the next execution step of a run? `internal/run`.
- Does it start, record, or finish a project's run on disk? `internal/projectrun`.
- Does it decide whether a project may be edited, or save an idle edit?
  `internal/project`.
- Does it edit a project's queue or run history on disk for more than one
  interface (CLI, Web, supervisor)? `internal/queueops`, with the pure queue
  transformation in `internal/queueedit`.
- Does it talk to a process or scheduler? `internal/executor`.
- Does it decide what status a job shows? `internal/jobstatus`.
- Does it decide which base directory, project, run, or job a selector
  names? `internal/resolve`.
- Is it flag parsing, message wording, or colors? `cmd/rotari`.

## Files in cmd/rotari

`cmd/rotari` is the largest package. Each command has a `cmdXxx` function,
dispatched from `run` in [main.go](../cmd/rotari/main.go).

| Role | Files |
| --- | --- |
| Entry, dispatch, async worker launch, `__worker-run` | `main.go` |
| Flag metadata, help, config defaults, completion | `cli_spec.go`, `config.go`, `completion.go`, `schema.go`, `guide.go`, `environment.go` |
| Queue editing (flags and output; `add`, `change`, `copy`, `remove`, and `delete` call `internal/queueops`) | `add.go`, `change.go`, `copy.go`, `remove.go`, `reset.go`, `delete.go`, `gc.go`, `unlock.go` |
| Starting a run (client and supervisor side) | `run_command.go`, `job_executor.go` |
| Default run registry wiring (`registerRun`, `resolveRunLocation`) | `run_registry.go` |
| Wiring the run lifecycle (`projectRunner`) | `project_run.go` |
| Supervisor, its server registry, and `show --basedirs` discovery | `server.go`, `registry.go` |
| Job control | `job_control.go`, `wait.go` |
| Reading results | `show.go`, `jobs.go`, `diff.go`, `diagnose.go`, `check.go` |
| Workflow manifests | `export.go`, `import.go`, `workflow_source.go` |
| Web server and assets | `web.go`, `web_assets.go`, `assets/` |
| Notifications and terminal output | `webhook.go`, `color.go`, `terminal*.go` |

## Walkthroughs

### `rotari add`

1. `cmdAdd` ([add.go](../cmd/rotari/add.go)) parses flags into
   `model.QueuedCommand` values.
2. `queueops.Editor.Add` ([internal/queueops/add.go](../internal/queueops/add.go))
   resolves `state.ProjectPaths` and calls `project.EditQueue` ([internal/project/edit.go](../internal/project/edit.go)),
   which takes the state lock, checks the project is idle, loads
   `queue.json`, runs the callback that appends the new commands, and writes
   `meta.json` and then `queue.json`.

`change`, `copy`, and `remove` follow the same path through their
`queueops.Editor` methods, and `import` through its own callback; `delete`
uses `project.Edit` for the lock and idle check alone. The Web UI and the
supervisor call the same `queueops.Editor` methods.

No supervisor is involved.

### `rotari run`

Client side, in [run_command.go](../cmd/rotari/run_command.go):

1. `cmdRun` resolves the target project and, for `--run-id`, `--failed`, or
   `--job-id`, first repopulates the queue from an earlier run
   (`queueops.Editor.Copy`, backed by `internal/queueedit`).
2. `ensureServer` starts the supervisor if needed.
3. It sends a `server.Request{Op: OpRun}`. Synchronous runs use
   `sendRunRequest`, which streams progress and handles Ctrl-C (cancel) and
   Ctrl-D (detach).

Supervisor side:

1. `server.Server.Handle` ([internal/server/serve.go](../internal/server/serve.go))
   dispatches `OpRun` to `serverOperations.Run` (sync) or `StartRun` (async)
   in [server.go](../cmd/rotari/server.go).
2. `runServerSync` / `startServerRun` share `prepareServerRun`: take the
   state lock, check the project is idle, and validate the queue. Then
   `projectrun.Runner.Begin`
   ([internal/projectrun/lifecycle.go](../internal/projectrun/lifecycle.go))
   writes `context.json`, takes the run lock (`running.lock`), registers the
   run, and marks `meta.json` as running.
3. A synchronous run continues with `Runner.Run` in the supervisor. An async
   run goes through `launchAsyncRun` ([main.go](../cmd/rotari/main.go)), which
   spawns `__worker-run`, hands it the run lock, and returns; `cmdWorkerRun`
   calls `Runner.Run` in the worker.

Execution, in `Runner.Execute`
([internal/projectrun/execute.go](../internal/projectrun/execute.go)):

1. Load the queue, convert it to `model.JobSpec`s, and validate IDs and
   dependencies.
2. `Runner.PlanSelection` → `run.PlanRerun`: decide which jobs execute and
   which results are carried from an earlier run.
3. Write `runs/<run-id>/commands.json`.
4. `run.NewDispatcher` builds one lane per executor with its own concurrency.
   `run.ExecuteJobs` ([internal/run/engine.go](../internal/run/engine.go)) is
   the loop: start jobs whose dependencies are satisfied, collect results,
   retry, and stop if the project is being cancelled.
5. Each lane calls the executor's `Submit` and `Wait`
   ([internal/executor/contracts.go](../internal/executor/contracts.go)).
   The executor writes attempt files under
   `runs/<run-id>/<job-id>/attempts/<attempt-id>/`.
6. `run.BuildRunSummary` builds the summary, with diagnoses attached, and it
   is written to `summary.json`.

Finish, in `Runner.Finish`:

1. Record the final load in `context.json`.
2. `Finalize` clears the consumed queue and finalizes `meta.json`
   (`state.FinalizeRun`) after checking that the run lock is still this run's,
   then calls the webhook hook.
3. Remove the run lock.

### `rotari show` and the Web UI

1. `cmdShow` ([show.go](../cmd/rotari/show.go)) or a Web handler resolves the
   run and job directories through `internal/state`.
2. Each job's outcome comes from `internal/jobstatus` (`ReadJob`,
   `ResolveAttempt`, `Timestamps`), which falls back from the attempt's
   `status` file to its wrapper `status.json` to `summary.json`.
3. The CLI formats text; `internal/web` ([loader.go](../internal/web/loader.go))
   builds JSON for the browser.

Never read status files directly in a renderer; go through `jobstatus` so the
CLI and Web UI agree.

### `rotari cancel`

1. `cmdCancel` ([job_control.go](../cmd/rotari/job_control.go)) resolves the
   project, the run a run or attempt ID names, and the job IDs with
   `resolve.JobSelection`, and sends `OpCancel` to the supervisor.
2. The supervisor's `serverOperations.Cancel` calls `jobcontrol.Controller.Cancel`
   ([internal/jobcontrol/jobcontrol.go](../internal/jobcontrol/jobcontrol.go)),
   which finds the running run through its run lock, rejects a named run that
   is not the active one, expands array job IDs to tasks, marks `meta.json` as
   `cancelling` for a whole-run cancel, and calls the executor's `Cancel` for
   each running job.
3. The run loop sees the cancelled results and the `cancelling` phase, and
   stops starting or retrying jobs.

## Naming pitfalls

- **Project vs. queue.** A project owns one queue, and much of the code still
  uses `queue` or `QueueName` for the project name
  (`server.Request.QueueName`, `queueName` variables). They mean the project.
- **Server vs. supervisor.** The background coordinating process is called the
  supervisor in documentation, and `server` in code (`internal/server`,
  `rotari server`). The HTTP process is the Web server.
- **Import aliases.** `cmd/rotari` imports `internal/run` as `runcontract` and
  `internal/server` as `serverinternal`, because `run` and `server` are
  already function names in `package main`.
- **Job vs. attempt.** A job has a stable ID within a run; each execution of it
  (including retries) is an attempt with its own directory and an `att_` ID.
