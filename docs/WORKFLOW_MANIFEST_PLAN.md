# Workflow manifest plan

Status: implemented

## Motivation

The `change` command is useful for one-off queue repair, but a sequence of
`copy`, `change`, and `run` commands is difficult to review and reproduce. A
workflow manifest should make the desired job graph durable while preserving
rotari's existing queue, run history, retry, and executor semantics.

This feature is a constrained declarative interface, not a new command
language. Shell scripts and command argument vectors remain the execution
language. YAML, TOML, and JSON are equivalent serializations of one schema.

The primary workflow should be:

```sh
rotari export --run-id RUN_ID > experiment.yaml
# Edit failed jobs in experiment.yaml.
rotari import experiment.yaml --project-name project
rotari run --project-name project
```

`export` turns an immutable run snapshot into an editable manifest. `import`
validates that manifest, reconciles it with the exported source run, and
materializes the next queue. `run` remains the ordinary queue execution command.

## Goals

- Describe a job graph in a file that can be reviewed and versioned.
- Export a run as an editable, portable workflow definition.
- Import the definition into the existing queue model with a clear summary of
  which jobs will execute or reuse results.
- Reuse successful work from an earlier run when its job definition is
  unchanged, while executing failed, unfinished, new, or changed jobs.
- Keep behavior consistent across local, SSH, Slurm, PBS, and LSF executors.
- Make the generated plan explicit enough to explain every reuse and execution
  decision before a run starts.

## Non-goals

- A programming language, expression evaluator, template language, or embedded
  shell parser.
- Automatic input/output discovery, timestamp checks, caching, or artifact
  storage.
- Scheduler-specific dependency semantics.
- Conditional jobs, loops other than bounded matrix/array expansion, includes,
  or remote manifest loading in version 1.
- Semantic merging of independently edited manifest files. Users may use Git
  or manual editing, then rely on strict import validation.
- Removing `add`, `copy`, `change`, or `retry` during initial delivery.

## Version 1 schema

Job `name` is optional. Non-empty names must be unique and remain dependency
targets. Run exports identify provenance with an attempt ID, which embeds its
source run ID and job ID; destination queue job IDs remain runtime state.

```yaml
version: 1

source:
  project: project
  run_ids: [20260925-120000-12345678]

jobs:
  - name: prepare
    stage: inputs
    command: [python, scripts/prepare.py]
    status: success
    attempt_id: att_20260925-120000-12345678-6aa4af5d9-0

  - name: train
    command: [python, scripts/train.py]
    depends_on: [inputs]
    executor: slurm
    executor_options: ["--partition=gpu"]
    environment: ["EPOCHS=20"]
    matrix: ["SEED=1,2,3", "MODEL=small,large"]

  - name: evaluate
    command: [python, scripts/evaluate.py]
    depends_on: [train]
    array: "1,3,4"
```

The common schema fields are:

| Location | Field | Type | Meaning |
| --- | --- | --- | --- |
| root | `version` | integer | Required schema version; initially `1`. |
| root | `source` | object | Optional exported source project and ordered run IDs used for reconciliation. |
| root | `jobs` | array | Ordered, non-empty job definitions. |
| job | `name` | string | Optional logical identity and dependency target; unique when present. |
| job | `command` | string array | Required executable and arguments; never parsed as a shell string. |
| job | `stage` | string | Optional stage membership. |
| job | `depends_on` | string array | Job or stage names. |
| job | `executor` | string | Optional executor override. |
| job | `executor_options` | string array | Options for the effective executor. |
| job | `working_directory` | string | Working directory used by the executor. |
| job | `environment` | string array | Same repeated `KEY=VALUE` values as `add --env`, preserving order and duplicates. |
| job | `array` | string | Same syntax as `add --array`: `FIRST-LAST` or `TASK[,TASK...]`. |
| job | `matrix` | string array | Repeated `KEY=VALUE[,VALUE...]` dimensions, in the same order as `add --matrix`. |
| job | `status` | string | Optional editable import disposition: `success`, `failed`, `cancelled`, or `unfinished`. |
| job | `attempt_id` | string | Optional source attempt; exported for inspection and normally left unchanged. |
| job | `instances` | array | Optional non-success matrix combinations or array tasks with attempt IDs and editable status. |

