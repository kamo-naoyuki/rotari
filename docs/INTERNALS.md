# Rotari internals

This is a compact design map for maintainers and coding agents. User-facing
behavior belongs in `README.md`; local implementation details belong in code
and tests. Update this file only when a cross-cutting contract changes, and
replace obsolete rules rather than accumulating history.

## Technology rationale

- The core implementation uses Go because Rotari is primarily a command-line
    and background-server tool that coordinates OS processes, files, locks,
    signals, Unix sockets, and external schedulers.
- A statically linked Go binary keeps installation and deployment simple on
    login nodes, worker nodes, and shared HPC environments. The core does not
    require a language runtime, daemon framework, or database service at runtime.
- Go's standard library provides the required filesystem, process, signal,
    networking, JSON, and concurrency primitives directly. This keeps the
    file-backed state model explicit and makes the local and server execution
    paths share the same implementation.
- Goroutines and channels fit the execution model: multiple jobs may run
    concurrently, while locks and a single server coordinate access to each
    project.
- Python is intentionally limited to the optional client interface. It wraps
    the installed CLI rather than reimplementing queue, persistence, or
    execution semantics, so there is one authoritative core implementation.
- This choice does not make Go a requirement for job commands or scheduler
    integrations. Jobs may use any executable, and executor-specific behavior
    remains behind the `JobExecutor` boundary.

## System model

- Rotari is file-backed.
- A base directory contains projects. Each project owns one mutable queue and
    a run history.
- A run snapshots the queue and stores execution state, results, and logs by
    stable job ID.
- The CLI, server, executors, and web UI project the same persisted state
    model.

The normal state layout is:

```text
<basedir>/
├── server.log
└── projects/<project>/
    ├── queue.json
    ├── meta.json
    ├── state.lock
    ├── running.lock
    └── runs/<run-id>/
        ├── commands.json
        ├── context.json
        ├── summary.json
        └── <job-id>/
            ├── command.json
            ├── output
            └── executor-specific state
```

- Files may appear incrementally while a run is active.
- Readers must tolerate missing optional or not-yet-written run files without
    inventing completed results.
- The background server writes lifecycle, request, and error events to
    `<basedir>/server.log`.
- The server log is bounded: before an event would make it exceed 1 MiB, the
    regular file is truncated and the new event is written.
- Run output remains the durable execution record.
- When `webhook.url` is configured, or `ROTARI_WEBHOOK_URL` is set, finalized
    runs send one `POST` summary to that endpoint. The default format is the
    generic rotari JSON payload; `webhook.format: slack` or
    `ROTARI_WEBHOOK_FORMAT=slack` selects a Slack Incoming Webhook payload.
    `webhook.on` and `ROTARI_WEBHOOK_ON` filter success/failure events;
    environment variables override config files. Delivery errors are warnings
    and do not change run status. A successful delivery is marked by
    `webhook.sent` inside the run directory. New formats must be implemented as
    webhook encoders without changing the generic payload contract.
- `diagnose` is an explicitly invoked, stateless external integration. It sends
    one job's command, recorded result, and at most the last 12,000 characters
    of output to the configured OpenAI Responses API-compatible endpoint. API
    keys and diagnoses are never persisted or injected into job environments.
    A valid explicit BCP 47 response-language tag is added to the prompt; when
    absent, Rotari makes no language selection.

## Core design contracts

The following rules govern the current CLI, server, web, and executor design.
An intentional change to one is an architectural change: update this document,
the user-facing documentation, and the affected tests together.

- A completed run is historical and immutable. Its persisted snapshot,
    results, and logs are not rewritten; it remains available until explicitly
    deleted.
- Every retry creates a new run. It may use a prior run as its reference, but
    never modifies that source run.
- Carry-forward writes reused results only into the destination run and never
    changes the source run.
- Carried-forward jobs retain an origin that identifies the source run and
    job (per task for arrays), so their output remains traceable across runs.
- The filesystem is the source of truth. Registries and in-memory state are
    indexes or coordination aids and must be recoverable from persisted files.
- The server coordinates access and execution; it is not persistent
    authority for project or run state.
- Each project owns one mutable current queue as the staging area for the
    next run. Queue edits change that queue; starting a run snapshots it, and
    normal completion clears the consumed queue. An interrupted run retains
    the queue until it is recovered or reset.
