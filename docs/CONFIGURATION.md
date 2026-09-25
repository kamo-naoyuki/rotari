# Configuration

Config files, environment variables, run completion webhooks, and shell completion.

## Configuration files

Run the following command to choose a config file location interactively and
generate a template. The available options and their descriptions are shown
there.

```sh
rotari config
```

Use `rotari config --list` to list every existing config file found in the
global and basedir locations plus every project below the basedir. It groups
common files under `Common:` and project-specific files under `Projects:`, with
each project name followed by indented paths. This is an inventory, not the
single config selected by priority. Supplying `--project-name` limits the
project-specific entries to that project.

```sh
rotari config --list --basedir DIR
```

The resolution order is:

```text
CLI option (e.g., --retry)
environment variable (e.g., ROTARI_RUN_RETRY)
project config path (e.g., <basedir>/projects/demo/config.yaml)
basedir config path (e.g., <basedir>/config.yaml)
global config path (e.g., ~/.config/rotari/config.yaml)
built-in default
```

`rotari show` includes the highest-priority config path in its header when
config files are present. The web UI uses the same project, basedir, then
global priority and displays only that one path.

Only the first existing config in that priority order is loaded; lower-priority
config files are ignored. When a run starts, rotari copies that selected config
into its run directory. The run page's `View config` displays the copy, so
later edits do not change historical run details. Its `Config:` location lists
the run-local copy path.

## Environment variables

The same environment can be used to configure the CLI and to inspect the
currently running job. Variables with a matching CLI option are read as that
option's default; an explicit command-line option always takes precedence. Job
variables are injected into command processes and can also be passed
explicitly to another rotari command.

Each command's `--help` output identifies an option's matching environment
variable, when one is available.

Use `rotari env` to print the same list with values from the current process.

## Run completion webhook

Set `webhook.url` or `ROTARI_WEBHOOK_URL` to receive a JSON `POST` when a run
finishes. For options, the payload, and Slack/Teams/Discord setup, see
[Webhook integrations](WEBHOOK_NOTIFICATIONS.md).

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

Completion covers subcommands, command options, executor values, run selection
values, and the `server` subcommands. Dynamic candidates include project names,
saved run IDs, and job IDs. For manual setup, `rotari completion bash`,
`rotari completion zsh`, and `rotari completion fish` print the raw completion
scripts.