Unknown fields are errors. YAML aliases and merge keys are rejected so all
three formats have the same data model. Environment and matrix entries use the
existing CLI parsers and validation. Matrix dimensions are expanded in array
order and produce names using the existing `add --matrix` suffix format.

Matrix entries are appended after the job environment and are available to the
command as ordinary environment variables, matching `add --matrix`. Version 1
does not substitute matrix values into command arguments or paths.

`source` is generated by `export`. A hand-written manifest may omit it; in that
case every imported job is new work. It contains no base directory, absolute
state path, result, or origin payload. Import resolves the source project and
runs through normal rotari state lookup and rejects missing or ambiguous state.

## Commands

### `export`

```text
rotari export [--run-id ID ...] [--project-name NAME] [--format yaml|toml|json]
rotari export --template [--format yaml|toml|json]
```

With one or more `--run-id` options, this command reads immutable run snapshots.
Without one, the command reads the current queue. It writes a manifest to
stdout; YAML is the default and shell redirection chooses the destination file.
Exported jobs preserve names, commands, stages, dependencies, executor settings,
environment, working directories, and array definitions. Job IDs, result
payloads, timestamps, and origin records are omitted; source job IDs remain
recoverable from attempt IDs.

A run export includes the root `source` reference. Each scalar job gets its
status and latest attempt ID; expanded groups use aggregate status and
`instances` as described below. A queue export contains definitions only: it
omits `source`, `status`, and `attempt_id`, so importing it schedules every job.
This remains true when queued jobs contain origins from `copy`; version 1 does
not infer one source from a queue that may combine multiple runs. Exporting an
empty queue is an error.

`--template` does not read project state. It writes a valid starter manifest
with concise comments for the fields that are difficult to recall:

```sh
rotari export --template > experiment.yaml
```

```yaml
version: 1

jobs:
  - name: example
    command: [echo, hello]

    # stage: prepare
    # depends_on: [other-job, prepare]
    # executor: slurm
    # executor_options: ["--partition=gpu", "--gres=gpu:1"]
    # working_directory: ./work
    # environment: ["EPOCHS=20", "DATA_ROOT=./data"]

    # Same syntax as `rotari add --array`.
    # array: "1-10"
    # array: "1,3,4"

    # Cartesian expansion, with the same syntax as `rotari add --matrix`.
    # matrix: ["SEED=1,2,3", "MODEL=small,large"]
```

The template is directly importable because every optional example is
commented out. It covers dependencies and stages, executor options, working
directory, environment, arrays, and matrices, but does not attempt to
duplicate the complete schema documentation. It contains no `source`, status,
or attempt ID. `--template` is mutually exclusive with `--run-id` and
`--project-name`.

YAML is the recommended template format. TOML output carries equivalent concise
comments. JSON does not support comments, so `--format json --template` emits
only the minimal importable structure; the command prints no separate prose to
stdout because that would corrupt redirected JSON.

Repeated run IDs merge snapshots by source job ID:

```sh
rotari export -p project -r RUN_A -r RUN_B > experiment.yaml
```

- A job ID present in only one run is included.
- The same job ID in multiple runs is included once with its latest available
  attempt. "Latest" means the latest attempt completion or submission time,
  then the containing run finish time, then attempt number and run ID as
  deterministic tie-breakers.
- A job ID without any attempt is exported once as unfinished. Because it has
  no concrete attempt reference, import treats it as fresh work.
- Different job IDs are all included, even when their definitions are equal.
  Rotari does not infer identity from command content.
