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

See [Concepts](CONCEPTS.md) for projects, queues, runs, and dependencies.

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

See [Workflow manifests](WORKFLOW_MANIFESTS.md) for the manifest workflow.

## Local web UI

Start the local web status UI separately from the job runner:

```sh
rotari web
```

See the [web demo](https://kamo-naoyuki.github.io/rotari/) for a read-only UI
using generated example data. For remote access, authentication, and read-only
mode, see the [FAQ](FAQ.md#web-ui) and [Security model](OPERATIONS.md#security-model).
