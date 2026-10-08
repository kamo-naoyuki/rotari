# rotari documentation

Rotari keeps track of experiment batches run from shell scripts: which jobs
are running, which failed and why, their logs, and every earlier run.

It runs the same batch on a workstation, over SSH, or on a shared Slurm, PBS,
LSF, or Sun Grid Engine cluster while keeping commands, results, and logs together.

## Start here

- [Getting started](GETTING_STARTED.md): installation and the first run.
- [Concepts](CONCEPTS.md): projects, queues, runs, IDs, and state.
- [Defining and controlling jobs](RUNNING.md): use arrays and matrices,
  dependencies, retries, timeouts, and job-level controls.
- [Async runs, waits, and interruptions](RUNS.md): start asynchronous runs,
  wait for completion, and understand interruption behavior.
- [Recovering failed runs](RECOVERING.md): rerun failed jobs, optionally after editing them.
- [Executors and schedulers](EXECUTORS.md): local, SSH, Slurm, PBS, LSF, and SGE execution backends.
- [Inspecting runs and jobs](INSPECT.md): status, logs, run readiness, and history.

The repository [README](https://github.com/kamo-naoyuki/rotari) provides the
project overview; installation and quick start instructions are maintained in
the [Getting started](GETTING_STARTED.md) guide.

## Reference

- [CLI reference](CLI_REFERENCE.md): commands and common options.
- [Python client](PYTHON_CLIENT.md): installation and a short usage guide.
- [Python API](python-api.md)
- [Go API](go-api.md)

## Asking an AI assistant about rotari

To get help from an AI assistant such as ChatGPT, Claude, or GitHub Copilot,
give it this documentation:

- If the assistant can open web links, share the
  [documentation index](https://kamo-naoyuki.github.io/rotari/docs/llms.txt).
  It lists every page, so the assistant can read the ones it needs.
- If it cannot, download the
  [full documentation as one text file](https://kamo-naoyuki.github.io/rotari/docs/llms-full.txt)
  and attach it to the conversation.