- Different job IDs with the same non-empty job name are an error because they
  would make the dependency namespace ambiguous. The user must rename or remove
  one after separate export; version 1 does not expose a conflict-resolution
  option on multi-run export.
- Stages and dependencies are validated after the union is built. Unknown
  dependencies, namespace collisions, or cycles reject the export.
- Duplicate run IDs are rejected. Version 1 requires all merged runs to belong
  to the selected project.
- Unnamed jobs are not guessed to be the same across runs. Each is exported as
  a distinct job and keeps its own attempt identity.

The manifest stores the ordered IDs in `source.run_ids`. A completed job's
attempt ID identifies the exact selected run and attempt. If the editable
manifest lacks an attempt ID, the job is treated as fresh work on import.

`status` is both an editing aid and an explicit import instruction. It makes
successful and failed jobs visible without another command. `attempt_id` can be
passed directly to inspection commands, but is provenance and should normally
remain unchanged. Import decodes and validates it against the source runs before
using it. Output locations are not repeated in the manifest.

An array or matrix group has no single attempt ID. Its job-level `status` is the
aggregate status. `instances` identifies non-success leaves with matrix values,
an optional array task, status, and attempt ID:

```yaml
- name: train
  command: [python, scripts/train.py]
  matrix: ["SEED=1,2", "MODEL=small,large"]
  array: "1-3"
  status: failed
  instances:
    - matrix: {SEED: "2", MODEL: large}
      task: 3
      status: failed
      attempt_id: att_20260925-120000-12345678-a1b2c3d4e-3-0
```

Successful leaves are omitted from `instances` and recovered authoritatively
from `source.run_ids` plus persisted matrix/array provenance. This keeps a large
mostly-successful group compact while showing exactly which leaves need review.
Editing an instance status to `success` manually accepts only that leaf.
Changing the job-level aggregate status to `success` accepts every listed
non-success instance. Missing or duplicate matrix/task coordinates are errors;
an attempt ID must resolve to the same coordinate.

Named exported jobs must remain unique after expansion. Unnamed jobs are valid,
but cannot be dependency targets or deduplicated across source runs. Export
never changes the source run or current queue.

### `import`

```text
rotari import FILE --project-name NAME [--overwrite] [--dry-run] [--json]
```

This command strictly parses, expands, and validates the manifest, reconciles it
with its optional `source`, and materializes the result as the current queue.
Human output lists each expanded job as `execute` or `reuse`, and lists removed
source jobs separately. `--json` emits the same versioned import plan for
tooling. `--dry-run` performs every read and validation but writes nothing.

Compilation and queue replacement must finish atomically while holding the
project state lock. A running project rejects the operation. A non-empty queue
requires `--overwrite`; an empty post-run queue supports the primary workflow
without that flag. There is no append mode or interactive overwrite in version
1.

### `run`

After import, the existing command runs the queue:

```sh
rotari run --project-name project
```

Retry count, concurrency, executor defaults, async mode, and run name remain
ordinary `run` options or configuration. Import does not start a run. There is
no separate `workflow run` execution path.

## Reconciliation with previous runs

When `source` is present, import loads the listed command snapshots and results.
Reconciliation decodes `attempt_id` to identify the exact source run, job, and
attempt. Array task attempts identify their concrete task job IDs in the same
way. If no attempt ID exists, the job is fresh work. The canonical definition
includes command arguments, environment, working directory, effective executor
and options, array task, stage, and dependencies.

Before any queue write, import verifies that every attempt ID is well formed,
its decoded run is either listed in `source.run_ids` or reachable through
`Origin`/`TaskOrigins` from a listed snapshot, its decoded job ID exists in that
source snapshot either directly or as a valid expanded array task, and its
attempt directory and recorded job exist. A malformed, unreachable, or missing
attempt or job is an error, never a silent fallback. The selected source
definition is then compared with the manifest: an exact match permits reuse or
manual acceptance, while a definition change executes the manifest job as new
work.

