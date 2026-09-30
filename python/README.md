# rotari Python client

This directory contains a thin Python client for the `rotari` executable. For
CLI behavior and examples, see the repository [README](../README.md).

## Usage and API documentation

See the [Python client guide](https://kamo-naoyuki.github.io/rotari/docs/python-client/)
for installation and a usage example. The [Python API reference](https://kamo-naoyuki.github.io/rotari/docs/python-api/)
contains every client command and its schema-generated options.

## Current API

`Rotari(project=...)` is a handle for working with a project. Jobs are
executable argument lists, not Python functions. The following works today:

```python
from rotari import Rotari

project = Rotari(project="experiment")
job = project.add(["python", "train.py"], job_name="train")
run = project.run(async_=True, run_name="trial-1")
summary = project.wait(run.id)
details = project.show(run_id=run.id)
```

`job.id`, `job.name`, and `job.command` describe the queued command;
`run.id` and `run.name` identify the new run. Both objects also retain
`args`, `returncode`, `stdout`, and `stderr` from the CLI call. Currently,
`wait()` takes a string selector and `show()` takes CLI options; neither
accepts these objects directly. See the client guide for matrix limitations.

## Proposed API (not implemented)

This sketch is for review, **not an executable example**. Keep the Python
client a thin wrapper over the CLI rather than reproducing the scheduler or
maintaining a background polling thread. Attributes hold information already
obtained; methods that query or change state invoke the CLI explicitly.

```python
project = Rotari(project="experiment")
job = project.add(["python", "train.py"], job_name="train")
run = project.run(async_=True, run_name="trial-1")

summary = project.wait(run)              # or project.wait(run.id)
summaries = project.wait([run1, run2])   # input order; failed runs still have summaries
run_info = project.show(run)
run_infos = project.show([run1, run2])
job_info = project.show(job)             # job in the latest saved run
infos = project.show([job1, job2])      # jobs in the latest saved run

project.cancel(job)                      # one job in the active run
project.cancel([job1, job2])             # jobs in the same active run
project.cancel(run, wait=True)           # whole active run
project.cancel()                         # this project's active run
project.suspend(job)                     # running job only
project.resume(job)                      # suspended job only
```

Single-target `wait()` and `show()` return one JSON object; list inputs
return lists in input order. `cancel()`, `suspend()`, and `resume()` return
`CommandResult` from the CLI. Job control accepts job IDs as well as `Job`
objects; `wait()` accepts run IDs or `Run` objects, not jobs. A list of jobs
for job control must belong to the same active run; mixed targets and
cross-project selections should fail rather than select something else.
`cancel([run1, run2])` is intentionally outside the initial scope. Without
a project bound to `Rotari`, `cancel()` should require an explicit target.

`show(job)` and `show([job1, job2])` target the project's latest **saved run**
when no run is specified, not the queue. A newly added job that is not in
that run should fail rather than silently fall back to the queue. Use
`show(job, run=run)` to inspect a specific run (including an active run).
The current CLI `show --json` does not support `--job-id`, so structured job
results need CLI support before this part can be implemented. Likewise,
multiple-run `wait()` needs to decode every JSON result rather than just the
first. The intended boundary is what the CLI can provide reliably, not a
second Python implementation of job status resolution.

Other small CLI-backed candidates: `check()` returning `check --json` as a
dict for project readiness, and returning a `Run` from the existing `retry()`
method (which currently returns `CommandResult`). Job logs can remain behind
`command("show", ...)` until there is a clear need for a dedicated method.

Searching for runs or jobs by name or command, listing a project's runs or a
run's jobs, and exposing live job status are useful possible follow-ups, not
part of this initial proposal. Matrix `add()` currently returns a `Job` with
no single ID; returning individual jobs would first require their IDs from
the CLI.
