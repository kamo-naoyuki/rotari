# Plan: Delete a Whole Project

**Created:** 2026-10-15
**Status:** Phases 1–4 done. See [work-log.md](work-log.md).

## Purpose

A project can be emptied but not removed. `rotari delete --all` deletes the
run history and keeps `queue.json`, `meta.json`, and the project's
configuration, so the project stays in `projects`, the Web UI's All projects
page, and the sidebar. Removing it today means deleting
`BASEDIR/projects/NAME` by hand, which leaves the project's runs in the
master run registry and skips the running and interrupted checks.

This plan adds whole-project deletion as `rotari delete --project` and as a
Delete button on each row of the Web UI's All projects page, both through one
shared operation.

## Non-goals

- MCP tools. MCP has no delete tools today.
- A Delete button on the project page or in the sidebar. The All projects
  row is the requested place; others can follow once this one exists.
- Updating other projects whose runs were copied from the deleted project's
  runs (`sources.json`, lineage). They lose their source, as they do after
  `delete --all` today.
- Removing the basedir registry entry when the last project is deleted.
- Recovering a deleted project (no trash that can be restored).

## Current implementation

- A project is the directory `BASEDIR/projects/NAME`, holding `queue.json`,
  `meta.json`, `state.lock`, `running.lock`, `runs/`,
  `.rotari-attachments/`, and the project-scope `config.{toml,yaml,json}`
  and `notifications.toml`
  ([internal/state/project_paths.go](../../internal/state/project_paths.go)).
- `project.Overviews` lists every directory under `BASEDIR/projects/` whose
  name is a valid path element, so a project with no runs and an empty queue
  is still listed ([internal/project/overview.go](../../internal/project/overview.go)).
  The CLI `projects` command and the Web API share it.
- Outside the project directory, each run has a master registry entry
  `MASTERDIR/runs/RUN_ID.json`, removed by `Editor.UnregisterRun`
  ([internal/queueops/editor.go](../../internal/queueops/editor.go)).
- `queueops.Editor.DeleteHistory` and `DeleteRun`
  ([internal/queueops/delete.go](../../internal/queueops/delete.go)) delete
  runs under `project.EditGuarded`, which holds `state.lock` and requires an
  idle project (SAFE-2, SAFE-3). `cmd/rotari/delete.go` and the Web
  `/api/clear-run` call them.
- Web mutating routes are gated by `allowControl` and return `403` in a
  read-only server or a static export
  ([contracts/05-web-assets-and-static-export.md](../../contracts/05-web-assets-and-static-export.md)).
  `renderOverview` in
  [internal/webui/assets/web_app_core.js](../../internal/webui/assets/web_app_core.js)
  draws the All projects table, whose row actions hold only Path.
- `state.lock` lives inside the project directory. On NFS, removing a
  directory that holds a file some process still has open leaves `.nfsXXXX`
  files and makes `os.RemoveAll` fail.

## Decisions

- **D1 What is deleted.** The whole `BASEDIR/projects/NAME` directory: queue,
  metadata, locks, run history, attachment state, and the project-scope
  configuration and notification files. The master registry entries of the
  project's runs are removed too. Basedir, workspace, and global
  configuration are left alone.
- **D2 CLI shape.** `rotari delete --project -p NAME`. `--project` is a
  command-line-only boolean. The project must be named by `--project-name/-p`
  on the command line; a project that would come from `ROTARI_PROJECT_NAME`,
  `.rotari.toml`, or a configuration file is refused with a message asking
  for `-p`, since deleting a project must never be a default. `--project`
  cannot be combined with a positional `RUN_ID`, `--run-id`, or `--all`
  (usage error). The positional keeps meaning `RUN_ID` (SEL-10: an option
  never turns a positional into a project). `--dry-run` and `--if-revision`
  work as for other guarded commands (CLI-7).
- **D3 Safety.** The operation runs under `project.EditGuarded` with
  operation name `delete`, so a running or interrupted project is rejected
  with the existing SAFE-2 / SAFE-3 messages and nothing changes. A project
  that does not exist is an error, with the CLI-11 missing-project
  diagnostics. No confirmation prompt, matching `delete --all`; the Web UI
  asks with `confirm()` as Delete run does.
- **D4 Removal order.** Under the state lock: list the run IDs, then rename
  the project directory to `BASEDIR/.rotari-deleted/NAME-RANDOM/NAME`,
  outside `projects/`, so it leaves every listing atomically. After
  releasing the lock, remove the renamed directory, then unregister the
  runs. A failed unregister leaves orphan registry entries that `gc`
  removes; the error says so. A failed final removal (for example
  `.nfsXXXX` files) still reports the project deleted, with a warning
  naming the leftover directory.
- **D5 Concurrent commands.** A command that was waiting on the old
  `state.lock` acquires it after the rename and then finds no project
  directory. It fails with its write error, or, for commands that create a
  project (`add`, `import`), creates a new empty project. This is accepted
  and not specially handled.
