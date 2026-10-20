# Plan: Launch commands and services from Artifacts

Created: 2026-10-20
Status: Proposed; no implementation changes

## Goal

Open a launcher from the Artifacts view, run any noninteractive shell command
the user has registered in a launcher configuration file against the selected
file or directory, and inspect or stop it from the Web UI. TensorBoard is the
first service the feature must support: a registered TensorBoard entry is
reachable through the existing `rotari web` connection without another SSH
port forward.

This is a personal-use feature. Registered commands are arbitrary shell code
that the user wrote, and they run with the OS permissions of the Web server's
user. The Web UI and API only choose a registered command and an artifact; they
never accept command text. Registration is therefore an executable allowlist
for what the Web interface can start, not a sandbox for what a command does.

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
- Arbitrary user-registered shell commands, defined in a dedicated launcher
  configuration file, with a verified TensorBoard entry in its template.
- Ordinary command mode and HTTP service mode.
- Registered cwd rule, startup-environment inheritance, live logs, exit
  status, service readiness, stop, and reuse of matching live launches.
- One launcher manager per Web server, shared across its basedir mounts.
- A server-wide launch list reachable after leaving the Artifacts view.

Exclude initially:

- Command text, arguments, or cwd supplied from the browser or Web API,
  including one-off edits of a registered command.
- Registering, editing, or deleting commands from the Web UI or API.
- Built-in commands that exist without the user registering them.
- Launching from the CLI, MCP server, or Python client. Their API listings
  must not gain launcher endpoints.
- Interactive stdin, PTYs, browser terminals, and terminal reconnection.
- SSH/Slurm/PBS/LSF/SGE launch backends or automatic execution on a job's host.
- Automatic reconstruction of a historical training environment.
- Surviving Web server restart or adopting another server's processes.
- A multi-user permissions model, containers, or OS sandboxing.
- Automatic launching when an artifact is discovered or a page is opened.
- A generic proxy that accepts an arbitrary upstream URL.

## User flow

1. Open a job's Artifacts view and select an entry or directory child.
2. Choose **Launch with > NAME**, listing the registered commands whose
   target kinds match the selection. Commands that do not match are not
   offered; with none registered, show how to create the launcher file.
3. Show a read-only dialog containing the command name and text, absolute
   target, actual execution host and user, originating job/attempt, cwd, and
   mode.
4. Validate and launch only after explicit confirmation. Show preparation and
   startup failures with their logs, rather than opening a broken link.
5. In service mode, enable **Open** after HTTP readiness succeeds. Open in a
   separate tab; embedding in an iframe is not required.
6. Show **Open**, **Logs**, and **Stop** beside a live service, and **Logs** and
   **Stop** beside an ordinary command. Retain exit status after completion.

The registered command chooses its cwd rule: the target directory (for a file,
its parent), the target's parent, or a fixed absolute directory. The server
validates the effective cwd as an accessible directory and never silently falls
back to the Web server's startup directory. A command registered only for
directories, such as TensorBoard, is not offered for a selected file; a
selected event file can have an explicit **Use parent directory** action, not
an implicit target change.

The target must be accessible on the Web host. A path recorded on a remote
node is not evidence that it exists here. Missing, unresolved, outside-root,
or inaccessible targets get an explicit explanation.

## Execution and registration model

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
shell source. Registered commands, including the template entries, quote the
variable expansions. Do not inherit the selected job's saved environment or
return raw inherited environment values to clients. Users may explicitly
write `conda run`, an absolute executable, or environment assignments in their
command.

An illustrative TensorBoard command is:

```sh
tensorboard \
  --logdir "$ROTARI_LAUNCHER_ARTIFACT" \
  --host 127.0.0.1 \
  --port "$ROTARI_LAUNCHER_PORT" \
  --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"
```

Verify these options and proxy behavior against an installed TensorBoard
before shipping the template entry. Rotari must not silently install it.
Report a missing executable/environment as a launch failure.

### Registered commands

Commands are registered in a dedicated `launchers.toml`, separate from
ordinary command configuration, following the precedent of
`notifications.toml`. Each entry has a unique name, the command text, the
target kinds it applies to (file, directory, or both), its cwd rule,
command/service mode, and, for services, a readiness path and timeout.
Unknown keys, duplicate names, empty commands, unknown modes or target kinds,
relative fixed cwds, and settings that do not apply to the entry's mode are
errors, not ignored values.

Read the file only from the global config directory (`config.HomeDir()`).
Do not read it from a workspace, basedir, or project scope: one Web server
mounts several basedirs, basedirs may live on shared storage written by
others, and the commands must not change with the selected basedir or job.
Never load executable definitions from artifact or run contents. Provide
`rotari config --launchers` to generate a commented template containing the
verified TensorBoard entry, mirroring `rotari config --notifications`, and
list the file in `rotari config --list`.

`rotari web --allow-exec` loads and validates the file once at startup. A
missing file means no commands are registered and the UI says how to create
one; a malformed or unreadable file fails startup with its path. Edits take
effect after the Web server restarts; live reload is a later feature. Parse
and validate through one owning package, which is the only source of
registered definitions for the manager and the Web layer.