Users should not edit `attempt_id`. However, rotari cannot distinguish an
accidental edit from an intentional selection when another real attempt has the
same job ID. Nor can it always detect a change to another existing job whose
canonical definition is identical. Such changes are accepted as implementation
limitations. They may select an older or unintended result, so import output
must report the decoded source run, job, and attempt before writing the queue.
This is not presented as a supported attempt-selection workflow.

- An identical source job with a successful result is `reuse`.
- A failed, cancelled, or unfinished source job is `execute` unless its
  manifest status is changed to `success`.
- A new job or a job whose execution definition changed is `execute`.
- Changing `success` to `failed`, `cancelled`, or `unfinished` forces execution.
- Dependents of an executing job are also `execute`; this closure prevents a
  stale downstream result from being reused.
- Source jobs absent from the manifest are `remove` and do not appear in the
  new run.
- Ambiguous names, missing dependencies, cycles, and incompatible expansion
  shapes fail planning before state is changed.

Import assigns fresh IDs to new or changed jobs. Unchanged jobs retain the
source job ID when safe and receive existing `Origin`/`TaskOrigins` metadata.
The imported queue also records an explicit carry-forward disposition for
those jobs. The ordinary run planner uses that disposition to carry successful
results, execute failed or unfinished work, and handle arrays per task. This
must be distinct from origin metadata alone: `copy` also records origins, but a
normal run after `copy` intentionally executes the copied jobs. Names remain
the public manifest identity and import output reports all assigned IDs.
Completed source runs remain immutable.

This is definition comparison, not file freshness detection. Editing a script
without changing its manifest entry does not invalidate a successful job. The
user can deliberately force all work by deleting `source` before import. An
explicit force selector can be considered after the base behavior is proven.

### Manual acceptance

Changing an unchanged job from `failed`, `cancelled`, or `unfinished` to
`success` means: accept the selected source attempt as successful without
executing it again. It does not modify the immutable source run. The destination
run records a successful carried result marked as manually accepted, while its
origin preserves the source status, attempt ID, and original exit code. CLI and
Web projections should display this as `success (accepted)` and link to the
source output.

Manual acceptance is allowed only when the job definition is unchanged and the
attempt ID resolves to a matching source job. A changed command, environment,
executor, working directory, array shape, stage, or dependencies always
executes, even if its manifest status says `success`. A missing status defaults
to the authoritative source status; a hand-written job without a source always
executes.

Because dependency and run success currently use `JobResult.ExitCode`, an
accepted destination result uses exit code zero and sets
`JobResult.Accepted` to true. The source exit code remains in the immutable
origin result and is resolved through `Origin` when displayed. This keeps
acceptance visible and auditable without duplicating or rewriting the source
result.

## Inherited run state

Import uses the same provenance contract as `copy`. The manifest stores the
source project and ordered run references plus compact status instructions,
while import reloads authoritative state from those runs. Edited status and
attempt IDs are validated against that state before they influence planning;
path text is never accepted as execution state.

| State | Import behavior |
| --- | --- |
| Job and task status | Preserve the source state as origin metadata. Carry unchanged successes; execute failed, cancelled, and unfinished work unless manually accepted in the manifest. |
| Result | Carry the complete `JobResult` for ordinary reused work. Manual acceptance creates an accepted success result with source exit code and origin; executed work produces a new result. |
| Origin | Preserve source run ID, job ID, attempt ID, status, source CWD, and submitted/finished timestamps using `Origin` and `TaskOrigins`. |
| Output and logs | Do not copy bytes. Resolve them through the source run/job/attempt origin, as `copy`, `show`, and Web already do. |
| Array state | Preserve and reconcile status, result, attempt, and output origin per task. |
| Job definition | Export it as editable manifest data and compare its canonical form during import. |

