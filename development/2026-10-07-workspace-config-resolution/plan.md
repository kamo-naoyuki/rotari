# Plan: Workspace Configuration and Merged Run Config

Created: 2026-10-07
Status: Planned

## Purpose

Make Rotari's target selection and configuration predictable per workspace. A workspace can choose its default basedir and project without creating a project during initialization, while normal configuration values inherit across global, workspace, basedir, and project scopes. Each run records the merged file configuration that was available when it started.

## Goals

- Discover a workspace file named `.rotari.toml` from the current working directory toward its ancestors, using the nearest one.
- Add workspace configuration between global and basedir/project configuration for normal config values.
- Merge all discovered config scopes, with closer/more specific scopes overriding lower-priority scopes.
- Support workspace defaults for basedir and project name without making location selection depend on files that can only be found after that location is selected.
- Add `rotari init [<basedir> [<project-name>]]` to write workspace location defaults only.
- Save one canonical merged **file-config** snapshot as `config.toml` for every run. CLI flags and environment overrides remain separate and are not included in this snapshot.
- Preserve existing state layout and support older runs that contain only an original config-file copy.

## Non-goals

- Do not create a project, queue, basedir directory, or registry entry as a side effect of `rotari init`.
- Do not change the precedence of CLI options and environment variables over configuration values.
- Do not include CLI/environment-resolved values or built-in defaults in the merged file-config snapshot.
- Do not merge notification configuration into the normal command-config chain; notifications remain a separate configuration system.
- Do not change retry semantics to reuse an old run's configuration. A retry is a new invocation using the configuration selected for that invocation.
- Do not infer a basedir from registry contents.

## Proposed configuration model

### Scope order

For ordinary configuration keys, load and merge in this order, lowest to highest priority:

1. Global config.
2. Workspace `.rotari.toml`.
3. Selected basedir config.
4. Selected project config.

Then preserve the existing runtime precedence: explicit CLI option, environment variable, merged file value, built-in default. Within file configuration, command-specific sections retain their existing precedence over same-file root keys unless implementation work establishes and tests a more coherent cross-scope rule.

Map values merge recursively. A higher-scope scalar replaces a lower-scope scalar; arrays replace rather than append; `false`, `0`, and empty strings remain explicit values. Define and test `null` behavior before implementation; recommended behavior is to retain the lower-scope value (treat null as unspecified), matching current `configValue` behavior.

Keep same-directory multiple-format detection. Two supported config files in one scope remain an error rather than having arbitrary format precedence.

### Location-selection phases and cycle prevention

Location selection must be staged; do not load a config from a location before that location can be determined.

1. Resolve workspace file from cwd/ancestors. Load global and workspace config.
2. Choose basedir using explicit CLI, environment, workspace `basedir`, global `basedir`, then existing built-in/local-state resolution behavior as specified by the resolution contract. A workspace-relative basedir is resolved against the directory containing `.rotari.toml`.
3. Load the selected basedir config. It may set `project-name`, but setting `basedir` is invalid and produces an error identifying the file and key.
4. Choose project using explicit CLI, environment, basedir `project-name`, workspace `project-name`, global `project-name`, then existing project discovery/default behavior. Confirm exact ordering against existing semantics and encode it in one shared resolver.
5. Load the selected project config. It may not set `basedir` or `project-name`; either key is an error, including when nested in command-specific sections.
6. Merge ordinary config values in global → workspace → basedir → project order, then apply CLI and environment precedence through the existing option layer.

`rotari init` writes a workspace file at cwd; normal commands search upward so invocation from a workspace subdirectory finds the same defaults. It never changes the caller's current directory.

### `rotari init`

Supported forms:

- `rotari init` writes default basedir `.rotari-state` and leaves project-name unset.
- `rotari init <basedir>` writes the relative basedir only.
- `rotari init <basedir> <project-name>` writes both defaults.

The basedir argument must be relative. Reject absolute paths and unsafe/invalid project path elements. Store the path as supplied after validation and interpret it relative to the workspace file's directory. Refuse to overwrite an existing `.rotari.toml` unless an explicit overwrite/update behavior is separately specified. Initialization writes only the file atomically; it does not create state directories or projects. Treat this as a special config/setup command in CLI metadata, help, schema, and shell completion.

### Run configuration snapshot

The CLI resolves and merges file configuration once. Pass the immutable merged file-config bytes/representation, not source paths to be reread, through the run request to the supervisor. At run begin, write canonical TOML to the run's existing config snapshot location as `config.toml` (confirm exact current path and keep it stable if possible). This snapshot excludes CLI overrides, environment overrides, and built-in defaults.

Record source paths and scope order separately from the single snapshot so the run view can explain which files contributed. Do not pair source-path metadata one-to-one with snapshot-file metadata. Keep notification snapshots independent. Preserve reading/display of legacy YAML/JSON/source-file snapshots for older runs. Ensure a source edited or removed after config loading cannot alter or prevent the run snapshot.

## Implementation phases

### Phase 1 — Configuration primitives and merge contract

