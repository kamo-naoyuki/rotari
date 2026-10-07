# Examples

The repository's [examples guide](https://github.com/kamo-naoyuki/rotari/blob/main/examples/README.md)
lists the standalone scripts for job dependencies, artifacts, arrays, retrying
failed work, async runs, Slurm, workflow manifests, and diagnosis. The script
sources are shown by GitHub as code rather than downloaded as files.

## Basic example

This example adds a command directly and through `sh -c`, then runs both jobs.
From the repository root, run it with:

```sh
./examples/basic.sh
```

The [script source on GitHub](https://github.com/kamo-naoyuki/rotari/blob/main/examples/basic.sh)
is included below for convenient browsing:

```sh
--8<-- "examples/basic.sh"
```

## Jobs with dependencies

This example adds two jobs and makes the second wait for the first to succeed.
From the repository root, run:

```sh
./examples/dependencies.sh
```

The [script source on GitHub](https://github.com/kamo-naoyuki/rotari/blob/main/examples/dependencies.sh)
is included below for convenient browsing:

```sh
--8<-- "examples/dependencies.sh"
```

## Artifact example

This example demonstrates job working directories and automatically discovered
artifact candidates. From the repository root, run:

```sh
./examples/artifacts.sh
```

See the [artifact inspection guide](INSPECT.md#artifacts) for other ways to
inspect candidates and preview files.

The [script source on GitHub](https://github.com/kamo-naoyuki/rotari/blob/main/examples/artifacts.sh)
is included below for convenient browsing:

```sh
--8<-- "examples/artifacts.sh"
```
