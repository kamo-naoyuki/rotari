# Workflow manifests

Export a run as an editable manifest, fix or accept jobs, and import it as the next queue.

Export a saved run, edit its failed jobs, import the result, and retry failed
and unfinished jobs while carrying other completed results forward:

```sh
rotari export -p sweep -r RUN_ID > experiment.yaml
# Edit commands, status, executor options, or dependencies.
rotari import -p sweep experiment.yaml
rotari retry -p sweep
```

`retry` selects failed and unfinished imported jobs by default. In contrast,
`run` without a result selection executes every job in the queue, including
jobs whose successful results were exported in the manifest.

The copy target and output file can be passed positionally. The target is a
project or run ID; when both a project and a run must be named explicitly, use
`--project-name` for the project:

```sh
rotari export sweep experiment.yaml
rotari export RUN_ID experiment.yaml
rotari export --project-name sweep RUN_ID experiment.yaml
rotari import experiment.yaml sweep
```

A project exports its queue when it has queued jobs, and otherwise its latest
run, which `export` names on stderr. A run that is still running or was
interrupted cannot be exported: wait for it with `rotari wait`, or recover it
with `rotari unlock`, first.

The selected results determine what executes, not whether a job definition
changed. With the default `retry` selection, failed and unfinished jobs execute,
along with their downstream dependents; other completed jobs carry their result
and output reference forward. A changed job that previously succeeded is still
successful for selection purposes, so mark its `status` as `unfinished` if it
should execute again. Changing an unchanged failed job's `status` to `success`
manually accepts that result; the new run displays `success (accepted)` and
retains a link to the original failed attempt.

Create a commented starter file without reading project state:

```sh
rotari export --template > experiment.yaml
```

A manifest uses the same job settings as the CLI. In YAML, `env` matches
`add --env`; its variables use a name-to-value mapping, as in GitHub Actions:

```yaml
version: 1
jobs:
  - name: train
    command: [python, train.py]
    executor: slurm
    executor_options:
      --partition: gpu
    env:
      EPOCHS: "20"
    array: [1, 2, 3]
    matrix:
      SEED: [1, 2]
      MODEL: [small, large]
    matrix_exclude:
      - SEED: 2
        MODEL: large
  - name: collect
    command: [python, collect.py]
    depends_on_finished: [train]
```

## Job options and values

- `depends_on` and `depends_on_finished` map to `add --depends-on` and
  `add --depends-on-finished`.
- `timeout`, `retry`, `retry_delay`, `retry_backoff`, and `retry_max_delay` map
  to the `add` options with the same names.
- `env` accepts a name-to-value mapping or a `KEY=VALUE` sequence, for example
  `env: ["EPOCHS=20", "DATA_ROOT=./data"]`. YAML `environment` remains an
  accepted alias. YAML export uses a mapping when variable names are unique;
  JSON and TOML use the `environment` sequence field.
- `array` accepts a range string, such as `array: "1-10"`, or a task list, such
  as `array: [1, 2, 4]`. JSON and TOML use the string form.
- `executor_options` accepts a string sequence or an option-to-value mapping.
  For example, `--partition: gpu` becomes `--partition=gpu`. YAML export keeps
  this field as a sequence; JSON and TOML use the `executor_options` sequence
  field as well.

## Matrix and exclusions

Each YAML `matrix` key defines a dimension, and its sequence lists the values
for that dimension. The compact form `matrix: ["SEED=1,2,3"]` is also accepted.

`matrix_exclude` omits combinations matching every assignment in an entry.
Entries may use only declared dimensions and values. A partial entry excludes
all combinations matching its assignments; for example, `- SEED: 2` excludes
every combination where `SEED` is `2`. Entries must be non-empty and unique,
and all rules together must leave at least one combination. JSON and TOML also
support this field. Queue and run exports preserve the exclusions, and import
expands only the combinations that remain.

## Artifacts

`artifacts` lists the files and directories declared by a job, like
`rotari add --artifact`; for example, `artifacts: [results/, "out/$SEED.csv"]`.
Each attempt records these as artifact candidates, expanding `$SEED`-style job
variables, `$ROTARI_ARRAY_TASK_ID`, and `$ROTARI_JOB_DIR`. Artifact declarations
do not affect whether a job counts as changed when matching runs.

## Import preview and exports

Run `rotari import --dry-run FILE` to validate a manifest and preview `execute`,
`reuse`, and `accept` decisions without changing the queue. The preview also
shows the source attempt and status for jobs with provenance (per task for
arrays), and lists source jobs missing from the manifest as `remove`. Each job
line ends with its command. `--json` reports the same plan and includes source
run and job IDs.

IDs in the preview have two cases:

- A job retaining its source job ID has the same ID in the preview and actual
  import.
- New or changed jobs receive a fresh ID on each import, so their preview IDs
  are provisional.

A non-empty destination queue requires `--overwrite`. Queue export omits
previous-run status, so its jobs import as fresh work. Run export includes
source and attempt references for reconciliation. Repeat `--run-id` to combine
saved runs by job ID: distinct IDs are retained, while duplicate non-empty job
names are rejected.

## Provenance and status editing

`attempt_id` is provenance and should normally remain unchanged. Import rejects
malformed, missing, or unreachable attempts. Status, by contrast, is
intentionally editable.

Matrix and array manifests store only non-success leaves under `instances`;
successful leaves are recovered from the source runs. A matrix or array job
does not have a `status` of its own. Edit the status of each combination or task
in its `instances` entry—for example, set it to `success` to accept that
failure. A leaf omitted from `instances` keeps its source result, even if all
entries are removed. Widening an array adds new tasks, which run as new work.

Import rejects `status` on a matrix or array job, except for the aggregate value
exported by older versions. To execute a whole matrix again after importing,
run `rotari run --matrix NAME`.
