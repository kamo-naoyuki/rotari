# Rotari FAQ

Rotari is a lightweight workflow runner for repeatedly executing shell commands while managing dependencies, parallelism, logs, and run history.

This FAQ gives short answers about rotari's behavior. See the [README](https://github.com/kamo-naoyuki/rotari#documentation) and the user guides it links to for usage. Detailed contracts are in the [contracts documentation](https://github.com/kamo-naoyuki/rotari/tree/main/contracts).

## Quick navigation

- [Why is it called rotari?](#why-is-it-called-rotari)
- [Projects, queues, runs, and registry](#projects-queues-runs-and-registry)
- [Language and implementation choices](#language-and-implementation-choices)
- [Retries, copying, arrays, and dependencies](#retries-copying-arrays-and-dependencies)
- [LLM diagnosis](#llm-diagnosis)
- [Interrupted runs and locking](#interrupted-runs-and-locking)
- [Client control and job cancellation](#client-control-and-job-cancellation)
- [Python interface](#python-interface)
- [Web UI](#web-ui)
- [Background server (supervisor)](#background-server-supervisor)
- [Timestamps and environment](#timestamps-and-environment)

## Why is it called rotari?

It is short, easy to type, and suggests repeatable workflow execution.

## Projects, queues, runs, and registry

### Do I need to install a database server?

No. State, history, and locks are stored in the filesystem. Shared multi-host use requires a filesystem that correctly supports locking and atomic operations.

### Can an older or newer rotari read my saved runs?

A newer rotari reads runs and queues written by older versions. `queue.json`, a run's `commands.json`, and its `summary.json` record a `state_version`; if a file comes from a newer rotari with a format this binary does not know, commands stop with an error asking you to upgrade instead of misreading it. This matters when several hosts share a basedir with different rotari versions.

### Can rotari manage jobs for multiple users like Slurm?

No. Rotari has no accounts, permissions, quotas, or fair-share scheduling. Use a separate `--basedir` per user and do not share writable state or Web UI tokens between untrusted users. Slurm, PBS, LSF, or SGE remains responsible for scheduler-side identity and resource policy.

### Which workflow-orchestrator features does rotari provide?

Rotari provides dependencies, parallel execution, retries, logs, history, array jobs, local/SSH/Slurm/PBS/LSF/SGE execution, a Web UI, completion webhooks, and an optional declarative workflow manifest.

The manifest is a constrained representation of the existing queue, not a programming language: commands remain argument arrays. Rotari does not provide file freshness checks, artifact caching, scheduled or event triggers, data lineage, RBAC, or quotas. Use tools such as Snakemake, `make`, or Airflow when those features are needed.

### What's the difference between a project, a queue, and a run?

A project groups the current queue and run history. The queue (`queue.json`) contains waiting commands; a run is an immutable snapshot taken when execution starts.

### Why doesn't `add` start a job immediately?

Rotari treats the queue as a workflow being defined, not as a continuously
executing scheduler queue. `add` lets you assemble jobs and dependencies;
`run` starts that definition as one execution. A run groups the commands,
results, logs, and attempts from that execution, even when it contains only one
job. It also uses the directory and environment of the shell that calls `run`
unless a job specifies its own. In an idle project with an empty queue, you can
run a single job with
`rotari add -p demo -- ./build.sh && rotari run -p demo`, or use `run --async`
to start it in the background.

### Can I add another job to a project while its run is active?

No. A project has one active run at a time, and its queue is retained as that
run's snapshot until it finishes. Jobs prepared together can execute
concurrently within the same run; an independent job that must start before
the current run finishes needs another project. That keeps its run history
separate. Rotari does not provide a continuously accepting queue for jobs
added after a run starts.

### I didn't pass `--project-name` — which project does rotari use?

Selection uses `--project-name`, `ROTARI_PROJECT_NAME`, the only project in the resolved state directory, and then `default`. Multiple candidates require an explicit selection. A bare `rotari show` lists projects in registered basedirs.

### How do I list projects in a state directory?

Run `rotari show`. Use `--basedir` to select a state directory, `--masterdir DIR` to select the registry, and `rotari show -p PROJECT` to inspect a project.

### I don't know which basedir contains my jobs. How do I find it?

Run `rotari show --basedirs`, then inspect one with `rotari show --basedir DIR`. Use `--masterdir DIR` to select the registry.

### Where can I put option defaults?

Put `config.yaml`, `config.toml`, or `config.json` in `$XDG_CONFIG_HOME/rotari` (normally `~/.config/rotari`), the basedir, or `projects/<project>/`. Priority is project, basedir, then global; CLI options and environment variables override config files.

### How do I find every config file below a basedir?

Run `rotari config --list --basedir DIR`. It includes `notifications.toml` and groups global and basedir paths under `Common:`, then project paths under `Projects:` with each project name followed by indented paths. Add `--project-name NAME` to limit project-specific entries to that project. This inventory includes files that are not selected by normal priority resolution.

### How do I see every configurable option?

Run `rotari config`, or use `--output FILE` to save a template. The selected config is copied into each run directory so historical views retain the configuration used at that time.

### How can I notify another service when a run or job finishes?

Generate `notifications.toml` with `rotari config --notifications`, then set `webhook.url` there or `ROTARI_WEBHOOK_URL`. Use `job_failure`, `job_success`, `run_failure`, and `run_success` to choose the events; jobs are reported once, after their retries end. Slack, Teams, and Discord formats are supported; see [Notifications](NOTIFICATIONS.md#webhook-notifications).

### How are concurrency and executor options selected?

`--local-concurrency` applies to local jobs; `--batch-concurrency` is the default for other executors. SSH, Slurm, PBS, LSF, and SGE also have executor-specific settings, with job-level settings taking priority.

### Can I limit CPU or memory for a local job without Slurm?

On Linux hosts with a working systemd user manager, use `systemd-run` as the
local job's command. It places the command in a systemd scope with cgroup
resource limits; no separate rotari executor is needed:

```sh
rotari add -p demo -- systemd-run --user --scope \
	-p MemoryMax=4G -p CPUQuota=200% -- ./train.sh
rotari run -p demo
```

`CPUQuota=200%` allows up to two CPUs' worth of CPU time; it does not reserve
two CPUs. `--local-concurrency` still only limits the number of local jobs,
not their combined resource usage. This requires access to the systemd user
bus and permission to set the limits, which may be unavailable in containers
or on some hosts. Check cancellation and out-of-memory behavior on your host
before relying on it for long-running jobs. Unlike Slurm, this does not queue
jobs until the requested resources become available.

### `rotari show` displayed my queue, not the run I expected — why?

Without a project selector, `show` lists projects. Use `--project-name/-p` for a project, `--run-id` for a run, and `--run-id latest` for the latest run.

### Why does `show` prefer a run over the queue when a project is running or interrupted?

Because the queue is only the active work target while the project is `idle`.
When a run starts, rotari snapshots the queue so the run has an immutable
record of what it was supposed to execute. During `running` and `interrupted`
states, the current subject is the run itself, while the queue remains a
retained snapshot or recovery context.

In short: `idle` means queue-first, while `running` and `interrupted` mean
run-first. This is why `show` often resolves the active run before the queued
commands after a start or crash.

### How do I clean up run registry entries left by manual deletion?

Run `rotari gc --dry-run [MASTERDIR]` to review candidates, then `rotari gc [MASTERDIR]` to remove them. Reappeared or changed candidates are skipped.

## Language and implementation choices

### Why is rotari written in Go instead of Python?

Go handles many small CLI operations, process actions, and filesystem updates with low startup overhead and can be distributed as one binary.

### Why not C++?

C++ could work, but Go provides the preferred balance of iteration speed, maintainability, and straightforward process and filesystem handling.

### Why not Rust?

Rust is also a strong option, but Go is currently a better balance of safety, implementation speed, and maintenance for this project.

### Does rotari run on Windows?

No. Rotari supports Linux and macOS, and on Windows it runs inside [WSL](https://learn.microsoft.com/windows/wsl/). WSL2 also supports CUDA, so GPU experiments on a Windows machine can run there. Native Windows support is not planned: rotari relies on POSIX file locks, signals, sessions and process groups, and shell wrapper scripts, and Slurm, PBS, LSF, and SGE run on Linux.

## Retries, copying, arrays, and dependencies

### How do I add a command and run it asynchronously?

Add it with `rotari add ...`, then run `rotari run --async`.

### Do I have to run `add` and `run` separately for one command?

Usually, yes. For one command, chain them:

```sh
rotari add --project-name demo -- ./build.sh && rotari run --project-name demo
```

### Can rotari rerun jobs when input or output files change, like Snakemake?

No. Rotari does not inspect file timestamps, hashes, or contents. Use Snakemake or `make` for file-based freshness decisions.

### What exit status does `rotari run` return when a job fails?

Synchronous runs return `0` if all jobs succeed and `1` if any job fails. `--async` returns `0` once the run starts; use `rotari wait PROJECT` or `rotari wait --run-id RUN_ID` for the result. `--quiet` only suppresses output.

### Can an array run only selected task IDs?

Yes. Use `--array 1,3,4`; ranges such as `1-10` are also supported. Slurm
uses its native sparse array for selected task IDs; PBS, LSF, and SGE submit
those tasks independently.

### I ran `rotari retry` — which jobs actually rerun?

Failed and unfinished jobs rerun, while successful jobs carry their results forward. Add `--success` to rerun successful jobs too.

### How can I edit a previous queue before rerunning only failed jobs?

Run `rotari copy` to restore every job from the latest run, edit the queue with
`change` or `remove`, then run `rotari run --failed`. Use `copy --run-id ID` to
restore a specific run. The result filter belongs on `run`, so the restored
queue remains available for inspection and editing before execution. With an
empty queue, `run --failed` restores the latest run automatically.

For a reproducible, reviewable edit, export and import a workflow manifest:

```sh
rotari export -p sweep -r RUN_ID > experiment.yaml
rotari import -p sweep --dry-run experiment.yaml
rotari import -p sweep experiment.yaml
rotari run -p sweep
```

Successful unchanged jobs carry forward; failed or changed jobs and their
downstream dependents execute.

### Can I mark a failed job as successful after reviewing its log?

Yes. In a run-exported manifest, change that unchanged job or instance from
`status: failed` to `status: success`. The next run records it as
`success (accepted)` and follows the original attempt for logs. The source run
and its non-zero exit code remain unchanged.

`attempt_id` is provenance and should normally not be edited. Import validates
its embedded run and job IDs against the source state. A malformed, missing, or
unreachable attempt rejects the import before the queue is written. Because a
different real attempt of the same job cannot be told apart from an intentional
choice, check the `source_attempt_id` that `import --dry-run` prints for each
reused or accepted job before importing.

### Does retrying an array job rerun every task?

By default, only matching tasks rerun. Use `--partial-array=false` to rerun the entire array.

### Can I submit a matrix of jobs?

Yes. Repeat `--matrix KEY=VALUE[,VALUE...]` with `add`. Rotari registers each
Cartesian-product combination as an independent job and exposes its values as
ordinary `KEY=VALUE` environment variables. Matrix jobs can be combined with
`--array`; the array is applied to each matrix combination. Workflow manifests
can use `matrix_exclude` to omit selected combinations; the rule is retained by
queue and run exports. `rotari add` also accepts repeatable
`--matrix-exclude KEY=VALUE[,KEY=VALUE...]` rules, which require `--matrix` and
are retained in queue and run exports.

### Can I reference a matrix value inside the command string, like `$KEY`?

Not directly. Rotari never parses the command as a shell string, so each
argument is passed through literally and `$KEY` is not expanded. Since the
matrix value is exported as an ordinary environment variable, invoke a shell
explicitly to expand it, for example:

```sh
rotari add --matrix VALUE=1,3 -- sh -c 'echo hello > $VALUE.log'
```

### Why did `copy` reuse the same job ID instead of generating a new one?

A new ID is assigned only when the original would collide in the destination. Otherwise the ID and copied dependency relationships are preserved.

### Can I copy a job without its prerequisite?

Yes, unless the omitted prerequisite has not succeeded. Copy the prerequisite as well in that case.

### If a prerequisite job (`--depends-on`) fails, what happens to the jobs that depend on it?

Dependent jobs become `blocked` and are not executed. They can run after the prerequisite succeeds in a later `run` or `retry`.

### Can only some jobs be retried automatically?

Yes. `rotari add --retry N` gives a job its own retry limit, which replaces `run --retry` for that job; `--retry 0` keeps a job from being retried even when the run retries others. Retries start as soon as the job fails, or after `--retry-delay` with optional `--retry-backoff` and `--retry-max-delay`. See [run and retry](RUNNING.md#run-and-retry).

### How do I stop jobs that hang?

Add them with `--timeout DURATION`, for example `rotari add --timeout 2h -- python train.py`. rotari stops the job that long after it starts running, on any executor, and records it as failed with exit code 124. See [Job timeouts](RUNNING.md#job-timeouts).

### Can a job run after its prerequisites finish even if some failed?

Yes. Use `--depends-on-finished NAME` instead of `--depends-on NAME`. The job starts once every listed job or stage member has a final result, including failed, blocked, or cancelled ones, which suits collecting partial sweep results or cleanup. A prerequisite that will still be retried by `run --retry` is not final yet. See [Dependencies and stages](RUNNING.md#dependencies-and-stages).

### Can jobs wait for a whole stage instead of listing every prerequisite?

Yes. Give related jobs the same `--stage NAME`, then use `--depends-on NAME`. Stage jobs run in parallel and dependents wait for all of them to succeed. Stage and job names cannot overlap.

### Can a job depend on a job in a different run or project?

No. Dependencies are limited to the current queue. Chain separate runs with `rotari wait --run-id RUN_ID && rotari run --project-name PROJECT` or a completion webhook.

## LLM diagnosis

### Can I diagnose common failures without sending logs to an LLM?

Yes. Failed jobs receive local rule-based diagnoses. You can also run `rotari diagnose --run-id RUN_ID --job-id JOB_ID --rules`; it needs no network access or API key and does not affect execution or retries.

### What does `rotari diagnose` send to an LLM?

It sends the selected command, exit information, and at most the last 12,000 characters of its output. Data is sent only when `ROTARI_LLM_API_KEY` and a model are supplied. Check logs for sensitive data first.

### How do I choose the diagnosis response language?

Use a BCP 47 tag such as `--language ja`, or set `ROTARI_LLM_LANGUAGE`.

## Interrupted runs and locking

### How can I check whether a project is ready to run without changing it?

Run `rotari check PROJECT` (or `rotari check --project-name PROJECT`). Its status and output identify active runs, stale locks, interruptions, and empty queues. `--deep` also checks required local executables but does not connect to remote hosts.
An uncreated project is reported as empty (exit status 1); `check` does not create it.

### What happens if runners on multiple hosts use the same project?

It works when the shared filesystem correctly provides locking and atomic operations, but it is not a distributed lock service. After a host failure, confirm that jobs stopped before using `rotari unlock PROJECT`.

### A runner or supervisor process died mid-run — what do I do?

Confirm that jobs have stopped, inspect `rotari show --run-id RUN_ID`, then run `rotari unlock PROJECT`. Use `rotari reset --recover` to discard the retained queue. `unlock` refuses a run whose supervisor is still alive on this host, so it cannot start a second runner beside a live one; use `rotari cancel` for that.
Without `--run-id`, `unlock` of a project with no lock or a project not yet created succeeds without changing state.

### A remote host's lock looks stuck even though the job actually stopped — why won't `unlock` go away automatically?

A lock from another host cannot be cleared by checking its PID. Confirm that the job stopped, then run `rotari unlock PROJECT` explicitly.

## Client control and job cancellation

### Is it safe to Ctrl-C a synchronous `rotari run`?

Yes. Ctrl-C returns code 130 immediately while the supervisor continues stopping jobs and finalizing the run. Operations on the same project may be rejected briefly during cleanup.

### Can I detach a synchronous run without cancelling it?

Yes. Press Ctrl-D while progress is displayed to exit the client while the run continues. You can also start with `rotari run --async`.

### What happens if I press Ctrl-Z during a synchronous run?

Only the client is suspended; the run continues. Use `fg` to resume it, but use Ctrl-D or `--async` when you intend to detach.

### Can `wait` find the run ID for me?

Yes. With no selector, it finds a running project and lists candidates when multiple projects are running.

### How does rotari actually stop a running job on Ctrl-C or `rotari cancel`?

Local and SSH jobs receive `SIGTERM` through their process groups. Slurm, PBS, LSF, and SGE use their native cancellation commands. Rotari does not automatically escalate to `SIGKILL`.

### Why do cross-host `cancel`/`suspend`/`resume` fail for local jobs?

A local job's PID is meaningful only on the host that started it. Run these commands on the runner host.

### Does whole-run `cancel` have the same cross-host problem?

Yes. A runner PID on another host cannot be used; rotari reports the host mismatch instead.

### Why do scheduler jobs fail from a different Web/CLI host?

Scheduler control requires the relevant Slurm, PBS, LSF, or SGE client command. Use a host where those scheduler clients are installed and configured.

### Does `running` mean a scheduler job is already executing?

Not necessarily. Rotari's `running` means submitted but unfinished, which includes scheduler-pending jobs. `cancel` works for pending jobs; `suspend` may be rejected until execution starts.

## Python interface

### Is there a Python API?

Yes. The `python/` package is a thin subprocess wrapper around the `rotari` executable. Install it with `python3 -m pip install --no-deps ./python`. It does not serialize Python functions as jobs.

## Web UI

### Does closing/stopping `rotari web` stop my jobs?

No. The Web UI is a separate process and closing it does not affect runs.

### Can I start a run from the Web UI?

No. The Web UI controls existing runs and edits queues; execution starts through `rotari run`.

### What does the Job activity link show?

It lists running jobs and jobs that finished during the preceding 24 hours,
using the same default scope as `rotari jobs`. Select a job name to open its
run page. `rotari jobs PROJECT` narrows the CLI list to one project. Enter a Go
duration such as `6h` or `168h` in `Since` to change the completed-job window.

### Can I search old jobs across projects?

Yes. Open `History search` in the Web sidebar to search project, run, and job
history in selected scopes. Choose a basedir, then optionally narrow it to a
project and run; add more scopes to include additional ranges. Scope selection
is independent of notification monitoring. Choose a time window and add
field/word conditions joined by AND or OR. The static Web export includes an
explanation page; history search requires the live Web UI.

### What do Create and Append do on a run page?

`Create` replaces the queue with selected jobs; `Append` adds them to the current queue. A separate runner executes the queue.

### Can I create a config file from the Web UI?

Yes. `Generate config` on the all-projects or project page offers the valid
global, basedir, and project locations and writes `config.toml` at the selected
location after confirmation. `View config` shows the highest-priority resolved
config for the current page and, on those pages, lets you save edits. Content is
validated as JSON, TOML, or YAML according to the file's extension; invalid
edits leave the existing file unchanged. Historical run pages only display
their recorded config copies. Read-only Web mode and the static demo present
the same flow but reject the final file write with the standard read-only
message.

### How do I view an older job attempt in the Web UI?

Use the arrow beside the attempt ID and select an attempt. Its status, timestamps, result, and log are shown.

### Why does a run show as `unreadable` in the Web UI?

Its `summary.json` or `commands.json` was written by a newer rotari, whose state format this version cannot read safely. The run's page shows the message; upgrade rotari to see it. The CLI refuses the same run with the same message, and the project's other runs are unaffected.

### Can the Web UI notify me when a run finishes?

Yes. It uses browser notifications locally and sends nothing to an external service. See [Notifications](NOTIFICATIONS.md#browser-notifications).

### Does the Web UI send run details to an AI service?

No. On a run page, the `Report` button in the toolbar (for the run, or for the selected jobs) or in a job's row previews a Markdown report with execution details, saved diagnosis, and recent relevant output. The copy button copies it. Rotari never submits the report; paste and send it yourself. Reports redact known hostnames and paths plus common absolute-path and hostname patterns in logs, but redaction is best-effort, so review the report before sharing it. A `Redact: On` / `Redact: Off` button in the report dialog lets you turn redaction off for a trusted team; it starts on by default and is not offered in the static (GitHub Pages) export, which always serves its precomputed, redacted report. Log dialogs can also copy only the last 100 lines.

### What does the project page's “Project runtime” panel show?

It shows the saved runner lock and the supervisor's PID record. It is not a liveness probe.

### Is `rotari web` safe to expose beyond `127.0.0.1`?

Set `ROTARI_WEB_AUTH_TOKEN` (or `--auth-token`) and use HTTPS or a trusted network. Do not expose the UI without a token. API clients send `Authorization: Bearer TOKEN` or `X-Rotari-Token: TOKEN`; browsers use Basic auth with username `rotari` and the token as password. This is authentication, not encryption. Environment variable values are never exposed; the UI shows only whether each variable is set.

### Can I run the Web UI read-only?

Yes. `--allow-control=false` (or `ROTARI_WEB_ALLOW_CONTROL=false`) keeps state, logs, job activity, and docs pages available and returns `403 Forbidden` for control and file-write APIs.

### Can I switch projects or state directories in the Web UI?

Yes. The sidebar groups projects under the startup basedir and other basedirs
registered in rotari's master registry. Expand a basedir to see its project
names, then select a project to load its runs and jobs. The Web UI reads full
state for only the selected basedir; expanding another one lists directory
names without scanning its run history. `rotari web --project-name` is not
supported; choose the project from the sidebar instead. Static exports remain
limited to the basedir used to generate them.

## Background server (supervisor)

### Do I need to start a server manually?

No. `run` and `retry` start a supervisor for each run, and it exits when the run ends; other commands never start one. `rotari server status`, `list`, and `shutdown` are for inspection or manual cleanup. `server shutdown` leaves an active run interrupted with its jobs still running, so use `cancel` to stop a run.

### Can other users control my run through its supervisor?

No. A supervisor has no socket or port: only the `run` command that started it talks to it, over inherited pipes. `cancel`, `suspend`, and `resume` act through the state files and job processes, so they need the same file and process permissions as the run's owner.

### Can other people on the cluster read my job logs/commands?

By default, state uses `0755`/`0644`, so others may be able to read it. Set `ROTARI_PRIVATE_STATE=true` for owner-only permissions on newly created state.

## Timestamps and environment

### What timezone are the times shown by `show`/`web` in?

Stored times are UTC. Display uses the valid IANA timezone from `TZ`, or the system's local timezone when `TZ` is unset.

### A CLI option and its matching `ROTARI_*` environment variable are both set — which wins?

The explicit CLI option takes precedence over the environment variable.

### Which working directory and environment do my jobs run with?

Those of the shell that runs `rotari run` or `rotari retry`, unless a job sets its own with `add --working-directory` or `add --env`. They are not part of the queue or an exported workflow, so the same jobs can be run again from another directory. Each run has its own supervisor, so a run started while another is active still uses its own shell's directory and environment. See [Workflow and execution environment](CONCEPTS.md#workflow-and-execution-environment).
