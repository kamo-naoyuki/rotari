# Plan: Launch commands and services from Artifacts

Created: 2026-10-20
Status: Proposed; no implementation changes

## Goal

Open a launcher from the Artifacts view, run an arbitrary noninteractive shell
command against the selected file or directory, and inspect or stop it from
the Web UI. Make TensorBoard the first built-in service preset, accessible
through the existing `rotari web` connection without another SSH port forward.

This is a personal-use feature. A user authorized to launch commands is
trusted to execute code with the OS permissions of the Web server's user.
Presets are conveniences, not an executable allowlist or a sandbox.

## Existing implementation

- [Web startup](../../cmd/rotari/web.go) defaults to loopback, optionally wraps
  all routes with token authentication, and wires shared operations through
  [webui.Options](../../internal/webui/options.go). Existing `allow-control`
  is enabled by default; it must not implicitly enable command execution.
- [Artifacts UI](../../internal/webui/assets/web_app_artifacts.js) currently
  opens in the output modal, not a standalone artifact page. Extend that view
  and its file/directory previews first; a new page route is not required.
- [Artifact resolution](../../internal/webui/artifact_files.go) identifies a
  recorded entry and optional relative child, applies allowed roots, and uses
  `os.Root` for preview reads. Reuse its policy rather than introduce a second
  HTTP path resolver.
- [Artifact listing](../../internal/webui/artifacts.go) uses the shared
  `jobstatus` projection, including carried results and explicit attempts.
  Launch context must follow the same selected attempt.
- The [architecture](../../docs/ARCHITECTURE.md) separates the Web process
  from per-run supervisors. Launcher processes must not become training jobs,
  occupy the project's active-run slot, or mutate completed run records.
- [Current Web contracts](../../contracts/05-web-assets-and-static-export.md)
  cover artifact access, authentication, read-only operation, and static
  export. Keep those behaviors unchanged unless explicitly extended below.

## Scope and non-goals

Include in the first implementation:

- Launch from a recorded artifact file, directory, or browsed directory child.
- Arbitrary editable shell commands, with TensorBoard as a built-in preset.
- Ordinary command mode and HTTP service mode.
- Explicit cwd, startup-environment inheritance, live logs, exit status,
  service readiness, stop, and reuse of matching live launches.
- One launcher registry per Web server, shared across its basedir mounts.
- A server-wide launcher list reachable after leaving the Artifacts view.
- Saving named presets outside project/run records.

Exclude initially:

- Interactive stdin, PTYs, browser terminals, and terminal reconnection.
- SSH/Slurm/PBS/LSF/SGE launch backends or automatic execution on a job's host.
- Automatic reconstruction of a historical training environment.
- Surviving Web server restart or adopting another server's processes.
- A multi-user permissions model, containers, or OS sandboxing.
- Automatic launching when an artifact is discovered or a page is opened.
- A generic proxy that accepts an arbitrary upstream URL.

## User flow

1. Open a job's Artifacts view and select an entry or directory child.
2. Choose **Launch with > TensorBoard** or **Launch with > Custom command**.
3. Show a dialog containing the absolute target, actual execution host and
   user, originating job/attempt, cwd, mode, and editable command.
4. Validate and launch only after explicit confirmation. Show preparation and
   startup failures with their logs, rather than opening a broken link.
5. In service mode, enable **Open** after HTTP readiness succeeds. Open in a
   separate tab; embedding in an iframe is not required.
6. Show **Open**, **Logs**, and **Stop** beside a live service, and **Logs** and
   **Stop** beside an ordinary command. Retain exit status after completion.
7. Offer to save an edited command as a named preset. Saving does not execute it.

For a directory, default cwd to that directory; for a file, use its parent.
Allow an explicit absolute cwd override and validate that it is an accessible
directory. Do not silently fall back to the Web server's startup directory.
TensorBoard is offered for directories; a selected event file can instead
have an explicit **Use parent directory** action, not an implicit target change.

The target must be accessible on the Web host. A path recorded on a remote
node is not evidence that it exists here. Missing, unresolved, outside-root,
or inaccessible targets get an explicit explanation.

## Execution and preset model

### Commands

Run commands using an explicitly documented shell (`/bin/sh -c` initially),
without a login shell or implicit activation scripts. Pipelines and
redirections work; commands requiring a terminal are unsupported. Close stdin.
The UI must name the shell so users do not assume Bash-specific syntax.

Inherit the Web server's startup environment and supply documented variables:

| Variable | Meaning |
| --- | --- |
| `ROTARI_LAUNCHER_ARTIFACT` | Absolute selected file/directory path |
| `ROTARI_LAUNCHER_CWD` | Validated effective working directory |
| `ROTARI_LAUNCHER_PORT` | Allocated loopback port, service mode only |
| `ROTARI_LAUNCHER_BASE_PATH` | Server-generated proxy path, service mode only |

