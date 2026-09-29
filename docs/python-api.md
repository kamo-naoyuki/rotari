# Python API

The client methods return `CommandResult` unless otherwise noted. Command
options are generated from the Rotari CLI schema.

<!-- BEGIN GENERATED CLI OPTIONS -->

## `Rotari.add`

```python
Rotari.add(command: Sequence[str], **options: object) -> CommandResult
```

Add a command to a queue.

| Option | Value | Description |
| --- | --- | --- |
| `executor` | `str` | job executor |
| `executor_options` | `Sequence[str]` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `output` | `Sequence[str]` | stdout destination; stderr also goes here unless --error is specified; may be repeated |
| `error` | `Sequence[str]` | stderr destination; defaults to --output destinations; may be repeated |
| `log_mode` | `str` | internal log mode |
| `open_mode` | `str` | external output file mode |
| `working_directory` | `str` | working directory for the job |
| `env` | `Sequence[str]` | environment variable for the job; may be repeated |
| `job_name` | `str` | job name label |
| `stage` | `str` | stage that contains the job |
| `depends_on` | `Sequence[str]` | name of a prerequisite job or stage; may be repeated |
| `depends_on_finished` | `Sequence[str]` | name of a prerequisite job or stage that must finish, whatever its result; may be repeated |
| `timeout` | `str` | stop the job this long after it starts, such as 90m or 2h; it then fails with exit code 124 |
| `retry` | `str` | retry the job up to N times when it fails, instead of the run's --retry; 0 disables retries |
| `retry_delay` | `str` | wait this long before the job's first retry, such as 30s; retries are immediate by default |
| `retry_backoff` | `str` | multiply the retry delay by this factor for each further retry, such as 2 |
| `retry_max_delay` | `str` | upper limit of the retry delay, such as 10m |
| `array` | `str` | create an array job range or selected tasks |
| `matrix` | `Sequence[str]` | expand a command into jobs from KEY=VALUE[,VALUE...] dimensions; may be repeated |
| `quiet` | `bool` | suppress success output |

## `Rotari.run`

```python
Rotari.run(**options: object) -> CommandResult
```

Execute queued commands, optionally selecting jobs from a run.

| Option | Value | Description |
| --- | --- | --- |
| `run_id` | `str` | repopulate the queue from this run before executing (copy --run-id + run); defaults to the latest run when a result filter is used |
| `overwrite` | `bool` | replace a non-empty queue without prompting; requires --run-id |
| `run_name` | `str` | run name label |
| `local_concurrency` | `str` | local worker concurrency |
| `batch_concurrency` | `str` | scheduler job concurrency (Slurm/PBS/...) |
| `retry` | `str` | retry failed jobs up to N times; explicit cancellations are not retried |
| `failed` | `bool` | only execute failed jobs; others carry forward their previous result |
| `unfinished` | `bool` | only execute unfinished jobs; others carry forward their previous result |
| `success` | `bool` | only execute successful jobs; others carry forward their previous result |
| `job_ids` | `Sequence[str]` | only execute this job; may be repeated; not with a result filter; others carry forward their previous result |
| `job_name` | `str` | only execute this job by name |
| `stage` | `str` | only execute jobs in this stage, narrowed by any result filter; others carry forward their previous result |
| `matrix` | `str` | only execute jobs of this matrix, named by its base job name, narrowed by any result filter; others carry forward their previous result |
| `partial_array` | `bool` | with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches |
| `async_` | `bool` | return after starting the run |
| `quiet` | `bool` | suppress progress and completion output |
| `executor` | `str` | execution executor override |
| `env` | `str` | caller environment propagation mode (default ALL) |
| `match_by` | `str` | job identity matching |
| `executor_options` | `Sequence[str]` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `ssh_concurrency` | `str` | SSH executor concurrency |
| `ssh_options` | `Sequence[str]` | SSH executor dispatch options; may be repeated |
| `slurm_concurrency` | `str` | Slurm executor concurrency |
| `slurm_options` | `Sequence[str]` | Slurm executor dispatch options; may be repeated |
| `slurm_submit_interval` | `str` | minimum Slurm submission interval |
| `slurm_submit_retry_limit` | `str` | maximum retries for transient Slurm submission failures |
| `pbs_concurrency` | `str` | PBS executor concurrency |
| `pbs_options` | `Sequence[str]` | PBS executor dispatch options; may be repeated |
| `pbs_submit_interval` | `str` | minimum PBS submission interval |
| `pbs_submit_retry_limit` | `str` | maximum retries for transient PBS submission failures |
| `lsf_concurrency` | `str` | LSF executor concurrency |
| `lsf_options` | `Sequence[str]` | LSF executor dispatch options; may be repeated |
| `lsf_submit_interval` | `str` | minimum LSF submission interval |
| `lsf_submit_retry_limit` | `str` | maximum retries for transient LSF submission failures |
| `filter_result` | `Sequence[str]` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `filter_exit_code` | `Sequence[str]` | select jobs with this exit code; may be repeated |
| `filter_failure_kind` | `Sequence[str]` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `filter_host` | `Sequence[str]` | select jobs run on a matching host; may be repeated |
| `filter_started_after` | `str` | select jobs started at or after this time |
| `filter_started_before` | `str` | select jobs started before this time |
| `filter_finished_after` | `str` | select jobs finished at or after this time |
| `filter_finished_before` | `str` | select jobs finished before this time |
| `filter_longer_than` | `str` | select jobs running at least this long |
| `filter_shorter_than` | `str` | select jobs running less than this long |
| `filter_command` | `str` | select jobs whose argv matches this Go regexp |
| `filter_stage` | `str` | select jobs in this stage; same as --stage |
| `filter_not_stage` | `Sequence[str]` | exclude jobs in this stage; may be repeated |
| `filter_matrix` | `str` | select jobs of this matrix, named by its base job name; same as --matrix |
| `filter_not_matrix` | `Sequence[str]` | exclude jobs of this matrix, named by its base job name; may be repeated |

