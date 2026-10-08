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

`rotari init` is a shortcut for setting just the workspace's location
defaults. For a full configuration template, use `rotari config` instead.
Run `rotari init [BASEDIR [PROJECT]]` from the workspace directory:

```sh
rotari init .rotari-state sweep
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

### Project defaults and listings

The project default created by `rotari init` selects a project for
single-project commands; it does not hide other projects from listings such as
`projects`, `runs`, and `jobs`. Use `--basedir` or a command's project filter
when you want to narrow a listing. For command examples, see
[Inspecting runs and jobs](INSPECT.md).

### Inspecting and recording configuration

`rotari show` lists the contributing source paths and scopes. The current Web
`View config` lists actual source files by scope/path; select one if several
exist, or open the single file directly. It never presents a merged current
viewer/editor. Saves affect only the selected, validated source. Workspace
context is the Web server's startup cwd, not a viewed job's working directory.
Live and static views use the same source-selection UI; static writes remain
disabled.

Every run saves one canonical merged **file configuration** as
`configs/config.toml`, from values captured by the client. It excludes CLI
options, environment overrides, and built-in defaults, and does not reread
sources when the supervisor starts. Source paths/scopes are recorded separately
in `context.json`, independently of the snapshot file list. Run `View config`
remains read-only and reads the saved copy, including older YAML/JSON snapshots.
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
Notification snapshots and their read-only run viewer remain separate.
Generate one with:

```sh
rotari config --notifications
```

`ROTARI_WEBHOOK_URL` overrides `webhook.url` so the endpoint can stay out of the
file. For the event settings, fields, and Slack/Teams/Discord setup, see
[Notifications](NOTIFICATIONS.md).

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