- **D6 One implementation.** `queueops.Editor.DeleteProject(baseDir,
  projectName string) (string, error)` in
  [internal/queueops/delete.go](../../internal/queueops/delete.go) owns the
  operation. `cmd/rotari/delete.go` and the Web handler only validate their
  inputs and call it. The message is
  `deleted project=NAME runs=N directory=PATH`.
- **D7 Web.** `POST /api/delete-project` with `{"project_name": NAME}`,
  gated by `allowControl`, using the request's basedir as `/api/clear-run`
  does, with the project name checked by `state.IsValidPathElement`. Each
  All projects row gets a Delete button after Path. It is disabled, with a
  title saying why, when the row shows a running run; an interrupted project
  is rejected by the server and the error is shown. After success the page
  reloads the project list. The static export shows the button as WEB-9
  requires, and the route answers the read-only `403`.
- **D8 Python client.** The generated CLI wrapper picks up `--project` when
  `python/rotari/generated_cli.py` is regenerated; nothing else changes.

## Phases

### Phase 1: Shared operation (done)

1. `Editor.DeleteProject` per D1, D3, D4, D6, with dry-run support.
2. Unit tests in `internal/queueops/delete_test.go`, with two projects that
   each have runs and registry entries: deletion removes only the target's
   directory and registry entries; a dry run changes nothing; running and
   interrupted projects are rejected unchanged; a missing project fails; a
   stale `--if-revision` fails; names with `/` and `\` are rejected.

### Phase 2: CLI (done)

The `delete --project` pairs of `TestCLIFlagPairMutations` get
`--project-name` from the adapter unless it is under test, and accept the
usage rejection with `--all` or `--run-id`. SEL-10 rows cover the
exclusions and a missing project.

1. `--project` in `cmd/rotari/delete.go` and the `delete` entry of
   `cmd/rotari/cli_spec.go` (D2).
2. Tests for the D2 refusals: implicit project from the environment and from
   `.rotari.toml`; combination with a positional run ID, `--run-id`, and
   `--all`.
3. Regenerate `docs/CLI_REFERENCE.md` and `python/rotari/generated_cli.py`;
   update the CLI golden files (`go test ./conformance -run Golden
   -update`), the flag-pair inventory in
   `conformance/03-interfaces/flag_pairs_test.go`, and
   `docs/internal/testing/cli-flag-pair-coverage.md`.

### Phase 3: Web (done)

The route was also added to `TestWebAPIRejectsUnsafePathElements`
(RES-9, RES-10). The system Node (v12) cannot run the page scripts; the
jsdom tests were run with the VS Code server's Node 24 first on `PATH`.

1. `/api/delete-project` in [internal/webui/webui.go](../../internal/webui/webui.go) (D7).
2. `deleteProject` in `web_app_actions.js` and the row button in
   `renderOverview`.
3. Tests in `internal/webui/webui_test.go`: the route deletes through the
   shared operation, rejects a running project, rejects invalid names, and
   returns `403` with `--allow-control=false`; the All projects row has the
   button, disabled for a running project. The local Node cannot run some
   webui JS tests (see repository memory); report which ones did not run.

### Phase 4: Contracts and documentation (done)

`delete --project` joined `support.GuardedCommands` and the SAFE-2 test's
commands, so `TestInterruptedProjectNeedsRecovery` and
`TestRunningProjectRejectsSecondRunAndDelete` also cover SAFE-12. The
conformance test checks WEB-11 through the Web API; the button itself is
checked by `TestWebAllProjectsDeleteProject`.

1. New **SAFE-12**: `delete --project` and `/api/delete-project` remove the
   project directory and its runs' registry entries, leave other projects
   alone, require an explicit project on the CLI, and are rejected for a
   running or interrupted project. Conformance test through the binary and
   the Web API in `conformance/04-coordination/`, with two projects, runs in
   each, and a running project; `covers(t, "SAFE-12")` and a row in
   `contracts/README.md`.
2. Extend SAFE-2 / SAFE-3 and the state table in
   `contracts/04-coordination-and-safety.md` to name `delete --project`.
3. Update the `delete` row of SEL-10 in `contracts/06-selectors.md` (add
   `--project` to Excludes) and its row in
   `conformance/06-selectors/positional_test.go`.
4. New **WEB-11** in `contracts/05-web-assets-and-static-export.md` for the
   All projects Delete button and its route, with a README row.
5. Documentation: the delete section of `docs/RUNNING.md`, a FAQ entry on
   removing a project in `docs/FAQ.md`, and the `queueops` and Web lines of
   `docs/ARCHITECTURE.md`. `README.md` is not edited unless
   `docs/GETTING_STARTED.md` changes.

## Validation

- `go test ./internal/queueops ./cmd/rotari ./internal/webui` with `-v` for
  the new tests.
- `go test ./conformance` (Golden, SAFE-12, SEL-10, flag pairs).
- `go test ./internal/doclinks` for the documentation links.
- `scripts/check.sh --short`, then `scripts/check.sh` as far as the host
  allows (npm is missing here; run its Go steps by hand).
- A manual check with `ROTARI_MASTERDIR` and the basedir under one `$TMPDIR`
  work directory: create two projects, run one, delete it from the CLI and
  the other from the Web UI, and confirm `projects`, `runs`, and the Web UI.