- A project has at most one active run and runner at a time. That runner may
    execute multiple jobs concurrently, while different projects can run
    independently.
- Executors implement job execution and scheduler integration, not run
    semantics. Run planning, dependency handling, carry-forward, and summary
    finalization belong to rotari's shared execution path.
- Run dispatch has a local concurrency lane and one independent lane per
    non-local executor. `local-concurrency` and `batch-concurrency` are common
    defaults used when no executor-specific setting is supplied; executor
    settings override those defaults, and job-specific executor options remain
    highest priority.

## Resolution rules

Without a run-location lookup, base directories resolve in this order:

1. `--basedir`
2. `ROTARI_BASEDIR`
3. `./.rotari-state` when present
4. `$XDG_STATE_HOME/rotari`
5. `~/.local/state/rotari`

- Projects resolve from `--project-name`, then
    `ROTARI_PROJECT_NAME`, then the only project in the resolved base directory.
    With no projects the name is `default`; multiple projects require an
    explicit choice. The bare `show` command is an exception: it warns and
    falls back to listing all projects.
- `show --projects` lists all projects in the resolved base directory and does
    not resolve one project name.
- `show --basedirs` lists state directories known to the run and live-server
    registries under the resolved master directory; this discovery is not
    exhaustive.
- Project names and job IDs are single path elements, never
    relative or absolute paths.