Pass these as environment values, not unescaped textual substitutions into
shell source. Presets quote variable expansions. Do not inherit the selected
job's saved environment or return raw inherited environment values to clients.
Users may explicitly write `conda run`, an absolute executable, or environment
assignments in their command.

An illustrative TensorBoard command is:

```sh
tensorboard \
  --logdir "$ROTARI_LAUNCHER_ARTIFACT" \
  --host 127.0.0.1 \
  --port "$ROTARI_LAUNCHER_PORT" \
  --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"
```

Verify these options and proxy behavior against an installed TensorBoard
before treating the preset as working. Rotari must not silently install it.
Report a missing executable/environment as a launch failure.

### Presets

A preset contains a name, command, applicable target kinds, command/service
mode, and, for services, a readiness path and timeout. Start with built-in
TensorBoard and editable custom-command defaults.

Store user presets in a dedicated launcher configuration associated with the
Web startup workspace, following existing config-location conventions. Capture
that scope once; switching basedirs or selecting a job must not change where a
preset is saved. Load and save through one owning package with schema validation
and atomic writes. Do not use browser storage as the authoritative preset store,
modify training config, or auto-load executable presets from artifact contents.
Malformed config produces an explicit error, not an empty preset list.

### Identity and reuse

Give each launch an opaque server-generated ID. Record its source basedir,
project, run, job, selected attempt, artifact entry/child, resolved target,
cwd, command, mode, effective preset settings, and lifecycle timestamps.

Serialize matching start requests: source context, target, cwd, command, and
service settings define the reuse key before generated port/path values.
Return the existing starting/ready/running launch on duplicate submission.
Provide an explicit **Start another instance** action. Never reuse an exited,
failed, or stopping instance; editing a preset must not change a live launch.

## Process ownership and lifecycle

Use a dedicated launcher manager, not `projectrun.Runner` or a project lease.
Keep the manager separate from HTTP handlers and reuse existing process
helpers only where their semantics match.

- Commands run in their own process groups; Stop affects the group, not just
  the shell. Send graceful termination, then bounded forced termination.
- Closing a modal, tab, or HTTP request does not stop an accepted launch.
- Startup validation and process-spawn errors, readiness timeout, nonzero exit,
  explicit stop, and cleanup failure remain distinguishable.
- Ordinary commands are running after spawn; services are starting until an
  HTTP probe succeeds. An exited service must no longer advertise a live URL.
- A readiness timeout stops the process group and preserves diagnostic logs.
- Web shutdown rejects new launches and stops all owned groups before exit.
  Existing `server.Close` alone is not sufficient lifecycle management.
- Abrupt Web death also needs cleanup. Prototype a private lifetime-pipe
  watchdog/helper that terminates owned groups on parent disappearance, without
  reusing the training supervisor. Prove this before shipping; normal-shutdown
  handling alone is not an acceptable substitute. State platform limitations,
  including the impossibility of cleanup if every supervising process is killed.
- Do not recover ownership or signal processes from a stale PID file after
  restart. Daemonizing commands that escape the process group are unsupported.

Use a private per-Web-session runtime directory outside project/run history
for logs and any manager records. Filesystem records own retained metadata;
memory indexes live process handles. A new Web server never adopts old sessions.
Use bounded log retention with visible truncation, paged log reads, and a
clear completed-record retention policy. Remove only this session's owned
runtime files during cleanup. Exact bounds belong in implementation tests/docs.

## Service access

Keep the upstream bound to `127.0.0.1` and proxy through the existing Web
listener. Allocate the upstream and proxy path on the server, never from a
browser-provided host or URL. Handle port allocation/bind races explicitly;
never mistake an unrelated listener for a successfully started service.

Preserve the allocated path prefix, query strings, streaming, and browser asset
requests. Use a service-specific prefix setting for TensorBoard; do not attempt
generic HTML rewriting. Arbitrary services that cannot work beneath a prefix
are not promised to work in the first release.

All proxy routes pass through Web authentication. Do not forward the Web token,
Authorization header, or Web authentication cookies to upstream services.
Proxy and launch IDs must work under existing basedir prefixes and supported
reverse-proxy deployments without mixing services between mounts.

The first version treats launched service content as trusted code, just like
the launched command. Document that exposing it under the Web origin gives it
browser-side access to rotari on that origin. Separate-origin isolation for
untrusted service content is a later feature, not an implied guarantee.

## Access boundaries

Introduce an explicit, default-off execution capability, provisionally
`rotari web --allow-exec`. Require both it and `allow-control` for launch,
stop, and preset mutations; reject conflicting startup options rather than
silently ignoring one. Static export must reject execution-only options and
show launch controls as unavailable.

When execution is enabled:

- Require authentication on non-loopback listeners; preserve existing warning
  behavior for servers without execution enabled. Recommend SSH forwarding or
  HTTPS, since a token does not encrypt HTTP.
- Validate Host against configured serving authorities and validate browser
  Origin for mutations; reject cross-origin requests. Define the authenticated
  non-browser path explicitly rather than treating a missing Origin as proof
  of safety. Do not enable wildcard CORS. Cover DNS-rebinding scenarios.
