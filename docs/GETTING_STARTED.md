# Getting started

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
[Shell completion](CONFIGURATION.md#shell-completion).

### Docker

For the published image, persistent state, and runtime requirements, see the
[Docker guide](DOCKER.md).

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

See [Projects, queues, runs, and state](CONCEPTS.md#projects-queues-runs-and-state)
for project selection and state layout, [Inspect](INSPECT.md#inspect) for
status and logs, and [Recover and rerun](RUNNING.md#recover-and-rerun) for
retries and asynchronous runs.

Use `--depends-on NAME` to run a job only after a prerequisite job or stage
succeeds:

```sh
rotari add --job-name prepare -- ./prepare.sh
rotari add --job-name train --depends-on prepare -- ./train.sh
```

Use `--depends-on-finished NAME` for aggregation or cleanup jobs that should
run once the prerequisite finishes, whatever its result. See
[Dependencies and stages](CONCEPTS.md#dependencies-and-stages) for stages and
multiple prerequisites.

## Example

```sh
./scripts/example.sh
```

The example adds a dependent job, a two-task array, and a job that
intentionally fails once. The first run therefore has failures; `rotari retry`
reruns only the failed array task and job, while carrying the successful work
forward.

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
`success (accepted)`. See [Workflow manifests](WORKFLOW_MANIFESTS.md).

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](FAQ.md#web-ui) and [Security model](OPERATIONS.md#security-model).
A run page summarizes each `--matrix` group in a collapsible grid colored by
status, with selectable row and column parameters, so a failing parameter
combination stands out. The UI can also show browser desktop notifications
when a run finishes or a job fails; see
[Web browser notifications](WEB_BROWSER_NOTIFICATIONS.md).