- Empty values, `.`, `..`, absolute paths, and values containing
    `/` or `\` are rejected before filesystem access. This applies to
    `resolvePaths` and `cancelJobs`, including requests from remote callers.
- Persisted timestamps use UTC RFC3339. Human-readable CLI and web
    views use the IANA timezone from `TZ` when valid, otherwise Go's local
    timezone.
- A supplied `--run-id` is exact, except that the reserved value `latest`
    selects the latest saved run using the normal metadata/newest-directory
    fallback.
    Existing-run commands use the master registry for its base directory and
    project.
- Explicit location options take priority, but conflicts with the
    registry fail. An unregistered run uses normal resolution for compatibility,
    while a missing explicit run is an error with no latest fallback.
- Without `--run-id`, history consumers use `meta.json`
    `last_run_id`, then the newest run directory where supported. `show` may
    prefer an active run, an interrupted run, or a non-empty idle queue before
    history.
- `wait` without a selector scans the resolved basedir's projects and waits
    when exactly one active `running.lock` exists; multiple active projects
    are listed for explicit selection, and no active project is an error. A
    positional selector is resolved in this order: project name, active run
    name, then run ID. An explicit `--run-id` bypasses this selector
    resolution.
- Run lookup applies to history commands (`show`, `wait`, `copy`,
    `change`, `remove`, `delete`, and rerun selection), not state-creating
    commands such as `add` or a plain new `run`.
- Multiple run IDs passed to `wait` are resolved independently, so
    one command may wait for runs from different projects or base directories.
- Shell completion follows the same location rules with narrower
    candidates: `project-name` lists project directories, `run-id` lists saved
    runs, and `job-id` lists queue and saved-run job IDs according to the
    selected run. Completion generation is implemented for Bash, Zsh, and
    Fish, and `rotari completion install` writes the appropriate shell-specific
    script for the detected or requested shell.
- Missing state directories produce no completion candidates
    instead of a shell error.

### Configuration files

- Configuration loading does not independently resolve a project or replace
    command resolution. The command owns normal `basedir` and project
    resolution; configuration only consumes the locations that are already
    explicit or known.
- When `--run-id` identifies a registered run, its registry entry supplies
    `base_dir` and `project_name` for configuration loading as well as for the
    command. When the project is still ambiguous, only global and basedir
    config are loaded; project selection remains the command's responsibility.
- CLI option defaults are loaded from one of `config.yaml`, `config.toml`, or
    `config.json` in `$XDG_CONFIG_HOME/rotari` (or `~/.config/rotari`), then
    the resolved base directory.
- After resolving the project name, the same lookup is performed in
    `projects/<project>/`. Project values override base-directory values.
- Later scopes override earlier scopes: home, basedir, then project.
- Multiple supported config files in the same directory are an error; file
    formats have no implicit priority.
- Common configuration keys (`basedir` and `project-name`) are at the root;
    command-specific keys are nested under their command name. Explicit CLI
    values take priority over environment defaults, which take priority over
    command sections and root config values.
- `rotari config` generates a template from the union of all CLI metadata
    options. YAML and JSON use `null` for unset values; TOML uses comments
    because it has no null value. Null values are ignored during resolution.
- Without `--output`, `rotari config` offers home, basedir, existing project
    config paths, stdout, and an arbitrary path interactively; an explicit
    `--output` is non-interactive.
- `rotari show` prints the resolved config path chain in its header so the
    active home, basedir, and project config files are visible in CLI output as
    well as in the web UI.
- The Web UI exposes config paths in its state and serves raw contents only for
    the resolved global/project files or config paths recorded in a run's
    `context.json`; it does not accept arbitrary filesystem paths.

### Shell completion

- Completion is generated from the same CLI metadata as command help.
- CLI environment defaults are declared in one flag-to-variable mapping, used
    for both default values and command-help descriptions.
- It covers subcommands, options, executor values, run-selection values, and
    `server` subcommands.
- Installation appends a marked rotari block only when it is absent, making
    repeated installs idempotent.
- Dynamic candidates follow the resolution rules above:
  - `project-name` lists project directories.
  - `run-id` lists saved runs.
  - `job-id` lists the current queue and saved runs, or only the selected run
    when `--run-id` is present. Run IDs found through the master registry are
    included in this selected-run lookup.

### Run registry

Run IDs contain a UTC timestamp and random suffix. They are collision-resistant
but do not encode a location. The master directory stores one index record per
run:

```text
<masterdir>/runs/<run-id>.json -> { base_dir, project_name, run_id }
```

The master directory resolves, and is also used for server discovery, in this
order:

1. `ROTARI_MASTERDIR`
2. `$XDG_STATE_HOME/rotari/master`
3. `~/.local/state/rotari/master`

- Register a run before exposing location-independent commands.
- Re-registering the same mapping is idempotent; mapping an existing ID to a
    different location fails.
- The registry is only an index. Run files remain authoritative.
- Deleting a run through CLI or web history controls removes its registry entry
    after the run files and metadata are updated.
- Runs deleted outside rotari can leave orphaned registry entries. `rotari gc`
    caches their plan for ten minutes; `rotari gc --apply` removes only
    unchanged entries whose run directories are still absent.
- Automatic garbage collection is not performed. Malformed or invalid registry
    files are reported and left untouched for manual inspection.

## Run lifecycle

- Queue-editing commands mutate `queue.json`. Starting a run assigns a new
    ID, snapshots the queue, records context, and marks it active. Completion
    writes results and summary, updates metadata, clears the consumed queue, and
    removes the active lock. Saved runs remain until explicitly deleted.
- Retries and filtered runs create new history. `--retry N` retries a failed
    job up to N additional times within the same run. A failed job is one whose
    result has a non-zero exit code and is not explicitly cancelled.
- An explicit cancellation is terminal for the current run. A job marked
    cancelled, or whose recorded execution state is `cancelled`, is not
    automatically retried by that run's `--retry` loop, even if its exit code is
    non-zero.
- A later explicit `rotari retry` may select a cancelled job through the
    normal `failed`/`unfinished` result filters. This is a new run, so it is a
    new user decision to execute the job again.
- In a filtered run, selected jobs execute. Completed jobs outside the
    selection carry forward their result and an origin pointing to the original
    output; jobs without a completed result remain unfinished.
- Dependencies use unique job names within a queue. Unknown names,
    duplicates, and cycles are rejected before execution. `add` also rejects a
    duplicate job name immediately, without writing the queue, so that mistake
    is never deferred to execution time. A `--depends-on` name may still refer
    to a job added later in the same queue, so unknown-name and cycle checks
    remain deferred to the execution boundary.
- An array queue command has an inclusive `first-last` range or an explicit
    comma-separated task list. Runtime expansion creates one `JobSpec` and
    persisted job directory per selected task. Local executors run those tasks
    as independent processes. Slurm, PBS, and LSF may submit a complete
    contiguous range as one native array; sparse selections fall back to
    independent submissions so scheduler support for sparse native arrays is
    not required.
- Result-based selection (`--failed`/`--unfinished`/`--success` in `copy`,
    and in rerun when `--partial-array=false`) and copied-job origin status
    operate on the unexpanded `QueuedCommand`, but results are recorded per
    expanded task ID. Matching an array command therefore aggregates its task
    results (`aggregatedJobResult` in `run_selection.go`): it is "finished" only
    once every task has a result, and any non-zero task exit code marks it failed
    as a whole.
- `run`/`retry` default to `--partial-array=true`. For a filtered rerun,
    `planRerunSelection` evaluates each array task's own result against the
    selection (`planArrayTaskSelection`) instead of the aggregate, so only the
    matching tasks (for example, the failed ones) re-execute while the rest
    carry their own result forward into the new run's summary. Each carried
    task's `Origin` is recorded in `QueuedCommand.TaskOrigins`, keyed by task ID
    such as `id-1`, separately from the whole-command `Origin` field. This is
    needed because one array command can have some tasks freshly executed and
    others carried in the same run. `loadRunOrigin`/`show`/`web` check both
    `Origin` and `TaskOrigins` when resolving where a job's output lives.
    `--partial-array=false` restores the older whole-array behavior: any match
    re-executes every task, using only the whole-command `Origin`.
- `--depends-on` ordering is resolved entirely by rotari itself, wave by
     wave, inside `executeMixedRun`; it never relies on scheduler-native
     dependency features such as Slurm's `--dependency`. This keeps dependency
    semantics identical across every executor, including mixes of local and
    remote ones in the same run. Jobs in a ready wave may run concurrently. A
    failed prerequisite prevents its dependents from executing; each is
    persisted with a non-zero result and `blocked by failed dependency` error.

## Run orchestration and executor responsibilities

- `executeMixedRun` (`mixed_run.go`) is the single execution engine for every
    run, regardless of executor mix.
- Both the synchronous path (`runServerSync`) and async worker path
    (`cmdWorkerRun`) drive it. It expands array plans, dispatches to executors,
    and writes the run summary. `add --run` uses the synchronous path after
    enqueueing; `add --run-async` uses the async worker path after enqueueing.
- Per-executor full-run orchestrators must not be added outside this path.
    Extend `JobExecutor` methods or `executeMixedRun` instead.
- The former Slurm-only orchestration path was removed because it duplicated
    this responsibility and became dead after `executeMixedRun` replaced it.
- `JobExecutor` is the scheduler boundary. Implementations share lifecycle
    and result semantics where supported; scheduler metadata belongs in the
    job's run directory.
- Polling executors persist normalized scheduler state in
    `scheduler_status.json`. Read projections use it without querying
    schedulers directly.
- Scheduler display names may offer inspection commands, but must not be the
    only way to locate state. `controlQueueJobs` also writes
    `scheduler_status.json` immediately after successful suspend/resume calls,
    including for executors whose `Wait` loop does not poll that file.
- Task wrappers normalize scheduler-specific task indexes into
    `ROTARI_ARRAY_TASK_ID` and related `ROTARI_ARRAY_*` variables. Job wrappers
    expose stable run, project, job, directory, working-directory, and
    executable-path variables prefixed with `ROTARI_`.
- `QueuedCommand.Environment` stores user-supplied `KEY=VALUE` entries from
    `--env`. Values are passed to every executor and copied into run snapshots.
    Generated `ROTARI_*` variables override user values, and invalid names are
    rejected at every queue mutation boundary.
- `QueuedCommand.WorkingDirectory` is copied into each expanded `JobSpec` and
    applied before commands start by local, SSH, Slurm, PBS, and LSF executors.
    An SSH path is resolved on the remote host, not from local `context.json`.
- The SSH executor treats its first executor option as the target host and
    the rest as `ssh` options. It records output and final status locally and
    does not require the remote host to mount the run directory. Its native ID
    is the local SSH process ID, so control is possible only while the
    supervising rotari process remains alive.

## Server and read projections

- The server supervises one base directory and may stop when idle, so durable
    behavior belongs in files, not memory.
- The web UI is a projection of the same model, not a separate database.
- The optional Python interface invokes the installed CLI with `subprocess`
    and must not implement queue or execution semantics itself.
- `wait --json` emits one `RunSummary` object per requested run, using NDJSON
    when multiple IDs are supplied.
- `show --json` emits one object with the resolved location, available run
    summary, and saved commands. JSON modes are additive; default CLI output
    remains human-facing.

## Job execution durability

- Every executor runs the command through a self-reporting wrapper that
    writes `<job-id>/status.json` with phase, exit code, and hosts.
- The wrapper records status independently of the process that launched it,
    so scheduler accounting lag cannot hide the result.
- The local executor uses the same wrapper. If the coordinating server or
    async worker is killed, the orphaned local job can finish and record its
    own status instead of leaving no result.
- Detached supervisors are not automatically restarted. Crash detection is
    file-backed: the run lock records the supervisor PID and host, and readers
    inspect per-job status files and missing summaries to report an active or
    interrupted run. Recovery remains an explicit operator action.
- The existing `show`/`web.go` fallback chain (`status` -> `status.json` ->
    `summary.json`) consumes this state without reader changes.
- This does not kill or reconcile leftover jobs during recovery;
    `reset --recover` and `unlock` still require the operator to confirm that
    jobs have stopped.

## Shared-state coordination

- Shared-base operation relies on exclusive file creation, atomic rename, and
    advisory `flock` semantics from the shared filesystem.
- The state lock serializes queue mutations and `running.lock` prevents a
    second runner from starting the same project.
- This is coordination, not distributed locking: it cannot fence a host after
    a network partition or determine whether a remote PID is alive. A remote
    run lock remains active until an operator confirms the run stopped and uses
    `unlock`.
- Project locks are scoped to project directories, so different projects
    largely isolate queue and run state. Server, registry, and filesystem state
    remain common base-level dependencies.
- `controlQueueJobs` and `cancelJobs` signal local jobs by process group.
    The wrapper's PID is also its process-group ID via `Setpgid`, so a negative
    PID reaches both the wrapper and its command.
- `localExecutorHostMismatch` compares the current host with
    `context.json` before signaling. A cross-host local control request gets an
    explicit host error instead of a misleading "job is not running" or a PID
    reuse hazard.
- Scheduler executors and SSH are expected to work from any host with the
    required access. Their control CLIs must still be installed on the host
    issuing the request.
- `schedulerCommandHint` turns missing scheduler binaries into explicit
    errors and preserves scheduler stdout/stderr, including explanations for
    rejected operations on queued jobs.
- Whole-run cancel has the same PID locality issue. `runningWorkerHostMismatch`
    checks `running.lock`'s host before signaling; a cross-host request fails
    instead of reporting success while leaving the real runner untouched.
- Finalization rechecks that `running.lock` belongs to the finishing run while
     holding the state lock. New locks are written to a temporary file and
     published without replacing an existing lock, preventing partial JSON.
- State and registry trees use centralized permission helpers. The default
     modes are `0755`/`0644`; `ROTARI_PRIVATE_STATE=true` switches new paths to
     owner-only `0700`/`0600`/`0700`.
- Permission settings apply only to newly created paths. Existing paths are
     not rechmoded, so changing the setting can produce mixed permissions.
- `generateStaticWeb` is the intentional exception and always emits
     publishable `0755`/`0644` output.

## Web and control-plane security

- `web` binds `--host`/`--port`, defaulting to `127.0.0.1:8787`.
- A busy default port scans upward for a free port. An explicit port,
    including `--port 0`, never falls back and fails immediately if unavailable.
- The actually bound address is reported after listener creation. Non-loopback
    hosts produce a warning when no Web UI token is configured.
- `--auth-token` or `ROTARI_WEB_AUTH_TOKEN` wraps every Web route and accepts
    `Authorization: Bearer TOKEN`, `X-Rotari-Token: TOKEN`, or Basic
    authentication with username `rotari` and the token as the password; this
    is authentication only and does not encrypt HTTP traffic.
- `loadWebState` exposes persisted runtime metadata: `running.lock` fields and
    the presence of `server.sock`/`server.pid`. The panel does not query process
    liveness or infer that `state.lock` is held from the file's existence.
- The Unix-socket control surface is separate from `ROTARI_PRIVATE_STATE`:
    reaching it means controlling the server, not merely reading state.
- `runServer` always sets the socket to `0600`. On Linux,
    `verifyPeerCredential` rejects connections whose UID differs from the
    server process; on other platforms the socket mode is the enforcement.
- Web mutating routes are gated by `allowControl`, enabled by default and
    configurable with `--allow-control` or `ROTARI_WEB_ALLOW_CONTROL`.
    `--allow-control=false` rejects them with `403` before reading request
    bodies. Read-only `GET` routes remain available.
- Web state exposes only whether an environment variable is set. Raw values
    never cross the HTTP boundary; `Value` is populated only by the local
    `rotari env` CLI command.

## Client connection lifecycle

The server distinguishes client cancellation, detach, and worker completion as
follows:

- A synchronous client disconnect, including Ctrl-C, requests cancellation and
    returns immediately with exit code 130. The server-side run continues in the
    background and then finalizes normally.
- Ctrl-D sends an explicit detach before disconnecting. The run is left
    uncancelled and completion cleanup moves to the background waiter.
- Ctrl-Z sends no rotari protocol message. The terminal suspends the client
    while the server-side run continues; a later EOF follows the normal
    disconnect path and requests cancellation.
- The server monitors async workers. Worker exit decrements the active-run
    count, and interrupted-run recovery is reserved for failures that bypass
    finalization.
- Completed sync and async runs decrement the active-run count immediately
    through `beginRun`/`endRun`. When it reaches zero, the server stops without
    waiting for the idle timeout.

## CLI presentation

- CLI colors are semantic presentation, not machine-readable output. They are
    emitted only on TTY streams; redirected and piped output remains plain text.
- Red denotes errors and failed results; green denotes success; yellow denotes
    warnings, running/blocked state, retries, and recovery/cancellation; cyan
    denotes informational labels and suggested actions; white denotes values.
- Parsers and tests must rely on text, not ANSI sequences or color choice.
- `show` uses a lazy pager. `--no-pager` and non-TTY output go directly to
    stdout. On a TTY, output of at most 24 lines is direct; longer output uses
    `$PAGER`, defaulting to `less -R`. Pager failure falls back to stdout.

## Concurrency and safety

- Project mutations hold the advisory `state.lock`.
- `running.lock` represents an active run and includes host information,
   because local PID checks cannot prove remote process liveness.

`inspectProjectRunState` (`main.go`) derives one of three states from just
`running.lock` and `meta.json` -- never from job-level files like a job's own
self-reported `status.json` (see "Job execution durability" above), which only
feeds `show`/the web UI, not this state machine:

| State                 | `running.lock`                                   | `meta.json` phase              | `run`/`add`/`copy`/`change`/`delete`/`remove` | `reset`                                  |
|------------------------|---------------------------------------------------|---------------------------------|------------------------------------------|--------------------------------------------|
| `projectIdle`          | absent, or present but stale (auto-removed)        | `collecting`/`finished`         | allowed                                  | allowed                                    |
| `projectRunning`       | present; owning coordinator PID is alive (a lock recorded on another host is always treated as alive, since liveness can't be checked remotely) | `running`/`cancelling`           | rejected: "is running; ... is not allowed" | rejected: same message                     |
| `projectInterrupted`   | absent, or present but the coordinator PID is dead (auto-removed on the same host) | `running`/`cancelling` with `last_run_id` set | rejected: "has interrupted run ...; recover with unlock" | `--recover` (or interactive confirmation) proceeds |

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
- `unlock` with the exact run ID acknowledges recovery, keeps the retained
    queue, and returns the phase to `collecting`.
- `reset` discards the current queue while keeping defaults and history. It
    confirms that jobs stopped before recovering an interrupted run, unless
    `reset --recover` supplies that confirmation. It rejects an active run.
- Server management is separate (`server status`, `server shutdown`); project
    commands do not stop or query the server as a side effect.
- Never silently remove a possibly active remote lock. Destructive commands
    reject ambiguous targets, and exact IDs never degrade into latest-item
    selection.
- JSON writes use the common atomic helper. Optional fields must retain
    backward-compatible reads, and unrelated history must not be rewritten.

## Code map

- Core state, paths, resolution, and locks: `main.go`, `run_registry.go`.
- CLI contracts and orchestration: `cli_spec.go`, command files, `server.go`.
- CLI option metadata, including short aliases and CLI-default environment
    variables, is defined centrally in `cli_spec.go`; help, parsing, and shell
    completion must consume the same definitions.
- Rerun semantics: `run_selection.go`.
- Execution engine (single entry point for every executor mix): `mixed_run.go`.
- Execution boundary: `job_executor.go`, `executor_*.go`.
- Read projections: `show.go`, `web.go`.

When behavior crosses these boundaries, add a focused test at the public
command or persisted-state boundary. Keep CLI metadata, completion, README
usage, and this document synchronized only where their contracts actually
change.