- Use POST for starting/stopping/saving; GET and page loading never execute.
- Resolve artifact identity on the server against registered basedirs and the
  shared artifact rules. Browser-supplied display paths are not authority.
- Do not claim that artifact-root checks constrain arbitrary commands: the
  command has the Web user's filesystem permissions and can write/delete files.
  Even a validated path can change before an external process opens it.
- Treat command text and logs as potentially sensitive. Do not include them in
  general `/api/state`, static exports, notifications, or public error pages.
  Render them as text, not executable HTML.

## Package and API boundaries

- Proposed `internal/launcher`: execution specification, preset schema/storage,
  lifecycle manager, deduplication, logs, readiness, and process ownership.
  No dependency on HTTP, job status, or project-run orchestration.
- `internal/webui`: resolve selected artifact context through existing shared
  helpers, enforce capabilities, adapt requests, serve the fixed-target proxy,
  and render dialogs/list/status. Factor shared artifact resolution only as
  needed; do not expose lower-level helpers that bypass its policy.
- `cmd/rotari`: parse execution flags, construct one manager, and coordinate
  HTTP and process shutdown. Add private helper dispatch only if required by
  the proven crash-cleanup design.

Provisional API: preset list/save, launch list/start/detail/logs/stop, and a
service proxy route keyed by launch ID. Keep start inputs structured: source
artifact identity, command, cwd, mode, and service settings. No free target-path
or upstream-URL API. Return explicit lifecycle/error fields and appropriate
HTTP errors; never convert startup errors into success-shaped responses.

## Implementation phases

1. **Feasibility and lifecycle checkpoint.** Prove TensorBoard path-prefix
   proxying over one forwarded port, readiness/port ownership, process-group
   stop, and parent-death cleanup in isolated fixtures. Finalize runtime paths,
   resource bounds, preset config location, and supported proxy authorities.
2. **Command manager.** Implement preset schema/storage and process ownership,
   command-mode start/list/log/stop, deduplication, and cleanup with unit tests.
3. **Web capability and Artifacts entry.** Wire opt-in/auth/request checks and
   artifact resolution; add custom-command dialog and server-wide launch list.
   Verify a real command can be launched, observed, and stopped from Artifacts.
4. **HTTP services and TensorBoard.** Add allocated upstreams, readiness,
   authenticated prefix proxying, TensorBoard preset, and Open/reuse actions.
5. **Contracts, docs, and end-to-end validation.** Document behavior and limits,
   cover interfaces/variants below, and complete repository checks.

Each phase is a coherent commit/checkpoint. Do not ship a launcher that lacks
its required shutdown or access protections as a temporary default.

## Acceptance and validation

- A file, directory, and nested child pass the exact intended path and cwd,
  including spaces, quotes, dollar signs, backticks, Unicode, and both path
  separators where the platform permits them; paths never become shell code.
- Plain jobs, array tasks, matrix members, older attempts, and carried results
  launch against the artifact currently displayed, not a different attempt.
- Multiple basedirs/projects with identical names remain distinct. One server
  owns one manager; other Web processes cannot adopt/stop its launches.
- Concurrent duplicate clicks spawn once; an explicit extra instance spawns
  separately. Missing paths/cwd/executables and malformed presets show errors.
- Logs stream/page without unbounded memory/disk use; exit codes are accurate.
  Stop kills shell descendants, readiness timeout cleans up, and both graceful
  and abrupt parent-exit fixtures verify process cleanup.
- Disabled execution, read-only mode, static export, bad auth, cross-origin and
  invalid Host requests cannot start/stop commands or save presets.
- Proxy tests cover prefixes, redirects, assets, queries, streaming, upstream
  exit, token stripping, guessed/unknown IDs, and attempted arbitrary targets.
- A real TensorBoard opens through only the rotari port and displays a known
  event fixture. Use a dedicated optional integration environment if it is not
  installed locally; a fake HTTP service alone does not verify this requirement.
- Browser tests cover launching from the current Artifacts modal, directory
  navigation, visible startup failure, ready-only Open, logs/stop, reuse, and
  returning through the global list.
- Add conformance rows through the built binary and Web API for the shared
  capability, artifact-selection, lifecycle, and proxy rules. Assign new
  contract IDs and synchronize the contract-status table.

Run targeted tests first, then package and conformance suites, pre-commit on
changed files, `scripts/check.sh --short`, and `scripts/check.sh`. Manual binary
checks use one temporary work area with isolated `ROTARI_MASTERDIR` and basedirs;
clean up its services and files afterwards.

Update the Web/artifact contracts, architecture, CLI metadata/generated
reference, relevant inspection/configuration/operations guides, FAQ, and tests
as each behavior is implemented. Do not document proposed flags as available
before implementation. If getting-started content changes, edit its source and
use the README synchronization script rather than editing README directly.
