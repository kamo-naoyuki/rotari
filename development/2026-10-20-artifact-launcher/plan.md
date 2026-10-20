# Plan: Launch commands and services from Artifacts

Created: 2026-10-20
Status: Phases 1 to 4 done; Phase 5 (contract IDs and conformance rows)
next. See [Phase 1 results](#phase-1-results),
[Phase 3 notes](#phase-3-notes), and [Phase 4 notes](#phase-4-notes).

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

When launching is unavailable, keep **Launch with** visible and explain on use
why it is unavailable and how to enable it, instead of hiding it or failing
without a reason:

| Server state | Explanation |
| --- | --- |
| Started without `--allow-exec` | Restart with `rotari web --allow-exec`, which requires signing in with a token (printed at startup unless `ROTARI_WEB_AUTH_TOKEN` is set); register commands in the launcher file (path shown), generated by `rotari config --launchers` |
| Started with `--allow-control=false` | Restart without `--allow-control=false`; launching also requires job control |
| `--allow-exec`, no commands registered | Register commands in the launcher file (path shown), then restart |
| No registered command matches the target kind | Name the registered commands and the target kinds they accept |
| Static export | Launching is unavailable in a static export; open the live `rotari web` server |

The server produces these explanations. The start API returns the same
reason and hint with its rejection status, and the UI shows that text, so the
CLI-facing hint is written once and the UI and API do not drift apart. The
existing read-only hint ("restart with --allow-control to enable job control")
is the precedent.

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

Add `127.0.0.1,localhost,::1` to `NO_PROXY` and `no_proxy` (appending to
inherited values) for every launch. Phase 1 found that with `http_proxy` set
and no `no_proxy`, as on the development host, TensorBoard's gRPC connection
to its own data server on localhost goes through the corporate proxy and
fails. Loopback traffic never needs a proxy, so this is not a behavior the
user can want. Rotari's own readiness probe and proxy transport set no proxy
at all.

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

Before reading the file, check that no other OS user can write or replace it,
in the manner of OpenSSH `StrictModes`; a failing check stops startup with the
offending path and the required fix:

- The file, after resolving symlinks, is a regular file owned by the Web
  server's user and not writable by group or others.
- Every directory from the resolved file's directory up to the filesystem
  root is owned by that user or root and not writable by group or others,
  unless it is root-owned and sticky (such as `/tmp`). A writable directory
  would let another user rename a different file into place.
- When symlinks are involved, apply both checks to every link and to the
  directories containing each link, not only to the final target.
- Read the file through the descriptor that was checked, so it cannot be
  swapped between the check and the read.

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
- Abrupt Web death also needs cleanup. Each launch runs under the hidden
  `rotari __launch` wrapper (`launcher.RunWrapper`), which holds the read end
  of a lifetime pipe whose write end only the Web process holds. When the Web
  process exits for any reason, including SIGKILL, the kernel closes the
  write end and every wrapper stops its command's group. The wrapper does not
  reuse the training supervisor. Limitations: a process that leaves the group
  (`setsid`, daemonizing) is not tracked, and killing the wrapper itself with
  SIGKILL (for example, `kill -9` on its group, or the OOM killer) leaves the
  command running.
- Do not recover ownership or signal processes from a stale PID file after
  restart. Daemonizing commands that escape the process group are unsupported.

Use a private per-Web-session runtime directory outside project/run history
for logs and any manager records. Filesystem records own retained metadata;
memory indexes live process handles. A new Web server never adopts old sessions.
Use bounded log retention with visible truncation, paged log reads, and a
clear completed-record retention policy. Remove only this session's owned
runtime files during cleanup. The exact paths and bounds are under
[Phase 1 results](#phase-1-results).

## Service access

Keep the upstream bound to `127.0.0.1` and proxy through the existing Web
listener. Allocate the upstream and proxy path on the server, never from a
browser-provided host or URL. Rotari picks the port (bind port 0 on loopback,
read the assigned port, close it) because the proxy must know where to send
requests and several services, such as TensorBoards for different
directories, run at once; a port fixed in `launchers.toml` would allow only
one instance per command.

Between allocation and the service binding the port, another local process,
possibly another user's, can bind it first. The service then usually fails to
bind and exits, but until it does, a foreign server can answer on the port.
Before sending the readiness probe, verify that the process listening on
`127.0.0.1:PORT` belongs to the launch's process group, so neither the probe
result nor any response from a foreign listener is ever used:

- Linux: map the listening socket's inode from `/proc/net/tcp` to a process
  through `/proc/<pid>/fd`, and compare that process's group.
- macOS: find the listening PID with `lsof -nP -iTCP:PORT -sTCP:LISTEN -Fp`,
  which macOS ships, and compare `getpgid(pid)`.

Implement this as one platform-neutral `internal/launcher` function with
per-OS implementations (`launcher.VerifyListener`). If the check cannot run
(for example, `lsof` is missing or `/proc` is unreadable), the launch fails
with that reason rather than skipping verification. A foreign listener stops
the launch's process group and reports that another process holds the port;
the launch never becomes ready, so its port is never proxied.

A port can also be lost to an outgoing connection: loopback connections use
ephemeral source ports from the same range that port 0 allocates from, and
such a connection blocks the bind. The launcher's parallel tests hit this.
The service then fails to bind and exits, which is reported as a startup
failure; Phase 4 retries once with a newly allocated port when a service
exits with no listener ever seen.

The service must listen on exactly `127.0.0.1:PORT`. A service in the launch
listening on `0.0.0.0` or `::` instead fails the check with a "bind it to
127.0.0.1" error: it would be reachable from the network, and on macOS,
where `lsof` cannot see other users' sockets, only an exact bind guarantees
that no other user holds the same address. Readiness is: wait until a TCP
connection to the port succeeds, then `VerifyListener`, then the HTTP probe.

Once the service holds the port, no other user can bind it, so one check at
readiness is enough while the service runs. When the service exits, the
proxy stops routing to the launch as soon as the manager observes the exit.
To close the window between that exit and its observation, re-verify
ownership before opening each new upstream connection; reused keep-alive
connections need no new check.

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

- Require token authentication on every listener, loopback included: on a
  shared login node other users can connect to `127.0.0.1`. When neither
  `--auth-token` nor `ROTARI_WEB_AUTH_TOKEN` is set, generate a random token
  at startup and print it on stderr with how to sign in (Basic auth, user
  `rotari`, the token as password). Never put the generated token in argv,
  logs under shared directories, or `/api/state`. Note that the token
  changes on every restart and recommend `ROTARI_WEB_AUTH_TOKEN` for a stable
  one rather than `--auth-token`, which other users can read through `ps`.
  Preserve existing behavior for servers without execution enabled.
  Recommend SSH forwarding or HTTPS, since a token does not encrypt HTTP.
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
   proxying over one forwarded port, readiness, process-group
   stop, and parent-death cleanup in isolated fixtures. Finalize runtime paths,
   resource bounds, the `launchers.toml` schema, and supported proxy
   authorities. Treat these as pass/fail gates before implementing the Web
   proxy: a local process that wins the allocated-port race must never make a
   launch appear ready or have its response proxied; forcibly killing the Web
   process must clean up its launcher process groups; and the launcher file
   must not be replaceable or writable by another OS user on supported
   platforms. If any gate cannot be met, revise the service or configuration
   design before proceeding.
2. **Registration and command manager.** Implement `launchers.toml` loading and
   validation, process ownership, command-mode start/list/log/stop,
   deduplication, and cleanup with unit tests, and dispatch `rotari
   __launch`. Done; `config --launchers`/`--list` moved to Phase 3 so that
   the user-visible options arrive with `--allow-exec`, which they refer to.
3. **Web capability and Artifacts entry.** Wire opt-in/auth/request checks,
   artifact resolution, and name-based command resolution; add
   `config --launchers`/`--list`, the **Launch with** menu, confirmation
   dialog, and server-wide launch list.
   Verify a registered command can be launched, observed, and stopped from
   Artifacts.
4. **HTTP services and TensorBoard.** Add allocated upstreams, readiness,
   authenticated prefix proxying, the TensorBoard template entry, and
   Open/reuse actions.
5. **Contracts, docs, and end-to-end validation.** Document behavior and limits,
   cover interfaces/variants below, and complete repository checks.

Each phase is a coherent commit/checkpoint. Do not ship a launcher that lacks
its required shutdown or access protections as a temporary default.

## Phase 1 results

The building blocks are in `internal/launcher`. Phase 2 added the registry,
the manager, the `__launch` dispatch in `cmd/rotari`, the `internal/archtest`
boundary rule (no rotari imports, no `net/http`), and the package's entry in
`docs/ARCHITECTURE.md` and `contracts/00-overview.md`. Phase 3 is the first
caller.

| Gate | Result | Evidence |
| --- | --- | --- |
| A process that wins the port race is never trusted | Passed on Linux; macOS compiles, untested (CI is Linux only) | `TestVerifyListenerRejectsForeignListener` (fails with the group check disabled), `TestVerifyListenerRejectsNonLoopbackBind`, `TestParseLsofListeners` |
| Killing the Web process cleans up its launches | Passed | `TestKilledOwnerStopsLaunches` SIGKILLs a stand-in Web process (fails when the wrapper ignores lifetime EOF), `TestLifetimeCloseStopsCommands` |
| `launchers.toml` cannot be written or replaced by another user | Passed for mode and ownership checks; another owner cannot be created without root, so that branch is untested | `TestOpenTrustedFileRejectsWritableElements` (file, directory, symlink in and to a writable directory), `TestOpenTrustedFileAcceptsPrivateFiles` (sticky directory) |
| TensorBoard works behind a fixed-target prefix proxy on one port | Passed with TensorBoard 2.21.0 | Manual run behind `httputil.ReverseProxy`; `TestTensorBoardBehindPrefix` when `ROTARI_TEST_LAUNCHER_TENSORBOARD` is set |

TensorBoard findings:

- `--path_prefix P` serves everything under `P`. Its redirect from `P` to
  `P/` uses a relative `Location`, its assets are relative (`index.js`), and
  paths outside `P` return 404. No HTML rewriting is needed.
- With its port taken, TensorBoard exits with status 255 within a second
  ("could not bind to port"). This is the startup failure the user sees.
- `--reuse_port true` would let rotari reserve the port with `SO_REUSEPORT`
  and hand it over. It only works for TensorBoard and is not needed while
  `VerifyListener` is in place, so it is not used.
- Inherited `http_proxy` without `no_proxy` breaks TensorBoard's own
  loopback gRPC; see the `NO_PROXY` rule under Commands.

Decisions:

- **Process model.** `launcher.Start` runs `rotari __launch --grace D --
  /bin/sh -c COMMAND` in a process group of its own; the wrapper runs the
  shell in another group and reports `started PGID` and the final
  `exit CODE` / `signal N` on descriptor 4. Stop signals the wrapper, which
  sends SIGTERM to the command's group and SIGKILL after the grace period
  (default 5s). When the shell exits, the members it left in its group are
  stopped the same way, so a finished command leaks nothing.
- **Runtime directory.** `os.MkdirTemp("", "rotari-web-launches-")` (mode
  0700) per Web server, removed at shutdown. It holds launch logs only; launch
  records live in memory and end with the server.
- **Logs.** The command writes to a pipe that the manager copies into two
  rotating 8 MiB segments per launch; a read past the dropped segment says
  that earlier output was dropped. Reads are paged.
- **Retention and limits.** At most 16 live launches per server; starting
  another fails with that reason. The 50 most recently ended launches are
  kept with their logs; older ones and their logs are removed.
- **Schema.** Defaults shown; `name`, `command`, `targets`, and `mode` are
  required:

  ```toml
  [[launcher]]
  name = "TensorBoard"
  command = '''tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"'''
  targets = ["directory"]  # "file", "directory"
  mode = "service"         # "command", "service"
  cwd = "target"           # "target", "parent", or an absolute directory
  ready_path = "/"         # service only; relative to the base path
  ready_timeout = "60s"    # service only
  stop_grace = "5s"
  ```

- **Proxy route.** Launches belong to the server, not to a basedir mount, so
  the proxy is the top-level `/_launch/<ID>/`, outside `/_basedir/<ID>/` and
  its prefix stripping; `ROTARI_LAUNCHER_BASE_PATH` is `/_launch/<ID>`.
  Rotari has no sub-path reverse-proxy support, so a deployment must serve
  rotari at the root of its origin.
- **Proxy authorities.** With `--allow-exec`, accept `Host` values
  `localhost`, `127.0.0.1`, and `[::1]` with any port, so SSH forwarding to a
  different local port works and DNS rebinding (which uses an attacker's
  name) does not. Also accept the literal `--host` address when it is a
  specific non-loopback address. Any other name, such as one served through
  an HTTPS reverse proxy, needs a new repeatable `--allowed-host NAME`
  option, added in Phase 3. A mutation's `Origin` must match its `Host`.

## Phase 3 notes

Decisions made while implementing the command-mode Web feature:

- `--allow-exec` and `--allowed-host` are command-line only, so a config file
  (possibly on shared storage) or an environment variable cannot turn
  execution on. Both are rejected with `--static-dir` (WEB-3), and
  `--allowed-host` without `--allow-exec`.
- Launched commands do not inherit `ROTARI_WEB_AUTH_TOKEN`.
- `GET /api/launch-preview` resolves exactly what a start would run
  (shared `launchStart`), so the confirmation dialog shows the server's
  cwd rather than recomputing the rule in the browser. A target kind the
  command does not accept is a 400 there and at start.
- `/api/launch-log?from=end` returns the last page; the Launches view shows
  the last 1 MiB.
- The Launches button sits in the top bar and is shown only when launching
  is available; Launch with sits in each preview's title line.
- A manager holds an flock on its runtime directory, and a new manager
  removes this user's unlocked `rotari-web-launches-*` directories older than
  a minute, which a SIGKILLed Web server leaves behind (found in the manual
  end-to-end check).
- The user guides document command mode and say that service mode is not
  available yet; Phase 4 replaces that note.

## Phase 4 notes

- Readiness is: TCP connect, `VerifyListener`, then one HTTP GET of the base
  path plus `ready_path` accepting 2xx or 3xx. `internal/launcher` speaks
  that one request over `net` itself, keeping its archtest rule (no
  `net/http`). `ready_path` may not contain spaces.
- A service that exits before anything ever listened on its port is
  restarted once on a newly allocated port, and the log says so. A failure
  after that, a foreign or non-loopback listener, or a readiness timeout
  stops the group and records the reason as a failed launch.
- The manager gets the base path from `ManagerConfig.ServicePath`, which
  `cmd/rotari` sets to `webui.LaunchProxyPath`, so only `internal/webui`
  knows the `/_launch/` route. `/_launch/ID` redirects to `/_launch/ID/`.
  The route answers 503 while a launch is starting or after it has ended,
  404 for an unknown ID, and 403 without `--allow-exec`.
- The proxy keeps one `http.Transport` per ready launch; its dialer connects
  only to the launch's loopback port and runs `VerifyListener` on every new
  connection. `Authorization` and `X-Rotari-Token` are removed; paths and
  queries pass unchanged, and responses stream (`FlushInterval: -1`).
- End-to-end with the built binary and TensorBoard 2.21.0, running the
  template entry unchanged except for the executable: the service was ready
  in about 1.5 s with `http_proxy` set and no `no_proxy` in the Web server's
  environment. Through the rotari port, the index page, its 5.9 MB script,
  `data/runs`, and the event fixture's scalars were served; unauthenticated
  requests got 401. SIGKILL of `rotari web` ended the wrapper, TensorBoard,
  and its data server within about 0.4 s.

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
- A local process that binds the allocated service port first and returns a
  successful readiness response is never shown as **Open**, and its response
  is never served by the Rotari proxy. The check must cover the launcher still
  being alive during the probe, not only the case where it has already exited.
- On supported platforms, forcibly terminating the Web process cleans up its
  launcher process groups. The `launchers.toml` file (including its resolved
  target when symlinks are involved) cannot be replaced or written by another
  OS user, including through a writable containing directory; reject unsafe
  ownership or permissions, or document a narrower supported configuration
  model with an explicit reason.
- Disabled execution, read-only mode, static export, bad auth, cross-origin and
  invalid Host requests cannot start or stop commands.
- With `--allow-exec`, a loopback server without a configured token generates
  and prints one, and unauthenticated requests to every route, including the
  service proxy, are rejected. A configured token is used as given and never
  printed.
- Each unavailable state in the User flow table shows its explanation and
  enabling step in the UI, and the start API returns the same reason.
- A service that exits because its port is already in use is reported as a
  startup failure with its log and never becomes **Open**.
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
