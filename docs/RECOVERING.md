# Recovering failed runs

When a run ends with failed or unfinished jobs, start a new run that executes
only those jobs again. Successful jobs are not executed again; their results
and output carry forward to the new run.

Three different things are called "retry" in rotari:

| You want to | Use | See |
| --- | --- | --- |
| Rerun failed jobs after a run has finished | `rotari retry` | [Rerun failed jobs](#rerun-failed-jobs) |
| Fix a job's command or options, then rerun it | `copy`, `change`, then `retry` | [Edit jobs before rerunning](#edit-jobs-before-rerunning) |
| Retry a failing job automatically within the same run | `run --retry N`, `add --retry N` | [Automatic retries](RUNNING.md#automatic-retries) |

## Rerun failed jobs

```sh
rotari show -p sweep --failed-logs   # see why jobs failed
rotari retry -p sweep                # rerun failed and unfinished jobs
```

`retry` starts a new run in which failed and unfinished jobs execute again and
successful jobs carry forward their result. Carried-forward jobs are not
executed, but they appear on the new run with a link to their original
output, so the whole run can be inspected in one place.

`retry` starts from the latest run when the queue is empty. When the queue
holds jobs, such as ones edited with `change`, it uses those instead and
does not silently add the latest run's failures. The preview and run report
the latest run's failed or unfinished jobs that are not represented in the
queue, with a `copy --failed --unfinished --append` command to include them.

## Edit jobs before rerunning

`copy` puts a run's jobs back into the queue without running them, so that
`change` can edit them first:

```sh
rotari copy                                   # the latest run; copy -r RUN_ID for another
rotari change --job-name train --executor-option="-p gpu"
rotari change --job-name train ./train-v2.sh
rotari change --job-name train --status unfinished
rotari retry
```

An edited job keeps its previous result, even when its command, environment,
or working directory changes, so `retry` would not run an edited job that
succeeded before. Mark it with `change --status unfinished` to make it run
again.

`change` edits one job (`--job-id/-j`, `--job-name`) or many (`--stage`,
`--matrix`, `--all`); an array job is changed as a whole. `copy` keeps job IDs
(unless they collide with queued ones) and dependencies, can restore only some jobs with the same selectors as
`retry`, and asks before replacing a non-empty queue (`--append` adds,
`--overwrite` replaces). See the [CLI reference](CLI_REFERENCE.md) for every
option. In contrast, `retry --run-id RUN_ID` builds the new run directly from
that saved run's snapshot and leaves the next queue untouched; `--overwrite`
is not accepted by `run` or `retry`.

For a reviewable edit kept in a file, export the run as a
[workflow manifest](WORKFLOW_MANIFESTS.md), edit it, and import it.

## Choosing which jobs rerun

By default `retry` reruns failed and unfinished jobs. These options choose
something else. Jobs that depend on a rerun job run again too; every other job
carries forward its result, or stays unfinished if it has none.

| Option | Reruns |
| --- | --- |
| (none) | Failed and unfinished jobs. |
| `--failed` | Only jobs that finished with a non-zero exit code. |
| `--unfinished` | Only jobs without a completed result. |
| `--success` | Only jobs that finished with exit code zero. |
| `-j JOB_ID`, `--job-name NAME` | That job, whatever its result. |
| `--stage STAGE`, `--matrix NAME` | Failed and unfinished jobs in that stage or matrix. |
| `--filter-not-stage`, `--filter-not-matrix` | Failed and unfinished jobs outside that stage or matrix. |

```sh
rotari retry -p sweep --stage train
rotari retry -p sweep --filter-not-stage report
rotari retry -p sweep -j JOB_ID
rotari retry -p sweep --dry-run      # list the jobs it would execute
```

The result options (`--failed`, `--unfinished`, `--success`) may be combined;
`--stage`, `--matrix`, and the other `--filter-*` options narrow them. A job
named with `-j` or `--job-name` cannot be combined with any of them. `--help`
lists every filter under "Filters", and
[contracts/06-selectors.md](../contracts/06-selectors.md#filters) gives the
full rules.

For an array job, only the matching tasks rerun; `--partial-array=false`
reruns every task of an array when any task matches.

`-r RUN_ID` starts from a saved run instead of the latest one. It uses that
run's snapshot directly and leaves the next queue untouched. The same applies
to a result selection on an empty queue.

`run` takes the same options. `retry` is `run --failed --unfinished` when no
other selection is given; `run` without one executes the whole queue.
`run --failed` on an empty queue builds its snapshot from the latest run, as
`retry` does, without writing that snapshot into the next queue.

To guard a rerun against concurrent edits, see
[Previewing and guarding changes](RUNNING.md#previewing-and-guarding-changes).

## Matching a new queue to an earlier run

If you add the jobs again instead of copying them, they get new job IDs.
`--match-by fingerprint` finds the earlier result of each job by its command,
saved inputs, and array or matrix parameters:

```sh
rotari add -p sweep ...              # the same commands as the earlier run
rotari run -p sweep --failed --unfinished --match-by fingerprint
```

The default `--match-by id-and-fingerprint` matches by job ID first and by
fingerprint for jobs that remain unmatched; `--match-by job-id` uses job IDs
only. Fingerprints are recalculated for each comparison, not stored.

## How results carry forward

Each job in the queue that came from an earlier run records an `Origin`: the
source run, job, and (when available) attempt. `copy` records it when it
copies a job; `--match-by` creates it for a job it matches. When the run
starts, the selection looks up each job's result through its `Origin`:
selected jobs execute, other finished jobs carry their result forward, and
jobs without a result stay unfinished.

```mermaid
flowchart LR
  subgraph SavedRun["Saved run RUN_ID"]
    SourceA["A · job-a"]
    SourceB["B · job-b"]
    SourceC["C · job-c"]
  end

  CopyCmd["rotari copy RUN_ID"]

  subgraph CurrentQueue["Copied queue"]
    CopiedA["A · job-a*"]
    OriginA["origin<br/>RUN_ID/job-a<br/>attempt-a"]
    StatusA["success"]
    CopiedB["B · job-b*"]
    OriginB["origin<br/>RUN_ID/job-b<br/>attempt-b"]
    StatusB["failed"]
    CopiedC["C · job-c*"]
    OriginC["origin<br/>RUN_ID/job-c"]
    StatusC["unfinished"]
    CopiedA --> OriginA --> StatusA
    CopiedB --> OriginB --> StatusB
    CopiedC --> OriginC --> StatusC
  end

  SourceA --> CopyCmd
  SourceB --> CopyCmd
  SourceC --> CopyCmd
  CopyCmd --> CopiedA
  CopyCmd --> CopiedB
  CopyCmd --> CopiedC

  subgraph NewRun["New run"]
    RunCmd["rotari run --failed --unfinished"]
    Filter["filter"]
    RunCmd --> Filter
    Filter -->|"selected"| Execute["execute"]
    Filter -->|"done, not selected"| Carry["carry result"]
    Filter -->|"no result"| Unfinished["unfinished"]
  end
  StatusA --> RunCmd
  StatusB --> RunCmd
  StatusC --> RunCmd

  classDef source fill:#fffbeb,stroke:#d97706,color:#78350f
  classDef queue fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef origin fill:#ccfbf1,stroke:#0f766e,color:#134e4a
  classDef filter fill:#fef3c7,stroke:#d97706,color:#78350f
  classDef execute fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef carried fill:#dcfce7,stroke:#16a34a,color:#14532d
  classDef unfinished fill:#fffbeb,stroke:#d97706,color:#78350f
  classDef command fill:#e0e7ff,stroke:#4f46e5,color:#1e1b4b
  classDef success fill:#dcfce7,stroke:#16a34a,color:#14532d
  classDef failed fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
  class SourceA,SourceB,SourceC source
  class CopiedA,CopiedB,CopiedC queue
  class OriginA,OriginB,OriginC origin
  class StatusA success
  class StatusB failed
  class StatusC unfinished
  class CopyCmd,RunCmd command
  class Filter filter
  class Execute execute
  class Carry carried
  class Unfinished unfinished
```
