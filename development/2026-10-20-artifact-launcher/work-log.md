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

## Port ownership verification dropped

Commit: `e1459fc8` (2026-10-20 14:55:39 +0900).

- **Change:** Removed the per-OS port ownership check (`/proc` on Linux,
  `lsof` on macOS) from the plan's Service access section, its acceptance
  item, and the Phase 1 checkpoint. Rotari still allocates the service port
  (bind port 0 on loopback, then pass the port to the command). A service
  is ready only when the readiness probe succeeds and its process is still
  running afterwards. Documented the remaining window as a known limitation.
- **Reason:** The user judged ownership verification excessive for a
  personal-use feature. A service that loses the port normally fails to bind
  and exits, which the alive-after-probe rule reports as a startup failure.
  Rotari still has to pick the port because the proxy must know the
  upstream, and a port fixed in `launchers.toml` would limit each command to
  one instance.
- **Plan impact:** Reverses the port check decided in `0f8a198d` and removes
  two per-OS implementations from Phase 1. Still open: whether `--allow-exec`
  also requires an auth token on loopback listeners.
- **Validation:** Documentation only; no tests run.
- **Remaining:** Decide loopback authentication for `--allow-exec`; then the
  plan phases.

## Token required for launching on every listener

Commit: `d34f6cc9` (2026-10-20 14:56:52 +0900).

- **Change:** Access boundaries now require token authentication whenever
  `--allow-exec` is set, on loopback listeners too. Without `--auth-token` or
  `ROTARI_WEB_AUTH_TOKEN`, the server generates a random token and prints it
  on stderr with sign-in instructions (Basic auth, user `rotari`). The plan
  recommends `ROTARI_WEB_AUTH_TOKEN` for a stable token, since `--auth-token`
  is visible through `ps`. Updated the disabled-launch hint and added an
  acceptance item covering generation, rejection of unauthenticated requests
  including the proxy, and not printing a configured token.
- **Reason:** The user accepted mandatory tokens. On a shared login node
  other users can reach `127.0.0.1`. Without a token they could start the
  user's registered commands and view the services proxied from them.
- **Plan impact:** Resolves the last open review item. Servers without
  `--allow-exec` keep their current authentication behavior.
- **Validation:** Documentation only; no tests run.
- **Remaining:** Phase 1 of the plan.

## Feasibility gates and restored port ownership check

Commits:

- `191ba21c` (2026-10-20 15:09:36 +0900): feasibility gates.
- `530761b7` (2026-10-20 15:18:46 +0900): ownership and permission checks.

- **Change:** Phase 1 now ends with pass/fail gates, with matching
  acceptance items:
  - a process that wins the allocated-port race is never shown as ready or
    proxied, even while the launched service is still alive;
  - forcibly killing the Web process cleans up its launcher process groups;
  - `launchers.toml` cannot be written or replaced by another OS user.

  To meet the first gate, Service access again verifies, before the
  readiness probe, that the listener on the allocated port belongs to the
  launch's process group: through `/proc` on Linux and through `lsof` plus
  `getpgid` on macOS. The launch fails if the check cannot run. Ownership is
  re-verified before each new upstream connection to cover the window
  between the service exiting and the manager observing it. Registered
  commands gained OpenSSH `StrictModes`-style checks: the owner and mode of
  the file and of every containing directory, applied through symlinks, with
  the file read through the checked descriptor.
- **Reason:** The user made the port race a hard gate, which contradicted the
  known-limitation wording left from `e1459fc8`, and chose to keep the gate
  (option 1) rather than relax it. Directory permissions are included because
  a writable directory lets another user rename a different file into place.
- **Plan impact:** Reverses `e1459fc8`; Phase 1 again includes per-OS
  ownership checks, now with the per-connection re-check, and gains the
  launcher file permission checks.
- **Validation:** Documentation only; no tests run.
- **Remaining:** Phase 1 of the plan.

## Phase 1: launcher process lifecycle and ownership checks

Commit: `d4ea8a1c` (2026-10-20 16:26:47 +0900).

- **Change:** Added `internal/launcher`. It is not called yet.
  - `Start`/`Process` run `/bin/sh -c COMMAND` under the hidden wrapper
    `RunWrapper` (`rotari __launch`, dispatch to come in Phase 2). The
    wrapper puts the command in a process group of its own. It stops that
    group (SIGTERM, then SIGKILL after a grace period) on Stop, when the
    shell exits and leaves members behind, or when a lifetime pipe held
    only by the Web process reports EOF.
  - `AllocatePort` picks the service port. `VerifyListener` requires a
    listener on exactly `127.0.0.1:PORT` owned by the launch's group, and
    no other listener that accepts loopback connections: `/proc` on Linux,
    `lsof` on macOS, an error elsewhere.
  - `OpenTrustedFile` applies StrictModes-style checks to every element of
    the path and its symlinks, then reads through the checked descriptor.
  - `TestTensorBoardBehindPrefix` runs a real TensorBoard and is skipped
    unless `ROTARI_LAUNCHER_TEST_TENSORBOARD` is set.

  The plan gained a Phase 1 results section, the `NO_PROXY` rule, the exact
  `127.0.0.1` bind requirement, and the wrapper description.
- **Reason:** Phase 1 requires the three feasibility gates and the TensorBoard
  proxy check to pass before any Web proxy is built.