### Identity and reuse

Give each launch an opaque server-generated ID. Record its source basedir,
project, run, job, selected attempt, artifact entry/child, resolved target,
cwd, the registered command's name, and a snapshot of its definition as
loaded, mode, and lifecycle timestamps.

Serialize matching start requests: the command name, resolved target, and
effective cwd define the reuse key before generated port/path values. Two
jobs whose artifacts resolve to the same directory share one live
TensorBoard. Return the existing starting/ready/running launch on duplicate
submission. Provide an explicit **Start another instance** action. Never reuse
an exited, failed, or stopping instance.

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
`rotari web --allow-exec`. Require both it and `allow-control` for launch and
stop; reject conflicting startup options rather than silently ignoring one.
Static export must reject execution-only options and show launch controls as
unavailable. There is no Web mutation of registered commands to protect.

When execution is enabled:

- Require authentication on non-loopback listeners; preserve existing warning
  behavior for servers without execution enabled. Recommend SSH forwarding or
  HTTPS, since a token does not encrypt HTTP.
- Validate Host against configured serving authorities and validate browser
  Origin for mutations; reject cross-origin requests. Define the authenticated
  non-browser path explicitly rather than treating a missing Origin as proof
  of safety. Do not enable wildcard CORS. Cover DNS-rebinding scenarios.
- Use POST for starting/stopping; GET and page loading never execute.
- Resolve artifact identity on the server against registered basedirs and the
  shared artifact rules. Browser-supplied display paths are not authority.
- Resolve the command by name against the definitions loaded at startup. A
  request carrying command text, cwd, mode, or service settings is rejected,
  not partially honored.
- Do not claim that artifact-root checks constrain arbitrary commands: the
  command has the Web user's filesystem permissions and can write/delete files.
  Even a validated path can change before an external process opens it.
- Treat command text and logs as potentially sensitive. Do not include them in
  general `/api/state`, static exports, notifications, or public error pages.
  Render them as text, not executable HTML.

## Package and API boundaries

- Proposed `internal/launcher`: execution specification, `launchers.toml`
  schema/loading/validation, lifecycle manager, deduplication, logs,
  readiness, and process ownership. It only reads the file; nothing in rotari
  writes it except the `config --launchers` template generator. No dependency
  on HTTP, job status, or project-run orchestration.
- `internal/webui`: resolve selected artifact context through existing shared
  helpers, enforce capabilities, adapt requests, serve the fixed-target proxy,
  and render dialogs/list/status. Factor shared artifact resolution only as
  needed; do not expose lower-level helpers that bypass its policy.
- `cmd/rotari`: parse execution flags, load the launcher file at startup,
  construct one manager, generate the `config --launchers` template, and
  coordinate HTTP and process shutdown. Add private helper dispatch only if
  required by the proven crash-cleanup design.

Provisional API: registered-command list (read-only), launch
list/start/detail/logs/stop, and a service proxy route keyed by launch ID.
Start inputs are only the source artifact identity, the registered command
name, and whether to start another instance. No command-text, cwd, free
target-path, or upstream-URL input. Return explicit lifecycle/error fields and
appropriate HTTP errors; never convert startup errors into success-shaped
responses.

## Implementation phases

1. **Feasibility and lifecycle checkpoint.** Prove TensorBoard path-prefix
   proxying over one forwarded port, readiness/port ownership, process-group
   stop, and parent-death cleanup in isolated fixtures. Finalize runtime paths,
   resource bounds, the `launchers.toml` schema, and supported proxy
   authorities.
2. **Registration and command manager.** Implement `launchers.toml` loading and
   validation, `config --launchers`/`--list`, process ownership, command-mode
   start/list/log/stop, deduplication, and cleanup with unit tests.
3. **Web capability and Artifacts entry.** Wire opt-in/auth/request checks,
   artifact resolution, and name-based command resolution; add the
   **Launch with** menu, confirmation dialog, and server-wide launch list.
   Verify a registered command can be launched, observed, and stopped from
   Artifacts.
4. **HTTP services and TensorBoard.** Add allocated upstreams, readiness,
   authenticated prefix proxying, the TensorBoard template entry, and
   Open/reuse actions.
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
  separately. Missing paths/cwd/executables show errors.
- A missing `launchers.toml` registers nothing and explains how to create it;
  a malformed one (unknown keys, duplicate names, bad modes/target kinds/cwd
  rules) fails `rotari web --allow-exec` startup with its path. Files in
  workspace, basedir, project, and artifact locations are never loaded.
- Commands are offered only for matching target kinds. Start requests naming
  an unknown command, a command for the wrong target kind, or carrying
  command text, cwd, mode, or service settings are rejected without spawning.
- Logs stream/page without unbounded memory/disk use; exit codes are accurate.
  Stop kills shell descendants, readiness timeout cleans up, and both graceful
  and abrupt parent-exit fixtures verify process cleanup.
- Disabled execution, read-only mode, static export, bad auth, cross-origin and
  invalid Host requests cannot start or stop commands.
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
