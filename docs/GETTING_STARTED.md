# Getting started

## Manage jobs, logs, and outputs in one place

- **A powerful web UI for every run.** Follow job status, browse logs, and
  inspect artifacts in one place.
- **Logs stay organized and easy to find.** Logs are kept with each workspace,
  without choosing a log path for every job, and runs can be explored across
  workspaces.
- **Retry only what needs work.** Rerun failed or unfinished jobs while
  keeping successful results.
- **Easily switch execution backends.** Run jobs locally, over SSH, or on
  Slurm and other schedulers.
- **Know when jobs finish.** Get browser notifications or send updates to
  Slack.
- **Follow outputs from command to artifact.** Rotari detects likely artifacts
  from job commands and lets you inspect their contents in the Web UI.

## Installation

Prebuilt binaries for Linux and macOS are on the
[GitHub Releases](https://github.com/kamo-naoyuki/rotari/releases) page; Go is
not required:

```sh
curl -fsSL https://raw.githubusercontent.com/kamo-naoyuki/rotari/main/scripts/install.sh | sh
```

This downloads the release binary matching your OS/architecture into
`~/.local/bin`. Set `ROTARI_INSTALL_DIR` to override the destination or
`ROTARI_VERSION` to pin a version.

Other options:

```sh
brew install kamo-naoyuki/tap/rotari
```

```sh
# bundles the rotari executable with the Python package
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari
```

```sh
# With go installed
go install github.com/kamo-naoyuki/rotari/cmd/rotari@latest
```

Run `rotari version` to check the installed version. Use
`rotari completion install` for Bash, Zsh, or Fish; see
[Shell completion](CONFIGURATION.md#shell-completion).

### Docker

For the published image, persistent state, and runtime requirements, see the
[Docker guide](DOCKER.md).

## Choose how to submit jobs

All three approaches use the same rotari queue, run history, and status tools.
Choose how you want to define the jobs:

| Approach | Start here | Best for |
| --- | --- | --- |
| **Shell (recommended starting point)** | [Shell quick start](#shell-quick-start) | Adding commands interactively or keeping a batch in a shell script. No new workflow format needed. |
| **Manifest** | [Manifest quick start](#manifest-quick-start) | Keeping a batch in an editable YAML file, or exporting a run to revise and import later. |
| **Python** | [Python quick start](#python-quick-start) | Submitting jobs from Python code and reading results as Python objects. Jobs still run as commands, not Python functions. |

You can switch approaches later: for example, export a run created from shell
commands as a manifest, or import a manifest with the Python client.

## Shell quick start

Optionally, run `rotari init .rotari-state sweep` from this directory to make
`.rotari-state` and `sweep` the defaults for commands started here. It writes
`.rotari.toml` only; the state directory and project are created when you add
jobs. The defaults save you from repeating `--basedir` and `--project-name`
(or setting environment variables) on each command. They apply only when the
current directory is this workspace, and explicit CLI options or environment
variables override them. See
[Workspace defaults](CONFIGURATION.md#workspace-defaults-and-initialization)
for the resolution rules.

After initialization, add and run commands directly. You can also save these
lines in a shell script to define a repeatable batch:

```sh
# Add commands to the workspace's default project queue.
rotari add python train.py --lr 0.1
rotari add python train.py --lr 0.01
rotari add python train.py --lr 0.001
# Execute the queued commands and wait for the run to finish.
rotari run
# List job status across projects.
rotari jobs
# Inspect the current project's queue or most relevant run.
rotari show
```

Use `rotari run --async` when the run should continue in the background. To
inspect failed logs and retry only failed or unfinished work:

```sh
# Show logs for failed jobs in the selected project.
rotari show -p sweep --failed-logs
# Start a new run for failed and unfinished jobs; successful jobs are reused.
rotari retry -p sweep
```

`add`, `run`, `show`, and `retry` cover most batches; the other commands are
available when needed. `jobs` gives a compact status overview across projects.
`show` provides details for a project, run, or job, including captured output
logs and saved results. `run` and `retry` inherit the caller's environment by
default; pass `--env=NONE` to suppress it while retaining job `--env` and
rotari metadata.

See [Projects, queues, runs, and state](CONCEPTS.md#projects-queues-runs-and-state)
for project selection and state layout, [Inspect](INSPECT.md#inspect) for
status and logs, [Recovering failed runs](RECOVERING.md) for
retries, [Async runs](RUNNING.md#async-runs) for asynchronous runs, [Executors and schedulers](EXECUTORS.md#executors-and-schedulers)
for execution backends, and [Array and matrix jobs](RUNNING.md#array-and-matrix-jobs)
for task expansion and matrix combinations.

Use `--depends-on NAME` to run a job only after a prerequisite job or stage
succeeds:

```sh
rotari add --job-name prepare ./prepare.sh
rotari add --job-name train --depends-on prepare ./train.sh
```

Use `--depends-on-finished NAME` for aggregation or cleanup jobs that should
run once the prerequisite finishes, whatever its result. See
[Dependencies and stages](RUNNING.md#dependencies-and-stages) for stages and
multiple prerequisites.

## Manifest quick start

If you prefer a file you can review and edit, save this as `experiment.yaml`:

```yaml
version: 1
jobs:
  - name: train-small
    command: [python, train.py, --lr, "0.1"]
  - name: train-large
    command: [python, train.py, --lr, "0.01"]
```

Import the file into a project queue and run it:

```sh
rotari import -p sweep experiment.yaml
rotari run -p sweep
rotari show -p sweep
```

Use `rotari export --template` for a starter file, or export an existing run
to edit and re-import it. See [Workflow manifests](WORKFLOW_MANIFESTS.md) for
export, import options, and result reuse.

## Python quick start

Install the [Python client](PYTHON_CLIENT.md#installation), which includes the
rotari executable on supported platforms. It submits command argument lists
through the CLI:

```python
from rotari import Rotari

rotari = Rotari(project="sweep")
rotari.add(["python", "train.py", "--lr", "0.1"])
rotari.run()
result = rotari.show()
```

The client can also [exchange manifests as dictionaries](PYTHON_CLIENT.md#usage).
See the [Python API reference](python-api.md) for available methods and options.

## Examples

Start with the [basic example](examples.md#basic-example) for direct commands,
`sh -c`, and command options:

```sh
./examples/basic.sh
```

The [examples guide](examples.md) has independent, no-argument
scripts for job dependencies, artifact candidates, arrays, retrying failed
work, async runs, Slurm, workflow manifests, and diagnosis. All examples use
rotari's normally resolved state directory and switch projects rather than
creating a basedir for each example. Slurm needs a configured cluster. To fix
a failed job and accept another job's result in an exported workflow, run
`./examples/workflow-reconcile.sh`. See
[Workflow manifests](WORKFLOW_MANIFESTS.md) for the details.

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](FAQ.md#web-ui) and [Security model](OPERATIONS.md#security-model).
