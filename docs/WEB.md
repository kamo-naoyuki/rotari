# Web UI

`rotari web` serves a status UI for your runs: job state, logs, history across
projects, artifacts, and config. It is a separate process from `rotari run`.
Closing or stopping it never affects runs; it only stops the commands it
launched itself (see [Launching commands](#launching-commands)).

```sh
rotari web                  # http://127.0.0.1:<default port>, with job control
```

A [read-only demo](https://kamo-naoyuki.github.io/rotari/) built from generated
data shows the UI without installing anything.

## Choosing a mode

Each mode adds capability, and with it, risk. Pick the smallest one that does
what you need.

| Mode | Command | What the browser can do | Token |
| --- | --- | --- | --- |
| Static export | `rotari web --static-dir DIR` | Browse a snapshot as plain files; nothing is served live. | n/a |
| Read-only | `rotari web --allow-control=false` | View everything the live server serves; control and file-write APIs return `403`. | Recommended beyond loopback |
| Control (default) | `rotari web` | Also copy, change, remove, cancel, and clear jobs, edit queues, and save config files. | Required beyond loopback |
| Launching | `rotari web --allow-exec` | Also run registered commands and built-in services against artifacts. | Always required |

- Job control does not start runs; execution starts only through `rotari run`.
- `--allow-exec` needs job control, so it cannot be combined with
  `--allow-control=false` or `--static-dir`.
- Options that only make sense for one mode, such as `--host` with
  `--static-dir`, are rejected rather than ignored.

## Options

The full list is in the [CLI reference](CLI_REFERENCE.md#rotari-web); the
environment variables are in
[Environment variables](ENVIRONMENT_VARIABLES.md). In short:

| Option | Purpose |
| --- | --- |
| `--host`, `--port` | Listen address; the default host is `127.0.0.1`. |
| `--auth-token` | Require a token. Prefer `ROTARI_WEB_AUTH_TOKEN` so it stays out of the process list. |
| `--allow-control` | Job control on (default) or `=false` for read-only. |
| `--allow-exec` | Let the UI launch registered commands; see below. |
| `--allowed-host NAME` | With `--allow-exec`, accept one more `Host` header name (repeatable). |
| `--artifact-root DIR` | Also serve recorded artifact files under `DIR` (repeatable). |
| `--static-dir DIR` | Write a static export instead of serving. |
| `--static-artifact-contents` | With `--static-dir`, copy previewable artifact files into the export. |
| `--notifications` | Default state of the browser notification toggle. |

## Reaching the UI

- **Same machine:** the default `127.0.0.1` binding needs no token unless you
  use `--allow-exec`.
- **Remote machine:** forward the port with `ssh -L PORT:127.0.0.1:PORT host`
  and keep the loopback binding. Launched services open through the same port,
  so no further forward is needed.
- **Shared network:** set a token and put an HTTPS reverse proxy in front.
  Browser sign-in is refused over plain HTTP. Sign in with the user name
  `rotari` and the token as the password; the session lasts 12 hours. API
  clients send `Authorization: Bearer TOKEN` or `X-Rotari-Token: TOKEN`.

The token is authentication, not encryption. See
[Security model](OPERATIONS.md#security-model) before exposing the server, and
the [FAQ](FAQ.md#is-rotari-web-safe-to-expose-beyond-127001).

## What you can do

- **Runs and jobs:** state, attempts, logs, and reports for a run or selected
  jobs. A report is never sent anywhere; see the
  [FAQ](FAQ.md#does-the-web-ui-send-run-details-to-an-ai-service).
- **Several projects and state directories:** switch from the sidebar.
  **Activity** shows runs across every state directory on a calendar;
  **Job activity** lists running and recently finished jobs;
  **History search** searches project, run, and job history (live server only).
- **Control** (not in read-only mode): copy, change, remove, cancel, and clear
  jobs, and **Create queue** or **Append to queue**, which a separate
  `rotari run` executes.
- **Artifacts:** preview recorded files and directories, download files, and
  open CSV/TSV files in the Table viewer, which works without `--allow-exec`.
  See [Artifacts](INSPECT.md#artifacts) and
  [Viewing tables](INSPECT.md#viewing-tables).
- **Config:** **Generate config** and **View config** edit one config source at
  a time; see [Configuration](CONFIGURATION.md).
- **Notifications:** the in-page inbox and optional desktop popups; see
  [Notifications](NOTIFICATIONS.md#browser-notifications).
- **Theme:** `Settings` selects Light, Dark, or System.

## Launching commands

With `--allow-exec`, the UI can run TensorBoard, Jupyter Lab, MLflow, Aim,
Streamlit, and the commands in your global `launchers.toml`, against an
artifact. Use **Launch with…** in a job's Artifacts, a job's **Launch** button,
or the run toolbar; **Launches** lists and stops them.

Start it with:

```sh
export ROTARI_WEB_AUTH_TOKEN=...      # optional; otherwise one is generated and printed
rotari web --allow-exec
```

What is different about this mode, in brief:

- The browser names a registered command and an artifact; command text,
  working directory, and mode come only from `launchers.toml`, which rotari
  refuses unless no other user can write it.
- A token is required even on `127.0.0.1`, and requests must have an
  acceptable `Host` and, for POST, `Origin`.
- Commands run as you with your file permissions; the artifact roots limit
  what can be selected, not what a command does.

The details are not repeated here:
[Launching commands on artifacts](INSPECT.md#launching-commands-on-artifacts)
for use, [Launcher commands](CONFIGURATION.md#launcher-commands) for the file,
and [Security model](OPERATIONS.md#security-model) for the guarantees and
limits, including the loopback port a launched service listens on.

## Static export

`rotari web --static-dir DIR` writes a snapshot you can publish or open
offline. It contains the raw config sources and saved config snapshots of the
runs, so share it only with readers you trust, and it holds artifact paths
only, unless `--static-artifact-contents` copies the files in. History search,
launching, and control are not available in it. See the
[Security model](OPERATIONS.md#security-model).

## Related

- [FAQ: Web UI](FAQ.md#web-ui)
- [CLI reference: `rotari web`](CLI_REFERENCE.md#rotari-web)
- [Operations: Security model](OPERATIONS.md#security-model)
