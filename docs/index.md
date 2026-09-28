# rotari documentation

rotari keeps track of experiment batches run from shell scripts: which jobs
are running, which failed and why, their logs, and every earlier run.

It runs the same batch on a workstation, over SSH, or on a shared Slurm, PBS,
or LSF cluster while keeping commands, results, and logs together.

## Start here

- [Getting started](GETTING_STARTED.md): installation and the first run.
- [CLI reference](CLI_REFERENCE.md): commands and common options.
- [Concepts](CONCEPTS.md): projects, queues, runs, dependencies, and state.
- [Running and recovering](RUNNING.md): execute, inspect, rerun, and retry jobs.
- [Executors and schedulers](EXECUTORS.md): local, SSH, Slurm, PBS, and LSF.
- [Inspecting and diagnosing](INSPECT.md): status, logs, and failure diagnosis.

The repository [README](https://github.com/kamo-naoyuki/rotari) provides the
project overview; installation and quick start instructions are maintained in
the [Getting started](GETTING_STARTED.md) guide.

## Reference

- [Python API](python-api.md)
- [Go API](go-api.md)
