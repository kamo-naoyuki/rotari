# Resolution, configuration, and registry

Representative implementation and tests:

- [internal/resolve/resolve.go](../internal/resolve/resolve.go) and
  [internal/resolve/resolve_test.go](../internal/resolve/resolve_test.go)
  for run, attempt, run-name, and job selector resolution shared by the
  commands, and [internal/state/config.go](../internal/state/config.go) and
  [internal/state/project.go](../internal/state/project.go) for base
  directory, master directory, and project resolution.
- [internal/config/config.go](../internal/config/config.go) and
  [internal/config/config_test.go](../internal/config/config_test.go) for
  config file locations, scope, and formats, and
  [cmd/rotari/config.go](../cmd/rotari/config.go) and
  [cmd/rotari/config_test.go](../cmd/rotari/config_test.go) for how config
  values become CLI option defaults and their precedence.
- [internal/runregistry/registry.go](../internal/runregistry/registry.go)
  and [internal/runregistry/registry_test.go](../internal/runregistry/registry_test.go)
  for run-location indexing and stale-entry garbage collection.
- [internal/basedirregistry/registry.go](../internal/basedirregistry/registry.go)
  and [internal/basedirregistry/registry_test.go](../internal/basedirregistry/registry_test.go)
  for basedir discovery indexing.
- [cmd/rotari/completion.go](../cmd/rotari/completion.go) and
  [cmd/rotari/coverage_extra_test.go](../cmd/rotari/coverage_extra_test.go)
  for shell completion; `TestShellCompletionCandidates` in
  [cmd/rotari/completion_shell_test.go](../cmd/rotari/completion_shell_test.go)
  drives the generated scripts in Bash, Zsh, and Fish for every command,
  subcommand, option, and option value in the CLI metadata.
- [cmd/rotari/guide.go](../cmd/rotari/guide.go) and
  [cmd/rotari/guide_test.go](../cmd/rotari/guide_test.go) for the agent
  guide.

## Resolution rules

The per-command view of these rules, with job selectors, is in
[06-selectors.md](06-selectors.md).

**RES-1** Without a run-location lookup, base directories resolve in this order:

1. `--basedir`
2. `ROTARI_BASEDIR`
3. Cwd workspace `basedir`, then global config `basedir`
4. `./.rotari-state` when present
5. `$XDG_STATE_HOME/rotari`
6. `~/.local/state/rotari`

- **RES-2** Projects resolve from `--project-name` (or the command's positional
  project), then `ROTARI_PROJECT_NAME`, then selected basedir, cwd workspace,
  and global file defaults, then the only project in the resolved base directory.
  With no projects the name is
  `default`; multiple projects require an explicit choice. The bare `show`
  command lists projects across known basedirs instead of resolving one; RES-26
  defines which target defaults aggregate views ignore.
- **RES-3** Commands that read or edit a missing project fail with
  `project "x" does not exist`, except `check` reports an empty, non-runnable queue without
  creating a project (exit status 1), and `unlock` without `--run-id` succeeds
  without creating a project. `add`, `import`, and `reset` create projects;
  `reset` on a missing project succeeds with an empty queue. `wait` selecting
  an uncreated project by name also succeeds
  without creating it. Explicit `--run-id` selection still requires its run.
- **RES-4** `check` and `reset` accept one optional positional project name as an
  alternative to `--project-name`; supplying both is a usage error.
- **RES-5** `jobs` also accepts one optional positional project name to filter the
  selected basedir's projects; it takes precedence over environment and config
  project defaults, and cannot be combined with an explicit `--project-name`.
  Its implicit basedir is the non-config cwd-local/XDG/home default; use CLI
  `--basedir` to choose a different basedir.
- **RES-6** `export` accepts one positional copy target and an optional output file
  (`export TARGET [FILE]`). The target names a project or saved run ID. When
  both a project and a run must be named explicitly, `--project-name` names
  the project and the positional target names the run. `--run-id` may still be
  repeated for merging saved runs.
- **RES-7** `unlock` likewise accepts one optional positional project name. It derives
  the run ID from that project's `running.lock`, or from interrupted metadata
  when the lock is already absent; `--run-id` optionally verifies the result.
- **RES-8** `show --basedirs` lists state directories known to the run and live-server
  registries under the resolved master directory; this discovery is not
  exhaustive.
**RES-26** Aggregate views do not use an implicit `project-name` from
`ROTARI_PROJECT_NAME` or any config scope to narrow results; only an explicit
CLI project selector does so. Basedir defaults are command-specific:

