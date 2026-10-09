# Environment variables

The table below is generated from `rotari schema --json`.

Column meanings:

- **CLI default**: the variable can provide the corresponding CLI option's default value.
- **Job**: the variable is available to commands executed as jobs.
- **Array**: the variable is available to array task commands.
- **Description**: what the variable controls or represents.

`rotari env` prints the same definitions together with whether each variable
is currently set.

Every CLI option has a corresponding environment variable unless its CLI
reference entry says **CLI only**. The option's exact variable is shown in the
CLI reference; by default, names use `ROTARI_<COMMAND>_<OPTION>` with uppercase
letters and hyphens converted to underscores. Shared options may keep a
documented variable name used across commands.

For options available from the CLI, environment, and configuration, values are
resolved in this order: explicit CLI value, environment variable, configuration
values, then built-in default. Ordinary files merge global → cwd workspace →
basedir → project; command sections then override root values. `--config FILE`
replaces automatic discovery with that file. Environment overrides are not
included in the saved merged file-config snapshot. See
[Configuration](CONFIGURATION.md#configuration-files).

<!-- BEGIN GENERATED ENVIRONMENT REFERENCE -->

| Variable | CLI default | Job | Array | Description |
| --- | --- | --- | --- | --- |
| `ROTARI_BASEDIR` | yes | yes | yes | State directory; --basedir default. |
| `ROTARI_PROJECT_NAME` | yes | yes | yes | Project name; --project-name default. |
| `ROTARI_MASTERDIR` | yes | no | no | Server registry directory; --masterdir default. |
| `ROTARI_RUN_ID` | yes | yes | yes | Current run ID; --run-id default. |
| `ROTARI_JOB_ID` | yes | yes | yes | Current job ID; --job-id default. |
| `ROTARI_ATTEMPT_ID` | no | yes | yes | Current job attempt ID. |
| `ROTARI_JOB_NAME` | yes | yes | yes | Current job name; --job-name default. |
| `ROTARI_EXECUTOR` | yes | yes | yes | Current executor; --executor default. |
| `ROTARI_EXECUTOR_OPTIONS` | yes | yes | yes | Default scheduler executor options. |
| `ROTARI_RUN_NAME` | yes | yes | yes | Run name; --run-name default. |
| `ROTARI_RUN_LOCAL_CONCURRENCY` | yes | yes | yes | Local worker limit; --local-concurrency default. |
| `ROTARI_RUN_BATCH_CONCURRENCY` | yes | yes | yes | Scheduler submission limit; --batch-concurrency default. |
| `ROTARI_RUN_SSH_CONCURRENCY` | yes | no | no | SSH worker limit; --ssh-concurrency default. |
| `ROTARI_RUN_SSH_OPTIONS` | yes | no | no | SSH dispatch options; --ssh-options default. |
| `ROTARI_RUN_SLURM_CONCURRENCY` | yes | no | no | Slurm worker limit; --slurm-concurrency default. |
| `ROTARI_RUN_SLURM_OPTIONS` | yes | no | no | Slurm dispatch options; --slurm-options default. |
| `ROTARI_RUN_SLURM_SUBMIT_INTERVAL` | yes | no | no | Slurm submit interval; --slurm-submit-interval default. |
| `ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT` | yes | no | no | Slurm transient submit retry limit; --slurm-submit-retry-limit default. |
| `ROTARI_RUN_PBS_CONCURRENCY` | yes | no | no | PBS worker limit; --pbs-concurrency default. |
| `ROTARI_RUN_PBS_OPTIONS` | yes | no | no | PBS dispatch options; --pbs-options default. |
| `ROTARI_RUN_PBS_SUBMIT_INTERVAL` | yes | no | no | PBS submit interval; --pbs-submit-interval default. |
| `ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT` | yes | no | no | PBS transient submit retry limit; --pbs-submit-retry-limit default. |
| `ROTARI_RUN_LSF_CONCURRENCY` | yes | no | no | LSF worker limit; --lsf-concurrency default. |
| `ROTARI_RUN_LSF_OPTIONS` | yes | no | no | LSF dispatch options; --lsf-options default. |
| `ROTARI_RUN_LSF_SUBMIT_INTERVAL` | yes | no | no | LSF submit interval; --lsf-submit-interval default. |
| `ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT` | yes | no | no | LSF transient submit retry limit; --lsf-submit-retry-limit default. |
| `ROTARI_RUN_SGE_CONCURRENCY` | yes | no | no | SGE worker limit; --sge-concurrency default. |
| `ROTARI_RUN_SGE_OPTIONS` | yes | no | no | SGE dispatch options; --sge-options default. |
| `ROTARI_RUN_SGE_SUBMIT_INTERVAL` | yes | no | no | SGE submit interval; --sge-submit-interval default. |
| `ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT` | yes | no | no | SGE transient submit retry limit; --sge-submit-retry-limit default. |
| `ROTARI_RUN_RETRY` | yes | yes | yes | Retry count; --retry default. |
| `ROTARI_RUN_ASYNC` | yes | yes | yes | Async run mode; --async default. |
| `ROTARI_DISCONNECT_ACTION` | yes | no | no | Default action when a synchronous run or wait client disconnects; detach or cancel. |
| `ROTARI_QUIET` | yes | yes | yes | Global quiet mode; --quiet default. |
| `ROTARI_ADD_QUIET` | yes | no | no | Add command quiet mode; --quiet default. |
| `ROTARI_COPY_QUIET` | yes | no | no | Copy command quiet mode; --quiet default. |
| `ROTARI_CHANGE_QUIET` | yes | no | no | Change command quiet mode; --quiet default. |
| `ROTARI_REMOVE_QUIET` | yes | no | no | Remove command quiet mode; --quiet default. |
| `ROTARI_RESET_QUIET` | yes | no | no | Reset command quiet mode; --quiet default. |
| `ROTARI_CHECK_QUIET` | yes | no | no | Check command quiet mode; --quiet default. |
| `ROTARI_RUN_QUIET` | yes | yes | yes | Run-specific quiet mode; --quiet default. |
| `ROTARI_WAIT_QUIET` | yes | no | no | Wait command quiet mode; --quiet default. |
| `ROTARI_ARRAY_RANGE` | yes | yes | yes | Array range; --array default. |
| `ROTARI_BIN` | no | yes | yes | Absolute path to the rotari binary. |
| `ROTARI_RUN_DIR` | no | yes | yes | Current run directory. |
| `ROTARI_JOB_DIR` | no | yes | yes | Current job directory. |
| `ROTARI_CWD` | no | yes | yes | Working directory from which the run started. |
| `ROTARI_ARRAY_TASK_ID` | no | no | yes | Current array task number. |
| `ROTARI_ARRAY_FIRST` | no | no | yes | First array task number. |
| `ROTARI_ARRAY_LAST` | no | no | yes | Last array task number. |
| `ROTARI_ARRAY_SIZE` | no | no | yes | Number of tasks in the array. |
| `ROTARI_WAIT_TIMEOUT` | yes | no | no | --timeout default for wait. |
| `ROTARI_WEB_HOST` | yes | no | no | --host default for web. |
| `ROTARI_WEB_PORT` | yes | no | no | --port default for web. |
| `ROTARI_WEB_STATIC_DIR` | yes | no | no | --static-dir default for web. |
| `ROTARI_WEB_ALLOW_CONTROL` | yes | no | no | --allow-control default for web. |
| `ROTARI_WEB_AUTH_TOKEN` | yes | no | no | --auth-token default for web; never exposed by the Web UI. |
| `ROTARI_WEB_NOTIFICATIONS` | yes | no | no | --notifications default for web. |
| `ROTARI_WEBHOOK_URL` | no | no | no | Webhook URL; overrides webhook.url in notifications.toml. |
| `ROTARI_PRIVATE_STATE` | no | no | no | set to true for 0700/0600 state directory permissions instead of the default 0755/0644 (shared state). |
| `ROTARI_INFO_JSON` | yes | no | no | Default for rotari info --json. |
| `ROTARI_CONFIG_LIST` | yes | no | no | Default for rotari config --list. |
| `ROTARI_CONFIG_FORMAT` | yes | no | no | Default for rotari config --format. |
| `ROTARI_CONFIG_OUTPUT` | yes | no | no | Default for rotari config --output. |
| `ROTARI_CHECK_JSON` | yes | no | no | Default for rotari check --json. |
| `ROTARI_CHECK_DEEP` | yes | no | no | Default for rotari check --deep. |
| `ROTARI_CANCEL_STAGE` | yes | no | no | Default for rotari cancel --stage. |
| `ROTARI_CANCEL_MATRIX` | yes | no | no | Default for rotari cancel --matrix. |
| `ROTARI_CANCEL_WAIT` | yes | no | no | Default for rotari cancel --wait. |
| `ROTARI_SUSPEND_STAGE` | yes | no | no | Default for rotari suspend --stage. |
| `ROTARI_SUSPEND_MATRIX` | yes | no | no | Default for rotari suspend --matrix. |
| `ROTARI_RESUME_STAGE` | yes | no | no | Default for rotari resume --stage. |
| `ROTARI_RESUME_MATRIX` | yes | no | no | Default for rotari resume --matrix. |
| `ROTARI_CHANGE_STAGE` | yes | no | no | Default for rotari change --stage. |
| `ROTARI_CHANGE_MATRIX` | yes | no | no | Default for rotari change --matrix. |
| `ROTARI_CHANGE_CLEAR_EXECUTOR_OPTIONS` | yes | no | no | Default for rotari change --clear-executor-options. |
| `ROTARI_CHANGE_WORKING_DIRECTORY` | yes | no | no | Default for rotari change --working-directory. |
| `ROTARI_CHANGE_CLEAR_WORKING_DIRECTORY` | yes | no | no | Default for rotari change --clear-working-directory. |
| `ROTARI_CHANGE_ENV` | yes | no | no | Default for rotari change --env. |
| `ROTARI_CHANGE_CLEAR_ENV` | yes | no | no | Default for rotari change --clear-env. |
| `ROTARI_CHANGE_SET_JOB_NAME` | yes | no | no | Default for rotari change --set-job-name. |
| `ROTARI_CHANGE_DEPENDS_ON` | yes | no | no | Default for rotari change --depends-on. |
| `ROTARI_CHANGE_CLEAR_DEPENDS_ON` | yes | no | no | Default for rotari change --clear-depends-on. |
| `ROTARI_CHANGE_DEPENDS_ON_FINISHED` | yes | no | no | Default for rotari change --depends-on-finished. |
| `ROTARI_CHANGE_CLEAR_DEPENDS_ON_FINISHED` | yes | no | no | Default for rotari change --clear-depends-on-finished. |
| `ROTARI_CHANGE_CLEAR_ARTIFACTS` | yes | no | no | Default for rotari change --clear-artifacts. |
| `ROTARI_CHANGE_CLEAR_TIMEOUT` | yes | no | no | Default for rotari change --clear-timeout. |
| `ROTARI_CHANGE_CLEAR_RETRY` | yes | no | no | Default for rotari change --clear-retry. |
| `ROTARI_CHANGE_RETRY_DELAY` | yes | no | no | Default for rotari change --retry-delay. |
| `ROTARI_CHANGE_RETRY_BACKOFF` | yes | no | no | Default for rotari change --retry-backoff. |
| `ROTARI_CHANGE_RETRY_MAX_DELAY` | yes | no | no | Default for rotari change --retry-max-delay. |
| `ROTARI_CHANGE_CLEAR_STATUS` | yes | no | no | Default for rotari change --clear-status. |
| `ROTARI_EXPORT_FORMAT` | yes | no | no | Default for rotari export --format. |
| `ROTARI_EXPORT_TEMPLATE` | yes | no | no | Default for rotari export --template. |
| `ROTARI_IMPORT_OVERWRITE` | yes | no | no | Default for rotari import --overwrite. |
| `ROTARI_IMPORT_JSON` | yes | no | no | Default for rotari import --json. |
| `ROTARI_REMOVE_STAGE` | yes | no | no | Default for rotari remove --stage. |
| `ROTARI_REMOVE_MATRIX` | yes | no | no | Default for rotari remove --matrix. |
| `ROTARI_RUNS_SINCE` | yes | no | no | Default for rotari runs --since. |
| `ROTARI_RUNS_JSON` | yes | no | no | Default for rotari runs --json. |
| `ROTARI_SHOW_QUEUE` | yes | no | no | Default for rotari show --queue. |
| `ROTARI_SHOW_FAILED` | yes | no | no | Default for rotari show --failed. |
| `ROTARI_SHOW_UNFINISHED` | yes | no | no | Default for rotari show --unfinished. |
| `ROTARI_SHOW_SUCCESS` | yes | no | no | Default for rotari show --success. |
| `ROTARI_SHOW_STAGE` | yes | no | no | Default for rotari show --stage. |
| `ROTARI_SHOW_MATRIX` | yes | no | no | Default for rotari show --matrix. |
| `ROTARI_SHOW_LOGS` | yes | no | no | Default for rotari show --logs. |
| `ROTARI_SHOW_FAILED_LOGS` | yes | no | no | Default for rotari show --failed-logs. |
| `ROTARI_SHOW_STREAM` | yes | no | no | Default for rotari show --stream. |
| `ROTARI_SHOW_TAIL` | yes | no | no | Default for rotari show --tail. |
| `ROTARI_SHOW_FOLLOW` | yes | no | no | Default for rotari show --follow. |
| `ROTARI_SHOW_NO_PAGER` | yes | no | no | Default for rotari show --no-pager. |
| `ROTARI_SHOW_JSON` | yes | no | no | Default for rotari show --json. |
| `ROTARI_SHOW_REPORT` | yes | no | no | Default for rotari show --report. |
| `ROTARI_SHOW_ARTIFACTS` | yes | no | no | Default for rotari show --artifacts. |
| `ROTARI_LINEAGE_JSON` | yes | no | no | Default for rotari lineage --json. |
| `ROTARI_JOBS_FORMAT` | yes | no | no | Default for rotari jobs --format. |
| `ROTARI_JOBS_SINCE` | yes | no | no | Default for rotari jobs --since. |
| `ROTARI_JOBS_JSON` | yes | no | no | Default for rotari jobs --json. |
| `ROTARI_WAIT_UNTIL_FAILURE` | yes | no | no | Default for rotari wait --until-failure. |
| `ROTARI_WAIT_JSON` | yes | no | no | Default for rotari wait --json. |
| `ROTARI_ADD_WORKING_DIRECTORY` | yes | no | no | Default for rotari add --working-directory. |
| `ROTARI_ADD_ENV` | yes | no | no | Default for rotari add --env. |
| `ROTARI_ADD_STAGE` | yes | no | no | Default for rotari add --stage. |
| `ROTARI_ADD_DEPENDS_ON` | yes | no | no | Default for rotari add --depends-on. |
| `ROTARI_ADD_DEPENDS_ON_FINISHED` | yes | no | no | Default for rotari add --depends-on-finished. |
| `ROTARI_ADD_RETRY_DELAY` | yes | no | no | Default for rotari add --retry-delay. |
| `ROTARI_ADD_RETRY_BACKOFF` | yes | no | no | Default for rotari add --retry-backoff. |
| `ROTARI_ADD_RETRY_MAX_DELAY` | yes | no | no | Default for rotari add --retry-max-delay. |
| `ROTARI_ADD_MATRIX` | yes | no | no | Default for rotari add --matrix. |
| `ROTARI_COPY_FAILED` | yes | no | no | Default for rotari copy --failed. |
| `ROTARI_COPY_UNFINISHED` | yes | no | no | Default for rotari copy --unfinished. |
| `ROTARI_COPY_SUCCESS` | yes | no | no | Default for rotari copy --success. |
| `ROTARI_COPY_STAGE` | yes | no | no | Default for rotari copy --stage. |
| `ROTARI_COPY_MATRIX` | yes | no | no | Default for rotari copy --matrix. |
| `ROTARI_COPY_APPEND` | yes | no | no | Default for rotari copy --append. |
| `ROTARI_COPY_OVERWRITE` | yes | no | no | Default for rotari copy --overwrite. |
| `ROTARI_RUN_FAILED` | yes | no | no | Default for rotari run --failed. |
| `ROTARI_RUN_UNFINISHED` | yes | no | no | Default for rotari run --unfinished. |
| `ROTARI_RUN_SUCCESS` | yes | no | no | Default for rotari run --success. |
| `ROTARI_RUN_STAGE` | yes | no | no | Default for rotari run --stage. |
| `ROTARI_RUN_MATRIX` | yes | no | no | Default for rotari run --matrix. |
| `ROTARI_RUN_PARTIAL_ARRAY` | yes | no | no | Default for rotari run --partial-array. |

<!-- END GENERATED ENVIRONMENT REFERENCE -->
