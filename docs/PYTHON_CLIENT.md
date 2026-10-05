# Python client

The Python client is a thin wrapper around the `rotari` executable. It submits
argument lists to the CLI and decodes machine-readable results; it does not
serialize Python functions or closures.

## Installation

Tagged releases publish wheels containing the matching `rotari` executable.
Install the package for the current platform from the static package index:

```sh
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari
```

For an unsupported platform or a source checkout, make sure `rotari` is on
`PATH` and install the client without dependencies:

```sh
python3 -m pip install --no-deps ./python
```

## Usage

Create a client, add executable argument lists, start a run, and wait for its
JSON summary:

```python
from rotari import Rotari

rotari = Rotari(basedir=".rotari-state", project="experiment")
job = rotari.add(["./train.sh"], job_name="train")
print(job.id, job.name, job.command)
run = rotari.run(async_=True, run_name="experiment-1")
print(run.id, run.name)
summary = run.wait()
details = job.show(run=run)
```

## Project operations

`Rotari(project=...)` is a handle for a project. You can also set `basedir`,
`cwd`, `env`, and the executable when creating it. The common methods are:

| Method | Purpose |
| --- | --- |
| `add(command, **options)` | Add a command with an optional name, dependencies, array or matrix, and executor settings; return a `Job`. |
| `run(**options)` / `retry(**options)` | Start a run (or retry selected work) and return a `Run`. `run_id=...` builds from that saved run without changing the next queue. Pass `async_=True` to return after starting it. |
| `check(**options)` | Return a JSON readiness dict even when the project is not runnable. |
| `reset()` | Clear queued work for the next run without changing an active or interrupted run. |
| `unlock(run_id=None)` | Recover an interrupted run, keeping the queue; then `retry(run_id=...)` reruns its failed and unfinished jobs. Confirm first that its jobs have stopped. |
| `export(as_dict=True, **options)` / `import_(manifest, **options)` | Exchange workflow manifests as dictionaries; without `as_dict=True`, `export()` returns `CommandResult`. |
| `command(*args)` | Invoke another CLI subcommand and return `CommandResult`. |

For example, when building a fresh batch:

```python
project = Rotari(basedir=".rotari-state", project="experiment")
project.reset()
train = project.add(["python", "train.py"], job_name="train")
evaluate = project.add(["python", "evaluate.py"], job_name="evaluate")
readiness = project.check()
run = project.run(async_=True, run_name="trial-1")
summary = run.wait()
```

After a run with retryable failures, `next_run = project.retry(async_=True)`
returns another `Run`. Whether `run()` or `retry()` can start depends on the
CLI's queue and project state rules. An added job has `id`/`job_id`,
`name`/`job_name`, and `command`; a run has `id`/`run_id` and `name`/`run_name`.
Both retain `args` (the full CLI argv), `returncode`, `stdout`, and `stderr`.
CLI errors raise `RotariError` with a `result` containing the invocation's
output. For a failed synchronous run, `result.run_id` is available if the CLI
reported its ID. Matrix `add()` currently returns one `Job` with no single
ID or name, because the CLI returns only a count. Returning each expanded
job would first require their IDs from the CLI. `add()`, `run()`, and `retry()`
disable CLI quiet output (even when configured) to capture these IDs.

Workflow manifests can be exchanged as dictionaries without an intermediate
file. `export(as_dict=True)` returns a JSON-compatible dict, and `import_()`
sends one to the CLI as JSON over stdin:

```python
manifest = project.export(as_dict=True)
manifest["jobs"][0]["name"] = "updated-train"
project.import_(manifest, overwrite=True)
```

The trailing underscore in `import_()` avoids Python's reserved `import`
keyword.

## Waiting and inspecting

```python
summary = project.wait(run)           # or run.wait(), or project.wait(run.id)
summaries = project.wait([run1, run2])  # run IDs or Run objects; input order
run_info = project.show(run)          # or run.show()
run_infos = project.show([run1, run2])
job_info = project.show(train)        # or train.show(): latest saved run
job_infos = project.show([train, evaluate])
specific = project.show(train, run=run)  # or train.show(run=run)
```

`run1` and `run2` above stand for runs already started in the same project.
`wait()` without a target retains the CLI's default selection; it accepts a
run ID or `Run`, not a job. A single target returns a JSON dict; a list
returns a list in input order, including summaries of failed runs. `show()`
has the same single/list convention. A `show()` call returns a snapshot,
not a live-updating object; lists may make one CLI call per target.
Objects can be passed between `Rotari` instances when their executable,
`basedir`, project, `cwd`, and `env` settings match. `check()`, `wait()`, and
`show()` always return decoded JSON; their output format is managed by the
Python API rather than a caller-supplied `json` option.

Without `run=`, `show(job)` looks in the project's latest **saved run**,
not the queue. A job absent from that run is an error, even if it is in the
current queue. Specify `run=run` to inspect a particular run, including an
active one. The CLI's structured job view includes each job's definition,
whether it has finished, and its resolved result when available; unfinished
jobs have no result yet. No attribute access starts a subprocess: `Run` and
`Job` methods delegate to the creating client only when called.

## Controlling active work

These are alternative operations on an **active** run, not a sequence to
execute after the completed-run example above:

```python
project.cancel()                       # entire active run in this project
project.cancel(run, wait=True)         # or run.cancel(wait=True)
project.cancel(train)                  # or train.cancel(): one job
project.cancel([train, evaluate])      # jobs in the same active run
project.suspend(train)                 # or train.suspend(): running job
project.suspend([train, evaluate])
project.resume(train)                  # or train.resume(): suspended job
project.resume([train, evaluate])
```

Control methods return `CommandResult`. A job ID string can be passed instead
of `Job`; a job without an ID cannot be selected. A job list must refer to
the same active run and project. Mixed runs and jobs, multiple run targets,
and cross-project objects are not accepted. `wait=True` applies only when
cancelling a whole run, not a job. `suspend()` and `resume()` select jobs, not
whole runs. `cancel()` without a target needs a project-bound `Rotari` client.

The Python client intentionally does not port every CLI command. Searching
for jobs or runs by name or command, listing a project's runs or a run's
jobs, and dedicated log/report methods remain possible follow-ups. Such
queries should be explicit methods, not attributes with hidden CLI calls;
they should reuse the CLI's status resolution instead of duplicating it in
Python. There is no background polling thread.

Command options are generated from the CLI schema. Unknown option names raise
`TypeError` instead of being silently ignored. Boolean flags that support an
explicit false value, such as `partial_array`, accept Python booleans; for
example, `run(partial_array=False)` passes `--partial-array=false`. To inspect
the complete schema and descriptions:

```sh
rotari schema --json
```

See the [Python API reference](python-api.md) for every client command and its
schema-generated options.
