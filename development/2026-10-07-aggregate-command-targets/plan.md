# Plan: Keep Aggregate Commands Independent of Implicit Targets

Created: 2026-10-07
Status: Planned

## Purpose

Workspace/global defaults for `basedir` and `project-name` are useful when a command is meant to operate on one project. They should not silently turn aggregate/listing commands into single-project views. Environment variables are implicit defaults too and should behave like config-file defaults for this distinction.

## Goals

- Keep normal project-scoped commands using the existing location precedence: CLI > environment > config > built-in/project discovery.
- For aggregate commands, only command-line target selectors may narrow the view. Do not use `basedir` or `project-name` from environment variables or config files to narrow those commands.
- Preserve each command's current non-config fallback and explicit flags; this change concerns implicit target defaults, not all command options.
- Make `lineage` useful without a project default by discovering candidate projects and giving deterministic guidance.
- Keep target resolution consistent for positional project/run/job/attempt selectors and the run registry.

## Proposed command behavior

| Command | Without explicit CLI scope selectors | With explicit CLI selectors |
|---|---|---|
| `show` | Preserve bare `show`'s aggregate project listing across registered basedirs. Ignore implicit config/environment basedir and project-name. | `--basedir` narrows basedirs; `--project-name` selects a project within the normal resolution rules. Run/job/attempt selectors retain registry-based resolution. |
| `jobs` | Show all projects in the command's ordinary non-config default basedir; ignore configured/environment basedir and project-name. Preserve existing `--all-basedirs` behavior for the registry-wide view. | `--basedir` and positional/`--project-name` explicitly narrow the target. |
| `lineage` | Ignore implicit project-name. Resolve the ordinary non-config default basedir, enumerate its project names, run lineage directly if there is exactly one candidate, and otherwise print sorted candidates with an example `-p NAME` invocation. If there are no candidates, report that clearly. Do not prompt interactively. | Explicit `--project-name` selects a project. Explicit run IDs/positional run selectors continue to use registry resolution and must not be mistaken for project defaults. |
| `config --list` | Ignore implicit project-name and list all projects' config files in the selected non-config default basedir. | Explicit `--basedir` selects the inventory scope; explicit `--project-name` narrows project-specific entries. |
| `show --basedirs`, `jobs --all-basedirs` | Continue listing all registered basedirs as their existing explicit aggregate modes require. | Existing explicit mode/filters remain unchanged. |

For aggregate target selectors, CLI arguments are the only way to scope/narrow the view. `ROTARI_BASEDIR`, `ROTARI_PROJECT_NAME`, global/workspace/basedir/project config defaults, and auto-select-single-project behavior must not narrow the aggregate view. Other options such as `--since`, output format, filters, and JSON remain eligible for their established config/environment behavior unless they themselves specify a location scope.

The “ordinary non-config default basedir” above means resolve with no requested basedir through the existing state resolver: local `.rotari-state` detection, then XDG/home. It must not apply basedir defaults from environment or config. This keeps `jobs`, `lineage`, and `config --list` deterministic without making them unexpectedly scan every registry entry.

## Non-goals

- Do not alter target defaults for commands that operate on one project, such as `add`, `run`, `retry`, `reset`, `check`, `copy`, or `change`.
- Do not change registry-based run/attempt ID location resolution.
- Do not make `jobs` scan all known basedirs by default; retain `--all-basedirs` as its existing explicit switch.
- Do not add interactive project selection to `lineage`.
- Do not change non-target option precedence or notifications behavior.

## Implementation approach

1. Inspect shared config location provenance (`cliLocationExplicit`, `cliLocationDefaults`) and command entry points for `show`, `jobs`, `lineage`, and `config --list`.
2. Add command-aware resolution helpers that distinguish CLI-explicit scope selectors from environment/config defaults. Avoid mutating global config state or duplicating priority checks in each command.
3. Preserve current bare `show` all-basedirs behavior, while ensuring only explicit CLI selectors narrow it. Verify interaction with `show --basedirs`, positional selectors, run IDs, attempt IDs, and explicit project names.
4. Update `jobs` to use only CLI scope selectors and the non-config state default when selecting the default basedir. Ensure `--all-basedirs` still wins as its explicit aggregate mode and `--project-name`/positional project behave consistently.
5. Update `lineage`: explicit project behaves as today; absent explicit project enumerates projects after resolving the non-config basedir. One candidate is selected automatically; multiple candidates are listed with a `rotari lineage -p NAME` hint; zero candidates returns a clear no-project result. Keep explicit run IDs routed through the run registry.
6. Update `config --list` so project-level config defaults do not narrow the inventory. Keep explicit CLI `--project-name` and `--basedir` useful; location config/env defaults do not narrow this aggregate listing.
7. Update help/comments if needed, contracts (resolution/config and selector contracts), contract status rows and `covers()` calls, CLI/conformance tests, and user docs/FAQ.

## Tests and validation

- Unit/table tests for each aggregate command under a matrix of CLI, environment, workspace, global, and automatic-single-project defaults.
- `show`: no scope defaults yields all registered projects; config/env defaults do not narrow; CLI basedir/project does; run/attempt IDs still resolve through registry.
- `jobs`: config/env project and basedir are ignored; explicit CLI project/basedir narrows; `--all-basedirs` still spans registry; positional project remains explicit.
- `lineage`: no explicit project with zero/one/multiple candidates; config/env defaults ignored; explicit `-p` selects; positional run IDs still work through registry.
- `config --list`: config/env project defaults do not hide project files; explicit project/basedir filters work; notifications remain separately listed.
- Conformance through the built binary for visible behavior and contract coverage. Update `contracts/README.md` status and `covers()` calls together.
- Run focused CLI tests, affected CLI and conformance packages, `go test ./conformance`, formatting/pre-commit, `scripts/check.sh --short`, then `scripts/check.sh`.

## Completion criteria

- Aggregate commands never narrow their target scope from environment/config location defaults or automatic single-project selection.
- Explicit CLI scope selectors still narrow as documented.
- `lineage` without an explicit project has deterministic zero/one/multiple-project behavior and preserves run-registry selector resolution.
- Existing project-scoped command defaults and notification configuration behavior are unchanged.
- Contracts, user docs, targeted tests, full conformance, and repository checks agree and pass.