## `Rotari.retry`

```python
Rotari.retry(**options: object) -> CommandResult
```

Run failed and unfinished jobs; with --job-id, run those jobs.

| Option | Value | Description |
| --- | --- | --- |
| `run_id` | `str` | repopulate the queue from this run before executing; defaults to the latest run |
| `overwrite` | `bool` | replace a non-empty queue without prompting; requires --run-id |
| `run_name` | `str` | run name label |
| `local_concurrency` | `str` | local worker concurrency |
| `batch_concurrency` | `str` | scheduler job concurrency (Slurm/PBS/...) |
| `retry` | `str` | retry failed jobs up to N times; explicit cancellations are not retried |
| `job_ids` | `Sequence[str]` | only execute this job instead of failed and unfinished jobs; may be repeated |
| `stage` | `str` | only retry jobs in this stage |
| `matrix` | `str` | only retry jobs of this matrix, named by its base job name |
| `async_` | `bool` | return after starting the run |
| `quiet` | `bool` | suppress progress and completion output |
| `executor` | `str` | execution executor override |
| `env` | `str` | caller environment propagation mode (default ALL) |
| `executor_options` | `Sequence[str]` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `ssh_concurrency` | `str` | SSH executor concurrency |
| `ssh_options` | `Sequence[str]` | SSH executor dispatch options; may be repeated |
| `slurm_concurrency` | `str` | Slurm executor concurrency |
| `slurm_options` | `Sequence[str]` | Slurm executor dispatch options; may be repeated |
| `slurm_submit_interval` | `str` | minimum Slurm submission interval |
| `slurm_submit_retry_limit` | `str` | maximum retries for transient Slurm submission failures |
| `pbs_concurrency` | `str` | PBS executor concurrency |
| `pbs_options` | `Sequence[str]` | PBS executor dispatch options; may be repeated |
| `pbs_submit_interval` | `str` | minimum PBS submission interval |
| `pbs_submit_retry_limit` | `str` | maximum retries for transient PBS submission failures |
| `lsf_concurrency` | `str` | LSF executor concurrency |
| `lsf_options` | `Sequence[str]` | LSF executor dispatch options; may be repeated |
| `lsf_submit_interval` | `str` | minimum LSF submission interval |
| `lsf_submit_retry_limit` | `str` | maximum retries for transient LSF submission failures |
| `filter_result` | `Sequence[str]` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `filter_exit_code` | `Sequence[str]` | select jobs with this exit code; may be repeated |
| `filter_failure_kind` | `Sequence[str]` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `filter_host` | `Sequence[str]` | select jobs run on a matching host; may be repeated |
| `filter_started_after` | `str` | select jobs started at or after this time |
| `filter_started_before` | `str` | select jobs started before this time |
| `filter_finished_after` | `str` | select jobs finished at or after this time |
| `filter_finished_before` | `str` | select jobs finished before this time |
| `filter_longer_than` | `str` | select jobs running at least this long |
| `filter_shorter_than` | `str` | select jobs running less than this long |
| `filter_command` | `str` | select jobs whose argv matches this Go regexp |
| `filter_stage` | `str` | select jobs in this stage; same as --stage |
| `filter_not_stage` | `Sequence[str]` | exclude jobs in this stage; may be repeated |
| `filter_matrix` | `str` | select jobs of this matrix, named by its base job name; same as --matrix |
| `filter_not_matrix` | `Sequence[str]` | exclude jobs of this matrix, named by its base job name; may be repeated |