The manifest contains only compact status and attempt annotations, not complete
results or a literal output path. Paths would become stale when a base directory
moves. Human and JSON import output may additionally display the currently
resolved output path as derived information, but it is not persisted as
manifest authority.

Live scheduler IDs, locks, process IDs, runtime scheduler status, Webhook state,
load samples, and source config snapshots are not inherited. A new run records
its own runtime and context. Source runs referenced by carried results must
remain available; deleting one has the same consequences for output lookup as
deleting a source run currently referenced by `copy` or a filtered retry.
Export follows existing `Origin`/`TaskOrigins` links, so a run that already
carried results can be exported and imported without flattening or losing its
ultimate source attempt.

## Architecture

Add an `internal/workflow` package with no filesystem or CLI dependency:

- schema types and strict format decoding
- expansion, reusing the model's CLI-compatible array parser
- canonical job definitions and comparison
- export projection and reconciliation plan generation
- conversion to `model.Queue`

### Matrix persistence

Matrix declarations must survive queue and run snapshots. Keep the existing
execution representation, where each matrix combination is an independent
`QueuedCommand` with its own job ID, but add optional matrix provenance to each
expanded command:

- one generated matrix group ID shared by every combination
- the original ordered dimensions and values
- the base job name and environment before matrix values were applied
- the concrete dimension values for this command

`add --matrix` and manifest import both populate this metadata. Queue copying,
run snapshots, and retry selection preserve it when the complete group is
retained. If `copy`, `remove`, or `change` selects or modifies only part of a
group, the destination clears matrix provenance from every affected member and
keeps the expanded commands as ordinary independent jobs. Export groups
commands only when all expected combinations are present and their shared base
definition and provenance agree; malformed persisted metadata is an error
instead of a reason to guess.

Legacy queues and runs have no matrix provenance. They remain readable and
executable, and export writes their already-expanded commands as independent
jobs. New writes do not retroactively infer a matrix from names or environment
entries.

Carry-forward remains leaf-based after compact export. Matrix provenance maps
each manifest combination back to its expanded command ID, and existing array
provenance maps each task to its task ID. Import rebuilds `Origin` and
`TaskOrigins` for successful leaves, executes non-success leaves, and applies
manual acceptance only to explicitly edited coordinates. Compacting the
definition never aggregates or discards carry state.

This approach deliberately does not move matrix expansion to runtime. Doing so
would change persisted job IDs, copy/retry selection, array composition, and
dependency behavior. Provenance preserves round trips without changing those
contracts.

Queue-level default executor and executor options are flattened into each
exported job's effective executor settings. The manifest has no defaults block,
so import does not need to infer whether repeated values originally came from a
queue default, configuration, or individual job.

Keep shared queue invariants in `internal/model`. The `cmd/rotari` adapter owns
file reading, path resolution, locks, source-run loading, output formatting,
and queue persistence. The existing `run` adapter and execution path are not
replaced. Its planner is extended only to consume the explicit carry-forward
disposition written by `import`; executor and orchestration behavior remains
unchanged.

## Delivery stages

- [x] **Schema and compiler:** implement strict YAML, TOML, and JSON decoding,
  matrix/array expansion, queue conversion, and pure unit tests in
  `internal/workflow`.
- [x] **Matrix provenance:** persist matrix group metadata from `add` and
  manifest import, preserve it through queue mutations and snapshots, and
  validate complete groups.
- [x] **Queue and run export:** add queue, run, and template `export`, stable
  manifest serialization, completion/schema metadata, identity validation, and
  round-trip fixtures. This stage is read-only.
- [x] **Fresh import:** add `import` and `--dry-run` for manifests without
  `source`. Atomically replace an empty queue and cover lock, overwrite, and
  validation behavior.
- [x] **Source-run reconciliation:** resolve exported provenance, compare
  canonical definitions, invalidate the downstream closure, preserve origins,
  add explicit carry-forward queue dispositions, and test import-plus-run
  equivalence with existing filtered retries and `copy` compatibility.