- **Plan impact:** All gates passed, and Phase 1 is complete. Decided: the
  runtime directory, log rotation (2 × 8 MiB per launch), limits (16 live
  launches, 50 ended), the `launchers.toml` schema, the `/_launch/<ID>/`
  route, and Host rules (loopback names with any port, the literal
  `--host`, and a new `--allowed-host` in Phase 3). New findings:
  - an inherited `http_proxy` without `no_proxy` broke TensorBoard's
    loopback gRPC, so launches now add loopback to `NO_PROXY`;
  - TensorBoard exits 255 when its port is taken;
  - its redirects and assets are relative, so no rewriting is needed.
- **Validation:**
  - `go test -race -count=1 ./internal/launcher` passed with Go 1.25.8 (23
    tests, including subtests; the TensorBoard test skipped without its
    variable).
  - With TensorBoard 2.21.0 installed in a scratch directory,
    `TestTensorBoardBehindPrefix` passed.
  - `go vet` passed for linux, darwin, and freebsd.
  - Mutation checks: disabling lifetime EOF handling made
    `TestKilledOwnerStopsLaunches` and `TestLifetimeCloseStopsCommands`
    fail, and disabling the group comparison made
    `TestVerifyListenerRejectsForeignListener` fail.
  - pre-commit passed on the changed files.
  - `scripts/check.sh --short` did not complete: the type check failed
    because this host has Node v12. `go test -short ./...` failed only in
    jsdom browser tests (`internal/webui`, `cmd/rotari`,
    `conformance/05-web`) for the same reason; those packages do not import
    `internal/launcher`. Under Go 1.27, `TestLiveServerServesEmbeddedFonts`
    also failed. It passes with the go.mod toolchain (1.25.8), which CI
    uses.
  - The macOS `lsof` path is compiled but untested; CI runs Linux only.
- **Remaining:** Phase 2: the manager, the `__launch` dispatch, loading
  `launchers.toml`, `config --launchers`/`--list`, the archtest rule, and the
  `docs/ARCHITECTURE.md` and `contracts/00-overview.md` package entries.

## Phase 2: registry, manager, and the `__launch` dispatch

Commits:

- `9573d8a9` (2026-10-20 16:53:07 +0900): package side.
- `e5bcbb72` (2026-10-20 16:57:30 +0900): `cmd/rotari` wiring and boundary.

- **Change:**
  - `internal/launcher` gained `registry.go`: the `launchers.toml` schema
    from the Phase 1 decisions, strict validation (unknown keys, duplicate
    names or targets, missing required fields, mode-specific settings in
    the wrong mode, relative or unclean cwd, non-positive durations), and
    `LoadRegistry`, which reads only through `OpenTrustedFile` and treats a
    missing file as no commands. `Template` holds the TensorBoard entry.
  - `manager.go`:
    - starts registered commands with `ROTARI_LAUNCHER_ARTIFACT`/`_CWD`,
      loopback added to `NO_PROXY`/`no_proxy`, and inherited
      `ROTARI_LAUNCHER_*` dropped;
    - reuses a running launch with the same name, target, and cwd
      unless asked for a new instance;
    - records start failures as failed launches;
    - limits live (16) and ended (50) launches, removing dropped logs;
    - stops everything on `Close`. Service mode returns an error until
      Phase 4.
  - `log.go` keeps output in two rotating segments with paged reads that
    report dropped output.
  - `process.go` sets `WaitDelay`, so an escaped process holding the output
    pipe cannot block `Wait`.
  - `cmd/rotari` dispatches `__launch` before loading configuration.
  - `internal/archtest` forbids rotari imports and `net/http` in
    `internal/launcher`; `contracts/00-overview.md` and
    `docs/ARCHITECTURE.md` list the package.
- **Reason:** Phase 2 of the plan. `config --launchers`/`--list` moved to
  Phase 3: they refer to `--allow-exec`, which does not exist yet, and the
  plan forbids documenting options before they work. Adding them now would
  also have touched `cli_spec.go` and the generated CLI files, which other
  work had uncommitted changes in.
- **Plan impact:** Phase 2 is complete; Phase 3 now includes `config
  --launchers`/`--list`. Two issues the tests found:
  - The test-only variables were named `ROTARI_LAUNCHER_TEST_*`, which the
    manager correctly strips, so they were renamed
    `ROTARI_TEST_LAUNCHER_*`. This includes
    `ROTARI_TEST_LAUNCHER_TENSORBOARD`, which the Phase 1 entry calls
    `ROTARI_LAUNCHER_TEST_TENSORBOARD`.
  - Parallel tests showed that an allocated port can be taken by an
    outgoing connection's ephemeral source port. The plan records that
    Phase 4 retries once with a new port in that case.

  `docs/ARCHITECTURE.md` had other uncommitted hunks, so the commit staged
  HEAD plus only the launcher entries.
- **Validation:**
  - `go test -count=5 -race ./internal/launcher`, and three later runs of
    `-count=3 -race` after the tests became parallel, all passed (Go
    1.25.8). With TensorBoard 2.21.0, `TestTensorBoardBehindPrefix` passed.
  - `go test ./internal/archtest ./internal/doclinks` passed.
  - `go vet` passed for `./cmd/rotari`, `./internal/launcher`, and
    `./internal/archtest`, and darwin vet for `./internal/launcher`.
  - `TestLaunchWrapperRunsBeforeConfiguration` passed, and failed with the
    dispatch removed.
  - `go test -short ./cmd/rotari` failed only in the two jsdom tests
    (`TestWebRunViewDrawsMatrixGrid`, `TestWebRunViewClampsLongCells`)
    because this host has Node v12.
  - pre-commit passed on the changed files.
- **Remaining:** Phase 3: `--allow-exec` with mandatory token, Host/Origin
  checks, `config --launchers`/`--list`, the Web API and Artifacts UI for
  command mode, the unavailable-state explanations, and the server-wide
  launch list.
