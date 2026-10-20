# Work log: Launch commands and services from Artifacts

See [plan.md](plan.md) for the current design.

## Plan drafted and revised to registered-only commands

Commits:

- `7155104d` (2026-10-20 14:34:58 +0900): initial plan.
- `b40e81b0` (2026-10-20 14:44:50 +0900): registration model revision.

- **Change:** Added `plan.md` for launching commands and HTTP services
  (TensorBoard first) from the Artifacts view, then revised it so commands are
  registered only in a dedicated `launchers.toml` in the global config
  directory. The browser and Web API select a registered command by name; the
  editable command dialog, browser-side preset saving, built-in presets, and
  browser-supplied cwd overrides were removed. Added `rotari config
  --launchers` for a template containing the TensorBoard entry and listing the
  file in `config --list`. Narrowed the reuse key to command name, resolved
  target, and cwd, and listed CLI, MCP, and Python client launching as
  non-goals.
- **Reason:** The user wants arbitrary commands they register themselves, and
  chose file-only registration over editing in the Web UI. With file-only
  registration, a leaked token or cross-site request can start only commands
  the user wrote, not supplied command text. The file is read only from the
  global scope because one Web server mounts several basedirs, basedirs can be
  on shared storage, and executable definitions must not follow the selected
  basedir, job, or artifact contents.
- **Plan impact:** The plan is still proposed. Decided: registration file,
  scope, schema validation, startup-time loading, and a name-only start API.
  Deferred: Web UI editing and live reload. Still open from review: requiring
  authentication for `--allow-exec` on loopback listeners (other users on a
  shared login node can reach `127.0.0.1`), and a concrete way to verify that
  the readiness listener belongs to the launched process group.
- **Validation:** Documentation only; no tests run.
- **Remaining:** Settle the two open review items before Phase 1; then the
  phases in the plan.

## Unavailable-launch guidance and port ownership check

Commit: `0f8a198d` (2026-10-20 14:48:03 +0900).

- **Change:** Added a table to the plan's User flow listing each state in
  which launching is unavailable (no `--allow-exec`, `--allow-control=false`,
  no registered commands, no command for the target kind, static export) with
  the explanation and enabling step to show. The server produces the text and
  the start API returns the same reason. Added a port ownership check to
  Service access: before readiness and on proxy reconnect, confirm that the
  listening process belongs to the launch's process group, through `/proc` on
  Linux and `lsof` (or `libproc`) on macOS. If the check cannot run, the launch
  fails. Added matching acceptance items.
- **Reason:** The user accepted `--allow-exec` as a required opt-in on the
  condition that the Web UI tells them to add it and how. Linux and macOS are
  both release platforms (`release.yml`), so the port check needs a real
  implementation on each, not Linux only.
- **Plan impact:** Resolved the port ownership open item. The
  disabled-launch explanation follows the existing read-only hint ("restart
  with --allow-control to enable job control"). Still open: whether
  `--allow-exec` also requires an auth token on loopback listeners.
- **Validation:** Documentation only; no tests run.
- **Remaining:** Decide loopback authentication for `--allow-exec`; then the
  plan phases.
