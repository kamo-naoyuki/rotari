# Rotari FAQ

Rotari is a lightweight workflow runner for repeatedly executing shell commands while managing dependencies, parallelism, logs, and run history.

This FAQ gives short answers about rotari's behavior. See [README.md](../README.md) for usage and [INTERNALS.md](INTERNALS.md) for detailed contracts.

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

### Can rotari manage jobs for multiple users like Slurm?

No. Rotari has no accounts, permissions, quotas, or fair-share scheduling. Use a separate `--basedir` per user and do not share writable state or Web UI tokens between untrusted users. Slurm, PBS, or LSF remains responsible for scheduler-side identity and resource policy.

### Which workflow-orchestrator features does rotari provide?

Rotari provides dependencies, parallel execution, retries, logs, history, array jobs, local/SSH/Slurm/PBS/LSF execution, a Web UI, completion webhooks, and an optional declarative workflow manifest.

The manifest is a constrained representation of the existing queue, not a programming language: commands remain argument arrays. Rotari does not provide file freshness checks, artifact caching, scheduled or event triggers, data lineage, RBAC, or quotas. Use tools such as Snakemake, `make`, or Airflow when those features are needed.

### What's the difference between a project, a queue, and a run?

A project groups the current queue and run history. The queue (`queue.json`) contains waiting commands; a run is an immutable snapshot taken when execution starts.

### I didn't pass `--project-name` — which project does rotari use?

Selection uses `--project-name`, `ROTARI_PROJECT_NAME`, the only project in the resolved state directory, and then `default`. Multiple candidates require an explicit selection. A bare `rotari show` lists projects.

### How do I list projects in a state directory?

Run `rotari show`. Use `--basedir` to narrow the state directory and `rotari show -p PROJECT` to inspect a project.

### I don't know which basedir contains my jobs. How do I find it?

Run `rotari show --basedirs`, then inspect one with `rotari show --basedir DIR`. Use `--masterdir DIR` to select the registry.

### Where can I put option defaults?

Put `config.yaml`, `config.toml`, or `config.json` in `$XDG_CONFIG_HOME/rotari` (normally `~/.config/rotari`), the basedir, or `projects/<project>/`. Priority is project, basedir, then global; CLI options and environment variables override config files.

### How do I find every config file below a basedir?

Run `rotari config --list --basedir DIR`. It groups global and basedir paths under `Common:`, then project paths under `Projects:` with each project name followed by indented paths. Add `--project-name NAME` to limit project-specific entries to that project. This inventory includes files that are not selected by normal priority resolution.

### How do I see every configurable option?

Run `rotari config`, or use `--output FILE` to save a template. The selected config is copied into each run directory so historical views retain the configuration used at that time.

### How can I notify another service when a run finishes?

Set `webhook.url` or `ROTARI_WEBHOOK_URL`. Use `webhook.on` with `success`, `failure`, or `always` (the default). Slack, Teams, and Discord formats are supported; see [Webhook integrations](WEBHOOK_NOTIFICATIONS.md).

### How are concurrency and executor options selected?

`--local-concurrency` applies to local jobs; `--batch-concurrency` is the default for other executors. SSH, Slurm, PBS, and LSF also have executor-specific settings, with job-level settings taking priority.

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

Run `rotari gc [MASTERDIR]` to review candidates, then `rotari gc --apply [MASTERDIR]` to remove them. Reappeared or changed candidates are skipped.

## Language and implementation choices

### Why is rotari written in Go instead of Python?

Go handles many small CLI operations, process actions, and filesystem updates with low startup overhead and can be distributed as one binary.

### Why not C++?

C++ could work, but Go provides the preferred balance of iteration speed, maintainability, and straightforward process and filesystem handling.

### Why not Rust?

Rust is also a strong option, but Go is currently a better balance of safety, implementation speed, and maintenance for this project.

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
uses its native sparse array for selected task IDs; PBS and LSF submit those
tasks independently.

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
rotari export -p build -r RUN_ID > experiment.yaml
rotari import -p build --dry-run experiment.yaml
rotari import -p build experiment.yaml
rotari run -p build
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
unreachable attempt rejects the import before the queue is written.

