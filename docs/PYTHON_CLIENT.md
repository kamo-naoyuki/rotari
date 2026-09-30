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
summary = rotari.wait(run.id)
```

`add()` returns a `Job` with `id` (`job_id`), `name` (`job_name`), and the
registered `command` argument tuple. `run()` returns a `Run` with the newly
allocated `id` (`run_id`) and `name` (`run_name`). Both retain the invocation's `args`,
`returncode`, `stdout`, and `stderr` as attributes; `args` are the full CLI
arguments, whereas `command` is the job's executable argument list. A matrix
`add()` creates multiple jobs, so its `Job.id` and `Job.name` are `None` (the
CLI currently returns only a count). `quiet=True` does not suppress captured
output for these two methods because they need it to obtain IDs.

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
`CommandResult`; command failures raise `RotariError`. For a failed synchronous
`run()`, the exception's `result.run_id` is available when the run started and
the CLI reported its ID.

Command options are generated from the CLI schema. To inspect the complete
schema and descriptions:

```sh
rotari schema --json
```

See the [Python API reference](python-api.md) for every client command and its
schema-generated options.
