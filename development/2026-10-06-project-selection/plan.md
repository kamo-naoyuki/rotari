# Plan: Reduce Project Selection Friction

**Created:** 2026-10-06

**Status:** Planning. The command inventory and measurement have not started. This plan is Phase 3 of [Work on a Project While Its Run Is Active](../2026-10-05-work-during-runs/plan.md); Phases 1 and 2 are complete. No implementation decision, including project pinning, has been made.

## Purpose

When a state directory contains multiple projects, the normal project resolution rule requires an explicit project name. Some commands may nevertheless have one unambiguous target, especially operations that act on a running run. This work measures the actual friction after queue edits and in-run retry are available, then relaxes resolution only where one target is safe and well-defined.

The goal is not to guess a project. Explicit choices and identifiers keep their existing precedence, ambiguous operations remain errors, and no mutable global “current project” is introduced.

## Scope and non-goals

### In scope

1. Inventory commands that fail with the multiple-project error and identify the target each command acts on: queue, active run, saved run, or project collection.
2. Determine which failures have one objectively unique target, and record examples or usage evidence for any change considered.
3. Consider selecting the only active run for active-run commands, following `wait`'s local/remote lock discovery. Candidate commands are `cancel`, `suspend`, `resume`, and `retry` when it can only mean retrying the active run.
4. Decide whether a per-working-directory project pin is justified after the inventory. Implement it only if the evidence shows that explicit project selection remains a material cost for otherwise unambiguous queue operations.
5. Keep CLI, configuration/environment precedence, shell completion, MCP, Web/API and Python behavior consistent wherever the same project-selection rule applies.

### Non-goals

- A mutable global project shared by shells or processes.
- Selecting one of several active runs by recency or arbitrary order.
- Overriding explicit `--project-name`, `ROTARI_PROJECT_NAME`, run ID, attempt ID, or other existing selectors with an inferred target.
- Changing queue-first job selection, retry source rules, or run lifecycle behavior.
- Implementing a project pin before the audit justifies it.
- Phase 4 work that adds jobs to an active run.

## Existing behavior and constraints

- `state.ResolveProjectName` resolves `--project-name`, then `ROTARI_PROJECT_NAME`, then the sole project in the selected basedir; multiple projects are an error. With no projects, it returns `default`. See [internal/state/project.go](../../internal/state/project.go).
- `wait` has a separate active-run mode: absent an explicit project or selector it scans the resolved basedir, counts locks classified as local-active or remote-active, waits for the sole active run, lists several active runs as an ambiguity, and errors when none are active. See [cmd/rotari/wait.go](../../cmd/rotari/wait.go) and RES-16.
- Run IDs and attempt IDs can identify a project through the run registry without a project name (RES-13/14). Job selectors also have command-specific queue/run precedence; do not classify these as ordinary project-name failures without tracing their full resolution path.
- `retry` is special: with an active project it may request retry within that run; while idle it starts a new run. Automatic active-project selection must not silently change idle retry's project or source semantics.
- A selected target can change state between discovery and action. Preserve existing state-lock and run-ID checks; never fall back to a different project after a race.
- Project pin priority is only a proposal, not a decision: explicit CLI project, then `ROTARI_PROJECT_NAME`, then a pin, then existing sole-project/default/error behavior.

## Work plan

### 1. Audit and measure before changing behavior

Create a table covering every command family with:

- the observed multiple-project failure, if any;
- existing explicit selectors and registry-based resolution;
- the object selected (queue, active run, saved run, or collection);
- whether exactly one candidate can be determined from current state;
- what a second candidate would mean;
- user-impact evidence and a recommendation: keep error, infer safely, or investigate further.

At minimum inspect CLI command resolution, the MCP tools, Web API/UI project selection, Python wrappers, shell completion, and related contracts. Distinguish commands that act on an active run from queue/history commands. Do not infer frequency from the error string alone.

### 2. Decide active-run resolution

For each candidate command, explicitly settle these cases before implementation:

| Active runs in the resolved basedir | Required decision |
| --- | --- |
| none | Preserve the command's established no-active-run behavior. In particular, idle `retry` must not be captured by active-run routing. |
| exactly one | Decide whether this command can safely target that run and under which selectors/options. |
| more than one | Keep an error listing projects/runs; never choose newest or first. |
| one local or remote lock | Treat it according to `state.InspectLock`; a remote lock identifies an active run even if local execution/control may later be unavailable. |
| interrupted or cancelling project | Do not classify it as a normal active run; preserve `unlock` / canceling semantics and actionable errors. |
| run ends during selection | Fail for the originally selected run; do not resolve again to another project or silently start a new run. |

Prefer a shared resolver for active-run candidate discovery rather than duplicating `wait`'s scan in `cancel`, `suspend`, `resume`, and `retry`. Keep operation-specific validation in the existing owning package.

### 3. Decision gate for the project pin (D6)

After the audit, add a short decision record here choosing one of:

- **No pin:** the remaining errors are rare or ambiguous, so keep explicit selection.
- **Pin warranted:** a reproducible, common queue-editing workflow remains unnecessarily blocked and a pin is the smallest safe remedy.

If warranted, specify before implementation:

- exact pin filename, format, and location;
- whether lookup uses only the current directory or walks parents;
- how a pin is associated with the resolved basedir and project, including multiple basedirs;
- precedence versus CLI, environment, config, and the sole-project/default rule;
- behavior for malformed, stale, or missing-project pins;
- create/change/clear workflow and whether commands may create the pin implicitly;
- shell completion and help/reference behavior;
- compatibility and migration behavior for existing `.rotari-state` usage.

Do not proceed with an implementation if these choices remain open.

### 4. Implement only decisions supported by the audit

Make the smallest set of shared resolution changes that implements the selected decisions. Preserve explicit selectors and reject ambiguity. For active-run commands, share discovery and continue to validate the exact run under the appropriate lock. If a pin is approved, implement pin reading/writing and precedence at one shared boundary, then route CLI/MCP/Web/Python callers through it rather than reimplementing lookup.

### 5. Contracts, docs, and tests

If behavior changes, review and update as applicable:

- RES rules in [contracts/01-resolution-and-config.md](../../contracts/01-resolution-and-config.md) and selector behavior in [contracts/06-selectors.md](../../contracts/06-selectors.md);
- contract coverage rows in [contracts/README.md](../../contracts/README.md), matching each `covers` call;
- [docs/CONCEPTS.md](../../docs/CONCEPTS.md), including project resolution;
- generated CLI reference and Python API docs if exposed options or Python behavior change;
- all affected CLI, MCP, Web/API, Python, and completion tests.

Test zero, one, and multiple projects; zero, one, and multiple active runs; local and remote locks; explicit options and environment precedence; run/attempt IDs; idle and active `retry`; interruptions/cancellation; and the race where a selected run ends. Add contract conformance for user-visible binary/Web behavior. If the pin is not implemented, do not update user-facing behavior docs merely because this plan was split.

## Related work

- [Phase 1–2 plan: Work on a Project While Its Run Is Active](../2026-10-05-work-during-runs/plan.md)
- [Resolution contracts](../../contracts/01-resolution-and-config.md)
- [Selector contracts](../../contracts/06-selectors.md)
- [Project resolution guide](../../docs/CONCEPTS.md#state-and-project-resolution)
- [Development planning conventions](../README.md)