- [x] **CLI simplification:** keep `change` compatible initially, then decide
  whether to deprecate it based on whether any workflows still require
  imperative queue edits.

Each stage should land with the smallest package tests first, followed by
`go test ./cmd/rotari`, `go test ./...`, and `go vet ./...` when its behavior
crosses package boundaries.

## Acceptance criteria

- The same manifest in YAML, TOML, and JSON compiles to equivalent queues.
- New matrix queues and run snapshots export back to one equivalent compact
  matrix declaration; legacy expanded matrices remain separate jobs.
- Compact matrix/array export preserves carry-forward per combination and task,
  while listing only non-success instances in the manifest.
- `export` performs no writes and its default YAML can be imported unchanged.
- `export --template` produces a minimal valid manifest without reading or
  writing project state.
- Queue export omits source state and round-trips definitions as fresh work;
  run export includes source state for reconciliation.
- Exporting multiple runs collapses the same job ID to its latest attempt, adds
  different job IDs independently, and rejects duplicate non-empty job names.
- `import --dry-run` performs no writes, including on invalid input.
- Exporting and importing an unchanged successful run reuses every job.
- Changing one failed job executes it and its downstream dependents while
  reusing unrelated successful jobs.
- Changing an unchanged failed job's manifest status to `success` skips its
  execution, unblocks dependents, records `success (accepted)`, and preserves a
  link to the failed source attempt and exit code.
- A `success` edit cannot suppress execution after the job definition changes.
- Changed commands, environments, directories, executor settings, arrays,
  matrices, stages, and dependencies are visible in the plan.
- Invalid dependencies, duplicate expanded names, unsafe generated IDs, and
  unknown fields fail before queue replacement.
- The normal `run` command uses the existing snapshots, attempts, retry
  behavior, executor lanes, status resolution, and Web/CLI projections.
- Import carry-forward instructions do not change the behavior of queues built
  by `add`, `copy`, or `change`.
- A failed import leaves the existing queue and metadata unchanged.

## Decisions

1. Use top-level `export` and `import` commands, followed by the existing `run`,
  as the main retry-and-adjust flow.
2. Keep names optional and unique when present. Decode source run and job IDs
  from attempt IDs, validate them against persisted state, and use names for
  dependencies and logical cross-run matching.
3. Automatically execute the downstream closure of every changed or failed
   job.
4. Keep run policy such as retry count and concurrency in CLI/config for
   version 1 instead of embedding it in the manifest.
5. Put source project and ordered run IDs in exported manifests, while keeping base
  directory and result metadata out of the editable file.
6. Defer interpolation, includes, and import append behavior until the core
  export/edit/import/run workflow has usage evidence.
7. Reuse `copy` provenance and result semantics; derive output paths from origin
  IDs instead of storing literal paths in manifests.
8. Treat editable status as an import instruction. A failed source attempt may
  be manually accepted as success, with explicit audit metadata in the new
  result and no mutation of the source run.
9. Merge repeated `--run-id` snapshots by job ID. The same ID selects its
  latest attempt; different IDs are both included unless their non-empty names
  collide.
10. Do not provide semantic manifest-file merge in version 1. Unlike run
   snapshots, independently edited files have no authoritative identity or
   precedence that rotari can safely infer.
11. Generate the starter manifest with `export --template`; keep one active job
  directly importable and document commonly edited fields with concise YAML
  and TOML comments rather than embedding the full schema.
12. Treat attempt IDs as provenance that users normally do not edit. Reject
  malformed IDs and missing decoded jobs, while documenting that another real
  attempt of the same job cannot be distinguished from intentional selection.
13. Persist manual acceptance as `JobResult.Accepted: true` with destination
  exit code zero. Keep the original exit code only in the immutable result
  referenced by `Origin`.
