# CLI reference

The command reference below is generated from `rotari schema --json`, the same
metadata used by command help and shell completion. Environment variables are
documented separately in [Environment variables](ENVIRONMENT_VARIABLES.md).

<!-- BEGIN GENERATED CLI REFERENCE -->

## Commands

### `rotari config`

generate a config file template

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--list` | `` | `` | list existing config files |
| `-o` / `--format` | `FORMAT` | `` | config format: yaml, toml, or json |
| `--output` | `FILE` | `` | output config file path |

### `rotari check`

check whether a project is ready to run

Usage: `rotari check [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--json` | `` | `` | print machine-readable JSON |
| `--deep` | `` | `` | check local executables and working directories |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |

### `rotari reset`

discard the current, not-yet-run queue

Usage: `rotari reset [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--recover` | `` | `ROTARI_RESET_RECOVER` | confirm an interrupted run has stopped without prompting |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |

### `rotari cancel`

cancel the active run or running jobs

Usage: `rotari cancel [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | cancel a running job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | cancel the unfinished jobs with this name; may be repeated |
| `--stage` | `STAGE` | `` | cancel the unfinished jobs of this stage |
| `--matrix` | `NAME` | `` | cancel the unfinished jobs of this matrix |
| `--wait` | `` | `` | wait until cancellation is complete |
| `--yes` | `` | `` | cancel the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `` | select jobs in this state; may be repeated |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari suspend`

suspend running jobs

Usage: `rotari suspend [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | suspend a running job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | suspend the running jobs with this name; may be repeated |
| `--stage` | `STAGE` | `` | suspend the running jobs of this stage |
| `--matrix` | `NAME` | `` | suspend the running jobs of this matrix |
| `--yes` | `` | `` | suspend the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `` | select jobs in this state; may be repeated |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari resume`

resume suspended jobs

Usage: `rotari resume [JOB_ID|ATTEMPT_ID|RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | resume a suspended job; may be repeated |
| `--job-name` | `NAME (repeatable)` | `ROTARI_JOB_NAME` | resume the suspended jobs with this name; may be repeated |
| `--stage` | `STAGE` | `` | resume the suspended jobs of this stage |
| `--matrix` | `NAME` | `` | resume the suspended jobs of this matrix |
| `--yes` | `` | `` | resume the jobs that filters select without asking |
| `--filter-state` | `STATE (repeatable)` | `` | select jobs in this state; may be repeated |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari delete`

delete saved run history

Usage: `rotari delete [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run to delete |
| `--all` | `` | `` | delete every run of the project |

### `rotari gc`

find and remove orphan run registry entries

Usage: `rotari gc [MASTERDIR]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `--apply` | `` | `` | remove the cached orphan entries |

### `rotari unlock`

remove a confirmed stale run lock

Usage: `rotari unlock [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | verify the run ID recorded in the stale lock |

### `rotari change`

change jobs in the current or previous batch

Usage: `rotari change <command ...>`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run ID to use when restoring a batch |
| `-j` / `--job-id` | `ID` | `ROTARI_JOB_ID` | target job ID |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | target job name |
| `--stage` | `STAGE` | `` | change every job in a stage |
| `--matrix` | `NAME` | `` | change every job of a matrix, named by its base job name |
| `--all` | `` | `` | change every job |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | replace job executor |
| `--executor-option` | `OPTION` | `ROTARI_EXECUTOR_OPTIONS` | replace executor options |
| `--clear-executor-options` | `` | `` | clear executor options |
| `--working-directory` | `DIR` | `` | working directory for the job |
| `--clear-working-directory` | `` | `` | clear the job working directory |
| `--env` | `KEY=VALUE (repeatable)` | `` | replace job environment variables; may be repeated |
| `--clear-env` | `` | `` | clear job environment variables |
| `--set-job-name` | `NAME` | `` | replace job name |
| `--depends-on` | `NAME (repeatable)` | `` | replace prerequisites; may be repeated |
| `--clear-depends-on` | `` | `` | clear prerequisites |
| `--depends-on-finished` | `NAME (repeatable)` | `` | replace prerequisites that only need to finish, whatever their result; may be repeated |
| `--clear-depends-on-finished` | `` | `` | clear prerequisites that only need to finish |
| `--timeout` | `DURATION` | `` | replace the job timeout, such as 90m or 2h |
| `--clear-timeout` | `` | `` | remove the job timeout |
| `--retry` | `N` | `` | replace the job's retry limit; 0 disables retries |
| `--clear-retry` | `` | `` | use the run's --retry limit for the job again and remove its retry delay settings |
| `--retry-delay` | `DURATION` | `` | replace the wait before the job's first retry, such as 30s |
| `--retry-backoff` | `FACTOR` | `` | replace the factor applied to the retry delay for each further retry |
| `--retry-max-delay` | `DURATION` | `` | replace the upper limit of the retry delay |
| `--status` | `STATUS` | `` | mark the job with a status that result filters of the next run read in place of its recorded result |
| `--clear-status` | `` | `` | remove the job's status mark |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari export`

export the current queue or saved runs as a workflow manifest

Usage: `rotari export [TARGET] [FILE]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID (repeatable)` | `ROTARI_RUN_ID` | run ID to export; may be repeated |
| `-o` / `--format` | `FORMAT` | `` | manifest format: yaml, toml, or json |
| `--template` | `` | `` | print a starter workflow manifest |
| `--output` | `FILE` | `` | write the workflow manifest to a file |

### `rotari import`

validate and replace a queue from a workflow manifest

Usage: `rotari import FILE [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--overwrite` | `` | `` | replace a non-empty queue |
| `--dry-run` | `` | `` | validate and print the import plan without writing |
| `--json` | `` | `` | print the import plan as JSON |

### `rotari remove`

remove jobs from the current or previous batch

Usage: `rotari remove [JOB_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run ID to use when restoring a batch |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | remove a job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | remove a job by name |
| `--stage` | `STAGE` | `` | remove every job in a stage |
| `--matrix` | `NAME` | `` | remove every job of a matrix, named by its base job name |
| `--all` | `` | `` | remove every job |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari show`

show queue or run status

Usage: `rotari show [SELECTOR]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run ID or latest |
| `--queue` | `` | `` | show the current queue even when a run is selected |
| `-j` / `--job-id` | `ID` | `ROTARI_JOB_ID` | job ID |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | job name |
| `--failed` | `` | `` | show failed jobs; may be combined with the other result filters |
| `--unfinished` | `` | `` | show unfinished jobs; may be combined with the other result filters |
| `--success` | `` | `` | show successful jobs; may be combined with the other result filters |
| `--stage` | `NAME` | `` | show jobs in this stage only |
| `--matrix` | `NAME` | `` | show jobs of this matrix only, named by its base job name |
| `--lineage` | `` | `` | list the project's runs oldest first with result counts and changes since the previous run |
| `--logs` | `` | `` | print output logs for all jobs |
| `--failed-logs` | `` | `` | print output logs for failed jobs |
| `--stream` | `STREAM` | `` | show both streams or select stdout/stderr |
| `--follow` | `` | `` | follow one selected log stream until the run completes |
| `--no-pager` | `` | `` | print logs directly instead of using a pager |
| `--basedirs` | `` | `` | list state directories known to the master registry |
| `--json` | `` | `` | print machine-readable JSON for a run |
| `--report` | `` | `` | print an AI-ready Markdown report |
| `--filter-result` | `RESULT (repeatable)` | `` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `--filter-exit-code` | `N (repeatable)` | `` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `--filter-diagnosis` | `VALUE (repeatable)` | `` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari diff`

compare job results and definitions between two runs

Usage: `rotari diff [[RUN_A] RUN_B]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--json` | `` | `` | print the comparison as JSON |
| `--unchanged` | `` | `` | also list jobs whose result and definition did not change |

### `rotari jobs`

list running and recently finished jobs across projects

Usage: `rotari jobs [PROJECT]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | master registry directory for --all |
| `--all-basedirs` | `` | `` | include all basedirs known to the master registry |
| `-o` / `--format` | `FORMAT` | `` | output fields; use %s %b %p %a %n %c %t %f %e (%f is finished time) |
| `--since` | `DURATION` | `` | include jobs finished within this duration; use 0 for running jobs only |

### `rotari diagnose`

diagnose one job with an LLM or local error rules

Usage: `rotari diagnose [JOB_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | run ID |
| `-j` / `--job-id` | `ID` | `ROTARI_JOB_ID` | failed job ID |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | failed job name |
| `--rules` | `` | `` | use local rule-based diagnosis without calling an LLM |
| `--provider` | `PROVIDER` | `ROTARI_LLM_PROVIDER` | LLM provider: openai, openai-chat, anthropic, gemini, or cohere |
| `--endpoint` | `URL` | `ROTARI_LLM_ENDPOINT` | LLM API endpoint |
| `--model` | `MODEL` | `ROTARI_LLM_MODEL` | LLM model name |
| `--language` | `TAG` | `ROTARI_LLM_LANGUAGE` | response language BCP 47 tag |

### `rotari wait`

wait for an asynchronous run by project, run name, or run ID

Usage: `rotari wait [PROJECT_OR_RUN_NAME_OR_RUN_ID ...]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID (repeatable)` | `ROTARI_RUN_ID` | run ID; may be repeated |
| `--timeout` | `DURATION` | `ROTARI_WAIT_TIMEOUT` | maximum wait duration |
| `--json` | `` | `` | print each completed run as one JSON object |

### `rotari add`

add a command to a queue

Usage: `rotari add <command ...>`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | job executor |
| `--executor-option` | `OPTION (repeatable)` | `ROTARI_EXECUTOR_OPTIONS` | option passed to the selected scheduler (sbatch/qsub/...); may be repeated |
| `--output` | `FILE (repeatable)` | `` | stdout destination; stderr also goes here unless --error is specified; may be repeated |
| `--error` | `FILE (repeatable)` | `` | stderr destination; defaults to --output destinations; may be repeated |
| `--log-mode` | `MODE` | `` | internal log mode |
| `--open-mode` | `MODE` | `` | external output file mode |
| `--working-directory` | `DIR` | `` | working directory for the job |
| `--env` | `KEY=VALUE (repeatable)` | `` | environment variable for the job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | job name label |
| `--stage` | `NAME` | `` | stage that contains the job |
| `--depends-on` | `NAME (repeatable)` | `` | name of a prerequisite job or stage; may be repeated |
| `--depends-on-finished` | `NAME (repeatable)` | `` | name of a prerequisite job or stage that must finish, whatever its result; may be repeated |
| `--timeout` | `DURATION` | `` | stop the job this long after it starts, such as 90m or 2h; it then fails with exit code 124 |
| `--retry` | `N` | `` | retry the job up to N times when it fails, instead of the run's --retry; 0 disables retries |
| `--retry-delay` | `DURATION` | `` | wait this long before the job's first retry, such as 30s; retries are immediate by default |
| `--retry-backoff` | `FACTOR` | `` | multiply the retry delay by this factor for each further retry, such as 2 |
| `--retry-max-delay` | `DURATION` | `` | upper limit of the retry delay, such as 10m |
| `--array` | `FIRST-LAST\|TASK[,TASK...]` | `ROTARI_ARRAY_RANGE` | create an array job range or selected tasks |
| `--matrix` | `KEY=VALUE[,VALUE...] (repeatable)` | `` | expand a command into jobs from KEY=VALUE[,VALUE...] dimensions; may be repeated |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |

### `rotari copy`

copy the latest run's jobs into the queue

Usage: `rotari copy [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | source run ID; defaults to the latest run |
| `--failed` | `` | `` | include failed jobs; may be combined with result filters |
| `--unfinished` | `` | `` | include unfinished jobs; may be combined with result filters |
| `--success` | `` | `` | include successful jobs; may be combined with result filters |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | copy a job; may be repeated |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | copy a job by name |
| `--stage` | `STAGE` | `` | only copy jobs in this stage, narrowed by any result filter |
| `--matrix` | `NAME` | `` | only copy jobs of this matrix, named by its base job name, narrowed by any result filter |
| `--append` | `` | `` | append to a non-empty queue |
| `--overwrite` | `` | `` | replace a non-empty queue |
| `--quiet` | `` | `ROTARI_QUIET` | suppress success output |
| `--filter-result` | `RESULT (repeatable)` | `` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `--filter-exit-code` | `N (repeatable)` | `` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `--filter-diagnosis` | `VALUE (repeatable)` | `` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari run`

execute queued commands, optionally selecting jobs from a run

Usage: `rotari run [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | repopulate the queue from this run before executing (copy --run-id + run); defaults to the latest run when a result filter is used |
| `--overwrite` | `` | `` | replace a non-empty queue without prompting; requires --run-id |
| `--run-name` | `NAME` | `ROTARI_RUN_NAME` | run name label |
| `--local-concurrency` | `N` | `ROTARI_RUN_LOCAL_CONCURRENCY` | local worker concurrency |
| `--batch-concurrency` | `N` | `ROTARI_RUN_BATCH_CONCURRENCY` | scheduler job concurrency (Slurm/PBS/...) |
| `--retry` | `N` | `ROTARI_RUN_RETRY` | retry failed jobs up to N times; explicit cancellations are not retried |
| `--failed` | `` | `` | only execute failed jobs; others carry forward their previous result |
| `--unfinished` | `` | `` | only execute unfinished jobs; others carry forward their previous result |
| `--success` | `` | `` | only execute successful jobs; others carry forward their previous result |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | only execute this job; may be repeated; not with a result filter; others carry forward their previous result |
| `--job-name` | `NAME` | `ROTARI_JOB_NAME` | only execute this job by name |
| `--stage` | `STAGE` | `` | only execute jobs in this stage, narrowed by any result filter; others carry forward their previous result |
| `--matrix` | `NAME` | `` | only execute jobs of this matrix, named by its base job name, narrowed by any result filter; others carry forward their previous result |
| `--partial-array` | `` | `` | with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches |
| `--async` | `` | `ROTARI_RUN_ASYNC` | return after starting the run |
| `--quiet` | `` | `ROTARI_QUIET` | suppress progress and completion output |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | execution executor override |
| `--env` | `ALL\|NONE` | `` | caller environment propagation mode (default ALL) |
| `--match-by` | `MODE` | `` | job identity matching |
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
| `--filter-result` | `RESULT (repeatable)` | `` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `--filter-exit-code` | `N (repeatable)` | `` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `--filter-diagnosis` | `VALUE (repeatable)` | `` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari retry`

run failed and unfinished jobs; with --job-id, run those jobs

Usage: `rotari retry [RUN_ID]`

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `-p` / `--project-name` | `NAME` | `ROTARI_PROJECT_NAME` | project name |
| `-r` / `--run-id` | `ID` | `ROTARI_RUN_ID` | repopulate the queue from this run before executing; defaults to the latest run |
| `--overwrite` | `` | `` | replace a non-empty queue without prompting; requires --run-id |
| `--run-name` | `NAME` | `ROTARI_RUN_NAME` | run name label |
| `--local-concurrency` | `N` | `ROTARI_RUN_LOCAL_CONCURRENCY` | local worker concurrency |
| `--batch-concurrency` | `N` | `ROTARI_RUN_BATCH_CONCURRENCY` | scheduler job concurrency (Slurm/PBS/...) |
| `--retry` | `N` | `ROTARI_RUN_RETRY` | retry failed jobs up to N times; explicit cancellations are not retried |
| `-j` / `--job-id` | `ID (repeatable)` | `ROTARI_JOB_ID` | only execute this job instead of failed and unfinished jobs; may be repeated |
| `--stage` | `STAGE` | `` | only retry jobs in this stage |
| `--matrix` | `NAME` | `` | only retry jobs of this matrix, named by its base job name |
| `--async` | `` | `ROTARI_RUN_ASYNC` | return after starting the run |
| `--quiet` | `` | `ROTARI_QUIET` | suppress progress and completion output |
| `-e` / `--executor` | `EXECUTOR` | `ROTARI_EXECUTOR` | execution executor override |
| `--env` | `ALL\|NONE` | `` | caller environment propagation mode (default ALL) |
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
| `--filter-result` | `RESULT (repeatable)` | `` | select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms |
| `--filter-exit-code` | `N (repeatable)` | `` | select jobs with this exit code; may be repeated |
| `--filter-failure-kind` | `KIND (repeatable)` | `` | select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error |
| `--filter-diagnosis` | `VALUE (repeatable)` | `` | select failed jobs matching a current diagnosis rule; may be repeated |
| `--filter-changed` | `` | `` | select queued jobs whose definition changed from the reference run |
| `--filter-new` | `` | `` | select queued jobs with no matching job in the reference run |
| `--filter-host` | `PATTERN (repeatable)` | `` | select jobs run on a matching host; may be repeated |
| `--filter-started-after` | `TIME` | `` | select jobs started at or after this time |
| `--filter-started-before` | `TIME` | `` | select jobs started before this time |
| `--filter-finished-after` | `TIME` | `` | select jobs finished at or after this time |
| `--filter-finished-before` | `TIME` | `` | select jobs finished before this time |
| `--filter-longer-than` | `DURATION` | `` | select jobs running at least this long |
| `--filter-shorter-than` | `DURATION` | `` | select jobs running less than this long |
| `--filter-command` | `RE` | `` | select jobs whose argv matches this Go regexp |
| `--filter-stage` | `NAME` | `` | select jobs in this stage; same as --stage |
| `--filter-not-stage` | `NAME (repeatable)` | `` | exclude jobs in this stage; may be repeated |
| `--filter-matrix` | `NAME` | `` | select jobs of this matrix, named by its base job name; same as --matrix |
| `--filter-not-matrix` | `NAME (repeatable)` | `` | exclude jobs of this matrix, named by its base job name; may be repeated |

### `rotari server`

manage the background server

| Subcommand | Description |
| --- | --- |
| `status` | show server status |
| `list` | list servers |
| `shutdown` | shut down server |

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `--masterdir` | `DIR` | `ROTARI_MASTERDIR` | server registry directory |

### `rotari web`

serve the web status UI

| Option | Value | Environment | Description |
| --- | --- | --- | --- |
| `-b` / `--basedir` | `DIR` | `ROTARI_BASEDIR` | state directory |
| `--host` | `HOST` | `ROTARI_WEB_HOST` | HTTP listen host |
| `--port` | `PORT` | `ROTARI_WEB_PORT` | HTTP listen port |
| `--static-dir` | `DIR` | `ROTARI_WEB_STATIC_DIR` | generate a static web UI |
| `--allow-control` | `` | `ROTARI_WEB_ALLOW_CONTROL` | enable job control (copy/change/remove/cancel/clear); pass --allow-control=false for a read-only UI |
| `--auth-token` | `TOKEN` | `ROTARI_WEB_AUTH_TOKEN` | require this token in Authorization: Bearer or X-Rotari-Token; prefer ROTARI_WEB_AUTH_TOKEN for secrets |
| `--notifications` | `` | `ROTARI_WEB_NOTIFICATIONS` | default state of the browser desktop-notification toggle; pass --notifications=false to default it off |

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
| `--json` | `` | `` | print the schema as JSON |

### `rotari guide`

print a usage guide for coding agents

### `rotari version`

print version

### `rotari env`

list ROTARI environment variables

<!-- END GENERATED CLI REFERENCE -->