- Extend `internal/config` with workspace discovery, scope-aware loading, validation, deep merge, and canonical TOML serialization.
- Keep configuration independent of CLI and do not introduce a dependency cycle into `internal/state`.
- Add tests for nearest ancestor discovery, missing workspace config, relative workspace basedir, scope override behavior, nested maps, arrays, null/false/zero/empty-string, malformed files, duplicate formats, and forbidden location keys at root and command sections.
- Define behavior for malformed automatically discovered scopes. Prefer failing with a path/scope diagnostic rather than silently dropping a layer, unless compatibility analysis requires a transition.

### Phase 2 — Staged target resolution and `init`

- Refactor CLI config loading to load workspace/global location defaults before basedir resolution, then basedir config before project resolution, and project config only after both are known.
- Centralize effective basedir/project resolution so commands do not independently reinterpret defaults.
- Preserve explicit CLI/environment precedence and distinguish an implicit workspace default from an explicit selector where run-ID registry resolution requires that distinction.
- Register `init` in dispatch, command metadata, usage/schema, shell completion, and agent guide as appropriate.
- Test all `init` arities, relative/absolute path rules, validation, overwrite refusal, atomic write behavior, no project/state/registry side effects, CLI/environment overrides, and execution from nested workspace directories.
- Cover plain jobs, arrays/matrices, run-ID and attempt-ID selectors, config generation/listing, completion, web startup, and MCP target selection where shared target resolution applies.

### Phase 3 — Layered config use and run snapshot

- Apply the merged file map consistently to CLI defaults for every command that loads configuration.
- Pass the exact merged file-config snapshot into run/retry supervisor start paths; do not reread source files in `projectrun`.
- Preserve separate runtime CLI/environment overrides and notification configuration.
- Test normal and async runs, retries, saved-run execution, source edits/deletion between client load and supervisor begin, snapshot write failure ordering, and empty/no-config snapshots.
- Update `RunContext`/snapshot metadata and Web run display to support one merged snapshot plus multiple source paths while reading legacy records.
- Inspect CLI `show`, Web UI, and static exports for source-versus-snapshot display consistency and secret exposure.

### Phase 4 — Contracts, docs, and conformance

- Update resolution/config contracts and assign contract IDs for workspace discovery, init side effects, scope merge/constraints, and run file-config snapshots.
- Add conformance coverage through the built binary for workspace discovery/precedence, init behavior, forbidden keys, and snapshot stability. Keep `covers()` calls synchronized with contract status rows.
- Update `docs/CONFIGURATION.md`, `docs/CONCEPTS.md`, `docs/GETTING_STARTED.md`, `docs/CLI_REFERENCE.md` (generated source/flow as appropriate), `docs/FAQ.md`, and `docs/ARCHITECTURE.md` if package/flow changes.
- Regenerate generated CLI/Python metadata and README sections using their source documents/scripts; do not edit generated output by hand.

## Validation plan

Run focused tests first, then broader affected packages and interfaces:

1. `go test ./internal/config ./internal/state ./cmd/rotari -run '<focused tests>' -count=1 -v`
2. `go test ./internal/config ./internal/state ./internal/projectrun ./internal/supervisor ./cmd/rotari`
3. Relevant resolution and interface conformance packages, followed by `go test ./conformance`.
4. Run generation checks, changed-file formatting/pre-commit, and `scripts/check.sh --short`.
5. Before completion, run `scripts/check.sh` including race checks, inspect the complete diff, and verify old-run snapshot compatibility.

## Risks and decisions to verify before implementation

- The required precedence for target selection must be specified precisely, especially interaction between workspace/global defaults and the existing auto-select-single-project behavior.
- The existing local `.rotari-state` detection is based on cwd and existence. Define how it interacts with an explicit workspace basedir default; explicit workspace configuration should win, while preserving CLI/env priority.
- Workspace discovery stops at the nearest `.rotari.toml`; nested workspaces intentionally shadow outer workspaces.
- `--config FILE` currently replaces automatic discovery. Decide whether it continues to bypass all workspace/scope files or becomes an additional/override layer; preserving replacement semantics is the safer compatibility default.
- Global config may currently contain location keys. Define and test whether global `basedir` and `project-name` remain usable as selection defaults.
- Existing generated templates expose `basedir` and `project-name` as common keys. Update generated templates and validation so location keys are accepted only in permitted scopes.
- Current `configValue` gives command-section values precedence over root values. Cross-scope merging needs tests for conflicts between a high-priority scope's root key and a lower-priority scope's command-specific key.
- Existing user-visible config output, `config --list`, Web config editing, and run view assume one selected source file; all must be checked for layered sources and the single merged snapshot.

## Completion criteria

- A workspace initialized with `rotari init` consistently selects the same basedir/project from its root and nested directories.
- CLI and environment overrides retain their documented precedence.
- All applicable config scopes merge deterministically, invalid location keys fail with actionable diagnostics, and no scope has a location-resolution cycle.
- A run's saved `config.toml` is the exact merged file configuration used by that invocation, remains stable when source files change, and excludes CLI/environment overrides.
- Existing state layout, run-ID resolution, CLI/server/Web/MCP consistency, notification config behavior, and legacy run views remain compatible or are changed through explicit contract updates.
- Focused tests, conformance, generation checks, and the prescribed repository checks pass.
