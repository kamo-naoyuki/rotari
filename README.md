<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/rotari-logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/rotari-logo-light.svg">
  <img src="docs/assets/rotari-logo-light.svg" alt="rotari logo">
</picture>

---

[![web demo](https://img.shields.io/website?url=https%3A%2F%2Fkamo-naoyuki.github.io%2Frotari%2F&label=web%20demo&style=flat)](https://kamo-naoyuki.github.io/rotari/) [![Documentation](https://img.shields.io/badge/docs-MkDocs%20Material-526CFE)](https://kamo-naoyuki.github.io/rotari/docs/) [![codecov](https://codecov.io/gh/kamo-naoyuki/rotari/graph/badge.svg)](https://codecov.io/gh/kamo-naoyuki/rotari) [![SonarCloud Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=kamo-naoyuki_rotari&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=kamo-naoyuki_rotari)


**Rotari keeps track of the experiment batches you run from shell scripts**: which jobs are running, which failed and why, their logs, and every earlier run. When jobs fail, `rotari retry` reruns only those, keeping the work that already succeeded.

Rotari is an **execution manager for researchers who run batches of experiments**, with commands and shell scripts as the building blocks and simple dependencies between them. It is a single binary with no daemon, database, or server to set up; state is kept in the filesystem.

**The same batch runs on your workstation, over SSH, or on a shared Slurm, PBS, or LSF cluster.** Rotari submits and tracks scheduler jobs itself, supports array and matrix jobs, and keeps logs and status consistent across backends, so you do not need to containerize workloads or set up a separate cluster service just to use it. An optional Docker image is available for trying Rotari locally. If you've used [Kaldi](https://github.com/kaldi-asr/kaldi)'s or [ESPnet](https://github.com/espnet/espnet)'s `run.pl`/`queue.pl`, the basic idea should feel familiar.

The [documentation site](https://kamo-naoyuki.github.io/rotari/docs/) covers installation, concepts, command reference, and running Rotari on clusters.

## How is rotari different?

| Plain shell (background jobs) | rotari |
| --- | --- |
| <img src="https://kamo-naoyuki.github.io/rotari/demo-shell.gif" alt="shell background jobs demo" width="400"> | <img src="https://kamo-naoyuki.github.io/rotari/demo-rotari.gif" alt="rotari demo" width="400"> |


Rotari deliberately stays out of the way. **You don't need a separate workflow language:** write the commands as you normally would, as `rotari add` lines in a shell script or typed one by one, and rotari provides the execution, parallelism, logs, status, and run history around them. The script stays the definition of the batch: each time it runs, rotari keeps that run's commands, results, and logs, so you can edit the script, run it again, and still see what every earlier run did. When a queue needs to be reproduced or edited as a unit, rotari can also export and import a constrained YAML, TOML, or JSON manifest; commands remain argument arrays rather than a new scripting language.

**The environment stays yours, too.** A queue records commands, not where or with what they run: jobs are started from the working directory and environment of the shell that runs `rotari run`, as the commands of a shell script would be. After `cd` into another experiment directory or activating another conda environment, the same queue runs there without editing; pin a job's directory or variables only where it must not depend on the caller. See [Workflow and execution environment](docs/CONCEPTS.md#workflow-and-execution-environment).

**So do resources.** Rotari does not allocate GPUs, memory, or nodes. On a cluster the scheduler does, from the options you pass with `--executor-option`; on a workstation, rotari only limits how many jobs run at once (`--local-concurrency`), and a job's own variables such as `CUDA_VISIBLE_DEVICES` do the rest.

### Choosing a tool

**Rotari is for batches of experiment commands that you run yourself**, from a shell script on a workstation, over SSH, or on a shared Slurm, PBS, or LSF cluster, and want a record of. On many hosts, a job whose command is correct can still fail because one node misbehaved; `run --retry N` retries such jobs within a run, and `rotari retry` later reruns only the failed and unfinished jobs, so the successful work is kept.

Rotari covers a narrow need, and other tools may fit yours better: [GNU Parallel](https://www.gnu.org/software/parallel/) for one command over many inputs, [pueue](https://github.com/Nukesor/pueue) or [task-spooler](https://github.com/justanhduc/task-spooler) for a personal queue on one machine, and a workflow engine such as [Snakemake](https://snakemake.github.io/), [Nextflow](https://www.nextflow.io/), [Dagu](https://dagu.sh/), or [Airflow](https://airflow.apache.org/) for a pipeline you share, rerun on new data, or run on a schedule. [Comparison with other tools](docs/TOOL_COMPARISON.md) explains what each one does and how rotari differs.

<!-- Do not edit `README.md` directly. The `BEGIN GETTING STARTED` section is generated from `docs/GETTING_STARTED.md`; edit that source document and run `python3 scripts/sync_readme.py` instead.-->

<!-- BEGIN GETTING STARTED -->

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
[Shell completion](docs/CONFIGURATION.md#shell-completion).

### Docker

For the published image, persistent state, and runtime requirements, see the
[Docker guide](docs/DOCKER.md).

## Quick start

Set the project once for the current shell, then add and run commands:

```sh
# Set the project once for the current shell. The default state directory is
# ~/.local/state/rotari; set ROTARI_BASEDIR to use another location.
export ROTARI_PROJECT_NAME=sweep
# Add commands to the current project queue.
rotari add -- python train.py --lr 0.1
rotari add -- python train.py --lr 0.01
rotari add -- python train.py --lr 0.001
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

See [Projects, queues, runs, and state](docs/CONCEPTS.md#projects-queues-runs-and-state)
for project selection and state layout, [Inspect](docs/INSPECT.md#inspect) for
status and logs, [Recover and rerun](docs/RUNNING.md#recover-and-rerun) for
retries and asynchronous runs, [Executors and schedulers](docs/EXECUTORS.md#executors-and-schedulers)
for execution backends, and [Array and matrix jobs](docs/RUNNING.md#array-and-matrix-jobs)
for task expansion and matrix combinations.

Use `--depends-on NAME` to run a job only after a prerequisite job or stage
succeeds:

```sh
rotari add --job-name prepare -- ./prepare.sh
rotari add --job-name train --depends-on prepare -- ./train.sh
```

Use `--depends-on-finished NAME` for aggregation or cleanup jobs that should
run once the prerequisite finishes, whatever its result. See
[Dependencies and stages](docs/RUNNING.md#dependencies-and-stages) for stages and
multiple prerequisites.

## Examples

Start with the [basic example](docs/../examples/basic.sh) for a dependent job:

```sh
./examples/basic.sh
```

The [examples guide](docs/../examples/README.md) has independent, no-argument
scripts for arrays, retrying failed work, async runs, Slurm, workflow
manifests, and diagnosis. All examples use `.example-state` in the current
working directory, with a separate project for each. Slurm needs a configured
cluster; LLM diagnosis needs API credentials. To fix a failed job and accept
another job's result in an exported workflow, run
`./examples/workflow-reconcile.sh`. See
[Workflow manifests](docs/WORKFLOW_MANIFESTS.md) for the details.

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](docs/FAQ.md#web-ui) and [Security model](docs/OPERATIONS.md#security-model).
A run page summarizes each `--matrix` group in a collapsible grid colored by
status, with selectable row and column parameters, so a failing parameter
combination stands out. The UI can also show browser desktop notifications
when a run finishes or a job fails; see
[Web browser notifications](docs/WEB_BROWSER_NOTIFICATIONS.md).

<!-- END GETTING STARTED -->
