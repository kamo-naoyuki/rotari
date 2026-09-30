# Environment variables

The table below is generated from `rotari schema --json`.

Column meanings:

- **CLI default**: the variable can provide the corresponding CLI option's default value.
- **Job**: the variable is available to commands executed as jobs.
- **Array**: the variable is available to array task commands.
- **Description**: what the variable controls or represents.

`rotari env` prints the same definitions together with whether each variable
is currently set.

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
| `ROTARI_RUN_RETRY` | yes | yes | yes | Retry count; --retry default. |
| `ROTARI_RUN_ASYNC` | yes | yes | yes | Async run mode; --async default. |
| `ROTARI_QUIET` | yes | yes | yes | Global quiet mode; --quiet default. |
| `ROTARI_ADD_QUIET` | yes | no | no | Add command quiet mode; --quiet default. |
| `ROTARI_COPY_QUIET` | yes | no | no | Copy command quiet mode; --quiet default. |
| `ROTARI_CHANGE_QUIET` | yes | no | no | Change command quiet mode; --quiet default. |
| `ROTARI_REMOVE_QUIET` | yes | no | no | Remove command quiet mode; --quiet default. |
| `ROTARI_RESET_QUIET` | yes | no | no | Reset command quiet mode; --quiet default. |
| `ROTARI_CHECK_QUIET` | yes | no | no | Check command quiet mode; --quiet default. |
| `ROTARI_RUN_QUIET` | yes | yes | yes | Run-specific quiet mode; --quiet default. |
| `ROTARI_ARRAY_RANGE` | yes | yes | yes | Array range; --array default. |
| `ROTARI_BIN` | no | yes | yes | Absolute path to the rotari binary. |
| `ROTARI_RUN_DIR` | no | yes | yes | Current run directory. |
| `ROTARI_JOB_DIR` | no | yes | yes | Current job directory. |
| `ROTARI_CWD` | no | yes | yes | Working directory from which the run started. |
| `ROTARI_ARRAY_TASK_ID` | no | no | yes | Current array task number. |
| `ROTARI_ARRAY_FIRST` | no | no | yes | First array task number. |
| `ROTARI_ARRAY_LAST` | no | no | yes | Last array task number. |
| `ROTARI_ARRAY_SIZE` | no | no | yes | Number of tasks in the array. |
| `ROTARI_RESET_RECOVER` | yes | no | no | --recover default for reset. |
| `ROTARI_WAIT_TIMEOUT` | yes | no | no | --timeout default for wait. |
| `ROTARI_WEB_HOST` | yes | no | no | --host default for web. |
| `ROTARI_WEB_PORT` | yes | no | no | --port default for web. |
| `ROTARI_WEB_STATIC_DIR` | yes | no | no | --static-dir default for web. |
| `ROTARI_WEB_ALLOW_CONTROL` | yes | no | no | --allow-control default for web. |
| `ROTARI_WEB_AUTH_TOKEN` | yes | no | no | --auth-token default for web; never exposed by the Web UI. |
| `ROTARI_WEB_NOTIFICATIONS` | yes | no | no | --notifications default for web. |
| `ROTARI_LLM_API_KEY` | no | no | no | API key for the diagnose command; never persisted or passed to jobs. |
| `ROTARI_LLM_PROVIDER` | yes | no | no | LLM provider (openai, openai-chat, anthropic, gemini, or cohere); --provider default for diagnose. |
| `ROTARI_LLM_ENDPOINT` | yes | no | no | LLM API endpoint; --endpoint default for diagnose. |
| `ROTARI_LLM_MODEL` | yes | no | no | Model name; --model default for diagnose. |
| `ROTARI_LLM_LANGUAGE` | yes | no | no | BCP 47 response language tag; --language default for diagnose. |
| `ROTARI_WEBHOOK_URL` | no | no | no | Webhook URL; overrides webhook.url in notifications.toml. |
| `ROTARI_PRIVATE_STATE` | no | no | no | set to true for 0700/0600 state directory permissions instead of the default 0755/0644 (shared state). |

<!-- END GENERATED ENVIRONMENT REFERENCE -->
