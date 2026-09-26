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

## Start an interactive container

Mount your working directory and a persistent state directory, then open a
shell in the container. Rotari and the commands you add run inside this
container, where you can use Rotari normally:

```sh
mkdir -p .rotari-state
docker run --rm -it --user "$(id -u):$(id -g)" \
  -v "$PWD:/workspace" \
  -v "$PWD/.rotari-state:/state" \
  -w /workspace \
  -e ROTARI_BASEDIR=/state \
  -e ROTARI_MASTERDIR=/state/master \
  --entrypoint /bin/bash \
  kamonaoyuki/rotari:latest
```

Inside the container, run Rotari commands as usual:

```sh
rotari add -- sh -c 'echo hello from rotari'
rotari run
rotari show
```

The `.rotari-state` directory holds the queue, run history, and registry. The
image defaults to a non-root user. The example maps the host UID/GID so Rotari
can write to bind-mounted directories. The state and working files remain on
the host after the container exits.

## Runtime considerations

- Jobs run inside this container. Mount input/output files and install any
  additional language runtimes needed by your commands.
- Keep the container running while an asynchronous run is active. Exiting the
  shell stops the container, which also stops its worker processes.
- Scheduler executables and SSH credentials are environment-specific and are
  not included. Provide them in a custom image and configure their access
  separately.
