# Security model

Rotari assumes a trusted single-user or HPC/lab environment. The Web UI token
provides HTTP authentication, not encryption.

## State-file permissions

`--basedir/-b`, `--masterdir`, and their contents (queues, metadata, locks,
job output and status, wrapper scripts) default to shared `0755`/`0644`
permissions, because colleagues on the same cluster commonly share a job's
log path directly.

Set `ROTARI_PRIVATE_STATE=true` for owner-only `0700`/`0600` permissions to
keep job commands, working directories, and output private. This affects only
newly created paths, which are not re-chmodded later, and applies to the whole
`--basedir/-b`.

## Supervisor

A run's supervisor opens no socket or port. Only the `run` command that
started it talks to it over inherited pipes. This works where Unix sockets are
blocked and with base directories of any length.

While it runs, the supervisor holds
`<basedir>/projects/<project>/server.lock` and records its PID in `server.pid`
next to it. A failed start is recorded in `<basedir>/server.log`.

## Web UI authentication and transport

Without `ROTARI_WEB_AUTH_TOKEN` or `--auth-token`, bind the Web UI to
`127.0.0.1`. With a token, browser login and session cookies work over plain
HTTP only on a direct loopback connection: a browser on the same host opening
`http://localhost` or `http://127.0.0.1`, with no proxy headers. Everywhere
else, use HTTPS through a trusted reverse proxy; browser login is refused over
other plain HTTP. API token credentials are also unencrypted over HTTP, so use them only on a
trusted network or over HTTPS. Prefer the environment variable so the token
does not appear in the process list.

Environment variable values are never exposed over HTTP. Project and run
pages do show raw global/workspace/basedir/project config sources and saved
merged run file-config snapshots, which may contain secrets. Static exports
include those contents as well; share them only with trusted readers.

The notification settings UI visually masks a saved webhook URL and does not
return it from its read API. This does not encrypt the connection: a
replacement URL is sent to the Web server when saved. Use HTTPS through a
trusted reverse proxy whenever the browser-to-server network is not trusted.

## Artifact files and static exports

`rotari run` reads the configuration files, shell scripts, and Python files a
job names, as the user running it, to find the paths the job uses. It records
only those paths in each attempt's `artifacts.json`, never the files'
contents.

The live Web UI serves the contents of recorded files and directories under
each job's working directory and under the directory holding its basedir.
Anyone who can open the Web UI can read them; with job control enabled, they
can also add a job that names another file there. The Web server reads files
as the user running it, whoever views the page.

Whoever can write a project's state decides what its jobs record. Therefore,
for a basedir another user can write (group-writable, or owned by someone
else), neither the working directory nor the basedir's parent is served; only
`--artifact-root` is served. Paths outside the allowed roots, and symlinks
leading out of them, are not served unless `rotari web --artifact-root DIR`
allows them. Keep such roots narrow. SVG files are shown only as images, so
their scripts do not run.

A static export contains only paths unless it is made with
`--static-artifact-contents`, which copies files into the export. Anyone who
can read the export, such as a published site, can then read those files. The
command reports how many files and bytes it copied.

## Launching services and commands

Launching is off by default. `rotari web --allow-exec` lets anyone who can
sign in run the built-in services (TensorBoard, Jupyter Lab, MLflow, Aim,
Streamlit) and commands registered in your global `launchers.toml`, as you,
against the job artifacts the Web UI may serve. See
[Launching commands on artifacts](INSPECT.md#launching-commands-on-artifacts).

The browser and API only name a registered command and an artifact. Command
text, working directories, and modes come only from `launchers.toml`, which
rotari refuses unless no other user can write or replace it. The listed
artifact roots limit what can be selected, not what a command does: a command
has all your file permissions, and a path can change between the check and
the command opening it.

With launching enabled, `rotari web` always requires a token, even on
`127.0.0.1`, since other users of a shared host can reach loopback ports. It
accepts only `localhost`, `127.0.0.1`, `[::1]`, a specific `--host`, and
`--allowed-host` names as the `Host` header, against DNS rebinding. It rejects
a POST from another origin, or one without `Origin` unless it authenticates
with `X-Rotari-Token` or a bearer token, which a browser never adds by itself.
Launched commands get neither the Web token nor stdin.

A launched service is served from the Web UI's own origin under `/_launch/ID/`,
behind the same token. Requests reach it without the `Authorization` or
`X-Rotari-Token` header. Its pages, however, run with the Web UI's origin and
can call the rotari API as the signed-in browser. Treat a registered service
as trusted code, like the command itself.

The proxy connects only to the launch's own `127.0.0.1` port. Before each new
connection, it checks that the listening process belongs to the launch
(through `/proc` on Linux, `/usr/sbin/lsof` on macOS).

A launched service also listens on that loopback port, which other users of
the host can reach directly without rotari's token. A service that can require
a secret should use the launch's `ROTARI_LAUNCHER_TOKEN`, which the proxy
presents through `proxy_headers`. The built-in Jupyter Lab does, so its
kernels serve only through the proxy. TensorBoard, MLflow, Aim, and Streamlit
cannot, so another user who finds their port can read what they show.
WebSocket handshakes, used by services such as Jupyter and Streamlit, need
the same `Origin` as other mutating requests.

Launched-command output stays in a private `0700` directory under `$TMPDIR`
until the Web server stops. A later `rotari web --allow-exec` removes
directories that a killed server left behind.

## Limits

This model does not defend against another user with access to your own UID
(for example, root or anyone who can read your home directory). It protects
only against other unprivileged users on a shared machine.
