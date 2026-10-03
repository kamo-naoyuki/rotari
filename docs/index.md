# rotari documentation

Rotari keeps track of experiment batches run from shell scripts: which jobs
are running, which failed and why, their logs, and every earlier run.

It runs the same batch on a workstation, over SSH, or on a shared Slurm, PBS,
LSF, or Sun Grid Engine cluster while keeping commands, results, and logs together.

## Start here

- [Getting started](GETTING_STARTED.md): installation and the first run.
- [Concepts](CONCEPTS.md): projects, queues, runs, IDs, and state.
- [Running and recovering](RUNNING.md): schedule, execute, inspect, rerun, retry, and use array and matrix jobs.
- [Executors and schedulers](EXECUTORS.md): local, SSH, Slurm, PBS, LSF, and SGE execution backends.
- [Inspecting and diagnosing](INSPECT.md): status, logs, and failure diagnosis.

The repository [README](https://github.com/kamo-naoyuki/rotari) provides the
project overview; installation and quick start instructions are maintained in
the [Getting started](GETTING_STARTED.md) guide.

## Reference

- [CLI reference](CLI_REFERENCE.md): commands and common options.
- [Environment variables](ENVIRONMENT_VARIABLES.md): variable meanings and scope.
- [Python client](PYTHON_CLIENT.md): installation and a short usage guide.
- [Python API](python-api.md)
- [Go API](go-api.md)
- [MCP server](MCP.md): experimental VS Code/agent integration for inspecting one job.

## For LLMs

The documentation build publishes an [LLM-friendly index](https://kamo-naoyuki.github.io/rotari/docs/llms.txt)
and a [single file with the rendered documentation](https://kamo-naoyuki.github.io/rotari/docs/llms-full.txt).
Share the index when the LLM can access links, or attach the full file when it cannot.
