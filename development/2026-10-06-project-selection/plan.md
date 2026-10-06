# Plan: Reduce Project Selection Friction

**Created:** 2026-10-06

**Status:** Steps 1 and 2 are complete as a repository behavior audit and
resolution proposal. Implementation has not started and is paused pending
D10/D11, the active-retry mode/attempt-revision and queue-versus-run target
decisions in the umbrella plan.
Static behavior is inventoried below; user-frequency evidence is unavailable
in the repository, so project pin D6 remains undecided. This plan is Phase 3 of
[Work on a Project While Its Run Is Active](../2026-10-05-work-during-runs/plan.md);
Phases 1 and 2 are complete.

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

## Step 1: Repository command-resolution audit

This is a static audit of the current code and tests, not a measurement of
command frequency. The repository has no usage telemetry or representative
user reports from which to infer how often an ambiguity occurs. Do not treat
the original motivation (“`--project-name` on every command”) as established:
many commands already resolve by run ID, attempt ID, or a unique job selector.

| Command family | Current behavior when multiple projects exist | Safe unique target already available? | Phase 3 treatment |
| --- | --- | --- | --- |
| `add`, `import`, `reset`, `check` and queue/project mutations | Need a project destination; normal project resolution errors unless an explicit project or other command-specific selector applies. | Usually no. A job's presence does not identify which project's queue should be edited. | Keep explicit project choice. A project pin is only considered if external evidence demonstrates recurring queue-edit friction. |
| `run` | Starts a new run from one project's queue. Run/attempt IDs and job selectors have established specialized resolution paths; otherwise normal project resolution can be ambiguous. | Sometimes an explicit run/attempt ID or a uniquely resolving job selector identifies the project. An active project is not a safe default: the user may intend a different idle project's queue. | Preserve existing selector resolution; never infer `run`'s project from the active-run set. |
| `retry` | Active retry currently resolves the project before active-run routing, so it can fail on multiple projects even if one project is running. Idle retry remains a new-run operation. | Run/attempt IDs and job IDs/names may identify a project under existing selector rules. A result selection such as `--failed` is run-local and has no cross-project identity by itself. | Apply D9 below only to retry forms whose intent is explicitly run-local by result/scope/filter selection and which do not provide a direct project/run/job identity. Preserve idle retry when there are no active runs. |
| `cancel`, `suspend`, `resume` | Without a project, run ID, attempt ID, or a uniquely resolvable job ID, normal project resolution errors. A job ID may already locate one active run; names and other filters generally require project/run context. | Yes when an existing run/attempt/job ID resolver uniquely locates the active target. | Keep those paths unchanged. For selectorless active-run control only, apply D9. |
| `wait` | Already scans the resolved basedir when no project/selector is given: waits for exactly one active run, lists multiple active runs, and errors when none exist. | Yes; this is the precedent for active-run candidate discovery. | Preserve RES-16 and share its lock classification/discovery rule where practical. |
| `show`, `copy`, `change`, `remove`, `export`, `lineage`, `delete` and history selectors | Run/attempt IDs use the run registry. Some job selectors search across projects and resolve only when a unique match exists; multiple matches are errors. Collection/list views intentionally show multiple projects. | Yes for registry IDs and unique existing job/run selectors. No for ambiguous names or collection destinations. | Preserve all current selector behavior; do not add active-project fallback to history or queue mutation. |
| MCP | Project mutations explicitly take `basedir_ref` and project; run-oriented reads/control use a run ID and registry lookup. | Yes through explicit project or run identity. | No implicit current-project selection. |
| Web UI/API | Index and project lists expose multiple projects; detail and mutation endpoints receive project/run identity from the selected UI context. | Yes through explicit UI selection. | No server-side inference from browser/session state. |
| Python client | Thin CLI wrapper; `Rotari(project=...)`, `Run`, and `Job` objects supply explicit context or IDs. No separate project inference. | Yes through the wrapped CLI selectors. | Keep CLI resolution as the source of truth. |
| Shell completion | Project-name completion lists projects; run/job completions use their existing basedir/project/run scopes. | Completion offers candidates but does not choose a target. | Update only if a later behavior decision changes candidates or adds a pin option. |

