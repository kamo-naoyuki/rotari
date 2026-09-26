# Workflow manifests

Export a run as an editable manifest, fix or accept jobs, and import it as the next queue.

Export a saved run, edit its failed jobs, import the result, and run the new
queue:

```sh
rotari export -p sweep -r RUN_ID > experiment.yaml
# Edit commands, status, executor options, or dependencies.
rotari import -p sweep experiment.yaml
rotari run -p sweep
```

The copy target and output file can be passed positionally. The target is a
project or run ID; when both a project and a run must be named explicitly, use
`--project-name` for the project:

```sh
rotari export sweep experiment.yaml
rotari export RUN_ID experiment.yaml
rotari export --project-name sweep RUN_ID experiment.yaml
rotari import experiment.yaml sweep
```

An unchanged successful job carries its result and output reference forward.
Failed, cancelled, unfinished, new, and changed jobs execute. Their downstream
dependents execute as well. Changing an unchanged failed job's `status` to
`success` manually accepts that result; the new run displays
`success (accepted)` and retains a link to the original failed attempt.

Create a commented starter file without reading project state:

```sh
rotari export --template > experiment.yaml
```

A manifest uses the same compact syntax as the CLI:

```yaml
version: 1
jobs:
  - name: train
    command: [python, train.py]
    executor: slurm
    executor_options: ["--partition=gpu"]
    environment: ["EPOCHS=20"]
    array: "1-3"
    matrix: ["SEED=1,2", "MODEL=small,large"]
  - name: collect
    command: [python, collect.py]
    depends_on_finished: [train]
```

`depends_on` and `depends_on_finished` correspond to `add --depends-on` and
`--depends-on-finished`; `timeout`, `retry`, `retry_delay`, `retry_backoff`,
and `retry_max_delay` correspond to the `add` options of the same names.

Use `rotari import --dry-run FILE` to validate and preview `execute`, `reuse`,
and `accept` decisions without changing the queue. Jobs with provenance also
show the source attempt and status they refer to (per task for arrays), and
source jobs no longer described by the manifest are listed as `remove`. Each
job line ends with its command. `--json` reports the same plan and also
includes the source run and job IDs. Jobs that keep a source job ID show the
same ID in `--dry-run` and in the real import, but new or changed jobs receive
a fresh ID each time, so their `--dry-run` IDs are only provisional. A non-empty destination
queue requires `--overwrite`. Queue export omits previous-run status and imports
as fresh work. Run export includes source and attempt references for
reconciliation. Repeat `--run-id` to combine saved runs by job ID; distinct job
IDs are retained, while duplicate non-empty job names are rejected.

`attempt_id` is provenance and should normally remain unchanged. Import rejects
malformed, missing, or unreachable attempts. Status is intentionally editable.
Matrix and array manifests keep only non-success leaves under `instances`, while
successful leaves are recovered from the source runs.
