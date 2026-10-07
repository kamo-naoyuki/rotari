# Plan: Keep Aggregate Commands Independent of Implicit Targets

Created: 2026-10-07
Status: Complete

## Progress

- Implemented command-specific location-default policy, CLI/conformance
	regressions, and contract/user-guide updates in the working tree.
- `scripts/check.sh --short` and the full non-race `go test ./...` phase passed.
- Registry-wide `show --basedirs` and `jobs --all-basedirs` reject an explicit
	`--basedir`, avoiding silent option discard.
- Final validation passed: `scripts/check.sh` (full go vet, all package and
	race tests), `go test ./cmd/rotari -count=1`, the RES-26 conformance test, and
	focused race tests for both rejected option combinations.
- Remaining: None.

## Purpose

Workspace/global defaults for `basedir` and `project-name` are useful when a
command operates on one project. Aggregate/listing commands should not silently
turn into single-project views because of an implicit project-name default.

## Goals

- Keep normal project-scoped commands using the existing location precedence:
	CLI > environment > config > built-in/project discovery.
- Never let an implicit project default narrow an aggregate view.
- Also ignore implicit basedir defaults for bare `show` and `jobs`, but retain
	normal basedir precedence for argumentless `lineage` and `config --list`.
- Preserve positional project/run/job/attempt selectors and registry-based run
	resolution.
- Make argumentless `lineage` useful through deterministic zero/one/multiple
	project-candidate behavior, without an interactive prompt.


## Proposed command behavior

| Command | Without explicit CLI scope selectors | With explicit CLI selectors |
|---|---|---|
| `show` | Preserve bare `show`'s aggregate project listing across registered basedirs. Ignore implicit config/environment basedir and project-name. | `--basedir` narrows basedirs; `--project-name` selects a project within the normal resolution rules. Run/job/attempt selectors retain registry-based resolution. |
| `jobs` | Show all projects in the command's ordinary non-config default basedir; ignore configured/environment basedir and project-name. Preserve existing `--all-basedirs` behavior for the registry-wide view. | `--basedir` and positional/`--project-name` explicitly narrow the target. |
| `lineage` | Ignore implicit project-name, but honor basedir defaults from CLI > env > workspace/basedir/global config > built-in local/XDG/home resolution. Enumerate projects in that basedir, run lineage directly if there is exactly one candidate, and otherwise print sorted candidates with an example `-p NAME` invocation. If there are no candidates, report that clearly. Do not prompt interactively. | Explicit `--project-name` selects a project. Explicit run IDs/positional run selectors continue to use registry resolution and must not be mistaken for project defaults. |
| `config --list` | Ignore implicit project-name, but honor basedir defaults from CLI > env > workspace/global config > built-in local/XDG/home resolution; list all projects' config files in the selected basedir. | Explicit `--basedir` selects the inventory scope; explicit `--project-name` narrows project-specific entries. |
| `show --basedirs`, `jobs --all-basedirs` | Continue listing all registered basedirs as their existing explicit aggregate modes require. | Existing explicit mode/filters remain unchanged. |

`show --basedirs` and `jobs --all-basedirs` reject an explicit `--basedir`
instead of silently ignoring the conflicting scope option.


Aggregate listings ignore implicit project defaults from environment, config,
and automatic sole-project selection. Bare `show` and `jobs` also ignore
implicit basedir defaults, using local `.rotari-state`/XDG/home resolution
unless scoped by CLI. `lineage` and `config --list` retain normal basedir
precedence, including environment and workspace/basedir/global config defaults.
Other options such as filters, JSON, and time windows retain their established
config/environment behavior.
- Do not make `jobs` scan all known basedirs by default; retain `--all-basedirs` as its existing explicit switch.
- Do not add interactive project selection to `lineage`.
- Do not change non-target option precedence or notifications behavior.

## Implementation approach

1. Inspect shared config location provenance (`cliLocationExplicit`, `cliLocationDefaults`) and command entry points for `show`, `jobs`, `lineage`, and `config --list`.
2. Add command-aware resolution helpers that distinguish CLI-explicit scope selectors from environment/config defaults. Avoid mutating global config state or duplicating priority checks in each command.
3. Preserve current bare `show` all-basedirs behavior, while ensuring only explicit CLI selectors narrow it. Verify interaction with `show --basedirs`, positional selectors, run IDs, attempt IDs, and explicit project names.
4. Update `jobs` to use only CLI scope selectors and the non-config state
	default for the basedir. Preserve `--all-basedirs` and explicit project
	filters.
5. Update `lineage`: ignore implicit environment/config project defaults,
	honor normal basedir resolution, and enumerate candidates. A sole candidate is selected
	automatically; multiple candidates are listed with a `rotari lineage -p
	NAME` hint; zero candidates returns a clear error. Keep run IDs routed
	through the registry.
6. Update `config --list` to ignore implicit environment/config project
	defaults while honoring normal basedir selection and explicit CLI filters.
7. Update help/comments if needed, contracts (resolution/config and selector contracts), contract status rows and `covers()` calls, CLI/conformance tests, and user docs/FAQ.

## Tests and validation

- Unit/table tests for each aggregate command under a matrix of CLI, environment, workspace, global, and automatic-single-project defaults.
- `show`: no scope defaults yields all registered projects; config/env defaults do not narrow; CLI basedir/project does; run/attempt IDs still resolve through registry.
- `jobs`: config/env project and basedir are ignored; explicit CLI project/basedir narrows; `--all-basedirs` still spans registry; positional project remains explicit.
- `lineage`: no explicit project with zero/one/multiple candidates; implicit project defaults ignored while CLI/env/workspace/global basedir defaults select the candidate scope; explicit `-p` selects; positional run IDs still work through registry.
- `config --list`: config/env project defaults do not hide project files; CLI/env/workspace/global basedir defaults select the inventory scope; explicit project/basedir filters work; notifications remain separately listed.
- Conformance through the built binary for visible behavior and contract coverage. Update `contracts/README.md` status and `covers()` calls together.
- Unit tests verify `lineage` and `config --list` keep normal basedir defaults while ignoring implicit project defaults, and positional `show PROJECT` uses normal basedir resolution.
- Run focused CLI tests, affected CLI and conformance packages, `go test ./conformance`, formatting/pre-commit, `scripts/check.sh --short`, then `scripts/check.sh`.

## Completion criteria

- Aggregate commands never narrow by an implicit project default. Bare `show` and `jobs` also ignore implicit basedir defaults; `lineage` and `config --list` retain normal basedir resolution.
- Explicit CLI scope selectors still narrow as documented.
- `lineage` without an explicit project has deterministic zero/one/multiple-project behavior and preserves run-registry selector resolution.
- Existing project-scoped command defaults and notification configuration behavior are unchanged.
- Contracts, user docs, targeted tests, full conformance, and repository checks agree and pass.
