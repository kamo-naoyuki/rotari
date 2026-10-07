# Plan: Workspace Configuration and Merged Run Config

Created: 2026-10-07
Status: Complete

## Progress

- 2026-10-07 16:57:38 +09:00 — Phase 1 committed as `19fa824d`: scoped
	discovery/validation, recursive merge, canonical file-only snapshots and
	provenance types. Focused and config/model package tests passed; architecture
	boundaries checked uncached. Null is unspecified; command sections retain
	their existing precedence after cross-scope merge.
- 2026-10-07 17:17:32 +09:00 — Phases 2–4 committed as `fe1975a9`:
	staged CLI location defaults/provenance, init/metadata/completion, captured
	file-config run requests, Web source selection and validated saves, legacy
	snapshot compatibility, notification regression, contracts/docs/generation.
	Relevant package tests, focused binary/Web conformance, root contract/golden
	tests, changed-file pre-commit, generator checks and 29 Python tests passed.
- 2026-10-07 17:54:40 +09:00 — `2cf1aab7` corrected the selector test observer
	to exclude run-level `configs/` metadata. The selector table and complete
	selector package passed. The isolated implementation worktree plus this
	correction passed `scripts/check.sh --short`.
- 2026-10-07 18:11:37 +09:00 — `3abd9599` made init use the shared workspace
	TOML template with active basedir/project defaults and commented option
	assignments. Omitted arguments now write `.rotari-state` and `default`.
	Existing-file diagnostics were clarified. Focused tests, affected packages,
	contract/link checks, and init-specific race tests passed.
