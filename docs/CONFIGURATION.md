# Configuration

Config files, environment variables, run completion webhooks, and shell completion.

## Configuration files

Run the following command to choose a config file location interactively and
generate a template. The available options and their descriptions are shown
there.

```sh
rotari config
```

Use `rotari config --list` to list every existing config file—including
`notifications.toml`—found in the global and basedir locations, cwd workspace
`.rotari.toml`, plus every
project below the basedir. It groups common files under `Common:` and
project-specific files under `Projects:`, with each project name followed by
indented paths. This is an inventory, including files in unselected projects.
Supplying `--project-name` limits the project-specific entries to
that project.

```sh
rotari config --list --basedir DIR
```

The resolution order is:

```text
CLI option (e.g., --retry)
environment variable (e.g., ROTARI_RUN_RETRY)
configuration value (from merged files)
built-in default
```

Commands that load configuration accept `--config FILE` to use a specific
YAML, TOML, or JSON file instead of looking in the project, basedir, workspace, and global
locations. Both `--config FILE` and `--config=FILE` are supported. An explicitly
selected file must exist and parse successfully; `--config` itself cannot be
set by an environment variable or another config file.
This value precedence applies across commands; see the
[CLI contract](https://github.com/kamo-naoyuki/rotari/blob/main/contracts/03-server-and-command-interfaces.md#cli-presentation).

Command help remains available if the selected configuration file is missing
or malformed: for example, `rotari run --config missing.toml --help` prints a
warning on stderr and help on stdout, exiting successfully. In that case,
help uses built-in and environment defaults rather than configuration values.
When the file loads successfully, help still shows the configured defaults.
Normal execution continues to fail on configuration errors; `--help` used as
an option value or inside an `add`/`change` job command is not a help request.

Without `--config`, ordinary configuration merges **global → workspace →
basedir → project**. Global, basedir, and project directories accept one of
`config.yaml`, `config.toml`, or `config.json`; multiple formats in one scope
are an error. Workspace configuration is only `.rotari.toml` in the **current
working directory**: parent directories are never searched. Malformed or
unreadable discovered files fail with their scope and path.

Maps merge recursively; higher-scope scalars and arrays replace lower ones.
`null` is unspecified and retains the lower value; `false`, `0`, and `""` are
explicit values. After merging, a command section (such as `[run]`) overrides
the root key, even when that section originated in a lower scope. CLI and
environment overrides still win. Output paths such as `export --output` are
command-line-only.

### Workspace defaults and initialization

Use `rotari init` when you want **one experiment working directory paired
with its own job and log storage**. Run it once in that directory to record
the basedir and default project. Subsequent commands from the same directory
automatically load those defaults: `add` and `run` use that storage, and
`show` inspects the same experiment without repeating `--basedir` and
`--project-name`.

This is why `init` exists separately from `config`: `rotari config` generates
a template for general configuration, while `rotari init` fills in the two
location defaults that associate a working directory with its experiment
history. They use the same configuration mechanism, not separate kinds of
workspace.

Run `rotari init [BASEDIR [PROJECT]]` from the experiment directory:

```sh
mkdir experiment-a
cd experiment-a
rotari init .rotari-state sweep

rotari add python train.py --lr 0.1
rotari run
rotari show
# Limit the cross-workspace job listing to this experiment's storage.
rotari jobs --basedir .rotari-state
```

It creates `.rotari.toml` with `basedir` and `project-name` filled in; other
settings remain commented out, as in the template from `rotari config`:

```toml
basedir = ".rotari-state"
project-name = "sweep"
```

Both arguments are optional: `BASEDIR` defaults to `.rotari-state` and
`PROJECT` to `default`. The basedir must be a relative path; the project must
be a single safe name (not `.` or `..`, and containing no `/` or `\\`).

With the example above, job definitions, run history, and captured stdout and
stderr logs live under `experiment-a/.rotari-state`. Initializing another
experiment directory the same way gives it separate storage, even if it uses
the same project name.

`init` writes only the configuration file; state is created when needed by
commands such as `add`. It refuses to overwrite an existing `.rotari.toml`;
edit that file to change the defaults. The association applies only when
invoking rotari from that directory, not its subdirectories. CLI options and
environment variables can override it. It does not change the working
directory in which jobs execute; see
[Workflow and execution environment](CONCEPTS.md#workflow-and-execution-environment).

### Project defaults and listings

The location defaults created by `rotari init` select the preferred storage
and project for single-project commands such as `add` and `run`. Read-only
commands such as `show` try that location first, then search other known
basedirs if the requested target is not found. Multiple matches in the
fallback search are reported as ambiguous; use `--basedir` to limit the
search. The defaults do not narrow the cross-workspace listings `projects`,
`runs`, and `jobs`: those commands still scan all known basedirs unless you
explicitly select one.
Use `--basedir .rotari-state` to inspect only the storage below the current
experiment directory, and a command's project filter to narrow it further.
For command examples, see
[Inspecting runs and jobs](INSPECT.md).

### Inspecting and recording configuration

`rotari show` lists the contributing source paths and scopes. The current Web
`View config` lists actual source files by scope/path; select one if several
exist, or open the single file directly. It never presents a merged current
viewer/editor. Saves affect only the selected, validated source. Workspace
context is the Web server's startup cwd, not a viewed job's working directory.
Live and static views use the same source-selection UI; static writes remain
disabled.

On a run page, **Settings** still edits the current settings of its project,
just as on the project page, including configuration generation and
notification settings. These changes apply to future runs, not the saved run.
Use **View config snapshot** in the run action toolbar, next to **Cancel run**
(while running) and **Delete run**, to inspect the configuration saved with
that run. It shows both ordinary and notification config as read-only text,
and is disabled when no snapshot was recorded.

Every run saves one canonical merged **file configuration** as
`configs/config.toml`, from values captured by the client. It excludes CLI
options, environment overrides, and built-in defaults, and does not reread
sources when the supervisor starts. Source paths/scopes are recorded separately
in `context.json`, independently of the snapshot file list. **View config
snapshot** reads the saved copy, including older YAML/JSON snapshots.
A retry is a new invocation using its current configuration, not the old run's.

## Environment variables

The same environment can be used to configure the CLI and to inspect the
currently running job. Variables with a matching CLI option are read as that
option's default; an explicit command-line option always takes precedence. Job
variables are injected into command processes and can also be passed
explicitly to another rotari command.

Each command's `--help` output identifies an option's matching environment
variable, when one is available. For the per-executor options, such as
`--slurm-concurrency`, it writes the variable once as
`ROTARI_RUN_<EXECUTOR>_CONCURRENCY`, where `<EXECUTOR>` is the executor in
upper case, such as `SLURM`.

To mark the runs a code agent starts, set `ROTARI_ACTOR` to a label in the
agent's own environment, for example `"env": {"ROTARI_ACTOR": "claude-code"}`
in Claude Code's settings, or in the shell profile it starts with. `run` and
`retry` record it as the run's actor (`--actor LABEL` overrides it for one
run), and `rotari mcp` records it for the runs it starts. `show`, `lineage`,
`runs --json`, the MCP run summary, the Web run page, and the Activity page
show it; the Activity page can narrow to one actor. rotari never guesses an
actor: a run without one is unlabelled.

See the [environment variable reference](ENVIRONMENT_VARIABLES.md) for the
complete definitions and meanings. Use `rotari env` to print the same list with
values from the current process.

## Notifications

Webhook and browser notifications are configured in `notifications.toml`, which
is separate from the command defaults above. Rotari uses the first one it finds
in the project, the basedir, then the global config directory, without merging
scopes. Unlike ordinary command configuration, it has **no workspace scope**
and does **not inherit** lower-file settings. A project notification file
replaces the global file entirely: an omitted webhook URL does not inherit a
global destination; omitted fields use notification built-in defaults.
Notification snapshots remain separate files and are included in the run's
read-only **View config snapshot** view.
Generate one with:

```sh
rotari config --notifications
```

`ROTARI_WEBHOOK_URL` overrides `webhook.url` so the endpoint can stay out of the
file. For the event settings, fields, and Slack/Teams/Discord setup, see
[Notifications](NOTIFICATIONS.md).

## Launcher commands

`rotari web --allow-exec` can run the built-in services (TensorBoard, Jupyter
Lab, MLflow, Aim, and Streamlit; see [Built-in services](#built-in-services))
and the commands `launchers.toml` registers, against a job's artifacts from
the Web UI (see
[Launching commands on artifacts](INSPECT.md#launching-commands-on-artifacts)).
An entry in the file with a built-in's name replaces it, for example to run
TensorBoard from a particular environment.
Rotari reads it **only from the global config directory**
(`$XDG_CONFIG_HOME/rotari/launchers.toml`, by default
`~/.config/rotari/launchers.toml`), never from a workspace, basedir, or project,
and only when `rotari web --allow-exec` starts; edits take effect after a
restart. Generate a commented template with:

```sh
rotari config --launchers
```

The Web UI's Settings menu does the same with **Generate launcher config**.
**Launcher config** shows the file, why `rotari web --allow-exec` would refuse
it, and the commands a restart would load, including whether each one's
program is installed. With `--allow-control`, the same view can edit and save
an existing trusted file; the Web UI validates the TOML and asks you to restart
`rotari web --allow-exec` before changes take effect. Saving is disabled on a
read-only server. A run's launcher config is still this server-wide file, not
part of the run's config snapshot. The generated file has mode `0600` in a
`0700` directory. Rotari refuses the file, and `rotari web --allow-exec` does
not start, unless no other user can change it: the file and every directory
above it, following symlinks, must be owned by you or root and not writable by
group or others (a sticky directory such as `/tmp` is allowed). The error names
the offending path and the `chmod` that fixes it. A missing file only means
that only the built-in services are available; any unknown key, duplicate
name, or setting that does not apply to an entry's mode is an error.
`rotari config --list` shows the file when it exists.

A command whose first program cannot be found on the Web server's `PATH` (or
whose absolute path is not executable) is shown disabled in the Web UI with
that reason, and does not start; this is checked whenever the list is shown,
so installing the program needs no restart. Only the first program is
checked: for `conda run -n ENV tensorboard ...` that is `conda`, not the
environment's TensorBoard. A first word that is a shell builtin, an
expansion, or a relative or `~` path is not checked.

Each `[[launcher]]` entry registers one command:

```toml
[[launcher]]
name = "Line count"                     # shown in Launch with; unique
command = 'wc -l "$ROTARI_LAUNCHER_ARTIFACT"'
targets = ["file"]                      # "file", "directory", or both
mode = "command"                        # "command" runs and exits; "service" serves HTTP
cwd = "target"                          # "target" (default), "parent", or an absolute directory
stop_grace = "5s"                       # SIGTERM to SIGKILL on Stop (default 5s)
```

`command` is shell source run with `/bin/sh -c` (not Bash and not a login
shell) with empty standard input, in the Web server's environment, as the user
running `rotari web`. Rotari never inserts paths into the command text; it sets
these variables, which the command should quote:

| Variable | Value |
| --- | --- |
| `ROTARI_LAUNCHER_ARTIFACT` | absolute path of the selected file or directory |
| `ROTARI_LAUNCHER_CWD` | the working directory chosen by `cwd` |
| `ROTARI_LAUNCHER_PORT` | service mode: the port to listen on |
| `ROTARI_LAUNCHER_BASE_PATH` | service mode: the URL path prefix to serve under |
| `ROTARI_LAUNCHER_TOKEN` | service mode: a secret made for this launch (see [Tokens for services](#tokens-for-services)) |

`cwd = "target"` is the selected directory, or a selected file's directory.
Launched commands also get `127.0.0.1,localhost,::1` added to `NO_PROXY` and
`no_proxy`, so loopback traffic never goes through an HTTP proxy, and do not
receive `ROTARI_WEB_AUTH_TOKEN`. To use a tool from a particular environment,
say so in the command, for example
`conda run --no-capture-output -n ENV tensorboard ...`.

### Services

`mode = "service"` registers a command that serves HTTP, such as the
built-in TensorBoard command below. The Web UI opens it through the `rotari web`
port itself, under `/_launch/ID/`, so no further SSH port forward is needed:

```toml
[[launcher]]
name = "TensorBoard"
command = '''tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"'''
targets = ["directory"]
mode = "service"
ready_path = "/"        # under the base path; answers 2xx or 3xx when ready (default "/")
ready_timeout = "60s"   # default 60s
```

The service must listen on exactly `127.0.0.1:$ROTARI_LAUNCHER_PORT`, not
`0.0.0.0`, and serve everything under `$ROTARI_LAUNCHER_BASE_PATH`, which is
how requests arrive; rotari does not rewrite paths or pages, so a program that
cannot serve under a path prefix does not work. Rotari picks the port. It
counts the service as ready only when the process listening on the port
belongs to the launch and `ready_path` answers, so another program that took
the port first is never shown; that, a bind to another address, or no answer
within `ready_timeout` fails the launch and stops it, with the reason and the
service's output. A service that exits without ever listening, as when another
process took its port, is restarted once on a new port.

#### Built-in services

These are registered without an entry in `launchers.toml`; each is shown
disabled while its program is not on the Web server's `PATH`, and an entry with
the same name replaces it:

| Name | Offered for | Notes |
| --- | --- | --- |
| TensorBoard | directories | `tensorboard --logdir` on the directory |
| Jupyter Lab | directories | rooted at the directory; requires the launch token (below) |
| MLflow | directories named `mlruns` | file-store tracking data (`MLFLOW_ALLOW_FILE_STORE=true` is set for it) |
| MLflow (SQLite) | files named `mlflow.db` | `sqlite:///` tracking database |
| Aim | directories | an Aim repository (`aim init`); Aim refuses other directories |
| Streamlit | `*.py` files | runs the app |

MLflow is limited to those names because it creates its default experiment, or
its tables, in whatever it opens. Each was checked through the proxy, including
Jupyter's kernel and Streamlit's WebSocket connections: TensorBoard 2.21,
JupyterLab 4, MLflow 3.17, Aim 3, and Streamlit 1.65.

#### Tokens for services

A service listens on a loopback port that other users of the same host can
connect to directly, without rotari's token. Each service launch therefore
gets a secret, `ROTARI_LAUNCHER_TOKEN`, and `proxy_headers` makes rotari's
proxy present it on every request, so a service that requires it serves only
through the proxy. The built-in Jupyter Lab, which runs code, uses it:

```toml
[[launcher]]
name = "Jupyter Lab"
command = '''jupyter lab --no-browser --ip=127.0.0.1 --port="$ROTARI_LAUNCHER_PORT" --ServerApp.base_url="$ROTARI_LAUNCHER_BASE_PATH" --IdentityProvider.token="$ROTARI_LAUNCHER_TOKEN" --ServerApp.root_dir="$ROTARI_LAUNCHER_ARTIFACT" --ServerApp.allow_remote_access=True --ServerApp.allow_origin='*''''
targets = ["directory"]
mode = "service"
ready_path = "/api"
proxy_headers = { Authorization = "token $ROTARI_LAUNCHER_TOKEN" }
```

The browser never sees the token, and the proxy drops the browser's own
`Authorization` header. TensorBoard, MLflow, Aim, and Streamlit cannot require
a token, so another user of the host who finds their port can read what they
show; launch them only where that is acceptable.

`patterns = ["*.py"]` limits any command to targets whose name matches one of
the patterns.

## Shell completion

Completion scripts are available for Bash, Zsh, and Fish:

```sh
# Install for the default shell reported by $SHELL
rotari completion install

# Select the shell explicitly when running a nested shell
rotari completion install bash
rotari completion install zsh
rotari completion install fish
```

`completion install` adds a marked block to the shell configuration only when
it is absent, so running it again does not duplicate it. Start a new shell, or
source the shell configuration, to apply it to the current shell.

Completion covers subcommands, command options, executor values, run selection
values, and the `server` subcommands. Options with a fixed set of values, such
as `--executor/-e`, also reject other values during parsing. Dynamic candidates
include project names, saved run IDs, and job IDs; job ID candidates come from
the current queue and saved runs, or only from the selected run when
`--run-id/-r` is present. For manual setup, `rotari completion bash`,
`rotari completion zsh`, and `rotari completion fish` print the raw completion
scripts. Run `rotari completion --help` to list the subcommands.
