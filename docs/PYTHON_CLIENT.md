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

`add()` returns a `Job` with `id` (`job_id`), `name` (`job_name`), and the
registered `command` argument tuple. `run()` returns a `Run` with the newly
allocated `id` (`run_id`) and `name` (`run_name`). Both retain the invocation's `args`,
`returncode`, `stdout`, and `stderr` as attributes; `args` are the full CLI
arguments, whereas `command` is the job's executable argument list. A matrix
`add()` creates multiple jobs, so its `Job.id` and `Job.name` are `None` (the
CLI currently returns only a count). `add()` and `run()` explicitly disable
CLI quiet output (including configured quiet) because their captured output
is needed to obtain IDs. `retry()` does the same.

Workflow manifests can be exchanged as Python dictionaries without creating
an intermediate file. `export(as_dict=True)` returns a JSON-compatible dict,
and `import_()` accepts one:

```python
manifest = rotari.export(as_dict=True)
manifest["jobs"][0]["name"] = "updated-train"
rotari.import_(manifest, overwrite=True)
```

The trailing underscore in `import_()` avoids Python's reserved `import`
keyword. The client sends the manifest as JSON over stdin to the CLI.

`wait()` and `show()` return decoded JSON objects. Other commands return a
`CommandResult` except `retry()`, which also returns a `Run`;
command failures raise `RotariError`. For a failed synchronous
`run()`, the exception's `result.run_id` is available when the run started and
the CLI reported its ID.

`check()` returns the CLI's JSON readiness report, including non-runnable
projects. `wait(run)` or `run.wait()` waits for one run; `wait([run1, run2])`
returns summaries in input order, including failed runs. `show(run)` and
`show(job)` retrieve snapshots; the latter looks in the **latest saved run**
unless `run=` names another run. `show([job1, job2])` returns a list of views.
No attribute access makes a CLI call. `job.show()` and `run.show()` delegate
to the same client methods.

Use `cancel()` to cancel the project's active run, `cancel(run, wait=True)`
for a specified active run, and `cancel(job)` or `cancel([job1, job2])` for
active jobs. `suspend(job)` and `resume(job)` also accept lists of jobs; the
equivalent object methods are `run.cancel()`, `job.cancel()`, `job.suspend()`,
and `job.resume()`. Job lists must belong to one active run. These methods
return `CommandResult`; they never start background polling threads. Search
and enumeration by job name or command remain future work; other CLI
operations are available through `command()`.

Command options are generated from the CLI schema. To inspect the complete
schema and descriptions:

```sh
rotari schema --json
```

See the [Python API reference](python-api.md) for every client command and its
schema-generated options.
