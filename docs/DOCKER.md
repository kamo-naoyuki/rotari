# Docker

Rotari is distributed primarily as a native binary. A Docker image is also
available for trying Rotari locally or embedding it in a container-based
workflow.

## Published image

Each version release publishes `kamonaoyuki/rotari` to Docker Hub for Linux
AMD64 and ARM64. Images are tagged with the release version and (for stable
releases) `latest`.

The image contains Rotari, Bash, and the OpenSSH client. It does not bundle job
language runtimes or Slurm, PBS, or LSF clients. Use a custom image when jobs
need additional software or scheduler commands.

## Try it in the current directory

Create a local state directory and define a shell function so each invocation
shares the workspace and persistent state:

```sh
mkdir -p .rotari-state
rotari() {
  docker run --rm --user "$(id -u):$(id -g)" \
    -v "$PWD:/workspace" \
    -v "$PWD/.rotari-state:/state" \
    -w /workspace \
    -e ROTARI_BASEDIR=/state \
    -e ROTARI_MASTERDIR=/state/master \
    kamonaoyuki/rotari:latest "$@"
}

rotari add -- sh -c 'echo hello from rotari'
rotari run
rotari show
```

The `.rotari-state` directory holds the queue, run history, and registry. The
image defaults to a non-root user. The example maps the host UID/GID so Rotari
can write to bind-mounted directories.

## Runtime considerations

- Jobs run inside this container. Mount input/output files and install any
  additional language runtimes needed by your commands.
- `rotari run --async` is not suitable for the one-command-per-container
  pattern above: Docker stops the detached worker when the container exits.
  Keep a container running for asynchronous jobs.
- Scheduler executables and SSH credentials are environment-specific and are
  not included. Provide them in a custom image and configure their access
  separately.
