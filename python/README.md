# rotari Python client

This directory contains a thin Python client for the `rotari` executable. For
CLI behavior and examples, see the repository [README](../README.md).

## Usage

Install the Python client together with the `rotari` executable using Conda:

```sh
conda install -c kamo-naoyuki rotari
```

The client is not published on PyPI. Wheels built from tagged releases bundle
the matching `rotari` executable and are available through a static
[package index](https://kamo-naoyuki.github.io/rotari/simple/) hosted on
GitHub Pages (Linux x86_64/arm64, macOS x86_64/arm64):

```sh
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari
```

This installs both the Python client and a `rotari` command on `PATH` for the
current platform; no separate Conda/Homebrew/binary install is needed. On an
unsupported platform, install the client from a checkout and place `rotari` on
`PATH` yourself.

To install the client from a checkout instead, ensure that the `rotari`
executable is available on your `PATH`, then run:

```sh
python3 -m pip install --no-deps ./python
```

Use `Rotari` to submit executable argument lists and inspect run state:

```python
from rotari import Rotari

rotari = Rotari(basedir=".rotari-state", project="experiment")
rotari.add(["./train.sh"], job_name="train")
rotari.run(async_=True)
summary = rotari.wait()
```

`wait()` and `show()` return decoded JSON objects. Other commands return a
`CommandResult` or raise `RotariError` when the command exits unsuccessfully.
Run `rotari schema --json` to inspect every dynamically generated command
option and its CLI description.

## API documentation

Build the HTML API reference from the repository root with:

```sh
python3 -m pip install "./python[docs]"
python3 -m sphinx -W -b html python/docs python/docs/_build/html
```

Open `python/docs/_build/html/index.html`. The reference obtains its method
signatures from the checked-in CLI schema, so it reflects the generated Python
interface.

## Generated CLI metadata

The CLI metadata used by completion and help can be inspected with:

```sh
rotari schema --json
```

This is the source for generating Python convenience methods without
duplicating the CLI option definitions.

The checked-in [generated_cli.py](rotari/generated_cli.py) contains only the
generated CLI schema and must not be edited manually. The handwritten
`client.py` uses that schema to build command arguments.

Run these commands from the repository root to regenerate it:

```sh
tmpdir=$(mktemp -d)
go run ./cmd/rotari schema --json > "$tmpdir/schema.json"
python3 scripts/generate_python_cli.py \
  --input "$tmpdir/schema.json" \
  --output python/rotari/generated_cli.py
```

The generated file is committed so source checkouts and source packages remain
self-contained. CI generates a second copy in a temporary directory and
compares it with the checked-in file. A difference fails CI.

Do not add Python option definitions separately from the Go metadata.
