# CLI reference

The command reference below is generated from `rotari schema --json`, the same
metadata used by command help and shell completion. Environment variables are
documented separately in [Environment variables](ENVIRONMENT_VARIABLES.md).

For options available from the CLI, environment, and configuration, values are
resolved in this order: explicit CLI value, environment variable, configuration
values, then built-in default. Ordinary files merge global → cwd workspace →
basedir → project; command sections then override root values. `--config FILE`
replaces automatic discovery with that file. See
[Configuration](CONFIGURATION.md#configuration-files) for location constraints,
`init`, source inspection, and file-only run snapshots.

## Short options

These short forms are aliases for their long options. Each is available only
for commands that support the corresponding long option; `-h` displays help.

| Short option | Long option | Purpose |
| --- | --- | --- |
| `-b DIR` | `--basedir DIR` | State directory |
| `-p NAME` | `--project-name NAME` | Project name |
| `-r ID` | `--run-id ID` | Run selector |
| `-j ID` | `--job-id ID` | Job selector; repeatable where supported |
| `-e EXECUTOR` | `--executor EXECUTOR` | Select or override the executor |
| `-h` | `--help` | Display help |

<!-- BEGIN GENERATED CLI REFERENCE -->

## Commands

### `rotari info`

show the current Rotari context and active run state

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `--json` | `` | `ROTARI_INFO_JSON` | print machine-readable JSON |

### `rotari init`

write cwd workspace location defaults without creating state

Usage: `rotari init [BASEDIR [PROJECT]]`

### `rotari config`

generate a config file template

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--list` | `` | `ROTARI_CONFIG_LIST` | list existing config files |
| `--notifications` | `` | `CLI only` | generate notifications.toml instead of command defaults |
| `--format` | `FORMAT` | `ROTARI_CONFIG_FORMAT` | config format: yaml, toml, or json (choices: yaml, toml, json) |
| `--output` | `FILE` | `ROTARI_CONFIG_OUTPUT` | output config file path |

### `rotari check`

check whether a project is ready to run

Usage: `rotari check [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--json` | `` | `ROTARI_CHECK_JSON` | print machine-readable JSON |
| `--deep` | `` | `ROTARI_CHECK_DEEP` | check local executables and working directories |
| `--quiet` | `` | `ROTARI_CHECK_QUIET` | suppress success output |

### `rotari reset`

discard the current, not-yet-run queue

Usage: `rotari reset [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--quiet` | `` | `ROTARI_RESET_QUIET` | suppress success output |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari cancel`

cancel the active run or running jobs

Usage: `rotari cancel [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | cancel a running job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | cancel the unfinished jobs with this name; may be repeated |
| `--stage` | `STAGE` | `ROTARI_CANCEL_STAGE` | cancel the unfinished jobs of this stage |
| `--matrix` | `NAME` | `ROTARI_CANCEL_MATRIX` | cancel the unfinished jobs of this matrix |
| `--wait` | `` | `ROTARI_CANCEL_WAIT` | wait until the cancel took effect: the whole run has stopped or, with a job selection, each selected job has stopped |
| `--yes` | `` | `CLI only` | cancel the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `CLI only` | select jobs in this state; may be repeated (choices: running, pending) |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari suspend`

suspend running jobs

Usage: `rotari suspend [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | suspend a running job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | suspend the running jobs with this name; may be repeated |
| `--stage` | `STAGE` | `ROTARI_SUSPEND_STAGE` | suspend the running jobs of this stage |
| `--matrix` | `NAME` | `ROTARI_SUSPEND_MATRIX` | suspend the running jobs of this matrix |
| `--yes` | `` | `CLI only` | suspend the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `CLI only` | select jobs in this state; may be repeated (choices: running) |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari resume`

resume suspended jobs

Usage: `rotari resume [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | resume a suspended job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | resume the suspended jobs with this name; may be repeated |
| `--stage` | `STAGE` | `ROTARI_RESUME_STAGE` | resume the suspended jobs of this stage |
| `--matrix` | `NAME` | `ROTARI_RESUME_MATRIX` | resume the suspended jobs of this matrix |
| `--yes` | `` | `CLI only` | resume the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `CLI only` | select jobs in this state; may be repeated (choices: running) |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari delete`

delete saved run history

Usage: `rotari delete [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run to delete |
| `--all` | `` | `CLI only` | delete every run of the project |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari gc`

find and remove orphan run registry entries

Usage: `rotari gc [MASTERDIR]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `--dry-run` | `` | `CLI only` | list the orphan entries without removing them |

### `rotari unlock`

remove a confirmed stale run lock

Usage: `rotari unlock [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | verify the run ID recorded in the stale lock |

### `rotari change`

change queued jobs, or with --run-id the jobs of that run, which replace the queue first; a command after the options replaces the jobs' command

Usage: `rotari change <command ...>`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | first replace the queue with this run's jobs, then change them |
| `-j` / `--job-id` | `ID` | `ROTARI_JOB_ID` | target job ID |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | target job name |
| `--stage` | `STAGE` | `ROTARI_CHANGE_STAGE` | change every job in a stage |
| `--matrix` | `NAME` | `ROTARI_CHANGE_MATRIX` | change every job of a matrix, named by its base job name |
| `--all` | `` | `CLI only` | change every job |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | replace job executor (choices: local, lsf, pbs, sge, slurm, ssh) |
| `--executor-option` | `OPTION` | `ROTARI_EXECUTOR_OPTIONS` | replace executor options |
| `--clear-executor-options` | `` | `ROTARI_CHANGE_CLEAR_EXECUTOR_OPTIONS` | clear executor options |
| `--working-directory` | `DIR` | `ROTARI_CHANGE_WORKING_DIRECTORY` | working directory for the job |
| `--clear-working-directory` | `` | `ROTARI_CHANGE_CLEAR_WORKING_DIRECTORY` | clear the job working directory |
| `--env` | `KEY=VALUE (repeatable)` | `ROTARI_CHANGE_ENV` | replace job environment variables; may be repeated |
| `--clear-env` | `` | `ROTARI_CHANGE_CLEAR_ENV` | clear job environment variables |
| `--set-job-name` | `NAME` | `ROTARI_CHANGE_SET_JOB_NAME` | replace job name |
| `--depends-on` | `NAME (repeatable)` | `ROTARI_CHANGE_DEPENDS_ON` | replace prerequisites; may be repeated |
| `--clear-depends-on` | `` | `ROTARI_CHANGE_CLEAR_DEPENDS_ON` | clear prerequisites |
| `--depends-on-finished` | `NAME (repeatable)` | `ROTARI_CHANGE_DEPENDS_ON_FINISHED` | replace prerequisites that only need to finish, whatever their result; may be repeated |
| `--clear-depends-on-finished` | `` | `ROTARI_CHANGE_CLEAR_DEPENDS_ON_FINISHED` | clear prerequisites that only need to finish |
| `--artifact` | `PATH (repeatable)` | `CLI only` | replace the declared artifact paths (see add --artifact); may be repeated |
| `--clear-artifacts` | `` | `ROTARI_CHANGE_CLEAR_ARTIFACTS` | clear the declared artifact paths |
| `--timeout` | `DURATION` | `CLI only` | replace the job timeout, such as 90m or 2h |
| `--clear-timeout` | `` | `ROTARI_CHANGE_CLEAR_TIMEOUT` | remove the job timeout |
| `--retry` | `N` | `CLI only` | replace the job's retry limit; 0 disables retries |
| `--clear-retry` | `` | `ROTARI_CHANGE_CLEAR_RETRY` | use the run's --retry limit for the job again and remove its retry delay settings |
| `--retry-delay` | `DURATION` | `ROTARI_CHANGE_RETRY_DELAY` | replace the wait before the job's first retry, such as 30s |
| `--retry-backoff` | `FACTOR` | `ROTARI_CHANGE_RETRY_BACKOFF` | replace the factor applied to the retry delay for each further retry |
| `--retry-max-delay` | `DURATION` | `ROTARI_CHANGE_RETRY_MAX_DELAY` | replace the upper limit of the retry delay |
| `--status` | `STATUS` | `CLI only` | mark the job with a status that result filters of the next run read in place of its recorded result (choices: success, failed, cancelled, unfinished) |
| `--clear-status` | `` | `ROTARI_CHANGE_CLEAR_STATUS` | remove the job's status mark |
| `--quiet` | `` | `ROTARI_CHANGE_QUIET` | suppress success output |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari export`

export the current queue or saved runs as a workflow manifest

Usage: `rotari export [TARGET] [FILE]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID (repeatable)` | `ROTARI_RUN_ID` | run ID to export; may be repeated |
| `--format` | `FORMAT` | `ROTARI_EXPORT_FORMAT` | manifest format: yaml, toml, or json (choices: yaml, toml, json) |
| `--template` | `` | `ROTARI_EXPORT_TEMPLATE` | print a starter workflow manifest |
| `--output` | `FILE` | `CLI only` | write the workflow manifest to a file |

### `rotari import`

validate and replace a queue from a workflow manifest

Usage: `rotari import FILE [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--overwrite` | `` | `ROTARI_IMPORT_OVERWRITE` | replace a non-empty queue |
| `--json` | `` | `ROTARI_IMPORT_JSON` | print the import plan as JSON |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari remove`

remove queued jobs, or with --run-id jobs of that run, which replace the queue first

Usage: `rotari remove [JOB_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | first replace the queue with this run's jobs, then remove from them |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | remove a job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | remove a job by name |
| `--stage` | `STAGE` | `ROTARI_REMOVE_STAGE` | remove every job in a stage |
| `--matrix` | `NAME` | `ROTARI_REMOVE_MATRIX` | remove every job of a matrix, named by its base job name |
| `--all` | `` | `CLI only` | remove every job |
| `--quiet` | `` | `ROTARI_REMOVE_QUIET` | suppress success output |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari basedirs`

list state directories known to the master registry

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |

### `rotari projects`

list projects across known state directories

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | limit the listing to this state directory |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |

### `rotari runs`

list active runs and recently finished runs across known state directories

Usage: `rotari runs [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `--since` | `DURATION` | `ROTARI_RUNS_SINCE` | include finished runs in this time window (default 1d), such as 24h or 7d; active or interrupted runs are always included |
| `--json` | `` | `ROTARI_RUNS_JSON` | print machine-readable JSON |

### `rotari show`

show details for a project, run, job, or attempt

Usage: `rotari show [SELECTOR]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run ID or latest |
| `--queue` | `` | `ROTARI_SHOW_QUEUE` | show the current queue even when a run is selected |
| `-j` / `--job-id` | `ID` | `ROTARI_JOB_ID` | job ID |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | job name |
| `--failed` | `` | `ROTARI_SHOW_FAILED` | show failed jobs; may be combined with the other result filters |
| `--unfinished` | `` | `ROTARI_SHOW_UNFINISHED` | show unfinished jobs; may be combined with the other result filters |
| `--success` | `` | `ROTARI_SHOW_SUCCESS` | show successful jobs; may be combined with the other result filters |
| `--stage` | `NAME` | `ROTARI_SHOW_STAGE` | show jobs in this stage only |
| `--matrix` | `NAME` | `ROTARI_SHOW_MATRIX` | show jobs of this matrix only, named by its base job name |
| `--logs` | `` | `ROTARI_SHOW_LOGS` | print output logs for all jobs |
| `--failed-logs` | `` | `ROTARI_SHOW_FAILED_LOGS` | print output logs for failed jobs |
| `--stream` | `STREAM` | `ROTARI_SHOW_STREAM` | show both streams or select stdout/stderr |
| `--tail` | `N` | `ROTARI_SHOW_TAIL` | print only the last N lines of each log, with --logs, --failed-logs, or a job's output |
| `--follow` | `` | `ROTARI_SHOW_FOLLOW` | follow one selected log stream until the run completes |
| `--no-pager` | `` | `ROTARI_SHOW_NO_PAGER` | print logs directly instead of using a pager |
| `--json` | `` | `ROTARI_SHOW_JSON` | print machine-readable JSON for a run |
| `--report` | `` | `ROTARI_SHOW_REPORT` | print an AI-ready Markdown report |
| `--artifacts` | `` | `ROTARI_SHOW_ARTIFACTS` | list every artifact candidate of one job attempt instead of its logs |
| `--filter-result` | `RESULT (repeatable)` | `CLI only` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms (choices: failed, unfinished, success) |
| `--filter-exit-code` | `N (repeatable)` | `CLI only` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `CLI only` | select jobs of this failure kind; may be repeated (choices: timeout, cancelled, blocked, oom, signal, error) |
| `--filter-diagnosis` | `VALUE (repeatable)` | `CLI only` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `CLI only` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `CLI only` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `CLI only` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `CLI only` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari lineage`

list runs, summarize one run, or compare two runs

Usage: `rotari lineage [RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--json` | `` | `ROTARI_LINEAGE_JSON` | print the lineage, summary, or comparison as JSON |

### `rotari jobs`

list running and recently finished jobs across known state directories

Usage: `rotari jobs [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `--format` | `FORMAT` | `ROTARI_JOBS_FORMAT` | output fields; use %s %b %p %a %n %c %t %f %e (%f is finished time) |
| `--since` | `DURATION` | `ROTARI_JOBS_SINCE` | include jobs finished within this duration (default 1d), such as 24h or 7d; use 0 for running jobs only |
| `--json` | `` | `ROTARI_JOBS_JSON` | print machine-readable JSON; not with --format |

### `rotari wait`

wait for a run by project, run name, or run ID

Usage: `rotari wait [PROJECT_OR_RUN_NAME_OR_RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--disconnect-action` | `ACTION` | `ROTARI_DISCONNECT_ACTION` | when the wait client's input closes, detach or cancel selected runs (default detach) (choices: detach, cancel) |
| `-r` / `--run-id` | `ID (repeatable)` | `ROTARI_RUN_ID` | run ID; may be repeated |
| `--timeout` | `DURATION` | `ROTARI_WAIT_TIMEOUT` | maximum wait duration |
| `--until-failure` | `` | `ROTARI_WAIT_UNTIL_FAILURE` | return as soon as a job of the run has failed with no retry left, without waiting for the rest |
| `--json` | `` | `ROTARI_WAIT_JSON` | print each completed run as one JSON object |
| `--quiet` | `` | `ROTARI_WAIT_QUIET` | suppress normal progress and completion output; keep failure diagnostics and JSON results |
| `--all` | `` | `ROTARI_WAIT_ALL` | without a selector, wait for every active run in the basedir, not only those started from this shell |

### `rotari add`

add a command to a queue

Usage: `rotari add <command ...>`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | job executor (choices: local, lsf, pbs, sge, slurm, ssh) |
| `--executor-option` | `OPTION (repeatable)` | `ROTARI_EXECUTOR_OPTIONS` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `--output` | `FILE (repeatable)` | `CLI only` | stdout destination; stderr also goes here unless --error is specified; may be repeated |
| `--error` | `FILE (repeatable)` | `CLI only` | stderr destination; defaults to --output destinations; may be repeated |
| `--artifact` | `PATH (repeatable)` | `CLI only` | a file or directory the job writes or reads, recorded as an artifact candidate of each attempt; $ROTARI_ARRAY_TASK_ID, $ROTARI_JOB_DIR, and the job's --env and matrix variables expand; may be repeated |
| `--log-mode` | `MODE` | `CLI only` | internal log mode (choices: merge, separate) |
| `--open-mode` | `MODE` | `CLI only` | external output file mode (choices: append, truncate) |
| `--working-directory` | `DIR` | `ROTARI_ADD_WORKING_DIRECTORY` | working directory for the job |
| `--env` | `KEY=VALUE (repeatable)` | `ROTARI_ADD_ENV` | environment variable for the job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | job name label |
| `--stage` | `NAME` | `ROTARI_ADD_STAGE` | stage that contains the job |
| `--depends-on` | `NAME (repeatable)` | `ROTARI_ADD_DEPENDS_ON` | name of a prerequisite job or stage; may be repeated |
| `--depends-on-finished` | `NAME (repeatable)` | `ROTARI_ADD_DEPENDS_ON_FINISHED` | name of a prerequisite job or stage that must finish, whatever its result; may be repeated |
| `--timeout` | `DURATION` | `CLI only` | stop the job this long after it starts, such as 90m or 2h; it then fails with exit code 124 |
| `--retry` | `N` | `CLI only` | retry the job up to N times when it fails, instead of the run's --retry; 0 disables retries |
| `--retry-delay` | `DURATION` | `ROTARI_ADD_RETRY_DELAY` | wait this long before the job's first retry, such as 30s; retries are immediate by default |
| `--retry-backoff` | `FACTOR` | `ROTARI_ADD_RETRY_BACKOFF` | multiply the retry delay by this factor for each further retry, such as 2 |
| `--retry-max-delay` | `DURATION` | `ROTARI_ADD_RETRY_MAX_DELAY` | upper limit of the retry delay, such as 10m |
| `--array` | `FIRST-LAST\|TASK[,TASK...]` | `ROTARI_ARRAY_RANGE` | create an array job range or selected tasks |
| `--matrix` | `KEY=VALUE[,VALUE...] (repeatable)` | `ROTARI_ADD_MATRIX` | expand a command into jobs from KEY=VALUE[,VALUE...] dimensions; may be repeated |
| `--matrix-exclude` | `KEY=VALUE[,KEY=VALUE...] (repeatable)` | `CLI only` | exclude matrix combinations matching KEY=VALUE[,KEY=VALUE...]; requires --matrix and may be repeated |
| `--quiet` | `` | `ROTARI_ADD_QUIET` | suppress success output |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari copy`

copy the latest run's jobs into the queue

Usage: `rotari copy [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | source run ID; defaults to the latest run |
| `--failed` | `` | `ROTARI_COPY_FAILED` | include failed jobs; may be combined with result filters |
| `--unfinished` | `` | `ROTARI_COPY_UNFINISHED` | include unfinished jobs; may be combined with result filters |
| `--success` | `` | `ROTARI_COPY_SUCCESS` | include successful jobs; may be combined with result filters |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | copy a job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | copy a job by name |
| `--stage` | `STAGE` | `ROTARI_COPY_STAGE` | only copy jobs in this stage, narrowed by any result filter |
| `--matrix` | `NAME` | `ROTARI_COPY_MATRIX` | only copy jobs of this matrix, named by its base job name, narrowed by any result filter |
| `--append` | `` | `ROTARI_COPY_APPEND` | append to a non-empty queue |
| `--overwrite` | `` | `ROTARI_COPY_OVERWRITE` | replace a non-empty queue |
| `--quiet` | `` | `ROTARI_COPY_QUIET` | suppress success output |
| `--filter-result` | `RESULT (repeatable)` | `CLI only` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms (choices: failed, unfinished, success) |
| `--filter-exit-code` | `N (repeatable)` | `CLI only` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `CLI only` | select jobs of this failure kind; may be repeated (choices: timeout, cancelled, blocked, oom, signal, error) |
| `--filter-diagnosis` | `VALUE (repeatable)` | `CLI only` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `CLI only` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `CLI only` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari run`

execute queued commands, optionally selecting jobs from a run; jobs that depend on a selected job execute too

Usage: `rotari run [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | build the run from this saved run without changing the next queue; without it, a result filter copies the latest run only into an empty queue and otherwise uses the queued jobs |
| `--run-name` | `NAME` | `ROTARI_RUN_NAME` | run name label |
| `--local-concurrency` | `N` | `ROTARI_RUN_LOCAL_CONCURRENCY` | local worker concurrency |
| `--batch-concurrency` | `N` | `ROTARI_RUN_BATCH_CONCURRENCY` | scheduler job concurrency (Slurm/PBS/LSF/SGE) |
| `--retry` | `N` | `ROTARI_RUN_RETRY` | retry failed jobs up to N times; explicit cancellations are not retried |
| `--failed` | `` | `ROTARI_RUN_FAILED` | only execute failed jobs; others carry forward their previous result |
| `--unfinished` | `` | `ROTARI_RUN_UNFINISHED` | only execute unfinished jobs; others carry forward their previous result |
| `--success` | `` | `ROTARI_RUN_SUCCESS` | only execute successful jobs; others carry forward their previous result |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | only execute this job and the jobs that depend on it, through --depends-on or --depends-on-finished; may be repeated; not with a result filter; others carry forward their previous result |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | only execute this job by name |
| `--stage` | `STAGE` | `ROTARI_RUN_STAGE` | only execute jobs in this stage, narrowed by any result filter; others carry forward their previous result |
| `--matrix` | `NAME` | `ROTARI_RUN_MATRIX` | only execute jobs of this matrix, named by its base job name, narrowed by any result filter; others carry forward their previous result |
| `--partial-array` | `` | `ROTARI_RUN_PARTIAL_ARRAY` | with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches |
| `--async` | `` | `ROTARI_RUN_ASYNC` | return after starting the run |
| `--disconnect-action` | `ACTION` | `ROTARI_DISCONNECT_ACTION` | when the client input or connection closes unexpectedly, detach or cancel (default detach) (choices: detach, cancel) |
| `--quiet` | `` | `ROTARI_RUN_QUIET` | suppress progress and completion output |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | execution executor override (choices: local, lsf, pbs, sge, slurm, ssh) |
| `--env` | `ALL\|NONE` | `CLI only` | caller environment propagation mode (default ALL) (choices: ALL, NONE) |
| `--match-by` | `MODE` | `CLI only` | job identity matching (choices: job-id, fingerprint, id-and-fingerprint) |
| `--executor-option` | `OPTION (repeatable)` | `ROTARI_EXECUTOR_OPTIONS` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `--ssh-concurrency` | `N` | `ROTARI_RUN_SSH_CONCURRENCY` | SSH executor concurrency |
| `--ssh-options` | `OPTION (repeatable)` | `ROTARI_RUN_SSH_OPTIONS` | SSH executor dispatch options; may be repeated |
| `--slurm-concurrency` | `N` | `ROTARI_RUN_SLURM_CONCURRENCY` | Slurm executor concurrency |
| `--slurm-options` | `OPTION (repeatable)` | `ROTARI_RUN_SLURM_OPTIONS` | Slurm executor dispatch options; may be repeated |
| `--slurm-submit-interval` | `DURATION` | `ROTARI_RUN_SLURM_SUBMIT_INTERVAL` | minimum Slurm submission interval |
| `--slurm-submit-retry-limit` | `N` | `ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT` | maximum retries for transient Slurm submission failures |
| `--pbs-concurrency` | `N` | `ROTARI_RUN_PBS_CONCURRENCY` | PBS executor concurrency |
| `--pbs-options` | `OPTION (repeatable)` | `ROTARI_RUN_PBS_OPTIONS` | PBS executor dispatch options; may be repeated |
| `--pbs-submit-interval` | `DURATION` | `ROTARI_RUN_PBS_SUBMIT_INTERVAL` | minimum PBS submission interval |
| `--pbs-submit-retry-limit` | `N` | `ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT` | maximum retries for transient PBS submission failures |
| `--lsf-concurrency` | `N` | `ROTARI_RUN_LSF_CONCURRENCY` | LSF executor concurrency |
| `--lsf-options` | `OPTION (repeatable)` | `ROTARI_RUN_LSF_OPTIONS` | LSF executor dispatch options; may be repeated |
| `--lsf-submit-interval` | `DURATION` | `ROTARI_RUN_LSF_SUBMIT_INTERVAL` | minimum LSF submission interval |
| `--lsf-submit-retry-limit` | `N` | `ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT` | maximum retries for transient LSF submission failures |
| `--sge-concurrency` | `N` | `ROTARI_RUN_SGE_CONCURRENCY` | SGE executor concurrency |
| `--sge-options` | `OPTION (repeatable)` | `ROTARI_RUN_SGE_OPTIONS` | SGE executor dispatch options; may be repeated |
| `--sge-submit-interval` | `DURATION` | `ROTARI_RUN_SGE_SUBMIT_INTERVAL` | minimum SGE submission interval |
| `--sge-submit-retry-limit` | `N` | `ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT` | maximum retries for transient SGE submission failures |
| `--filter-result` | `RESULT (repeatable)` | `CLI only` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms (choices: failed, unfinished, success) |
| `--filter-exit-code` | `N (repeatable)` | `CLI only` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `CLI only` | select jobs of this failure kind; may be repeated (choices: timeout, cancelled, blocked, oom, signal, error) |
| `--filter-diagnosis` | `VALUE (repeatable)` | `CLI only` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `CLI only` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `CLI only` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `CLI only` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `CLI only` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari retry`

run failed and unfinished jobs; with --job-id, run those jobs; jobs that depend on them execute too

Usage: `rotari retry [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | build the retry from this saved run without changing the next queue; without it, retry copies the latest run only into an empty queue and otherwise uses the queued jobs |
| `--run-name` | `NAME` | `ROTARI_RUN_NAME` | run name label |
| `--local-concurrency` | `N` | `ROTARI_RUN_LOCAL_CONCURRENCY` | local worker concurrency |
| `--batch-concurrency` | `N` | `ROTARI_RUN_BATCH_CONCURRENCY` | scheduler job concurrency (Slurm/PBS/LSF/SGE) |
| `--retry` | `N` | `ROTARI_RUN_RETRY` | retry failed jobs up to N times; explicit cancellations are not retried |
| `--failed` | `` | `ROTARI_RUN_FAILED` | only execute failed jobs, instead of failed and unfinished jobs; others carry forward their previous result |
| `--unfinished` | `` | `ROTARI_RUN_UNFINISHED` | only execute unfinished jobs, instead of failed and unfinished jobs; others carry forward their previous result |
| `--success` | `` | `ROTARI_RUN_SUCCESS` | only execute successful jobs, instead of failed and unfinished jobs; others carry forward their previous result |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | only execute this job and the jobs that depend on it, through --depends-on or --depends-on-finished, instead of failed and unfinished jobs; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | only execute this job by name |
| `--stage` | `STAGE` | `ROTARI_RUN_STAGE` | only retry jobs in this stage |
| `--matrix` | `NAME` | `ROTARI_RUN_MATRIX` | only retry jobs of this matrix, named by its base job name |
| `--partial-array` | `` | `ROTARI_RUN_PARTIAL_ARRAY` | with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches |
| `--async` | `` | `ROTARI_RUN_ASYNC` | return after starting the run |
| `--disconnect-action` | `ACTION` | `ROTARI_DISCONNECT_ACTION` | when the client input or connection closes unexpectedly, detach or cancel (default detach) (choices: detach, cancel) |
| `--quiet` | `` | `ROTARI_RUN_QUIET` | suppress progress and completion output |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | execution executor override (choices: local, lsf, pbs, sge, slurm, ssh) |
| `--env` | `ALL\|NONE` | `CLI only` | caller environment propagation mode (default ALL) (choices: ALL, NONE) |
| `--match-by` | `MODE` | `CLI only` | job identity matching (choices: job-id, fingerprint, id-and-fingerprint) |
| `--executor-option` | `OPTION (repeatable)` | `ROTARI_EXECUTOR_OPTIONS` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `--ssh-concurrency` | `N` | `ROTARI_RUN_SSH_CONCURRENCY` | SSH executor concurrency |
| `--ssh-options` | `OPTION (repeatable)` | `ROTARI_RUN_SSH_OPTIONS` | SSH executor dispatch options; may be repeated |
| `--slurm-concurrency` | `N` | `ROTARI_RUN_SLURM_CONCURRENCY` | Slurm executor concurrency |
| `--slurm-options` | `OPTION (repeatable)` | `ROTARI_RUN_SLURM_OPTIONS` | Slurm executor dispatch options; may be repeated |
| `--slurm-submit-interval` | `DURATION` | `ROTARI_RUN_SLURM_SUBMIT_INTERVAL` | minimum Slurm submission interval |
| `--slurm-submit-retry-limit` | `N` | `ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT` | maximum retries for transient Slurm submission failures |
| `--pbs-concurrency` | `N` | `ROTARI_RUN_PBS_CONCURRENCY` | PBS executor concurrency |
| `--pbs-options` | `OPTION (repeatable)` | `ROTARI_RUN_PBS_OPTIONS` | PBS executor dispatch options; may be repeated |
| `--pbs-submit-interval` | `DURATION` | `ROTARI_RUN_PBS_SUBMIT_INTERVAL` | minimum PBS submission interval |
| `--pbs-submit-retry-limit` | `N` | `ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT` | maximum retries for transient PBS submission failures |
| `--lsf-concurrency` | `N` | `ROTARI_RUN_LSF_CONCURRENCY` | LSF executor concurrency |
| `--lsf-options` | `OPTION (repeatable)` | `ROTARI_RUN_LSF_OPTIONS` | LSF executor dispatch options; may be repeated |
| `--lsf-submit-interval` | `DURATION` | `ROTARI_RUN_LSF_SUBMIT_INTERVAL` | minimum LSF submission interval |
| `--lsf-submit-retry-limit` | `N` | `ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT` | maximum retries for transient LSF submission failures |
| `--sge-concurrency` | `N` | `ROTARI_RUN_SGE_CONCURRENCY` | SGE executor concurrency |
| `--sge-options` | `OPTION (repeatable)` | `ROTARI_RUN_SGE_OPTIONS` | SGE executor dispatch options; may be repeated |
| `--sge-submit-interval` | `DURATION` | `ROTARI_RUN_SGE_SUBMIT_INTERVAL` | minimum SGE submission interval |
| `--sge-submit-retry-limit` | `N` | `ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT` | maximum retries for transient SGE submission failures |
| `--filter-result` | `RESULT (repeatable)` | `CLI only` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms (choices: failed, unfinished, success) |
| `--filter-exit-code` | `N (repeatable)` | `CLI only` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `CLI only` | select jobs of this failure kind; may be repeated (choices: timeout, cancelled, blocked, oom, signal, error) |
| `--filter-diagnosis` | `VALUE (repeatable)` | `CLI only` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `CLI only` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `CLI only` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `CLI only` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `CLI only` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `CLI only` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `CLI only` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `CLI only` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `CLI only` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `CLI only` | select jobs running less than this long |
| `--filter-command` | `RE` | `CLI only` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `CLI only` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `CLI only` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `CLI only` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `CLI only` | exclude jobs of this matrix, named by its base job name; may be repeated |
| `--dry-run` | `` | `CLI only` | print what the command would change, and the project revision, without writing |
| `--if-revision` | `REVISION` | `CLI only` | apply only if the project is still at this revision, as printed by --dry-run |

### `rotari server`

manage the background server

| Subcommand | Description |
| --- | --- |
| `status` | show server status |
| `list` | list servers |
| `shutdown` | shut down server |

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | server registry directory |

### `rotari web`

serve the web status UI

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `--host` | `HOST` | `ROTARI_WEB_HOST` | HTTP listen host (live server only; cannot be combined with --static-dir) |
| `--port` | `PORT` | `ROTARI_WEB_PORT` | HTTP listen port (live server only; cannot be combined with --static-dir) |
| `--static-dir` | `DIR` | `ROTARI_WEB_STATIC_DIR` | generate a static web UI |
| `--allow-control` | `` | `ROTARI_WEB_ALLOW_CONTROL` | enable job control (copy/change/remove/cancel/clear) in the live server; false makes it read-only; incompatible with --static-dir |
| `--auth-token` | `TOKEN` | `ROTARI_WEB_AUTH_TOKEN` | require this token in Authorization: Bearer or X-Rotari-Token on the live server; cannot be combined with --static-dir; prefer ROTARI_WEB_AUTH_TOKEN for secrets |
| `--notifications` | `` | `ROTARI_WEB_NOTIFICATIONS` | default state of the browser desktop-notification toggle; pass --notifications=false to default it off |
| `--static-artifact-contents` | `` | `CLI only` | copy previewable artifact files (up to 10 MiB each, 100 MiB in all) into the static export so previews work there; requires --static-dir |
| `--artifact-root` | `DIR (repeatable)` | `CLI only` | also serve recorded artifact files under this directory, besides each job's working directory (live server only; may be repeated; cannot be combined with --static-dir) |

### `rotari mcp`

serve rotari's tools for agents over MCP on stdio

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--config` | `FILE` | `CLI only` | config file to use |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory whose runs and basedirs the tools serve |

### `rotari completion`

print shell completion script

| Subcommand | Description |
| --- | --- |
| `bash` | bash completion |
| `zsh` | zsh completion |
| `fish` | fish completion |
| `install` | install completion for the current shell |

### `rotari schema`

print the CLI schema as JSON

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--json` | `` | `CLI only` | print the schema as JSON |

### `rotari guide`

print a usage guide for coding agents

### `rotari version`

print version

### `rotari env`

list ROTARI environment variables

<!-- END GENERATED CLI REFERENCE -->
