<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/rotari-logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/rotari-logo-light.svg">
  <img src="docs/assets/rotari-logo-light.svg" alt="rotari logo">
</picture>

---

[![web demo](https://img.shields.io/website?url=https%3A%2F%2Fkamo-naoyuki.github.io%2Frotari%2F&label=web%20demo&style=flat)](https://kamo-naoyuki.github.io/rotari/) [![Go API](https://img.shields.io/badge/Go%20API-go%20doc-00ADD8)](https://kamo-naoyuki.github.io/rotari/go-api/) [![Python API](https://img.shields.io/badge/Python%20API-Sphinx-3776AB)](https://kamo-naoyuki.github.io/rotari/python-api/) [![Go CI](https://github.com/kamo-naoyuki/rotari/actions/workflows/ci.yml/badge.svg)](https://github.com/kamo-naoyuki/rotari/actions/workflows/ci.yml) [![Slurm + PBS CI](https://img.shields.io/github/actions/workflow/status/kamo-naoyuki/rotari/scheduler-integration.yml?branch=main&label=Slurm%20%2B%20PBS%20CI)](https://github.com/kamo-naoyuki/rotari/actions/workflows/scheduler-integration.yml) [![codecov](https://codecov.io/gh/kamo-naoyuki/rotari/graph/badge.svg)](https://codecov.io/gh/kamo-naoyuki/rotari) [![SonarCloud Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=kamo-naoyuki_rotari&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=kamo-naoyuki_rotari)

<div align="center">

[[Documentation]](#documentation)

</div>


**Rotari turns trial-and-error into a repeatable loop**: build a batch of jobs from the CLI, see which failed, fix only their commands, and run it again — without losing the history of what already worked.

Rotari is an **execution manager for researchers who run batches of experiments**, with commands and shell scripts as the building blocks and simple dependencies between them. It is a single binary with no daemon, database, or server to set up; state is kept in the filesystem.

**The same batch runs on your workstation, over SSH, or on a shared Slurm, PBS, or LSF cluster.** Rotari submits and tracks scheduler jobs itself, supports array and matrix jobs, and keeps logs and status consistent across backends, so you do not need to rebuild your environment as containers or a separate cluster service. If you've used [Kaldi](https://github.com/kaldi-asr/kaldi)'s or [ESPnet](https://github.com/espnet/espnet)'s `run.pl`/`queue.pl`, the basic idea should feel familiar.


## How is rotari different?

| Plain shell (background jobs) | rotari |
| --- | --- |
| <img src="https://kamo-naoyuki.github.io/rotari/demo-shell.gif" alt="shell background jobs demo" width="400"> | <img src="https://kamo-naoyuki.github.io/rotari/demo-rotari.gif" alt="rotari demo" width="400"> |


Rotari deliberately stays out of the way. **You don't need a separate workflow language:** write the commands as you normally would in a shell script, and rotari provides the execution, parallelism, logs, status, and run history around them. When a queue needs to be reproduced or edited as a unit, rotari can also export and import a constrained YAML, TOML, or JSON manifest; commands remain argument arrays rather than a new scripting language.

Workflow systems are usually a good fit once a pipeline has settled. Rotari is for the stage before that, while you are still finding out which commands and settings work.

* [**Snakemake**](https://github.com/snakemake/snakemake) is built around rules, inputs, outputs, and dependencies. It fits when the structure of the pipeline is known and worth formalizing. **Rotari fits while that structure is still changing, and the shell script you already have is the workflow.**

* [**Nextflow**](https://github.com/nextflow-io/nextflow) provides a DSL for describing processes, dataflow, and workflows. It fits pipelines that are shared, reproduced, and run at scale. **Rotari fits experiments you are still changing: failed commands are fixed and rerun directly, without first translating them into a dataflow language.**

* [**Airflow**](https://github.com/apache/airflow), [**Prefect**](https://github.com/PrefectHQ/prefect), and [**Dagster**](https://github.com/dagster-io/dagster) orchestrate workflows expressed as programs, typically production pipelines that run on a schedule and need monitoring. **Rotari fits batches you start by hand, check, fix, and run again.**

* [**Dagu**](https://dagu.sh/) is a single-binary workflow engine with file-based state, a Web UI, scheduling, and event triggers. It fits workflows you define once in YAML and then operate. **Rotari fits the stage before that: a batch you are still changing, where failed jobs are fixed one by one and rerun while the history of what already worked is kept.**

The goal is not to replace shell scripts or these workflow systems. **It is to make the trial-and-error loop around the commands you already use manageable, and let the commands remain the workflow.**

## Installation

Prebuilt binaries for Linux and macOS are on the
[GitHub Releases](https://github.com/kamo-naoyuki/rotari/releases) page; Go is
not required:

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -fL "https://github.com/kamo-naoyuki/rotari/releases/latest/download/rotari-${os}-${arch}" \
  -o /tmp/rotari
install -m 755 /tmp/rotari ~/.local/bin/rotari
```

Other options:

```sh
brew install kamo-naoyuki/tap/rotari   # Homebrew / Linuxbrew
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari  # bundles the executable
go install github.com/kamo-naoyuki/rotari/cmd/rotari@latest  # from source
```

Run `rotari version` to check the installed version, and
`rotari completion install` to set up shell completion for Bash, Zsh, or Fish
(see [Shell completion](docs/CONFIGURATION.md#shell-completion)).

## Quick start

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

Add commands first, then run the queue explicitly. Use `rotari run --async` when
the run should continue in the background. To inspect failed-job logs and retry
only failed or unfinished work:

```sh
# Show logs for failed jobs in the selected project.
rotari show -p sweep --failed-logs
# Start a new run for failed and unfinished jobs; successful jobs are reused.
rotari retry -p sweep
```

`jobs` gives a compact status overview across projects. `show` provides details
for a project, run, or job, including logs and saved results.

See [Projects, queues, runs, and state](docs/CONCEPTS.md#projects-queues-runs-and-state) for
project selection and state layout, [Inspect](docs/INSPECT.md#inspect) for status and logs,
[Recover and rerun](docs/RUNNING.md#recover-and-rerun) for retries, and [Async runs](docs/RUNNING.md#async-runs)
for background execution.

Use `--depends-on NAME` to run a job only after a prerequisite job or stage
succeeds:

```sh
rotari add --job-name prepare -- ./prepare.sh
rotari add --job-name train --depends-on prepare -- ./train.sh
```

See [Dependencies and stages](docs/CONCEPTS.md#dependencies-and-stages) for
stages and multiple prerequisites.

## Commands at a glance

| Command | Purpose |
| --- | --- |
| `add` | Add a command to the current queue. |
| `run` | Run the queued jobs. |
| `reset` | Discard the current queue. |
| `show` | Show queue, run, job, or log details. |
| `jobs` | List running and recently finished jobs across projects. |
| `web` | Start the local web status UI. |
| `retry` | Rerun failed or unfinished jobs. |
| `cancel` | Cancel running jobs. |
| `wait` | Wait for an asynchronous run to finish. |
| `config` | Create or inspect configuration files. |
| `suspend` / `resume` | Suspend or resume running jobs. |
| `copy` | Copy jobs from a saved run into the queue. |
| `change` | Change a queued or restored job. |
| `export` | Export a queue, saved runs, or a starter workflow manifest. |
| `import` | Validate a workflow manifest and replace the current queue. |
| `remove` | Remove jobs from the queue. |
| `delete` | Delete saved run history. |
| `check` | Check whether a project is ready to run. |
| `diagnose` | Diagnose a failed job. |
| `env` | Show CLI and job environment variables. |
| `completion` | Generate shell completion scripts. |
| `gc` | Find and remove orphaned run registry entries. |
| `unlock` | Recover a confirmed stale run lock. |
| `server` | Inspect or control the project server. |
| `schema` | Print the machine-readable CLI schema. |

## Common options

Frequently used options have short forms:

| Long option | Short option |
| --- | --- |
| `--project-name` | `-p` |
| `--basedir` | `-b` |
| `--run-id` | `-r` |
| `--job-id` | `-j` |
| `--executor` | `-e` |

## Example

```sh
./scripts/example.sh
```

The example adds a dependent job, a two-task array,
and a job that intentionally fails once. The first run therefore has failures;
`rotari retry` reruns only the failed array task and job, while carrying the
successful work forward.

To run the array job through Slurm instead, pass the optional flag. The local
`prepare` and `failing-job` jobs remain local:

```sh
./scripts/example.sh --slurm
```

The first positional argument selects the project name, for example
`./scripts/example.sh --slurm scheduler-demo`.

For the workflow manifest flow, run:

```sh
./scripts/example-workflow.sh
```

It imports a small manifest with a stage, a matrix, and two failing jobs, then
exports the finished run, fixes one job's command and accepts the other's
failed result in the exported file, and imports it again. The second run
executes only the fixed job; the others are reused or recorded as
`success (accepted)`. See [Workflow manifests](docs/WORKFLOW_MANIFESTS.md).

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](docs/FAQ.md#web-ui) and [Security model](docs/OPERATIONS.md#security-model).
The UI can also show browser desktop notifications when a run finishes or a job
fails; see [Web browser notifications](docs/WEB_BROWSER_NOTIFICATIONS.md).

## Documentation

- [Concepts](docs/CONCEPTS.md): projects, queues, runs, IDs, dependencies and
  stages, and state and project resolution.
- [Executors and schedulers](docs/EXECUTORS.md): local, SSH, Slurm, PBS, and
  LSF execution, plus array and matrix jobs.
- [Workflow manifests](docs/WORKFLOW_MANIFESTS.md): export, edit, and import a
  run as a declarative file.
- [Running and recovering](docs/RUNNING.md): async runs, reruns and retries,
  and queue and job control.
- [Inspecting and diagnosing](docs/INSPECT.md): status, logs, readiness
  checks, and failure diagnosis.
- [Configuration](docs/CONFIGURATION.md): config files, environment variables,
  run completion webhooks, and shell completion.
- [Operations](docs/OPERATIONS.md): server management, run registry
  maintenance, shared filesystems, and the security model.
- [FAQ](docs/FAQ.md): short answers about rotari's behavior.
- [Python client](python/README.md): installation, usage, and API
  documentation for the Python interface.
- Integrations: [webhook notifications](docs/WEBHOOK_NOTIFICATIONS.md),
  [LLM diagnosis](docs/LLM_DIAGNOSIS.md), and
  [local diagnosis rules](docs/LOCAL_DIAGNOSIS.md).

## Development

See [rotari internals](docs/INTERNALS.md) for the architecture, persistent-state
contracts, resolution rules, and code ownership used by maintainers and coding agents.
