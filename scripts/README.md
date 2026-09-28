# Repository scripts

These scripts support development, testing, documentation, releases, and
examples. They are not part of the `rotari` command-line interface.

## Examples

- `example.sh`: run a local example batch with dependencies, an array, and a
  deliberate failure.
- `example-workflow.sh`: exercise workflow manifest import and export.
- `example-diagnose.sh`: demonstrate failure diagnosis.

## Documentation and generated assets

- `generate-demos.sh`: record the terminal demo GIFs.
- `generate-static-web.sh`: build the static web demo.
- `generate_go_docs.py`: generate the Go API HTML reference.
- `generate_cli_reference.py`: generate the CLI and environment reference from
  the CLI schema.
- `generate_python_api_docs.py`: generate Python client option documentation
  for all public methods from the CLI schema.
- `generate_python_cli.py`: generate the Python CLI wrapper from the CLI schema.
- `generate_pypi_index.py`: generate the static package index used by GitHub Pages.
- `sync_readme.py`: synchronize `docs/GETTING_STARTED.md` into the GitHub README.

Run `python3 scripts/sync_readme.py --check` to verify that the README is in
sync with the Getting Started guide.

## Updating documentation

Use the source documents as the editing locations:

- User onboarding: `docs/GETTING_STARTED.md`
- CLI and environment reference: CLI schema in `cmd/rotari/cli_spec.go` and
  `cmd/rotari/environment.go`
- Python client usage: `docs/PYTHON_CLIENT.md`
- Go and Python API output: the existing generators and CLI schema

After changing CLI or environment metadata, regenerate the checked-in views:

```sh
tmpdir=$(mktemp -d)
go run ./cmd/rotari schema --json > "$tmpdir/schema.json"
python3 scripts/generate_python_cli.py \
  --input "$tmpdir/schema.json" \
  --output python/rotari/generated_cli.py
PYTHONPATH=python python3 scripts/generate_cli_reference.py
PYTHONPATH=python python3 scripts/generate_python_api_docs.py
python3 scripts/sync_readme.py
```

For documentation-only edits, run the relevant generator after editing the
source document. Before committing, run the complete local validation:

```sh
PYTHONPATH=python python3 scripts/generate_cli_reference.py --check
PYTHONPATH=python python3 scripts/generate_python_api_docs.py --check
python3 scripts/sync_readme.py --check
PYTHONPATH=python python3 -m mkdocs build --strict --site-dir "$TMPDIR/mkdocs-site"
```

CI runs these synchronization checks and the strict MkDocs build for pull
requests and pushes to `main`.

## Checks and integration

- `check.sh`: run the repository's full validation checks.
- `scheduler-integration.sh`: exercise scheduler integration tests.
- `loadtest.sh`: run the web/API load test.
- `loadtest-job.sh`: submit a load-test job.

## Other tooling

- `generate-dep-graph.sh`: generate the Go package dependency graph.
- `install.sh`: install a released `rotari` binary.
- `templates/`: templates used by the documentation generators.
