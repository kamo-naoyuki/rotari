# Repository scripts

These scripts support development, testing, documentation, and releases. They
are not part of the `rotari` command-line interface. Runnable samples are in
[examples](../examples/README.md).

## Documentation and generated assets

- `generate-demos.sh`: record the terminal demo GIFs.
- `generate-static-web.sh`: build the static web demo.
- `screenshot-web.sh`: screenshot every page of the static web demo at desktop
  and phone widths in light and dark, for reviewing Web UI changes; needs
  Chrome or Chromium and Node 22 (`screenshot-web.mjs` drives the browser).
- `web-structure.sh`: record every Web UI page's title, toolbars, headings, and
  tables, static and live, for one state directory; diff the output of two
  builds to see what a Web UI change did to the pages. `DEMO_WORK_DIR=DIR
  generate-static-web.sh DIR/static` keeps a demo state in `DIR/state` for it.
  `web-browser.mjs` is the headless Chrome helper both scripts share.
- `demo_artifacts.py`: write the static web demo's sample artifact files
  (images, audio, video, tables, text, NumPy arrays) with the standard
  library; the demo's video clip is `templates/demo/clip.mp4`.
- `generate_go_docs.py`: generate the Go API HTML reference.
- `generate_cli_reference.py`: generate the CLI and environment reference from
  the CLI schema.
- `generate_python_api_docs.py`: generate Python client option documentation
  for all public methods from the CLI schema.
- `generate_python_cli.py`: generate the Python CLI wrapper from the CLI schema.
- `generate_pypi_index.py`: generate the static package index used by GitHub Pages.
- `mkdocs_llms.py`: generate `llms.txt` and `llms-full.txt` from rendered pages during
  the MkDocs build.
- `sync_readme.py`: synchronize `docs/GETTING_STARTED.md` into the GitHub README.

Run `python3 scripts/sync_readme.py --check` to verify that the README is in
sync with the Getting Started guide.

## Updating documentation

Use the source documents as the editing locations:

- User onboarding: `docs/GETTING_STARTED.md`
- CLI and environment reference: CLI schema in `cmd/rotari/cli_spec.go` and
  `cmd/rotari/environment.go`, generated into `docs/CLI_REFERENCE.md` and
  `docs/ENVIRONMENT_VARIABLES.md`
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
test -s "$TMPDIR/mkdocs-site/llms.txt"
test -s "$TMPDIR/mkdocs-site/llms-full.txt"
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

Dependency graph generation requires Graphviz (`dot`). If no `goda` binary is
available, the script installs `goda v0.7.1`, which supports the project's Go
1.23 minimum without downloading a newer Go toolchain. Keep that compatibility
when updating the pinned version. Set `GODA_BINARY` to reuse a specific binary.

Run `python3 -m unittest discover -s scripts -p test_generate_dep_graph.py -v`
to check the dependency graph bootstrap without downloading tools.
