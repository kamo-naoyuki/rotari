# rotari Python client

This directory contains a thin Python client for the `rotari` executable. For
CLI behavior and examples, see the repository [README](../README.md).

## Usage and API documentation

See the [Python client guide](https://kamo-naoyuki.github.io/rotari/docs/python-client/)
for installation and a usage example. The [Python API reference](https://kamo-naoyuki.github.io/rotari/docs/python-api/)
contains every client command and its schema-generated options.

## Current API (available now)

`Rotari(project=...)` is a handle for a project; `basedir`, `cwd`, `env`, and
the executable can also be set on the client. Jobs are executable argument
lists, not serialized Python functions. This example works today:

```python
from rotari import Rotari

project = Rotari(basedir=".rotari-state", project="experiment")
job = project.add(["python", "train.py"], job_name="train")
run = project.run(async_=True, run_name="trial-1")
summary = project.wait(run.id)
details = project.show(run_id=run.id)
```

| Method | Current behavior |
| --- | --- |
| `add(command, **options)` | Add a command (with optional name, dependencies, array, matrix, executor settings); return a `Job`. |
| `run(**options)` | Execute the queue, synchronously or with `async_=True`; return a `Run`. |
| `retry(**options)` | Retry selected work; return a new `Run`. |
| `wait(selector=None, **options)` | Wait for a run ID or `Run`, or a list of them; return JSON summaries even for failed runs. |
| `show(target=None, *, run=None, **options)` | Return JSON views of a run or job (or an ordered list of either); the old `show(run_id=...)` form remains supported. |
| `check(**options)` | Return the CLI's JSON project-readiness report, including when not runnable. |
| `cancel(target=None, **options)` | Cancel the project's active run, a specific `Run`, or active jobs; return a `CommandResult`. |
| `suspend(target, **options)` / `resume(target, **options)` | Control one or more active jobs; return a `CommandResult`. |
| `reset(recover=False)` | Discard the project's queued work; return a `CommandResult`. |
| `export(as_dict=True, **options)` / `import_(manifest, **options)` | Exchange manifests as dictionaries; `export()` without `as_dict=True` returns a `CommandResult`. |
| `command(*args)` | Invoke any other CLI subcommand and return a `CommandResult`. |

`job.id`/`job.job_id`, `job.name`/`job.job_name`, and `job.command` describe
the added job. `run.id`/`run.run_id` and `run.name`/`run.run_name` identify the
new run. Both retain `args` (full CLI argv), `returncode`, `stdout`, and
`stderr`. CLI errors raise `RotariError`, whose `result` holds that output.
Matrix `add()` currently yields a `Job` with no single ID or name because
the CLI only prints a count. Other CLI operations remain available through
`command()`.

## Object API

The examples below show a small, CLI-backed convenience layer. A
`Rotari` instance represents the chosen project; `Run` and `Job` keep a
reference to that client for convenience methods. Attributes contain values
already obtained; **calling a method** may invoke the CLI. No hidden
subprocess on attribute access, background polling thread, or second Python
implementation of CLI status rules.

### Submitting and checking a project

```python
project = Rotari(basedir=".rotari-state", project="experiment")
ready = project.check()            # dict from CLI check --json
project.reset()                    # clears the queue
job1 = project.add(["python", "train.py"], job_name="train")
job2 = project.add(["python", "evaluate.py"], job_name="evaluate")
run = project.run(async_=True, run_name="trial-1")
project.wait(run)                  # finish the run before trying a retry
next_run = project.retry(async_=True)  # if work failed: return Run
```

Whether a run can start depends on the CLI's queue and state
rules. `reset()` discards queued work, not saved run history.

### Waiting and inspecting

```python
summary = project.wait(run)          # or run.wait(), or project.wait(run.id)
summaries = project.wait([run, next_run])  # results in input order
run_info = project.show(run)         # or run.show()
run_infos = project.show([run, next_run])
job_info = project.show(job1)        # or job1.show(): latest saved run
job_infos = project.show([job1, job2])  # jobs in latest saved run
specific = project.show(job1, run=run)  # or job1.show(run=run)
```

The examples assume a retryable failure before `next_run`; without one,
`retry()` may not start another run. `wait()` accepts `Run` objects or run ID
strings, not jobs; `wait()` without a target keeps the CLI's existing
resolution. A single target returns a JSON dict;
a list returns a list of dicts in input order (including failed run
summaries). `show()` follows the same single/list return convention. Each
method call explicitly queries the CLI; the returned dict is a snapshot.

For a job without `run=`, `show(job)` means that job **in the project's latest
saved run**, not the current queue. If it is not in that run, raise an error
instead of silently showing the queue or a different run. `show(job, run=run)`
selects a particular run, including an active run. `run.show()` and
`job.show()` delegate to the same project methods; no `job.status` property
silently fetches new state. Structured per-job inspection uses the CLI's
`show --json --job-id` projection; unfinished jobs have no result yet.
For multiple runs, `wait()` decodes all CLI JSON responses in input order.
`show()` on a list may make one CLI call per target; a single call never
silently fetches fresh data through an attribute.

The same operations can be written from objects that remember their creating
project (each method delegates to the project, rather than managing state):

```python
run.wait()
run.show()
job1.show()             # latest saved run, unless run= is supplied
job1.show(run=run)      # inspect this specific run
```

### Controlling active work

```python
project.cancel()                       # whole active run in this project
project.cancel(run, wait=True)         # or run.cancel(wait=True)
project.cancel(job1)                   # or job1.cancel(): one active job
project.cancel([job1, job2])           # jobs in the same active run
project.suspend(job1)                  # or job1.suspend(): running job
project.suspend([job1, job2])
project.resume(job1)                   # or job1.resume(): suspended job
project.resume([job1, job2])
```

These are alternatives, not a sequence to execute after the completed-run
examples above; they require an active run (and an appropriate job state).
The equivalent object methods are `run.cancel(wait=True)`, `job1.cancel()`,
`job1.suspend()`, and `job1.resume()`.
These operations return a CLI `CommandResult`. `cancel(run)` acts on the
whole run only if it is active; `cancel()` uses the project's active run,
and requires an explicit target if no project is bound to `Rotari`. Job
control accepts job IDs or `Job` objects, but no job with a missing ID.
Lists must select jobs in the **same active run and project**. Mixed jobs
and runs, multiple runs (`cancel([run1, run2])`), and cross-project targets
are outside this initial scope. `wait=True` applies only to whole-run
cancellation, not individual jobs. `suspend()` and `resume()` are for jobs,
not whole runs. Convenience methods on `Run` and `Job` delegate to the same
`Rotari` methods; they do not maintain their own execution state.

### Workflow manifests and low-level access

```python
manifest = project.export(as_dict=True)  # existing dict API
project.import_(manifest, overwrite=True)
project.command("jobs")                  # existing escape hatch for CLI features
```

This is intentionally **not** a port of every CLI command. Dedicated Python
methods are worth adding for frequently used, structured operations; the
rest remain available via `command()`. In particular, no dedicated log or
report method is proposed yet.

### Open items / later work

- Search for runs or jobs by name or command and return `Run`/`Job` objects,
  and list a project's runs or a run's jobs, are separate follow-ups. They
  should be explicit methods (such as `list_runs()` and `list_jobs(run)`),
  not attributes that launch CLI calls.
- Matrix `add()` currently returns one `Job` without an ID; a future
  collection of individual jobs would require the CLI to return their IDs.
- More detailed live job phases, if needed, should come from structured CLI
  output queried by a method, not a thread or a silently refreshing property.
