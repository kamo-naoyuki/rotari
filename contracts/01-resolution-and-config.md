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
3. `./.rotari-state` when present
4. `$XDG_STATE_HOME/rotari`
5. `~/.local/state/rotari`

- **RES-2** Projects resolve from `--project-name`, then `ROTARI_PROJECT_NAME`, then the
  only project in the resolved base directory. With no projects the name is
  `default`; multiple projects require an explicit choice. The bare `show`
  command lists projects across known basedirs instead of resolving one.
- **RES-3** Commands that read or edit a missing project fail with
  `project "x" does not exist`, except `check` reports an empty, non-runnable queue without
  creating a project (exit status 1), and `unlock` without `--run-id` succeeds
  without creating a project. `add`, `import`, and `reset` create projects;
  `reset` on a missing project succeeds with an empty queue, with or without
  `--recover`. `wait` selecting an uncreated project by name also succeeds
  without creating it. Explicit `--run-id` selection still requires its run.
- **RES-4** `check` and `reset` accept one optional positional project name as an
  alternative to `--project-name`; supplying both is a usage error.
- **RES-5** `jobs` also accepts one optional positional project name to filter the
  selected basedir's projects; it takes precedence over environment and config
  defaults, and cannot be combined with an explicit `--project-name`.
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
  An unregistered run uses normal resolution for compatibility, while a missing
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
  looks like a run ID. An explicit project without a selector follows the
  same rule, but no selector and no project still reports no active runs.
  An explicit `--run-id` bypasses this selector resolution.
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

- Configuration loading does not independently resolve a project or replace
  command resolution. The command owns normal `basedir` and project resolution;
  configuration only consumes the locations that are already explicit or known.
- When `--run-id` identifies a registered run, its registry entry supplies
  `base_dir` and `project_name` for configuration loading as well as for the
  command. When the project is still ambiguous, only global and basedir config
  are loaded; project selection remains the command's responsibility.
- After resolving the project name, config lookup chooses the first directory
  containing one supported file: `projects/<project>/`, then the resolved
  basedir, then `$XDG_CONFIG_HOME/rotari` (or `~/.config/rotari`). It loads only
  that config; lower-priority scopes are ignored rather than merged.
- `rotari show` and the web UI display that selected config path.
- Multiple supported config files in the same directory are an error; file
  formats have no implicit priority.
- Common configuration keys (`basedir` and `project-name`) are at the root;
  command-specific keys are nested under their command name. Explicit CLI values
  take priority over environment defaults, which take priority over command
  sections and root config values.
- `rotari config` generates a template from the union of all CLI metadata
  options. YAML and JSON use `null` for unset values; TOML uses comments because
  it has no null value. Null values are ignored during resolution.
- A flag whose metadata sets `CommandLineOnly` (for example, `export --output`)
  ignores config and environment defaults and is left out of the template and
  `configOptionNames`. Covered by
  `TestCommandLineOnlyFlagIgnoresConfigAndStaysOutOfTemplate` in
  [`cmd/rotari/config_test.go`](../cmd/rotari/config_test.go).
- `rotari config --list` is an inventory rather than a resolution operation. It
  lists every supported config found in the global and basedir scopes under
  `Common:`, then scans every `projects/<project>/` directory and lists paths
  beneath each project name under `Projects:`. An explicit `--project-name`
  limits only project-specific entries to that project.
- Without `--output`, `rotari config` offers home, basedir, existing project
  config paths, stdout, and an arbitrary path interactively; an explicit
  `--output` is non-interactive.
- `rotari show` prints the highest-priority resolved config path in its header so the
  active home, basedir, and project config files are visible in CLI output as
  well as in the web UI.
- The Web UI exposes config paths in its state and serves raw contents only for
  the selected config. At run creation, that config is copied below the run
  directory and listed in `context.json`; a run page serves the immutable copy
  rather than rereading the source path. It does not accept arbitrary filesystem
  paths. Older runs without snapshots retain the legacy resolved-path fallback.
  The projected run context separately records the snapshot path for the Web
  UI's `Config:` location display.
- On all-projects and project pages, the Web UI also offers a control-gated
  `config-targets`/`generate-config` pair. It uses the same `configTemplate`
  generator as the CLI to write TOML at a selected global, basedir, or project
  path and may replace an existing TOML config. It refuses to add a second
  supported config format in the same directory; run pages stay view-only
  because their paths describe historical execution context.
- The control-gated `save-config` endpoint writes only the currently resolved
  config for an all-projects or project page. It accepts a project name and
  content, never a filesystem path or run ID. Run pages expose only the copied
  configuration snapshots recorded at run creation. Before writing, the
  endpoint parses JSON, TOML, or YAML according to the existing file extension,
  so an invalid edit cannot replace the valid config.
- The Web side is implemented by `loadWebConfigFiles`, `loadRunConfigFiles`,
  `saveWebConfig`, `webConfigTargets`, and `generateWebConfig` in
  [`internal/webui/webui.go`](../internal/webui/webui.go). Representative tests are
  `TestWebConfigAPIReadsResolvedFiles`, `TestWebConfigAPIReadsRunConfigSnapshots`,
  `TestWebSaveConfigWritesOnlyTheResolvedCurrentConfig`,
  `TestWebSaveConfigRejectsReadOnlyMode`, and
  `TestStaticWebUsesGenerateConfigReadOnlyFlow` in
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
  embedded with Go `embed`, followed by a command reference generated from
  `cliCommandSpecs`. Flag lines use `cliFlagDescription`, so choices and
  environment variables match `--help`.
- `TestAgentGuideCoversEveryCommandAndFlag` fails when a command or flag is
  missing from the reference. Keep the hand-written part free of flag lists
  and update its examples when the commands they use change.
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
- Runs deleted outside rotari can leave orphaned registry entries. `rotari gc`
  caches their plan for ten minutes; its optional positional master directory
  is an alternative to `--masterdir`; `rotari gc --apply [MASTERDIR]` removes
  only unchanged entries whose run directories are still absent.
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
not register from read-only commands such as `show` or `jobs`. The basedir
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
