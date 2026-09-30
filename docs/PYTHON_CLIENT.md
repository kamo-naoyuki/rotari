# Python client

The Python client is a thin wrapper around the `rotari` executable. It submits
argument lists to the CLI and decodes machine-readable results; it does not
serialize Python functions or closures.

## Installation

Tagged releases publish wheels containing the matching `rotari` executable.
Install the package for the current platform from the static package index:

```sh
python3 -m pip install --index-url https://kamo-naoyuki.github.io/rotari/simple/ rotari
```

For an unsupported platform or a source checkout, make sure `rotari` is on
`PATH` and install the client without dependencies:

```sh
python3 -m pip install --no-deps ./python
```

## Usage

Create a client, add executable argument lists, start a run, and wait for its
JSON summary:

```python
from rotari import Rotari

rotari = Rotari(basedir=".rotari-state", project="experiment")
rotari.add(["./train.sh"], job_name="train")
rotari.run(async_=True)
summary = rotari.wait()
```

Workflow manifests can be exchanged as Python dictionaries without creating
an intermediate file. `export(as_dict=True)` returns a JSON-compatible dict,
and `import_()` accepts one:

```python
manifest = rotari.export(as_dict=True)
manifest["jobs"][0]["name"] = "updated-train"
rotari.import_(manifest, overwrite=True)
```

The trailing underscore in `import_()` avoids Python's reserved `import`
keyword. The client sends the manifest as JSON over stdin to the CLI.

`wait()` and `show()` return decoded JSON objects. Other commands return a
`CommandResult`; command failures raise `RotariError`.

Command options are generated from the CLI schema. To inspect the complete
schema and descriptions:

```sh
rotari schema --json
```

See the [Python API reference](python-api.md) for every client command and its
schema-generated options.
