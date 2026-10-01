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
`notifications.toml`—found in the global and basedir locations plus every
project below the basedir. It groups common files under `Common:` and
project-specific files under `Projects:`, with each project name followed by
indented paths. This is an inventory, not the single config selected by
priority. Supplying `--project-name` limits the project-specific entries to
that project.

```sh
rotari config --list --basedir DIR
```

The resolution order is:

```text
CLI option (e.g., --retry)
environment variable (e.g., ROTARI_RUN_RETRY)
configuration value (from the selected file)
built-in default
```

Commands that load configuration accept `--config FILE` to use a specific
YAML, TOML, or JSON file instead of looking in the project, basedir, and global
locations. Both `--config FILE` and `--config=FILE` are supported. An explicitly
selected file must exist and parse successfully; `--config` itself cannot be
set by an environment variable or another config file.
This value precedence applies across commands; see the
[CLI contract](../contracts/03-server-and-command-interfaces.md#cli-presentation).

Without `--config`, the selected configuration file is the first one found in
the project, basedir, then global locations. Lower-priority files are not
merged. Output paths such as `export --output` are read only from the command
line; config files and environment variables do not set them.

`rotari show` includes the highest-priority config path in its header when
config files are present. The web UI uses the same project, basedir, then
global priority and displays only that one path.

Only the first existing config in that priority order is loaded; lower-priority
config files are ignored. An explicit `--config FILE` selects that file instead.
When a run starts, rotari copies only the config actually loaded into its run
directory as `configs/config.<ext>` (for example, `configs/config.toml`). The
run page's `View config` displays the copy, so later edits do not change
historical run details. Its `Config:` location lists the run-local copy path.

## Environment variables

The same environment can be used to configure the CLI and to inspect the
currently running job. Variables with a matching CLI option are read as that
option's default; an explicit command-line option always takes precedence. Job
variables are injected into command processes and can also be passed
explicitly to another rotari command.

Each command's `--help` output identifies an option's matching environment
variable, when one is available.

See the [environment variable reference](ENVIRONMENT_VARIABLES.md) for the
complete definitions and meanings. Use `rotari env` to print the same list with
values from the current process.

## Notifications

Webhook and browser notifications are configured in `notifications.toml`, which
is separate from the command defaults above. Rotari uses the first one it finds
in the project, the basedir, then the global config directory, without merging
scopes. Generate one with:

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