- Final verification: `scripts/check.sh` completed successfully after the init
  fixes: go vet, all package/conformance tests, and all race tests passed
  (exit code 0; some unchanged packages used Go's test cache). The validation
  worktree was removed after confirming its correction was committed in main.
- Remaining: None.

## Purpose

Make Rotari's target selection and configuration predictable per workspace. A workspace can choose its default basedir and project without creating a project during initialization, while normal configuration values inherit across global, workspace, basedir, and project scopes. Each run records the merged file configuration that was available when it started.

## Goals

- Discover `.rotari.toml` only in the current working directory; do not search parent directories.
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

1. Look for `.rotari.toml` in cwd only. Load global and workspace config.
2. Choose basedir using explicit CLI, environment, workspace `basedir`, global `basedir`, then existing built-in/local-state resolution behavior as specified by the resolution contract. A workspace-relative basedir is resolved against the directory containing `.rotari.toml`.
3. Load the selected basedir config. It may set `project-name`, but setting `basedir` is invalid and produces an error identifying the file and key.
4. Choose project using explicit CLI, environment, basedir `project-name`, workspace `project-name`, global `project-name`, then existing project discovery/default behavior. Confirm exact ordering against existing semantics and encode it in one shared resolver.
5. Load the selected project config. It may not set `basedir` or `project-name`; either key is an error, including when nested in command-specific sections.
6. Merge ordinary config values in global → workspace → basedir → project order, then apply CLI and environment precedence through the existing option layer.

`rotari init` writes a workspace file at cwd. Normal commands read only that directory's workspace file, so invocation from a subdirectory does not inherit a parent's workspace defaults. It never changes the caller's current directory.

### `rotari init`

Supported forms:

- `rotari init` writes default basedir `.rotari-state` and project-name `default`.
- `rotari init <basedir>` writes the relative basedir and project-name `default`.
- `rotari init <basedir> <project-name>` writes both defaults.

Use the same workspace TOML template as `rotari config`: fill in only the
basedir/project-name assignments and leave all other configurable option
assignments commented out. Both location values are always written.

The basedir argument must be relative. Reject absolute paths and unsafe/invalid project path elements. Store the path as supplied after validation and interpret it relative to the workspace file's directory. Refuse to overwrite an existing `.rotari.toml` unless an explicit overwrite/update behavior is separately specified. Initialization writes only the file atomically; it does not create state directories or projects. Treat this as a special config/setup command in CLI metadata, help, schema, and shell completion.

### Run configuration snapshot

The CLI resolves and merges file configuration once. Pass the immutable merged file-config bytes/representation, not source paths to be reread, through the run request to the supervisor. At run begin, write canonical TOML to the run's existing config snapshot location as `config.toml` (confirm exact current path and keep it stable if possible). This snapshot excludes CLI overrides, environment overrides, and built-in defaults.

Record source paths and scope order separately from the single snapshot so the run view can explain which files contributed. Do not pair source-path metadata one-to-one with snapshot-file metadata. Keep notification snapshots independent. Preserve reading/display of legacy YAML/JSON/source-file snapshots for older runs. Ensure a source edited or removed after config loading cannot alter or prevent the run snapshot.

### Web configuration display and editing

- For current configuration, `View config` displays the actual source files, not a synthesized merged configuration. If multiple files are found, let the user select one by scope and path; if only one is found, open it directly.
- Edit and save only the selected source file. The save request identifies the selected scope/source, and the server validates it against allowed targets rather than accepting an arbitrary filesystem path or reselecting the highest-priority file at save time. Apply the same basedir/project key restrictions to Web saves as to CLI loading.
- Do not add a merged current-config viewer or editor. Runtime config merging is separate from source-file inspection.
- For a run, keep the existing read-only `View config` behavior: display the saved run-local configuration. For new runs this is the merged `config.toml`; legacy snapshots remain readable. Never reconstruct a run's config by rereading its source files.
- Workspace context is the Web server's startup cwd, not a viewed job's working directory. Do not discover parent workspace files or switch workspace context when navigating projects.
- Cover live Web and static-export configuration views consistently.

### Notification configuration remains separate

`notifications.toml` is not part of the normal configuration merge chain. Keep its existing project → basedir → global selection: use the first existing file, with no inheritance from lower-priority notification files and no new workspace notification scope. Omitted fields use notification built-in defaults, not lower-scope values. Preserve the independent notification settings UI and notification snapshot behavior.

Document the distinction explicitly in both `docs/CONFIGURATION.md` and `docs/NOTIFICATIONS.md`: normal command config loads and merges all applicable scopes, whereas notification config selects a single highest-priority file. Explain that a project notification file replaces the lower-scope configuration rather than inheriting a global webhook destination.

## Implementation phases

### Phase 1 — Configuration primitives and merge contract

- Extend `internal/config` with workspace discovery, scope-aware loading, validation, deep merge, and canonical TOML serialization.
- Keep configuration independent of CLI and do not introduce a dependency cycle into `internal/state`.
- Add tests for cwd-only discovery, ignored parent workspace files, missing workspace config, relative workspace basedir, scope override behavior, nested maps, arrays, null/false/zero/empty-string, malformed files, duplicate formats, and forbidden location keys at root and command sections.
- Define behavior for malformed automatically discovered scopes. Prefer failing with a path/scope diagnostic rather than silently dropping a layer, unless compatibility analysis requires a transition.

### Phase 2 — Staged target resolution and `init`

- Refactor CLI config loading to load workspace/global location defaults before basedir resolution, then basedir config before project resolution, and project config only after both are known.
- Centralize effective basedir/project resolution so commands do not independently reinterpret defaults.
- Preserve explicit CLI/environment precedence and distinguish an implicit workspace default from an explicit selector where run-ID registry resolution requires that distinction.
- Register `init` in dispatch, command metadata, usage/schema, shell completion, and agent guide as appropriate.
- Test all `init` arities, relative/absolute path rules, validation, overwrite refusal, atomic write behavior, no project/state/registry side effects, CLI/environment overrides, and non-inheritance of workspace defaults when executing from a subdirectory.
- Cover plain jobs, arrays/matrices, run-ID and attempt-ID selectors, config generation/listing, completion, web startup, and MCP target selection where shared target resolution applies.

### Phase 3 — Layered config use and run snapshot

- Apply the merged file map consistently to CLI defaults for every command that loads configuration.
- Pass the exact merged file-config snapshot into run/retry supervisor start paths; do not reread source files in `projectrun`.
- Preserve separate runtime CLI/environment overrides and notification configuration.
- Test normal and async runs, retries, saved-run execution, source edits/deletion between client load and supervisor begin, snapshot write failure ordering, and empty/no-config snapshots.
- Update `RunContext`/snapshot metadata and Web run display to support one merged snapshot plus multiple source paths while reading legacy records.
- Update current Web `View config` to select among source files when more than one exists, without showing a merged current-config view. Test single/multiple/missing sources, selected-file saves, scope restrictions, allowed-target validation, startup-cwd workspace context, and static export.
- Retain notification single-file precedence and add regression coverage showing that a project notification file does not inherit lower-scope webhook settings.
- Inspect CLI `show`, Web UI, and static exports for source-versus-snapshot display consistency and secret exposure.

### Phase 4 — Contracts, docs, and conformance

- Update resolution/config contracts and assign contract IDs for workspace discovery, init side effects, scope merge/constraints, and run file-config snapshots.
- Add conformance coverage through the built binary for workspace discovery/precedence, init behavior, forbidden keys, and snapshot stability. Keep `covers()` calls synchronized with contract status rows.
- Update `docs/CONFIGURATION.md`, `docs/NOTIFICATIONS.md`, `docs/CONCEPTS.md`, `docs/GETTING_STARTED.md`, `docs/CLI_REFERENCE.md` (generated source/flow as appropriate), `docs/FAQ.md`, and `docs/ARCHITECTURE.md` if package/flow changes. Explicitly contrast normal config's multi-scope merge with notification config's highest-priority single-file selection, and explain Web source selection versus run snapshot display.
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
- Workspace discovery is cwd-only. A parent `.rotari.toml` must not affect a command executed in a child directory.
- `--config FILE` currently replaces automatic discovery. Decide whether it continues to bypass all workspace/scope files or becomes an additional/override layer; preserving replacement semantics is the safer compatibility default.
- Global config may currently contain location keys. Define and test whether global `basedir` and `project-name` remain usable as selection defaults.
- Existing generated templates expose `basedir` and `project-name` as common keys. Update generated templates and validation so location keys are accepted only in permitted scopes.
- Current `configValue` gives command-section values precedence over root values. Cross-scope merging needs tests for conflicts between a high-priority scope's root key and a lower-priority scope's command-specific key.
- Existing user-visible config output, `config --list`, Web config editing, and run view assume one selected source file; all must be checked for layered sources and the single merged snapshot.

## Completion criteria

- A workspace initialized with `rotari init` selects its configured basedir/project when invoked from that directory; child directories do not implicitly inherit the workspace file.
- CLI and environment overrides retain their documented precedence.
- All applicable config scopes merge deterministically, invalid location keys fail with actionable diagnostics, and no scope has a location-resolution cycle.
- A run's saved `config.toml` is the exact merged file configuration used by that invocation, remains stable when source files change, and excludes CLI/environment overrides.
- Current Web configuration views select individual source files; run configuration views display saved snapshots. Notification configuration remains single-file and the difference from normal config merging is documented.
- Existing state layout, run-ID resolution, CLI/server/Web/MCP consistency, notification config behavior, and legacy run views remain compatible or are changed through explicit contract updates.
- Focused tests, conformance, generation checks, and the prescribed repository checks pass.
