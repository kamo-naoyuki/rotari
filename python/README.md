# rotari Python client

This directory contains a thin Python client for the `rotari` executable. For
CLI behavior and examples, see the repository [README](../README.md).

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
