<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/rotari-logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/rotari-logo-light.svg">
  <img src="docs/assets/rotari-logo-light.svg" alt="rotari logo">
</picture>

---

[![web demo](https://img.shields.io/website?url=https%3A%2F%2Fkamo-naoyuki.github.io%2Frotari%2F&label=web%20demo&style=flat)](https://kamo-naoyuki.github.io/rotari/) [![Documentation](https://img.shields.io/badge/docs-MkDocs%20Material-526CFE)](https://kamo-naoyuki.github.io/rotari/docs/) [![Go API](https://img.shields.io/badge/Go%20API-go%20doc-00ADD8)](https://kamo-naoyuki.github.io/rotari/go-api/) [![Go CI](https://github.com/kamo-naoyuki/rotari/actions/workflows/ci.yml/badge.svg)](https://github.com/kamo-naoyuki/rotari/actions/workflows/ci.yml) [![Slurm + PBS CI](https://img.shields.io/github/actions/workflow/status/kamo-naoyuki/rotari/scheduler-integration.yml?branch=main&label=Slurm%20%2B%20PBS%20CI)](https://github.com/kamo-naoyuki/rotari/actions/workflows/scheduler-integration.yml) [![codecov](https://codecov.io/gh/kamo-naoyuki/rotari/graph/badge.svg)](https://codecov.io/gh/kamo-naoyuki/rotari) [![SonarCloud Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=kamo-naoyuki_rotari&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=kamo-naoyuki_rotari)

<div align="center">

[[Documentation]](#documentation)

</div>


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

## Installation and quick start

See the [Getting started guide](https://kamo-naoyuki.github.io/rotari/docs/getting-started/)
for installation, Docker, and the first run. It is the single maintained
source for those instructions.

## Commands at a glance

| Command | Purpose |
| --- | --- |
| `add` | Add a command to the current queue. |
| `run` | Run the queued jobs. |
| `reset` | Discard the current queue. |
| `show` | Show queue, run, job, or log details. |
| `jobs` | List running and recently finished jobs across projects. |
| `diff` | Compare two runs: fixed, still failing, and newly failing jobs, and changed definitions. |
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
| `guide` | Print a usage guide for coding agents. |

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
A run page summarizes each `--matrix` group in a collapsible grid colored by
status, with selectable row and column parameters, so a failing parameter
combination stands out; clicking a cell offers that job's actions, such as its
log, report, or cancel. The UI can also show browser desktop notifications when a run finishes or a job
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
- [Comparison with other tools](docs/TOOL_COMPARISON.md): shell background
  jobs, GNU Parallel, pueue, task-spooler, submitit, scheduler scripts, and
  workflow engines such as Snakemake, Nextflow, Dagu, and Airflow.
- [Python client](python/README.md): installation, usage, and API
  documentation for the Python interface.
- Integrations: [webhook notifications](docs/WEBHOOK_NOTIFICATIONS.md),
  [LLM diagnosis](docs/LLM_DIAGNOSIS.md), and
  [local diagnosis rules](docs/LOCAL_DIAGNOSIS.md).

## Development

See [code architecture](docs/ARCHITECTURE.md) for how the processes and
packages fit together, and [design contracts](contracts/README.md) for the
persistent-state, resolution, and safety rules that maintainers and coding
agents must preserve.
