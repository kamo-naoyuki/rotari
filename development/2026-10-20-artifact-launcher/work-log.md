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

## Phase 3: `--allow-exec`, the launch API, the Artifacts UI, and docs

Commits:

- `2a1de96b` (2026-10-20 17:29:41 +0900): server side.
- `8cf6021b` (2026-10-20 17:41:19 +0900): Web UI.
- `957edaae` (2026-10-20 17:44:35 +0900): output tail fix.
- `9a90d46f` (2026-10-20 17:44:38 +0900): docs and plan notes.
- `910cc68f` (2026-10-20 17:53:50 +0900): flag-pair inventory.
- `cf63599f` (2026-10-20 18:07:10 +0900): browser test wait.

- **Change:**
  - **CLI:**
    - `rotari web --allow-exec` (command-line only) loads the global
      `launchers.toml` at startup and refuses to start if the file is
      untrusted or invalid.
    - It requires a token on every listener, and generates and prints one
      when none is set.
    - It wraps the handler in `WithExecGuard`: Host must be a loopback
      name, a specific `--host`, or a repeatable `--allowed-host`, and a
      non-GET request needs a same-origin `Origin` or an explicit
      token/bearer header.
    - It closes the manager before the listener on shutdown, and strips
      `ROTARI_WEB_AUTH_TOKEN` from launch environments.
    - `config --launchers` writes a 0600 template in a 0700 global
      directory and checks it; `config --list` shows the global file only.
    - `cli_spec.go` carries the new options. The schema golden, the CLI
      reference, and `generated_cli.py` were regenerated (raw generator
      output, matching the committed file's format).
  - **`internal/webui`:** `launches.go` serves:
    - `/api/launchers`: availability, with the server's reason and
      enabling hint in every unavailable state, including per-kind
      "unmatched" texts and the static export's answer;
    - `/api/launch-preview`;
    - `/api/launch`: strict JSON, so any field besides name and artifact
      identity is rejected;
    - `/api/launches`, `/api/launch-log` (paged, `from=end`), and
      `/api/launch-stop`.

    Artifacts resolve through `resolveArtifact` and `os.Root`, and a target
    must exist on the Web host.
  - **Web UI:** **Launch with…** in every preview title line, a
    confirmation built from the preview, reuse with **Start another
    instance**, and a **Launches** view with output and Stop, behind a
    top-bar button shown only when launching is available.
  - **`internal/launcher`:** each manager flocks its runtime directory and
    sweeps unlocked ones left by killed servers.
  - **Docs:** `CONFIGURATION.md` (launcher file), `INSPECT.md` (launching
    walkthrough), `OPERATIONS.md` (security model), `FAQ.md`, and
    `ARCHITECTURE.md` (process model, packages, `cmd` files) are updated.
    The WEB-3 contract and its pair conformance test now cover
    `--allow-exec` and `--allowed-host`.
- **Reason:** Phase 3 of the plan: command mode usable end to end from the
  Web UI, with the access protections it depends on.
- **Plan impact:** Phase 3 is complete; decisions are in the plan's "Phase 3
  notes". Two problems the manual end-to-end check and review found:
  - A SIGKILLed server left its runtime directory, which led to the sweep.
  - The Launches view showed the oldest kept output instead of the latest,
    which led to `from=end`.

  Two existing tests asserted layouts that intentionally changed: the
  top-bar item list, and WEB-3's server-only option list.
- **Validation:** Run with Go 1.25.8 and Node 22.23.3, checksum-verified in a
  scratch directory, because this host has Node v12:
  - **Unit, API, and browser tests:**
    - `go test` of `internal/launcher`, `internal/webui`, `cmd/rotari`,
      `internal/archtest`, `internal/doclinks`, `conformance`,
      `conformance/03-interfaces`, `conformance/03-interfaces/pairweb`, and
      `conformance/05-web` passed.
    - New jsdom tests cover the Launch with flow, unavailable explanations,
      and the static export. `npm run typecheck` and `go vet ./...` passed.
    - Python tests: 65 passed, 2 skipped.
    - The CLI reference and API docs `--check` passed.
  - **Full `scripts/check.sh`:**
    - The first run failed in `conformance/03-interfaces`: the flag-pair
      inventory and classification needed the new options (fixed in
      `910cc68f`), plus the timing flake `TestRetryProgressCountsOnlyExecutedJobs`.
    - The second run passed vet, typecheck, and `go test`. Under `-race`, it
      failed only `TestWebLaunchInBrowser`, whose fixed wait was too short
      for the race-instrumented wrapper (fixed in `cf63599f`).
    - After that, `go test -race ./internal/webui` passed, and the launch
      tests passed three times under `-race`.
    - A third full `scripts/check.sh` failed only
      `TestUnlockExecutionEvidenceOverridesTerminalOutcome/MCP/complete`
      ("live refusal mutated state"; unlock code is untouched here), which
      passed five times in isolation. That flake and
      `TestRetryProgressCountsOnlyExecutedJobs` are now in `ISSUES.md`. The
      race step did not run in that pass; the launcher packages' race tests
      had passed in the previous run.
  - **Earlier flakes:** `TestLocalJobCancelStopsCommandBeforeForegroundExec`
    and `TestReadJJ` failed once under load in the first short run, and
    passed when rerun alone.
  - **Manual end-to-end check with the built binary:** isolated
    `ROTARI_MASTERDIR`, `XDG_CONFIG_HOME`, and basedir under one scratch
    directory, removed afterwards.
    - The server printed a generated token and served nothing without it
      (401); a bad Host got 421, and a cross-origin POST with Basic auth 403.
    - A registered command ran on a recorded artifact, and its log and exit
      code were correct.
    - After SIGKILL of `rotari web`, the launched `sleep` was gone within
      about 0.4 s.
- **Remaining:** Phase 4 (service mode, port checks in readiness, the
  `/_launch/<ID>/` proxy, TensorBoard, the startup retry for a lost port,
  and replacing the "service mode is not available yet" notes), then
  Phase 5 (contract IDs and conformance rows).

## Phase 4: services, readiness, and the `/_launch/` proxy

Commits:

- `d0b9846a` (2026-10-20 21:20:01 +0900): code.
- `ea3c7495` (2026-10-20 21:20:01 +0900): docs, plan, and issues.

- **Change:**
  - **Manager (`internal/launcher`):**
    - A service launch now has `StateStarting`, a port picked per start,
      and `BasePath` from the new `ManagerConfig.ServicePath`. It gets
      `ROTARI_LAUNCHER_PORT` and `ROTARI_LAUNCHER_BASE_PATH`.
    - `service.go` checks readiness: connect, `VerifyListener`, then one
      raw HTTP/1.1 GET. A service that exits without ever listening is
      restarted once on a new port. A foreign or non-loopback listener, or
      a timeout, stops the group and records the reason.
    - `ServiceUpstream` exposes only ready services. `ready_path` may not
      contain spaces.
  - **Web (`internal/webui`):**
    - `launch_proxy.go` serves `/_launch/ID/` at the top level (not under
      basedir mounts). It uses a per-launch transport whose dialer reaches
      only the loopback port and runs `VerifyListener` per connection.
    - It strips `Authorization` and `X-Rotari-Token`, redirects the bare
      prefix, and answers 503/404/403 for starting or ended, unknown, and
      disabled launches.
    - The UI follows a starting service to ready or failure and shows
      **Open** in the panel and in Launches.
  - **`cmd/rotari`:** sets `ServicePath: webui.LaunchProxyPath`.
  - **Docs:** `CONFIGURATION.md` gains a Services section; `FAQ.md`,
    `INSPECT.md`, `OPERATIONS.md` (service content is trusted, same origin;
    token headers stripped; per-connection owner check), and
    `ARCHITECTURE.md` now cover services in place of the "not available
    yet" notes.
- **Reason:** Phase 4 of the plan: TensorBoard and other HTTP services
  through the existing Web port, with the port race gate enforced end to end.
- **Plan impact:** Phase 4 is complete; decisions and the end-to-end result
  are in the plan's "Phase 4 notes". The Phase 2 test that expected service
  mode to be rejected now expects only a manager without `ServicePath` to
  reject it. `internal/launcher` still imports no `net/http` (the probe uses
  `net`).
- **Validation:**
  - **Service tests:** the new service tests (ready and upstream, never
    ready, wildcard bind, exit without listening, foreign listener, retry on
    a new port, stop while starting, missing `ServicePath`) passed in three
    runs of `go test -race ./internal/launcher`. Darwin vet passed.
  - **Mutation check:** without `VerifyListener` in readiness,
    `TestServiceRejectsForeignListenerOnItsPort` failed.
  - **Proxy and browser tests:**
    - The `internal/webui` proxy tests pass: forwarded path, query,
      header stripping, redirect, status codes, and the per-connection
      owner check (`TestLaunchTransportChecksPortOwner`).
    - The browser test's service flow, with the Open link and its proxied
      page, passes with Node 22.
    - With TensorBoard 2.21.0, `TestLaunchProxyTensorBoard` (the template
      entry through the proxy) passed; it is skipped without the variable.
  - **Manual end-to-end check:** with the built binary, TensorBoard was
    served through the rotari port, `http_proxy` set, and an event fixture.
    SIGKILL cleanup worked. Details are in the plan's notes; the scratch
    directory and the killed server's runtime directory were removed.
  - **Repository checks:**
    - `go vet ./...`, `npm run typecheck`, and pre-commit passed.
    - `go test -short ./...` failed only
      `conformance/06-selectors` `TestPositionalArguments/check/project`,
      which passed three times in isolation.
    - The full `scripts/check.sh` failed only `conformance/06-selectors`
      `TestSelectorTable/copy/named_filter_form_of_failed_and_stage`; the
      whole package then passed three times in isolation. Both are now in
      `ISSUES.md`.
    - Because that run stopped before its race step, `go test -race
      -timeout 15m ./...` was run separately and passed for every package.
- **Remaining:** Phase 5: contract rules with IDs for the launcher
  behaviors (capability and token, request guards, registered-only
  starts, artifact selection, lifecycle, proxy), conformance rows through
  the binary and the Web API, and the contract-status table.

## Phase 5: contract WEB-13 to WEB-17 and conformance

Commit: `3ba65525` (2026-10-20 21:57:23 +0900).

- **Change:** `contracts/05-web-assets-and-static-export.md` gained the
  "Launching commands on artifacts" section, with rules WEB-13 to WEB-17:
  - **WEB-13:** the opt-in and token;
  - **WEB-14:** the Host and Origin guards;
  - **WEB-15:** only registered commands from the trusted global file
    start, against an artifact resolved as WEB-6 resolves it, and never see
    the token;
  - **WEB-16:** reuse, list, log, stop, and cleanup after SIGKILL of
    `rotari web`;
  - **WEB-17:** service readiness and the proxy.

  The rows in the `contracts/README.md` status table are all "conformance".

  `conformance/05-web/launch_test.go` checks each rule through the built
  binary and the Web API, using fixture data that tells cases apart: a file
  and a directory artifact, a declared artifact absent from the host, two
  instances with separate PIDs, and a foreign listener on a service's port.
  `conformance/support` gained `HTTPDo`, which sends headers and sets Host,
  and `05-web`'s `TestMain` lets the test binary act as the launched HTTP
  service.
- **Reason:** Phase 5 of the plan. AGENTS.md asks for conformance rows for
  behavior visible through the binary or the Web API, with the status table
  matching the `covers` calls.
- **Plan impact:** All phases are done; the plan's status points to the
  contract and conformance tests.
- **Validation:**
  - **New tests:** the five new conformance tests passed, and so did
    `go test ./conformance/...` (all contract groups and the status-table
    check) and `./internal/doclinks`.
  - **Mutation checks through the binary:**
    - Removing the `WithExecGuard` wiring failed three
      `TestWebLaunchGuardsRequests` cases.
    - Passing the unfiltered environment to launches made
      `TestWebLaunchRunsOnlyRegisteredCommands` print the token.
  - **Full `scripts/check.sh`:** go vet, typecheck, `go test`, and
    `go test -race` all passed, with Go 1.25.8 and Node 22.23.3.
  - pre-commit passed on the changed files.
- **Remaining:** none for this plan. The load-dependent failures seen in
  other suites during the launcher work are recorded in `ISSUES.md`.

## Follow-up: Download and Launch with in artifact rows

Commit: `e0f90caa` (2026-10-21 04:12:01 +0900).

- **Change:** `web_app_artifacts.js` adds Download and Launch columns to the
  Artifacts listing and to an opened directory's children
  (`artifactActionCells`). Download is offered for files the server may
  serve; Launch with for files and directories it may serve. Other rows have
  empty cells, and a static export without copied contents has no such
  columns. `toggleArtifactLaunch` takes the clicked button: from a row, the
  panel is a full-width row below it, and the same button closes it. In a
  preview's title line, Download and Launch with are now 1rem apart. The
  launching walkthrough in `docs/INSPECT.md` describes the row buttons.
- **Reason:** The user asked for both actions in each table row rather than
  only after opening an artifact, and found the title-line Download and
  Launch with too close together.
- **Plan impact:** None; this is a UI layout change within Phase 3's
  Artifacts entry.
- **Validation:**
  - jsdom browser tests with Node 22, new assertions:
    - `TestWebArtifactPreviewInBrowser`: the column headers; per-row
      actions for an image, a directory, an entry outside the roots, and a
      missing entry; the row download link; and file and directory child
      rows.
    - `TestWebLaunchInBrowser`: the row panel's placement, span, choices,
      and closing.
  - `go test` of `./internal/webui`, `./conformance/05-web`,
    `./cmd/rotari`, and `./internal/doclinks` passed; `npm run typecheck`
    and pre-commit passed.
  - Not checked in a real browser (none on this host); the layout is
    checked through jsdom's DOM only.
- **Remaining:** None.

## Follow-up: artifacts beside the basedir, only from trusted records

Commit: `05dd3ae3` (2026-10-21 04:45:55 +0900).

- **Change:**
  - `internal/webui/artifact_trust.go` adds `artifactRecordsTrust`.
    A listing is trusted when the basedir, `projects`, the project,
    `queue.json`, `meta.json`, `runs`, and the run directory are each
    owned by the Web server's user or root and are not group/other
    writable (permission bits only).
  - It also adds `basedirParentRoot`: the directory holding the basedir,
    never `/`.
  - `artifactRoots` now serves, for trusted listings, the working
    directory, the basedir's parent, and `--artifact-root`; for untrusted
    ones, only `--artifact-root`. The reason goes in `/api/artifacts`
    `roots_note`, and the Artifacts view shows it.
  - Previews, downloads, Launch with, and static-export copies all follow
    this through `resolveArtifact`.
  - Contracts: WEB-6 states the roots and the trust rule; WEB-7 refers to
    them; the status row lists the new conformance test.
  - `docs/INSPECT.md` and `docs/OPERATIONS.md` explain both rules.
- **Reason:** One `rotari web` usually watches many basedirs, and outputs
  beside a `.rotari-state` directory were not served without listing every
  directory in `--artifact-root`. Discussing this with the user
  established that the server reads files as its own user, whoever views
  the page. Whoever can write a project's state can make a job record any
  working directory and path. So the working directory was as exposed as
  a new parent root would be, and both now depend on that state being
  writable only by this user.
- **Plan impact:** Changes WEB-6 and WEB-7 beyond the launcher plan;
  Launch with targets follow automatically (WEB-15). A basedir owned by
  another user, previously served from its jobs' working directories, now
  serves only `--artifact-root`.
- **Validation:**
  - New tests:
    - `TestBasedirParentRoot`.
    - `TestArtifactRootsFollowRecordTrust`: a file beside the basedir is
      served; a group-writable project is refused with its note but
      allowed under `--artifact-root`; a writable run directory is
      detected.
    - Conformance `TestWebServesArtifactsOnlyFromTrustedRecords`, through
      the binary.
  - The artifact fixture's basedir moved into its own directory, so its
    parent does not hold the fixture's "outside" directory.
  - Mutation check: forcing trust made the untrusted-listing assertion
    fail.
  - Full `scripts/check.sh` (vet, typecheck, `go test`, `go test -race`)
    passed with Go 1.25.8 and Node 22.23.3.
- **Remaining:** None.

## Follow-up: built-in TensorBoard and unavailable commands

Commits:

- `befdc676` (2026-10-21 05:08:17 +0900): flaky test fix.
- `22a0fde5` (2026-10-21 05:08:17 +0900): built-in TensorBoard and unavailable commands.

- **Change:**
  - **Built-in TensorBoard (`internal/launcher/registry.go`):**
    - `BuiltinCommands` provides TensorBoard: the command the template
      used to carry, directory targets, service mode. `LoadRegistry` adds
      it unless the file has an entry with that name.
    - The template now registers nothing; its TensorBoard replacement and
      a file command are commented examples, checked by
      `TestTemplateExamples`.
  - **Unavailable commands:**
    - `executable.go` adds `Command.Executable`, the first program of the
      command parsed with `mvdan.cc/sh`, skipping assignments, builtins,
      expansions, and relative or `~` paths. It also adds
      `Command.Unavailable`, which reports a program missing from PATH or
      a non-executable absolute path.
    - `/api/launchers` returns `builtin` and `unavailable` per command.
      `launchStart` (preview and start) refuses an unavailable command.
    - The UI shows such a command disabled, with the reason.
  - **Startup:** lists each command, marked built in or unavailable.
  - **Docs and contract:** WEB-15, `CONFIGURATION.md`, `INSPECT.md`,
    `FAQ.md`, and `OPERATIONS.md` are updated. The plan's "no built-in
    commands" non-goal is marked reversed.
  - **Flaky test:** `TestVerifyListenerRejectsMissingListener` now tries
    another port when a parallel test's listener takes the allocated one.
- **Reason:** The user asked for TensorBoard to be registered by default, and
  for commands whose program is missing to stay visible but disabled.
- **Plan impact:** Reverses the non-goal "built-in commands". A file entry
  named TensorBoard still replaces it, so environments with TensorBoard
  elsewhere keep working.
- **History note:**
  - A jj operation at 2026-10-21 04:03 detached git HEAD and moved `main`
    to the PR #10 merge `d71b83ed`. The six commits from `e0f90caa` to
    `22a0fde5` were made with git on the detached HEAD.
  - At the user's request, they were rebased onto `main` with
    `jj rebase`, and `main` was moved to `22a0fde5`. The commit IDs in
    this log are the rebased ones.
  - Three working-copy snapshots that jj had left as children were
    abandoned; their trees equalled the final commits.
- **Validation:**
  - New tests (`TestBuiltinTensorBoard`, `TestTemplateExamples`,
    `TestLoadRegistryReplacesBuiltins`, `TestCommandExecutable`,
    `TestCommandUnavailable`, `TestLaunchUnavailableCommand`), the
    browser check of the disabled button, and the extended conformance
    `TestWebLaunchRunsOnlyRegisteredCommands` passed.
  - `go test -race ./internal/launcher` passed three times.
  - The full `scripts/check.sh` passed vet, typecheck, and `go test`. The
    race step failed only `conformance/03-interfaces`
    `TestUnlockFinalizationRepair/array/MCP` (`server.pid` missing), which
    passed three times under `-race` in isolation and does not involve the
    launcher; it is recorded in `ISSUES.md`.
- **Remaining:** None.