### Does retrying an array job rerun every task?

By default, only matching tasks rerun. Use `--partial-array=false` to rerun the entire array.

### Can I submit a matrix of jobs?

Yes. Repeat `--matrix KEY=VALUE[,VALUE...]` with `add`. Rotari registers each
Cartesian-product combination as an independent job and exposes its values as
ordinary `KEY=VALUE` environment variables. Matrix jobs can be combined with
`--array`; the array is applied to each matrix combination. `include` and
`exclude` customization is not supported by the version 1 workflow manifest.

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

### What happens if runners on multiple hosts use the same project?

It works when the shared filesystem correctly provides locking and atomic operations, but it is not a distributed lock service. After a host failure, confirm that jobs stopped before using `rotari unlock PROJECT`.

### A runner or supervisor process died mid-run — what do I do?

Confirm that jobs have stopped, inspect `rotari show --run-id RUN_ID`, then run `rotari unlock PROJECT`. Use `rotari reset --recover` to discard the retained queue.

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

Local and SSH jobs receive `SIGTERM` through their process groups. Slurm, PBS, and LSF use their native cancellation commands. Rotari does not automatically escalate to `SIGKILL`.

### Why do cross-host `cancel`/`suspend`/`resume` fail for local jobs?

A local job's PID is meaningful only on the host that started it. Run these commands on the runner host.

### Does whole-run `cancel` have the same cross-host problem?

Yes. A runner PID on another host cannot be used; rotari reports the host mismatch instead.

### Why do scheduler jobs fail from a different Web/CLI host?

Scheduler control requires the relevant Slurm, PBS, or LSF client command. Use a host where those scheduler clients are installed and configured.

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

### What do Create and Append do on a run page?

`Create` replaces the queue with selected jobs; `Append` adds them to the current queue. A separate runner executes the queue.

### Can I create a config file from the Web UI?

Yes. `View config` and `Generate config` can save configuration. The format is validated first, and invalid edits leave the existing file unchanged.
The read-only static demo presents the same flow but rejects the final file
write with the standard read-only message.

### How do I view an older job attempt in the Web UI?

Use the arrow beside the attempt ID and select an attempt. Its status, timestamps, result, and log are shown.

### Can the Web UI notify me when a run finishes?

Yes. It uses browser notifications locally and sends nothing to an external service. See [Web browser notifications](WEB_BROWSER_NOTIFICATIONS.md).

### Does the Web UI send run details to an AI service?

No. AI reports are generated in the browser; external AI pages are only opened for you to review and paste manually. Check for sensitive data first.

### What does the project page's “Project runtime” panel show?

It shows the saved runner lock and local coordinator socket/PID records. It is not a liveness probe.

### Is `rotari web` safe to expose beyond `127.0.0.1`?

Set `ROTARI_WEB_AUTH_TOKEN` (or `--auth-token`) and use HTTPS or a trusted network. Use `--allow-control=false` for read-only access. Do not expose the UI without a token.

## Background server (supervisor)

### Do I need to start a server manually?

Usually not. `run`, `add`, and `copy` start the supervisor when needed. `rotari server status`, `list`, and `shutdown` are for inspection or manual cleanup.

### Who can control my jobs through the server's Unix socket?

Users who can connect as the same UID can control jobs. The socket is `0600`, and Linux also checks the peer UID; this does not protect against root or access through the same UID.

### Can other people on the cluster read my job logs/commands?

By default, state uses `0755`/`0644`, so others may be able to read it. Set `ROTARI_PRIVATE_STATE=true` for owner-only permissions on newly created state.

## Timestamps and environment

### What timezone are the times shown by `show`/`web` in?

Stored times are UTC. Display uses the valid IANA timezone from `TZ`, or the system's local timezone when `TZ` is unset.

### A CLI option and its matching `ROTARI_*` environment variable are both set — which wins?

The explicit CLI option takes precedence over the environment variable.