- Bare `show` ignores implicit project and basedir defaults, listing projects
  across known basedirs plus the cwd-local/XDG/home default state directory.
  CLI `--basedir` limits it to that basedir. Project/run/job selectors and
  project views retain normal location defaults.
- `jobs` ignores implicit project and basedir defaults, using the cwd-local,
  XDG, or home default basedir. CLI `--basedir` selects a different one;
  `--all-basedirs` retains its registry-wide scope. A positional project or
  CLI `--project-name` explicitly filters that scope.
- Argumentless `lineage` ignores implicit project defaults but retains normal
  basedir precedence: CLI, environment, workspace/basedir/global file defaults,
  then local/XDG/home fallback. It selects the sole existing project, errors
  when none exist, and lists multiple candidates with guidance to pass
  `--project-name`. `lineage PROJECT` continues to mean run IDs, which resolve
  through the run registry.
- `config --list` ignores implicit project defaults but retains normal basedir
  precedence: CLI, environment, workspace/global file defaults, then
  local/XDG/home fallback. It inventories every project config in that basedir;
  CLI `--project-name` narrows the inventory.

`show --basedirs` and `jobs --all-basedirs` reject an explicit `--basedir`
rather than silently discarding the conflicting scope. Implementation:
[aggregate target handling](../cmd/rotari/show.go) and
[jobs target handling](../cmd/rotari/jobs.go). Tests:
[aggregate CLI regressions](../cmd/rotari/show_projects_test.go),
[jobs regressions](../cmd/rotari/jobs_test.go), and
[built-binary conformance](../conformance/01-resolution/aggregate_target_defaults_test.go).

Other non-location option defaults remain applicable to aggregate views.
- **RES-9** Project names and job IDs are single path elements, never relative or
  absolute paths.