## `Rotari.reset`

```python
Rotari.reset(*, recover: bool = False) -> CommandResult
```

Discard the current, not-yet-run queue.

| Option | Value | Description |
| --- | --- | --- |
| `recover` | `bool` | confirm an interrupted run has stopped without prompting |
| `quiet` | `bool` | suppress success output |

## `Rotari.wait`

```python
Rotari.wait(selector: str | None = None, **options: object) -> dict[str, object]
```

Wait for an asynchronous run by project, run name, or run id.

| Option | Value | Description |
| --- | --- | --- |
| `run_id` | `Sequence[str]` | run ID; may be repeated |
| `timeout` | `str` | maximum wait duration |
| `json` | `bool` | print each completed run as one JSON object |

## `Rotari.show`

```python
Rotari.show(**options: object) -> dict[str, object]
```

Show queue or run status.

| Option | Value | Description |
| --- | --- | --- |
| `masterdir` | `str` | master registry directory |
| `run_id` | `str` | run ID or latest |
| `queue` | `bool` | show the current queue even when a run is selected |
| `job_ids` | `str` | job ID |
| `job_name` | `str` | job name |
| `failed` | `bool` | show failed jobs; may be combined with the other result filters |
| `unfinished` | `bool` | show unfinished jobs; may be combined with the other result filters |
| `success` | `bool` | show successful jobs; may be combined with the other result filters |
| `stage` | `str` | show jobs in this stage only |
| `matrix` | `str` | show jobs of this matrix only, named by its base job name |
| `lineage` | `bool` | list the project's runs oldest first with result counts and changes since the previous run |
| `logs` | `bool` | print output logs for all jobs |
| `failed_logs` | `bool` | print output logs for failed jobs |
| `stream` | `str` | show both streams or select stdout/stderr |
| `follow` | `bool` | follow one selected log stream until the run completes |
| `no_pager` | `bool` | print logs directly instead of using a pager |
| `basedirs` | `bool` | list state directories known to the master registry |
| `json` | `bool` | print machine-readable JSON for a run |
| `report` | `bool` | print an AI-ready Markdown report |
| `filter_result` | `Sequence[str]` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `filter_exit_code` | `Sequence[str]` | select jobs with this exit code; may be repeated |
| `filter_failure_kind` | `Sequence[str]` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `filter_host` | `Sequence[str]` | select jobs run on a matching host; may be repeated |
| `filter_started_after` | `str` | select jobs started at or after this time |
| `filter_started_before` | `str` | select jobs started before this time |
| `filter_finished_after` | `str` | select jobs finished at or after this time |
| `filter_finished_before` | `str` | select jobs finished before this time |
| `filter_longer_than` | `str` | select jobs running at least this long |
| `filter_shorter_than` | `str` | select jobs running less than this long |
| `filter_command` | `str` | select jobs whose argv matches this Go regexp |
| `filter_stage` | `str` | select jobs in this stage; same as --stage |
| `filter_not_stage` | `Sequence[str]` | exclude jobs in this stage; may be repeated |
| `filter_matrix` | `str` | select jobs of this matrix, named by its base job name; same as --matrix |
| `filter_not_matrix` | `Sequence[str]` | exclude jobs of this matrix, named by its base job name; may be repeated |

<!-- END GENERATED CLI OPTIONS -->

::: rotari
    options:
      members:
        - CommandResult
        - RotariError