The command families above were traced through `state.ResolveProjectName`,
the run registry, the CLI selector paths, `wait`'s active-lock scan, MCP/Web
inputs, and the existing resolution/selector tests. This establishes behavior,
not how frequently users encounter each case. D6 therefore remains open pending
usage evidence; the static audit alone does not justify a project pin.

## Step 2: Active-run resolution decision (D9)

Settled on 2026-10-06:

- **D9** Active-run inference is limited to operations whose intent is to act
  on an already running run: selectorless `cancel`, `suspend`, and `resume`,
  plus `retry` when its effective selection is run-local: an explicit result
  selection, `retry`'s default failed+unfinished selection, a stage/matrix
  scope, or a per-job filter. It is not used by `run`, queue mutations,
  history operations, MCP, or Web APIs. It applies only when the project is
  not otherwise identified and the resolved basedir contains exactly one
  active run.
- Existing identity wins: explicit project from CLI/config/environment,
  registered run/attempt ID, positional run selector, or a job ID/name that
  already resolves uniquely keeps its present resolution and validation.
  `--run-id latest` continues to mean the latest settled run; it is not an
  alias for the active run. If a direct selector identifies another project
  or a different run, fail rather than redirecting it to the sole active run.
- The active-run candidate set is based on `state.InspectLock`: local-active
  and remote-active locks count, while an interrupted run with no active lock
  does not. A cancelling run is still identified by its active lock, but the
  operation's existing phase checks decide whether the operation is allowed;
  inference does not make a cancelling run accept retry.
- With zero active runs, preserve current project resolution and each command's
  existing idle/no-active behavior. With more than one active run, return an
  ambiguity error listing the candidate projects and run IDs; never choose by
  recency or iteration order.
- Discovery only proposes a target. Before acting, retain the selected
  project/run identity and revalidate it using the owning operation's lock and
  state checks. If it ended or changed, fail for that identity; do not rescan,
  select a different project, or let `retry` silently become a new run.

D9 records a candidate resolution policy, not an implementation commitment.
Its `retry` part is provisional until D10 settles the active retry's invocation
mode and per-attempt revision, and D11 settles how the next-run queue is kept
distinct from active-run edits. Revise D9 to match those contracts before
implementing Phase 3. Step 4 must still define a shared active-run resolver and
enumerate command-specific selector/config interactions before code changes.
The project pin remains a separate D6 decision gate and is not implied by D9.

## Work plan

### 1. Audit project resolution — complete

The static repository audit is recorded in the table above. It covers the
CLI's queue/run/history command families, existing run/job selector paths,
`wait`, MCP, Web, Python, and completion. It found that the original claim
that every command needs `--project-name` was too broad: run/attempt IDs and
unique job selectors already resolve many operations. The repository provides
no usage telemetry or other frequency evidence, so this audit does not claim
which errors users encounter most often. D6 remains open until such evidence
or a reproducible workflow is available.

### 2. Decide active-run resolution — complete (D9)

D9 above settles the candidate operations and their boundaries: selectorless
`cancel` / `suspend` / `resume`, and `retry` with its effective run-local
selection, may infer the only active run in the selected basedir. Existing
project, run/attempt, and uniquely resolving job selectors keep precedence;
`run` and queue/history operations do not infer an active project. Zero active
runs preserve existing behavior, multiple active runs remain an error, remote
locks count as active, and race revalidation remains pinned to the discovered
run. This is a design decision only; implementation and tests remain in step 4.

### 3. Decision gate for the project pin (D6)

If later user reports or other reliable usage evidence demonstrates material
friction, add a short decision record here choosing one of:

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
