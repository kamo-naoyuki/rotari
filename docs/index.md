# rotari documentation

rotari keeps track of experiment batches run from shell scripts: which jobs
are running, which failed and why, their logs, and every earlier run.

It runs the same batch on a workstation, over SSH, or on a shared Slurm, PBS,
or LSF cluster while keeping commands, results, and logs together.

## Start here

- [Concepts](CONCEPTS.md): projects, queues, runs, dependencies, and state.
- [Running and recovering](RUNNING.md): execute, inspect, rerun, and retry jobs.
- [Executors and schedulers](EXECUTORS.md): local, SSH, Slurm, PBS, and LSF.
- [Inspecting and diagnosing](INSPECT.md): status, logs, and failure diagnosis.

See the [installation and quick start guide](https://github.com/kamo-naoyuki/rotari#installation)
in the repository README.

## Reference

- [Python API](python-api.md)
- [Go API](go-api.md)
