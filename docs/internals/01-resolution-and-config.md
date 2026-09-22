# Resolution, configuration, and registry

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
  command is an exception: it warns and falls back to listing all projects.
- `show --projects` lists all projects in the resolved base directory and does
  not resolve one project name.
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
- CLI option defaults are loaded from one of `config.yaml`, `config.toml`, or
  `config.json` in `$XDG_CONFIG_HOME/rotari` (or `~/.config/rotari`), then the
  resolved base directory.
- After resolving the project name, the same lookup is performed in
  `projects/<project>/`. Project values override base-directory values.
- Later scopes override earlier scopes: home, basedir, then project.
- `rotari show` and the web UI display only the highest-priority existing
  config path: project, then basedir, then home. Config loading still merges
  all three scopes.
- Multiple supported config files in the same directory are an error; file
  formats have no implicit priority.
- Common configuration keys (`basedir` and `project-name`) are at the root;
  command-specific keys are nested under their command name. Explicit CLI values
  take priority over environment defaults, which take priority over command
  sections and root config values.
- `rotari config` generates a template from the union of all CLI metadata
  options. YAML and JSON use `null` for unset values; TOML uses comments because
  it has no null value. Null values are ignored during resolution.
- Without `--output`, `rotari config` offers home, basedir, existing project
  config paths, stdout, and an arbitrary path interactively; an explicit
  `--output` is non-interactive.
- `rotari show` prints the highest-priority resolved config path in its header so the
  active home, basedir, and project config files are visible in CLI output as
  well as in the web UI.
- The Web UI exposes config paths in its state and serves raw contents only for
  the resolved global/project files or config paths recorded in a run's
  `context.json`; it does not accept arbitrary filesystem paths.

## Shell completion

- Completion is generated from the same CLI metadata as command help.
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
  caches their plan for ten minutes; `rotari gc --apply` removes only unchanged
  entries whose run directories are still absent.
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