- **RES-10** Empty values, `.`, `..`, absolute paths, and values containing `/` or `\`
  are rejected before filesystem access, by the CLI and by the Web API alike.
- **RES-11** Persisted timestamps use UTC RFC3339. Human-readable CLI and web views use the
  IANA timezone from `TZ` when valid, otherwise Go's local timezone.
- **RES-12** A supplied `--run-id` is exact, except that the reserved value `latest`
  selects the latest settled (completed, neither active nor interrupted) run
  using the normal metadata/newest-directory fallback. `latest` is accepted wherever a
  run is given, as `--run-id` or positionally; the exceptions are `unlock`,
  whose `--run-id` confirms the locked run, and `cancel`, `suspend`, and
  `resume`, which act on running jobs. It is reserved: `add`, `change`,
  `import`, and `run --run-name` reject a new project, run, job, stage, or
  matrix named `latest`; existing ones keep
  working through options. Existing-run commands use the master registry for
  its base directory and project.
- **RES-13** A run ID or attempt ID alone resolves the base directory, project, and run
  in every command that takes one; see "Complete IDs" in
  [06-selectors.md](06-selectors.md).
- **RES-14** Explicit location options take priority, but conflicts with the registry fail.
  File-derived location defaults are not explicit registry constraints; their
  provenance is retained through CLI parsing. An unregistered run uses normal resolution for compatibility, while a missing
  explicit run is an error with no latest fallback.
- **RES-15** Without `--run-id`, history consumers use `meta.json` `last_run_id`, then the
  newest run directory where supported. `show` may prefer an active run,
  an interrupted run, or a non-empty idle queue before history, and `export`
  picks a non-empty queue before the latest run; see
  [06-selectors.md](06-selectors.md) for the options that skip the queue.
- **RES-16** `wait` without a selector scans the resolved basedir's projects and waits
  when exactly one active `running.lock` exists; multiple active projects are
  listed for explicit selection, and no active project is an error. A positional
  selector is resolved in this order: `latest`, a project name, a run name,
  then a run ID. A project or run name waits for its active run, or else
  returns the result of the latest matching run at once, as a finished run ID
  does, so a run that ends before `wait` starts is not an error; a run name
  whose latest runs are in several projects is ambiguous. If a selected
  project does not yet exist, `wait` succeeds as a no-op; a name that matches
  neither a project nor a run is treated as an uncreated project unless it
  looks like a run ID. A project selected by `--project-name/-p` or
  `ROTARI_PROJECT_NAME` without a selector follows the same active-then-latest
  rule, but no selector and no project still reports no active runs.
  An explicit `--run-id` bypasses this selector resolution.
  Implementation: [project wait resolution](../cmd/rotari/wait.go).
  Tests: [option and environment resolution](../cmd/rotari/wait_project_test.go)
  and [selector conformance](../conformance/01-resolution/export_target_test.go).
- **RES-17** Run lookup applies to history commands (`show`, `wait`, `copy`, `change`,
  `remove`, `delete`, and rerun selection), not state-creating commands such as
  `add` or a plain new `run`.
- **RES-18** `cancel`, `suspend`, and `resume` merge positional selectors with repeated
  `--job-id/-j` (mutually exclusive with each other) and accept plain job IDs,
  `att_` attempt IDs, and a bare run ID in the same list. A bare run ID
  locates the target run through the run registry; it is stripped before the
  remaining IDs are sent as job selectors, so passing only a run ID behaves
  like omitting `--job-id/-j` (all running jobs in that run). The run a bare
  run ID or an attempt ID names must be the project's active run; another run
  is an error, never a request for the active one. Mixing IDs that resolve to
  different runs is rejected. See "Job control" in
  [06-selectors.md](06-selectors.md).
- **RES-19** Multiple run IDs passed to `wait` are resolved independently, so one command
  may wait for runs from different projects or base directories.
- **RES-20** Shell completion follows the same location rules with narrower candidates:
  `project-name` lists project directories, `run-id` lists saved runs, and
  `job-id` lists queue and saved-run job IDs according to the selected run.
  Completion generation is implemented for Bash, Zsh, and Fish, and
  `rotari completion install` writes the appropriate shell-specific script for
  the detected or requested shell.
- **RES-21** Missing state directories produce no completion candidates instead of a shell
  error.
- **RES-22** A state directory or master directory given relative to the
  working directory, by `--basedir`, `ROTARI_BASEDIR`, `--masterdir`, or
  `ROTARI_MASTERDIR`, including one that goes up with `..`, is resolved to
  its absolute path before use, so every command accepts it and the
  registries record the absolute path. Implemented once in
  `state.ResolveBaseDir` and `state.ResolveMasterDir` in
  [internal/state/config.go](../internal/state/config.go); checked by
  `TestRelativeStateDirectoriesResolveAgainstTheWorkingDirectory`.

Implementation and tests for these rules:

- Location, project, and run resolution, including `latest` and the
  missing-project error: `resolve.ExistingRun`, `resolve.ExistingRunID`,
  `resolve.RequireProject`, `resolve.ProjectNames`, and `resolve.IsRunID` in
  [internal/resolve/resolve.go](../internal/resolve/resolve.go), with
  [internal/resolve/resolve_test.go](../internal/resolve/resolve_test.go) and
  [cmd/rotari/export_test.go](../cmd/rotari/export_test.go). Reserved names:
  `model.ValidateReservedName`.
- Path elements (RES-9, RES-10): `state.IsValidPathElement` and
  `state.ResolveProjectPaths` in
  [internal/state/paths.go](../internal/state/paths.go), also applied by
  `jobcontrol.Controller.CancelJobs` and the Web handlers; checked end to end
  by [conformance/06-selectors/paths_test.go](../conformance/06-selectors/paths_test.go).
- Display times (RES-11): `model.FormatDisplayTimestamp` in
  [internal/model/time.go](../internal/model/time.go) and
  `joblist.FormatTimestamp` format in `time.Local`; checked end to end by
  `TestDisplayTimesFollowTZ` in
  [conformance/01-resolution/display_time_test.go](../conformance/01-resolution/display_time_test.go).

## Configuration files

**RES-23** Ordinary configuration loads and merges global → cwd workspace →
selected basedir → selected project. Workspace discovery reads only cwd
`.rotari.toml`, never ancestors. Other scopes accept one `config.yaml`,
`config.toml`, or `config.json`; duplicate formats and malformed/unreadable
discovered files fail with scope/path diagnostics. Maps recurse, arrays and
scalars replace, null is unspecified, and false/zero/empty strings are explicit.
After merging, command sections override root values, including cross-scope
conflicts. CLI/environment precedence remains CLI-3. `--config FILE` replaces
automatic discovery, not an additional layer.

Location loading is staged: global/workspace before basedir resolution,
basedir config before project resolution, then project config. A basedir
config forbids `basedir` but may set `project-name`; a project config forbids
both, including command sections, with errors rather than ignored values.
Ambiguous projects load only the known scopes and remain the command's decision.
Registered run/attempt locations supply config locations without making file
defaults explicit conflicting selectors.

**RES-24** `rotari init [BASEDIR [PROJECT]]` atomically creates cwd
`.rotari.toml` using the same workspace TOML template as `rotari config`, with
only `basedir` and `project-name` values filled in; other option assignments
remain commented out. Omitted arguments default to `.rotari-state` and
`default`, respectively. Basedir must be non-empty
and relative; project must be a safe, non-reserved path element. Init writes
only active location defaults, creates no basedir/project/queue/registry, and refuses
to replace any existing workspace file or symlink, preserving its settings.

**RES-25** Every new run saves canonical merged file values as
`configs/config.toml`, captured by the client and passed to the supervisor
without rereading ordinary source files. CLI/env overrides and built-in
defaults are excluded, including when no file config exists (empty snapshot).
Source scope/path metadata is independent of snapshot filename/path metadata.
Notification snapshots remain separate. Run views are read-only and use the
saved copy; older TOML/YAML/JSON snapshots and the legacy allow-listed fallback
remain readable. A retry uses the new invocation's configuration.

Implemented by [internal/config/layers.go](../internal/config/layers.go),
[cmd/rotari/config.go](../cmd/rotari/config.go),
[cmd/rotari/config_selectors.go](../cmd/rotari/config_selectors.go),
[cmd/rotari/init_workspace.go](../cmd/rotari/init_workspace.go), and
[internal/projectrun/context.go](../internal/projectrun/context.go). Tests:
[merge/scope tests](../internal/config/layers_test.go),
[CLI workspace tests](../cmd/rotari/workspace_config_test.go),
[immutable snapshot tests](../internal/projectrun/config_snapshot_test.go), and
[binary conformance](../conformance/01-resolution/workspace_config_test.go).

- `config` generates scope-appropriate templates from CLI metadata. YAML/JSON
  nulls are unspecified; TOML uses comments. Command-line-only options are
  excluded. `config --list` inventories global, cwd workspace, basedir, and
  project files, including separate notification files but no workspace
  notifications. Duplicate formats can still be inventoried. Interactive
  generation offers global, workspace TOML, basedir, project, stdout, and
  arbitrary paths. `show` lists contributing source paths/scopes.
- Current Web `View config` lists actual source scope/path pairs, opens a single
  file directly, and selects among multiple files; there is no merged current
  viewer/editor. Control-gated saves identify and validate the selected source
  against allowed targets, parse its format, apply location restrictions, and
  atomically write only that source. Workspace context is server startup cwd,
  not viewed-job cwd. Generation uses the same scope-aware CLI templates.
  Live and static exports retain the same source-selection UI, with static
  writes refused. Run pages retain separate read-only command and notification
  snapshot viewers.
  Implemented by `loadWebConfigFiles`, `saveSelectedConfig`, `loadRunConfigFiles`,
  and `generateWebConfig` in
  [internal/webui/webui.go](../internal/webui/webui.go), tested by
  [workspace source tests](../internal/webui/workspace_config_test.go) and
  [legacy/static config tests](../internal/webui/webui_test.go).

## Notification configuration

- Notification settings live in `notifications.toml`, which is separate from
  the command-default config above and is never merged into it. `rotari config
  --notifications` writes the template; the flag is command-line-only and the
  file is TOML regardless of `--format`.
- Lookup chooses the first `notifications.toml` found in
  `projects/<project>/`, then the resolved basedir, then
  `$XDG_CONFIG_HOME/rotari` (or `~/.config/rotari`). Lower-priority scopes are
  ignored rather than merged, with no workspace scope. Omitted keys use
  notification built-in defaults; a project file does not inherit a lower-file
  webhook URL. An unknown or duplicated key fails parsing
  instead of being ignored.
- `[webhook]` and `[browser]` accept the same settings but hold independent
  values: `job_failure`, `job_success`, `run_failure`, `run_success`, `fields`,
  and `max_jobs`, plus `url` and `format` for webhooks. `link` is valid only
  for `[browser]`. `ROTARI_WEBHOOK_URL` overrides `webhook.url`.
- A run resolves its webhook settings when it starts and snapshots the file
  with its other configs, so later edits do not change that run. `rotari web`
  reads browser settings for its own basedir and serves per-project settings
  from `/api/notification-settings`.
- The Web UI's `notification-config`, `generate-notification-config`, and
  `save-notification-config` endpoints are control-gated, accept a project name
  rather than a filesystem path, validate content before writing, and write
  atomically. Read responses mask the webhook URL and report only whether one
  is set; a save keeps the stored URL unless the request explicitly changes it.
  Implemented by [`internal/notification`](../internal/notification) and
  `loadWebNotificationConfig`, `generateWebNotificationConfig`, and
  `saveWebNotificationConfig` in
  [`internal/webui/webui.go`](../internal/webui/webui.go), with tests in
  [`internal/notification/config_test.go`](../internal/notification/config_test.go)
  and `TestWebNotificationConfig*` in
  [`internal/webui/webui_test.go`](../internal/webui/webui_test.go).

## Shell completion

- Completion is generated from the same CLI metadata as command help. Each
  shell offers a command's own options only, completes values for options
  with fixed `Values` and for `--project-name`, `--run-id`, and `--job-id`,
  and completes on the first TAB after Zsh autoloads the script from `fpath`.
  Job ID candidates skip a run's config snapshot directory. Covered by
  `TestShellCompletionCandidates` in
  [`cmd/rotari/completion_shell_test.go`](../cmd/rotari/completion_shell_test.go).
- `completion` and `server` dispatch on a subcommand instead of parsing a
  FlagSet, so they check `-h`/`--help` before dispatch and call
  `printSubcommandHelp` in [`cmd/rotari/cli_spec.go`](../cmd/rotari/cli_spec.go).
  It prints the usage and subcommand descriptions from the same metadata to
  stderr with exit status 1, matching FlagSet help. Covered by
  `TestSubcommandCommandsPrintHelp` in
  [`cmd/rotari/coverage_extra_test.go`](../cmd/rotari/coverage_extra_test.go).
- A string option whose CLI metadata declares `Values` uses those values for
  parse-time choice validation as well as completion and schema generation.
- CLI environment defaults are declared in one flag-to-variable mapping, used
  for both default values and command-help descriptions.
- Flag registration looks up metadata in the parsing command's spec
  (`cliCommandFlag`, keyed by the FlagSet name), so commands that share a flag
  name keep their own help text; internal commands without metadata fall back
  to the first definition. The config template uses the same per-command
  lookup. Covered by `TestFlagHelpUsesTheParsingCommandsDescription`.
- Per-executor run settings (`--<executor>-concurrency`, `-options`,
  `-submit-interval`, `-submit-retry-limit`) are read by the function that
  `cliExecutorRunSettings` returns, which callers invoke after `fs.Parse`.
  Covered by `TestExecutorRunSettingsIncludeCommandLineValues`.
- It covers subcommands, options, executor values, run-selection values, and
  `server` subcommands.
- Installation appends a marked rotari block only when it is absent, making
  repeated installs idempotent.
- Dynamic candidates follow the resolution rules above:
  - `project-name` lists project directories.
  - `run-id` lists saved runs.
  - `job-id` lists the current queue and saved runs, or only the selected run
    when `--run-id` is present. Run IDs found through the master registry are
    included in this selected-run lookup.

## Agent guide

- `rotari guide` prints the hand-written rules in
  [`cmd/rotari/assets/agent_guide.md`](../cmd/rotari/assets/agent_guide.md),
  embedded with Go `embed`, followed by the common options and a one-line
  index of the commands generated from `cliCommandSpecs`. It stays short
  because agents read it first, and points to `rotari COMMAND --help` for
  options. `TestAgentGuideIndexesEveryCommand` fails when a command is
  missing from the index. Keep the hand-written part free of flag lists and
  update its examples when the commands they use change.
- `rotari COMMAND --help` (or `-h`) prints, to stdout with exit 0, the
  command's description, usage, subcommands, and every option of its
  `cliCommandSpecs` entry, with `cliFlagDescription` and the option's
  effective default after config files and the environment; the `--filter-*`
  options come under their own heading. The per-executor options, such as
  `--slurm-concurrency`, come under an "Executor options" heading, one entry
  per kind that names every executor's option, with the environment variable
  written as `ROTARI_RUN_<EXECUTOR>_...`. A default or the choices are stated
  once, also where the description states the default. The help names the
  command the user typed, also where commands share a FlagSet, as `retry`
  shares `run`'s. Implemented once in `writeCommandHelp`; covered by
  `TestCommandHelpCoversEveryOptionAndExitsZero` and
  `TestCommandHelpStatesEachNoteOnce`. The generated CLI and Python API
  references list an option's choices from the schema.
- Like `schema`, `guide` skips config loading, so a broken config file does not
  hide it.
- The top-level usage (bare `rotari`, `rotari --help`, `-h`, or `help`) starts
  by pointing coding agents to `rotari guide`. The help forms exit 0; a bare
  `rotari` still exits 1. Covered by `TestTopLevelUsagePointsAgentsToGuide`.

## Run registry

Run IDs contain a UTC timestamp and random suffix. They are collision-resistant
but do not encode a location. The master directory stores one index record per
run:

```text
<masterdir>/runs/<run-id>.json -> { base_dir, project_name, run_id }
```

The master directory resolves, and is also used for server discovery, in this
order:

1. `ROTARI_MASTERDIR`
2. `$XDG_STATE_HOME/rotari/master`
3. `~/.local/state/rotari/master`

- Register a run before exposing location-independent commands.
- A run's directory and initial `context.json` are written before its registry
  entry. During normal execution, a missing `summary.json` may mean the run is
  still active, but a registry entry whose run directory is missing is stale.
  Existing-run commands must report that case and direct the user to `rotari gc`
  rather than falling back to another run.
- Re-registering the same mapping is idempotent; mapping an existing ID to a
  different location fails.
- The registry is only an index. Run files remain authoritative.
- Deleting a run through CLI or web history controls removes its registry entry
  after the run files and metadata are updated.
- Runs deleted outside rotari can leave orphaned registry entries.
  `rotari gc [MASTERDIR]` removes them, and basedir records whose basedir is
  gone; its optional positional master directory is an alternative to
  `--masterdir`. Each removal checks again that the entry is unchanged and its
  directory still absent. `rotari gc --dry-run` lists the candidates without
  removing anything.
- Automatic garbage collection is not performed. Malformed or invalid registry
  files are reported and left untouched for manual inspection.

### Implementation note: basedir discovery registry

This is an internal indexing decision, not a user-facing contract. Do not add
SQLite for registry metadata. Keep the filesystem state authoritative and
maintain a separate one-record-per-basedir index under the master directory:

```text
<masterdir>/basedirs/<hash>.json -> { base_dir }
```

Register a basedir idempotently when a command creates or adopts state there:
queue/project creation, run creation, import, copy, and server startup. Do
not register from read-only commands such as `show` or `jobs`, or from a
`--dry-run`, which writes nothing. `add` and `copy` register through
`queueops.Editor.RegisterBaseDir`, import through `workflowstate.Import`;
`TestCommandsThatCreateAProjectRegisterItsBasedir` checks the commands that
create a project. The basedir
registry is used for discovery by bare `show` and `show --basedirs`, so those
commands do not need to scan every historical run record. Existing
installations are backfilled from the run registry when the basedir index is
empty; a deliberate repair or fallback path must remain available for older
state.

The run registry remains the run-ID lookup index:

```text
<masterdir>/runs/<run-id>.json -> { base_dir, project_name, run_id }
```

Registering a run also registers its basedir. Deleting a run removes only the
run-registry entry; it does not remove the basedir entry because queues,
projects, or other runs may still use that basedir. The basedir registry is a
discovery index only, so a stale entry is harmless and is skipped when the
basedir cannot be read. The first implementation does not GC basedir entries
and never deletes state directories. Run-registry orphan GC remains separate
because stale run entries can interfere with run-ID resolution.

### Why the registry is run-scoped

The registry is intentionally indexed by `run-id`, not by job or attempt. A
run is the smallest unit needed to recover `base_dir` and `project_name`, while
all jobs and attempts are already owned by that run's directory. Registering
every job or every retry attempt would multiply the number of master-index
records and accelerate registry growth without adding location information.

An `attempt-id` must therefore not introduce a second registry. The current
format is the readable, path-safe value
`att_<run-id>-<job-id>-<attempt-number>`. Generated run IDs have a fixed
24-byte format, so decoding uses that fixed boundary and the final numeric
component; hyphens in an array task ID remain part of the job ID. The ID is
self-contained without copying `base_dir` or `project_name` into every attempt
ID, and the existing run registry resolves the basedir and project from its
decoded run ID. The attempt directory and its metadata remain authoritative.

This format deliberately depends on current ID invariants: run IDs are
generated by Rotari in the fixed timestamp/random format, and job IDs are
generated path-safe values (array task suffixes are also supported). If run or
job IDs become freely user-defined, the parser must gain escaping, length
prefixes, or a versioned format. The format is an identifier, not encryption;
it exposes run and job IDs. IDs written in the former JSON/Base64 format are
not decoded by the new parser and require migration or an explicit legacy
reader if old state must remain addressable.
