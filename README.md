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

## Documentation
- [Documentation site](https://kamo-naoyuki.github.io/rotari/docs/)

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
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari
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
export ROTARI_PROJECT_NAME=sweep
rotari add -- python train.py --lr 0.1
rotari add -- python train.py --lr 0.01
rotari add -- python train.py --lr 0.001
rotari run
rotari jobs
rotari show
```

Use `rotari run --async` when the run should continue in the background. To
inspect failed logs and retry only failed or unfinished work:

```sh
rotari show -p sweep --failed-logs
rotari retry -p sweep
```

See [Concepts](docs/CONCEPTS.md) for projects, queues, runs, and dependencies.

## Example

```sh
./scripts/example.sh
```

The example adds a dependent job, a two-task array, and a job that
intentionally fails once. The first run therefore has failures; `rotari retry`
reruns only the failed array task and job, while carrying the successful work
forward.

To run the array job through Slurm instead, pass the optional flag:

```sh
./scripts/example.sh --slurm
```

For the workflow manifest flow, run:

```sh
./scripts/example-workflow.sh
```

See [Workflow manifests](docs/WORKFLOW_MANIFESTS.md) for the manifest workflow.

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](docs/FAQ.md#web-ui) and [Security model](docs/OPERATIONS.md#security-model).

<!-- END GETTING STARTED -->
