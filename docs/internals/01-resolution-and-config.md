# Resolution, configuration, and registry

Representative implementation and tests:

- [cmd/rotari/main.go](../../cmd/rotari/main.go) and
  [cmd/rotari/main_test.go](../../cmd/rotari/main_test.go) for location
  resolution and project paths.
- [cmd/rotari/config.go](../../cmd/rotari/config.go) and
  [cmd/rotari/config_test.go](../../cmd/rotari/config_test.go) for configuration
  precedence and formats.
- [cmd/rotari/run_registry.go](../../cmd/rotari/run_registry.go) and
  [cmd/rotari/run_registry_test.go](../../cmd/rotari/run_registry_test.go) for
  run-location indexing.
- [cmd/rotari/completion.go](../../cmd/rotari/completion.go) and
  [cmd/rotari/coverage_extra_test.go](../../cmd/rotari/coverage_extra_test.go)
  for shell completion.

## Resolution rules

Without a run-location lookup, base directories resolve in this order:

1. `--basedir`
2. `ROTARI_BASEDIR`
3. `./.rotari-state` when present
4. `$XDG_STATE_HOME/rotari`
5. `~/.local/state/rotari`

- Projects resolve from `--project-name`, then `ROTARI_PROJECT_NAME`, then the
  only project in the resolved base directory. With no projects the name is
  `default`; multiple projects require an explicit choice. The bare `show`
  command lists projects across known basedirs instead of resolving one.
- `check` and `reset` accept one optional positional project name as an
  alternative to `--project-name`; supplying both is a usage error.
- `jobs` also accepts one optional positional project name to filter the
  selected basedir's projects; it takes precedence over environment and config
  defaults, and cannot be combined with an explicit `--project-name`.
- `export` and `import` accept positional selectors that name a project or a
  saved run ID (`export [PROJECT|RUN_ID ...]`, `import FILE [PROJECT|RUN_ID]`).
  A selector with the generated run ID shape is a run ID; anything else is a
  project name, and at most one project may be named. A positional project
  cannot be combined with `--project-name`. `export` merges positional run IDs
  with repeated `--run-id`. `import` uses a run ID only to locate the
  destination basedir and project through the run registry, and rejects a run
  ID that does not exist. Classification lives in
  `splitProjectOrRunSelectors` in
  [cmd/rotari/run_registry.go](../../cmd/rotari/run_registry.go), covered by
  [cmd/rotari/export_test.go](../../cmd/rotari/export_test.go) and
  [cmd/rotari/import_test.go](../../cmd/rotari/import_test.go).
- `unlock` likewise accepts one optional positional project name. It derives
  the run ID from that project's `running.lock`, or from interrupted metadata
  when the lock is already absent; `--run-id` optionally verifies the result.
- `show --basedirs` lists state directories known to the run and live-server
  registries under the resolved master directory; this discovery is not
  exhaustive.
- Project names and job IDs are single path elements, never relative or
  absolute paths.
- Empty values, `.`, `..`, absolute paths, and values containing `/` or `\`
  are rejected before filesystem access. This applies to `resolvePaths` and
  `cancelJobs`, including requests from remote callers.
- Persisted timestamps use UTC RFC3339. Human-readable CLI and web views use the
  IANA timezone from `TZ` when valid, otherwise Go's local timezone.
- A supplied `--run-id` is exact, except that the reserved value `latest`
  selects the latest saved run using the normal metadata/newest-directory
  fallback. Existing-run commands use the master registry for its base directory
  and project.
- Explicit location options take priority, but conflicts with the registry fail.
  An unregistered run uses normal resolution for compatibility, while a missing
  explicit run is an error with no latest fallback.
- Without `--run-id`, history consumers use `meta.json` `last_run_id`, then the
  newest run directory where supported. `show` may prefer an active run,
  an interrupted run, or a non-empty idle queue before history.
- `wait` without a selector scans the resolved basedir's projects and waits
  when exactly one active `running.lock` exists; multiple active projects are
  listed for explicit selection, and no active project is an error. A positional
  selector is resolved in this order: project name, active run name, then run ID.
  An explicit `--run-id` bypasses this selector resolution.
- Run lookup applies to history commands (`show`, `wait`, `copy`, `change`,
  `remove`, `delete`, and rerun selection), not state-creating commands such as
  `add` or a plain new `run`.
- `cancel`, `suspend`, and `resume` merge positional selectors with repeated
  `--job-id/-j` (mutually exclusive with each other) and accept plain job IDs,
  `att_` attempt IDs, and a bare run ID in the same list. A bare run ID only
  locates the target run through the run registry; it is stripped before the
  remaining IDs are sent as job selectors, so passing only a run ID behaves
  like omitting `--job-id/-j` (all running jobs in that run). Mixing IDs that
  resolve to different runs is rejected.
- Multiple run IDs passed to `wait` are resolved independently, so one command
  may wait for runs from different projects or base directories.
- Shell completion follows the same location rules with narrower candidates:
  `project-name` lists project directories, `run-id` lists saved runs, and
  `job-id` lists queue and saved-run job IDs according to the selected run.
  Completion generation is implemented for Bash, Zsh, and Fish, and
  `rotari completion install` writes the appropriate shell-specific script for
  the detected or requested shell.
- Missing state directories produce no completion candidates instead of a shell
  error.

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

## Shell completion

- Completion is generated from the same CLI metadata as command help.
- `completion` and `server` dispatch on a subcommand instead of parsing a
  FlagSet, so they check `-h`/`--help` before dispatch and call
  `printSubcommandHelp` in [`cmd/rotari/cli_spec.go`](../../cmd/rotari/cli_spec.go).
  It prints the usage and subcommand descriptions from the same metadata to
  stderr with exit status 1, matching FlagSet help. Covered by
  `TestSubcommandCommandsPrintHelp` in
  [`cmd/rotari/coverage_extra_test.go`](../../cmd/rotari/coverage_extra_test.go).
- A string option whose CLI metadata declares `Values` uses those values for
  parse-time choice validation as well as completion and schema generation.
- CLI environment defaults are declared in one flag-to-variable mapping, used
  for both default values and command-help descriptions.
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
