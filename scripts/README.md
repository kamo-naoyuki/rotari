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

## Checks and integration

- `check.sh`: run the repository's full validation checks.
- `scheduler-integration.sh`: exercise scheduler integration tests.
- `loadtest.sh`: run the web/API load test.
- `loadtest-job.sh`: submit a load-test job.

## Other tooling

- `generate-dep-graph.sh`: generate the Go package dependency graph.
- `install.sh`: install a released `rotari` binary.
- `templates/`: templates used by the documentation generators.
