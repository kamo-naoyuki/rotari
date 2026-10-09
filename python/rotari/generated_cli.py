"""Generated from `rotari schema --json`; do not edit manually."""

from __future__ import annotations

from typing import Any

CLI_SCHEMA: dict[str, Any] = {
    "commands": [
        {
            "description": "show the current Rotari context and active run state",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "description": "print machine-readable JSON",
                    "environment": "ROTARI_INFO_JSON",
                    "name": "json",
                },
            ],
            "name": "info",
        },
        {
            "description": "write cwd workspace location defaults without creating "
            "state",
            "name": "init",
            "positional": "[BASEDIR [PROJECT]]",
        },
        {
            "description": "generate a config file template",
            "flags": [
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "list existing config files",
                    "environment": "ROTARI_CONFIG_LIST",
                    "name": "list",
                },
                {
                    "command_line_only": True,
                    "description": "generate notifications.toml instead of "
                    "command defaults",
                    "name": "notifications",
                },
                {
                    "description": "config format: yaml, toml, or json",
                    "environment": "ROTARI_CONFIG_FORMAT",
                    "name": "format",
                    "value_name": "FORMAT",
                    "values": ["yaml", "toml", "json"],
                },
                {
                    "description": "output config file path",
                    "environment": "ROTARI_CONFIG_OUTPUT",
                    "name": "output",
                    "value_name": "FILE",
                },
            ],
            "name": "config",
        },
        {
            "description": "check whether a project is ready to run",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "print machine-readable JSON",
                    "environment": "ROTARI_CHECK_JSON",
                    "name": "json",
                },
                {
                    "description": "check local executables and working directories",
                    "environment": "ROTARI_CHECK_DEEP",
                    "name": "deep",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_CHECK_QUIET",
                    "name": "quiet",
                },
            ],
            "name": "check",
            "positional": "[PROJECT]",
        },
        {
            "description": "discard the current, not-yet-run queue",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_RESET_QUIET",
                    "name": "quiet",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "reset",
            "positional": "[PROJECT]",
        },
        {
            "description": "cancel the active run or running jobs",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "cancel a running job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "cancel the unfinished jobs with this name; "
                    "may be repeated",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "cancel the unfinished jobs of this stage",
                    "environment": "ROTARI_CANCEL_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "cancel the unfinished jobs of this matrix",
                    "environment": "ROTARI_CANCEL_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "cancel the whole run and wait until it has "
                    "stopped; not with a job selection, after "
                    "which rotari wait follows the run",
                    "environment": "ROTARI_CANCEL_WAIT",
                    "name": "wait",
                },
                {
                    "command_line_only": True,
                    "description": "cancel the jobs that filters select without asking",
                    "name": "yes",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running", "pending"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
            ],
            "name": "cancel",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "suspend running jobs",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "suspend a running job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "suspend the running jobs with this name; may "
                    "be repeated",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "suspend the running jobs of this stage",
                    "environment": "ROTARI_SUSPEND_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "suspend the running jobs of this matrix",
                    "environment": "ROTARI_SUSPEND_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "suspend the jobs that filters select without "
                    "asking",
                    "name": "yes",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
            ],
            "name": "suspend",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "resume suspended jobs",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "resume a suspended job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "resume the suspended jobs with this name; "
                    "may be repeated",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "resume the suspended jobs of this stage",
                    "environment": "ROTARI_RESUME_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "resume the suspended jobs of this matrix",
                    "environment": "ROTARI_RESUME_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "resume the jobs that filters select without asking",
                    "name": "yes",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
            ],
            "name": "resume",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "delete saved run history",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "run to delete",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "command_line_only": True,
                    "description": "delete every run of the project",
                    "name": "all",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "delete",
            "positional": "[RUN_ID]",
        },
        {
            "description": "find and remove orphan run registry entries",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "command_line_only": True,
                    "description": "list the orphan entries without removing them",
                    "name": "dry-run",
                },
            ],
            "name": "gc",
            "positional": "[MASTERDIR]",
        },
        {
            "description": "remove a confirmed stale run lock",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "verify the run ID recorded in the stale lock",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
            ],
            "name": "unlock",
            "positional": "[PROJECT]",
        },
        {
            "description": "change queued jobs, or with --run-id the jobs of that "
            "run, which replace the queue first; a command after the "
            "options replaces the jobs' command",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "first replace the queue with this run's "
                    "jobs, then change them",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "target job ID",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "target job name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "change every job in a stage",
                    "environment": "ROTARI_CHANGE_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "change every job of a matrix, named by its "
                    "base job name",
                    "environment": "ROTARI_CHANGE_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "change every job",
                    "name": "all",
                },
                {
                    "description": "replace job executor",
                    "environment": "ROTARI_EXECUTOR",
                    "name": "executor",
                    "short": "e",
                    "value_name": "EXECUTOR",
                    "values": ["local", "lsf", "pbs", "sge", "slurm", "ssh"],
                },
                {
                    "description": "replace executor options",
                    "environment": "ROTARI_EXECUTOR_OPTIONS",
                    "name": "executor-option",
                    "value_name": "OPTION",
                },
                {
                    "description": "clear executor options",
                    "environment": "ROTARI_CHANGE_CLEAR_EXECUTOR_OPTIONS",
                    "name": "clear-executor-options",
                },
                {
                    "description": "working directory for the job",
                    "environment": "ROTARI_CHANGE_WORKING_DIRECTORY",
                    "name": "working-directory",
                    "value_name": "DIR",
                },
                {
                    "description": "clear the job working directory",
                    "environment": "ROTARI_CHANGE_CLEAR_WORKING_DIRECTORY",
                    "name": "clear-working-directory",
                },
                {
                    "description": "replace job environment variables; may be repeated",
                    "environment": "ROTARI_CHANGE_ENV",
                    "name": "env",
                    "repeated": True,
                    "value_name": "KEY=VALUE",
                },
                {
                    "description": "clear job environment variables",
                    "environment": "ROTARI_CHANGE_CLEAR_ENV",
                    "name": "clear-env",
                },
                {
                    "description": "replace job name",
                    "environment": "ROTARI_CHANGE_SET_JOB_NAME",
                    "name": "set-job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "replace prerequisites; may be repeated",
                    "environment": "ROTARI_CHANGE_DEPENDS_ON",
                    "name": "depends-on",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "clear prerequisites",
                    "environment": "ROTARI_CHANGE_CLEAR_DEPENDS_ON",
                    "name": "clear-depends-on",
                },
                {
                    "description": "replace prerequisites that only need to "
                    "finish, whatever their result; may be "
                    "repeated",
                    "environment": "ROTARI_CHANGE_DEPENDS_ON_FINISHED",
                    "name": "depends-on-finished",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "clear prerequisites that only need to finish",
                    "environment": "ROTARI_CHANGE_CLEAR_DEPENDS_ON_FINISHED",
                    "name": "clear-depends-on-finished",
                },
                {
                    "command_line_only": True,
                    "description": "replace the declared artifact paths (see add "
                    "--artifact); may be repeated",
                    "name": "artifact",
                    "repeated": True,
                    "value_name": "PATH",
                },
                {
                    "description": "clear the declared artifact paths",
                    "environment": "ROTARI_CHANGE_CLEAR_ARTIFACTS",
                    "name": "clear-artifacts",
                },
                {
                    "command_line_only": True,
                    "description": "replace the job timeout, such as 90m or 2h",
                    "name": "timeout",
                    "value_name": "DURATION",
                },
                {
                    "description": "remove the job timeout",
                    "environment": "ROTARI_CHANGE_CLEAR_TIMEOUT",
                    "name": "clear-timeout",
                },
                {
                    "command_line_only": True,
                    "description": "replace the job's retry limit; 0 disables retries",
                    "name": "retry",
                    "value_name": "N",
                },
                {
                    "description": "use the run's --retry limit for the job "
                    "again and remove its retry delay settings",
                    "environment": "ROTARI_CHANGE_CLEAR_RETRY",
                    "name": "clear-retry",
                },
                {
                    "description": "replace the wait before the job's first "
                    "retry, such as 30s",
                    "environment": "ROTARI_CHANGE_RETRY_DELAY",
                    "name": "retry-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "replace the factor applied to the retry "
                    "delay for each further retry",
                    "environment": "ROTARI_CHANGE_RETRY_BACKOFF",
                    "name": "retry-backoff",
                    "value_name": "FACTOR",
                },
                {
                    "description": "replace the upper limit of the retry delay",
                    "environment": "ROTARI_CHANGE_RETRY_MAX_DELAY",
                    "name": "retry-max-delay",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "mark the job with a status that result "
                    "filters of the next run read in place of its "
                    "recorded result",
                    "name": "status",
                    "value_name": "STATUS",
                    "values": ["success", "failed", "cancelled", "unfinished"],
                },
                {
                    "description": "remove the job's status mark",
                    "environment": "ROTARI_CHANGE_CLEAR_STATUS",
                    "name": "clear-status",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_CHANGE_QUIET",
                    "name": "quiet",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "change",
            "positional": "<command ...>",
        },
        {
            "description": "export the current queue or saved runs as a workflow "
            "manifest",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "run ID to export; may be repeated",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "repeated": True,
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "manifest format: yaml, toml, or json",
                    "environment": "ROTARI_EXPORT_FORMAT",
                    "name": "format",
                    "value_name": "FORMAT",
                    "values": ["yaml", "toml", "json"],
                },
                {
                    "description": "print a starter workflow manifest",
                    "environment": "ROTARI_EXPORT_TEMPLATE",
                    "name": "template",
                },
                {
                    "command_line_only": True,
                    "description": "write the workflow manifest to a file",
                    "name": "output",
                    "value_name": "FILE",
                },
            ],
            "name": "export",
            "positional": "[TARGET] [FILE]",
        },
        {
            "description": "validate and replace a queue from a workflow manifest",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "replace a non-empty queue",
                    "environment": "ROTARI_IMPORT_OVERWRITE",
                    "name": "overwrite",
                },
                {
                    "description": "print the import plan as JSON",
                    "environment": "ROTARI_IMPORT_JSON",
                    "name": "json",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "import",
            "positional": "FILE [PROJECT]",
        },
        {
            "description": "remove queued jobs, or with --run-id jobs of that run, "
            "which replace the queue first",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "first replace the queue with this run's "
                    "jobs, then remove from them",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "remove a job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "remove a job by name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "remove every job in a stage",
                    "environment": "ROTARI_REMOVE_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "remove every job of a matrix, named by its "
                    "base job name",
                    "environment": "ROTARI_REMOVE_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "remove every job",
                    "name": "all",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_REMOVE_QUIET",
                    "name": "quiet",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "remove",
            "positional": "[JOB_ID ...]",
        },
        {
            "description": "list state directories known to the master registry",
            "flags": [
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                }
            ],
            "name": "basedirs",
        },
        {
            "description": "list projects across known state directories",
            "flags": [
                {
                    "description": "limit the listing to this state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
            ],
            "name": "projects",
        },
        {
            "description": "list active runs and recently finished runs across "
            "known state directories",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "description": "include finished runs in this time window "
                    "(default 1d), such as 24h or 7d; active or "
                    "interrupted runs are always included",
                    "environment": "ROTARI_RUNS_SINCE",
                    "name": "since",
                    "value_name": "DURATION",
                },
                {
                    "description": "print machine-readable JSON",
                    "environment": "ROTARI_RUNS_JSON",
                    "name": "json",
                },
            ],
            "name": "runs",
            "positional": "[PROJECT]",
        },
        {
            "description": "show details for a project, run, job, or attempt",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "run ID or latest",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "show the current queue even when a run is selected",
                    "environment": "ROTARI_SHOW_QUEUE",
                    "name": "queue",
                },
                {
                    "description": "job ID",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "job name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "show failed jobs; may be combined with the "
                    "other result filters",
                    "environment": "ROTARI_SHOW_FAILED",
                    "name": "failed",
                },
                {
                    "description": "show unfinished jobs; may be combined with "
                    "the other result filters",
                    "environment": "ROTARI_SHOW_UNFINISHED",
                    "name": "unfinished",
                },
                {
                    "description": "show successful jobs; may be combined with "
                    "the other result filters",
                    "environment": "ROTARI_SHOW_SUCCESS",
                    "name": "success",
                },
                {
                    "description": "show jobs in this stage only",
                    "environment": "ROTARI_SHOW_STAGE",
                    "name": "stage",
                    "value_name": "NAME",
                },
                {
                    "description": "show jobs of this matrix only, named by its "
                    "base job name",
                    "environment": "ROTARI_SHOW_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "print output logs for all jobs",
                    "environment": "ROTARI_SHOW_LOGS",
                    "name": "logs",
                },
                {
                    "description": "print output logs for failed jobs",
                    "environment": "ROTARI_SHOW_FAILED_LOGS",
                    "name": "failed-logs",
                },
                {
                    "description": "show both streams or select stdout/stderr",
                    "environment": "ROTARI_SHOW_STREAM",
                    "name": "stream",
                    "value_name": "STREAM",
                },
                {
                    "description": "follow one selected log stream until the run "
                    "completes",
                    "environment": "ROTARI_SHOW_FOLLOW",
                    "name": "follow",
                },
                {
                    "description": "print logs directly instead of using a pager",
                    "environment": "ROTARI_SHOW_NO_PAGER",
                    "name": "no-pager",
                },
                {
                    "description": "print machine-readable JSON for a run",
                    "environment": "ROTARI_SHOW_JSON",
                    "name": "json",
                },
                {
                    "description": "print an AI-ready Markdown report",
                    "environment": "ROTARI_SHOW_REPORT",
                    "name": "report",
                },
                {
                    "description": "list every artifact candidate of one job "
                    "attempt instead of its logs",
                    "environment": "ROTARI_SHOW_ARTIFACTS",
                    "name": "artifacts",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this failure kind; may be repeated",
                    "name": "filter-failure-kind",
                    "repeated": True,
                    "value_name": "KIND",
                    "values": [
                        "timeout",
                        "cancelled",
                        "blocked",
                        "oom",
                        "signal",
                        "error",
                    ],
                },
                {
                    "command_line_only": True,
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
            ],
            "name": "show",
            "positional": "[SELECTOR]",
        },
        {
            "description": "list runs, summarize one run, or compare two runs",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "print the lineage, summary, or comparison as JSON",
                    "environment": "ROTARI_LINEAGE_JSON",
                    "name": "json",
                },
            ],
            "name": "lineage",
            "positional": "[RUN_ID ...]",
        },
        {
            "description": "list running and recently finished jobs across known "
            "state directories",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "description": "output fields; use %s %b %p %a %n %c %t %f "
                    "%e (%f is finished time)",
                    "environment": "ROTARI_JOBS_FORMAT",
                    "name": "format",
                    "value_name": "FORMAT",
                },
                {
                    "description": "include jobs finished within this duration "
                    "(default 1d), such as 24h or 7d; use 0 for "
                    "running jobs only",
                    "environment": "ROTARI_JOBS_SINCE",
                    "name": "since",
                    "value_name": "DURATION",
                },
                {
                    "description": "print machine-readable JSON; not with --format",
                    "environment": "ROTARI_JOBS_JSON",
                    "name": "json",
                },
            ],
            "name": "jobs",
            "positional": "[PROJECT]",
        },
        {
            "description": "wait for a run by project, run name, or run ID",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "when the wait client's input closes, detach "
                    "or cancel selected runs (default detach)",
                    "environment": "ROTARI_DISCONNECT_ACTION",
                    "name": "disconnect-action",
                    "value_name": "ACTION",
                    "values": ["detach", "cancel"],
                },
                {
                    "description": "run ID; may be repeated",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "repeated": True,
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "maximum wait duration",
                    "environment": "ROTARI_WAIT_TIMEOUT",
                    "name": "timeout",
                    "value_name": "DURATION",
                },
                {
                    "description": "return as soon as a job of the run has "
                    "failed with no retry left, without waiting "
                    "for the rest",
                    "environment": "ROTARI_WAIT_UNTIL_FAILURE",
                    "name": "until-failure",
                },
                {
                    "description": "print each completed run as one JSON object",
                    "environment": "ROTARI_WAIT_JSON",
                    "name": "json",
                },
                {
                    "description": "suppress normal progress and completion "
                    "output; keep failure diagnostics and JSON "
                    "results",
                    "environment": "ROTARI_WAIT_QUIET",
                    "name": "quiet",
                },
            ],
            "name": "wait",
            "positional": "[PROJECT_OR_RUN_NAME_OR_RUN_ID ...]",
        },
        {
            "description": "add a command to a queue",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "job executor",
                    "environment": "ROTARI_EXECUTOR",
                    "name": "executor",
                    "short": "e",
                    "value_name": "EXECUTOR",
                    "values": ["local", "lsf", "pbs", "sge", "slurm", "ssh"],
                },
                {
                    "description": "option passed to the selected scheduler "
                    "(sbatch/qsub/...); may be repeated",
                    "environment": "ROTARI_EXECUTOR_OPTIONS",
                    "name": "executor-option",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "command_line_only": True,
                    "description": "stdout destination; stderr also goes here "
                    "unless --error is specified; may be repeated",
                    "name": "output",
                    "repeated": True,
                    "value_name": "FILE",
                },
                {
                    "command_line_only": True,
                    "description": "stderr destination; defaults to --output "
                    "destinations; may be repeated",
                    "name": "error",
                    "repeated": True,
                    "value_name": "FILE",
                },
                {
                    "command_line_only": True,
                    "description": "a file or directory the job writes or reads, "
                    "recorded as an artifact candidate of each "
                    "attempt; $ROTARI_ARRAY_TASK_ID, "
                    "$ROTARI_JOB_DIR, and the job's --env and "
                    "matrix variables expand; may be repeated",
                    "name": "artifact",
                    "repeated": True,
                    "value_name": "PATH",
                },
                {
                    "command_line_only": True,
                    "description": "internal log mode",
                    "name": "log-mode",
                    "value_name": "MODE",
                    "values": ["merge", "separate"],
                },
                {
                    "command_line_only": True,
                    "description": "external output file mode",
                    "name": "open-mode",
                    "value_name": "MODE",
                    "values": ["append", "truncate"],
                },
                {
                    "description": "working directory for the job",
                    "environment": "ROTARI_ADD_WORKING_DIRECTORY",
                    "name": "working-directory",
                    "value_name": "DIR",
                },
                {
                    "description": "environment variable for the job; may be repeated",
                    "environment": "ROTARI_ADD_ENV",
                    "name": "env",
                    "repeated": True,
                    "value_name": "KEY=VALUE",
                },
                {
                    "description": "job name label",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "stage that contains the job",
                    "environment": "ROTARI_ADD_STAGE",
                    "name": "stage",
                    "value_name": "NAME",
                },
                {
                    "description": "name of a prerequisite job or stage; may be "
                    "repeated",
                    "environment": "ROTARI_ADD_DEPENDS_ON",
                    "name": "depends-on",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "name of a prerequisite job or stage that "
                    "must finish, whatever its result; may be "
                    "repeated",
                    "environment": "ROTARI_ADD_DEPENDS_ON_FINISHED",
                    "name": "depends-on-finished",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "stop the job this long after it starts, such "
                    "as 90m or 2h; it then fails with exit code "
                    "124",
                    "name": "timeout",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "retry the job up to N times when it fails, "
                    "instead of the run's --retry; 0 disables "
                    "retries",
                    "name": "retry",
                    "value_name": "N",
                },
                {
                    "description": "wait this long before the job's first retry, "
                    "such as 30s; retries are immediate by "
                    "default",
                    "environment": "ROTARI_ADD_RETRY_DELAY",
                    "name": "retry-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "multiply the retry delay by this factor for "
                    "each further retry, such as 2",
                    "environment": "ROTARI_ADD_RETRY_BACKOFF",
                    "name": "retry-backoff",
                    "value_name": "FACTOR",
                },
                {
                    "description": "upper limit of the retry delay, such as 10m",
                    "environment": "ROTARI_ADD_RETRY_MAX_DELAY",
                    "name": "retry-max-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "create an array job range or selected tasks",
                    "environment": "ROTARI_ARRAY_RANGE",
                    "name": "array",
                    "value_name": "FIRST-LAST|TASK[,TASK...]",
                },
                {
                    "description": "expand a command into jobs from "
                    "KEY=VALUE[,VALUE...] dimensions; may be "
                    "repeated",
                    "environment": "ROTARI_ADD_MATRIX",
                    "name": "matrix",
                    "repeated": True,
                    "value_name": "KEY=VALUE[,VALUE...]",
                },
                {
                    "command_line_only": True,
                    "description": "exclude matrix combinations matching "
                    "KEY=VALUE[,KEY=VALUE...]; requires --matrix "
                    "and may be repeated",
                    "name": "matrix-exclude",
                    "repeated": True,
                    "value_name": "KEY=VALUE[,KEY=VALUE...]",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_ADD_QUIET",
                    "name": "quiet",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "add",
            "positional": "<command ...>",
        },
        {
            "description": "copy the latest run's jobs into the queue",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "source run ID; defaults to the latest run",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "include failed jobs; may be combined with "
                    "result filters",
                    "environment": "ROTARI_COPY_FAILED",
                    "name": "failed",
                },
                {
                    "description": "include unfinished jobs; may be combined "
                    "with result filters",
                    "environment": "ROTARI_COPY_UNFINISHED",
                    "name": "unfinished",
                },
                {
                    "description": "include successful jobs; may be combined "
                    "with result filters",
                    "environment": "ROTARI_COPY_SUCCESS",
                    "name": "success",
                },
                {
                    "description": "copy a job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "copy a job by name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "only copy jobs in this stage, narrowed by "
                    "any result filter",
                    "environment": "ROTARI_COPY_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only copy jobs of this matrix, named by its "
                    "base job name, narrowed by any result filter",
                    "environment": "ROTARI_COPY_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "append to a non-empty queue",
                    "environment": "ROTARI_COPY_APPEND",
                    "name": "append",
                },
                {
                    "description": "replace a non-empty queue",
                    "environment": "ROTARI_COPY_OVERWRITE",
                    "name": "overwrite",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_COPY_QUIET",
                    "name": "quiet",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this failure kind; may be repeated",
                    "name": "filter-failure-kind",
                    "repeated": True,
                    "value_name": "KIND",
                    "values": [
                        "timeout",
                        "cancelled",
                        "blocked",
                        "oom",
                        "signal",
                        "error",
                    ],
                },
                {
                    "command_line_only": True,
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "copy",
            "positional": "[RUN_ID]",
        },
        {
            "description": "execute queued commands, optionally selecting jobs from "
            "a run; jobs that depend on a selected job execute too",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "build the run from this saved run without "
                    "changing the next queue; without it, a "
                    "result filter copies the latest run only "
                    "into an empty queue and otherwise uses the "
                    "queued jobs",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "run name label",
                    "environment": "ROTARI_RUN_NAME",
                    "name": "run-name",
                    "value_name": "NAME",
                },
                {
                    "description": "local worker concurrency",
                    "environment": "ROTARI_RUN_LOCAL_CONCURRENCY",
                    "name": "local-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "scheduler job concurrency (Slurm/PBS/LSF/SGE)",
                    "environment": "ROTARI_RUN_BATCH_CONCURRENCY",
                    "name": "batch-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "retry failed jobs up to N times; explicit "
                    "cancellations are not retried",
                    "environment": "ROTARI_RUN_RETRY",
                    "name": "retry",
                    "value_name": "N",
                },
                {
                    "description": "only execute failed jobs; others carry "
                    "forward their previous result",
                    "environment": "ROTARI_RUN_FAILED",
                    "name": "failed",
                },
                {
                    "description": "only execute unfinished jobs; others carry "
                    "forward their previous result",
                    "environment": "ROTARI_RUN_UNFINISHED",
                    "name": "unfinished",
                },
                {
                    "description": "only execute successful jobs; others carry "
                    "forward their previous result",
                    "environment": "ROTARI_RUN_SUCCESS",
                    "name": "success",
                },
                {
                    "description": "only execute this job and the jobs that "
                    "depend on it, through --depends-on or "
                    "--depends-on-finished; may be repeated; not "
                    "with a result filter; others carry forward "
                    "their previous result",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "only execute this job by name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "only execute jobs in this stage, narrowed by "
                    "any result filter; others carry forward "
                    "their previous result",
                    "environment": "ROTARI_RUN_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only execute jobs of this matrix, named by "
                    "its base job name, narrowed by any result "
                    "filter; others carry forward their previous "
                    "result",
                    "environment": "ROTARI_RUN_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "with a result filter, select array jobs per "
                    "task instead of all-or-nothing (default "
                    "true); pass =false to re-execute the whole "
                    "array when any task matches",
                    "environment": "ROTARI_RUN_PARTIAL_ARRAY",
                    "name": "partial-array",
                },
                {
                    "description": "return after starting the run",
                    "environment": "ROTARI_RUN_ASYNC",
                    "name": "async",
                },
                {
                    "description": "when the client input or connection closes "
                    "unexpectedly, detach or cancel (default "
                    "detach)",
                    "environment": "ROTARI_DISCONNECT_ACTION",
                    "name": "disconnect-action",
                    "value_name": "ACTION",
                    "values": ["detach", "cancel"],
                },
                {
                    "description": "suppress progress and completion output",
                    "environment": "ROTARI_RUN_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "execution executor override",
                    "environment": "ROTARI_EXECUTOR",
                    "name": "executor",
                    "short": "e",
                    "value_name": "EXECUTOR",
                    "values": ["local", "lsf", "pbs", "sge", "slurm", "ssh"],
                },
                {
                    "command_line_only": True,
                    "description": "caller environment propagation mode (default ALL)",
                    "name": "env",
                    "value_name": "ALL|NONE",
                    "values": ["ALL", "NONE"],
                },
                {
                    "command_line_only": True,
                    "description": "job identity matching",
                    "name": "match-by",
                    "value_name": "MODE",
                    "values": ["job-id", "fingerprint", "id-and-fingerprint"],
                },
                {
                    "description": "option passed to the selected scheduler "
                    "(sbatch/qsub/...); may be repeated",
                    "environment": "ROTARI_EXECUTOR_OPTIONS",
                    "name": "executor-option",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "SSH executor concurrency",
                    "environment": "ROTARI_RUN_SSH_CONCURRENCY",
                    "name": "ssh-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "SSH executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SSH_OPTIONS",
                    "name": "ssh-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "Slurm executor concurrency",
                    "environment": "ROTARI_RUN_SLURM_CONCURRENCY",
                    "name": "slurm-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "Slurm executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SLURM_OPTIONS",
                    "name": "slurm-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum Slurm submission interval",
                    "environment": "ROTARI_RUN_SLURM_SUBMIT_INTERVAL",
                    "name": "slurm-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient Slurm "
                    "submission failures",
                    "environment": "ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT",
                    "name": "slurm-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "PBS executor concurrency",
                    "environment": "ROTARI_RUN_PBS_CONCURRENCY",
                    "name": "pbs-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "PBS executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_PBS_OPTIONS",
                    "name": "pbs-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum PBS submission interval",
                    "environment": "ROTARI_RUN_PBS_SUBMIT_INTERVAL",
                    "name": "pbs-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient PBS submission "
                    "failures",
                    "environment": "ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT",
                    "name": "pbs-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "LSF executor concurrency",
                    "environment": "ROTARI_RUN_LSF_CONCURRENCY",
                    "name": "lsf-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "LSF executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_LSF_OPTIONS",
                    "name": "lsf-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum LSF submission interval",
                    "environment": "ROTARI_RUN_LSF_SUBMIT_INTERVAL",
                    "name": "lsf-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient LSF submission "
                    "failures",
                    "environment": "ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT",
                    "name": "lsf-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "SGE executor concurrency",
                    "environment": "ROTARI_RUN_SGE_CONCURRENCY",
                    "name": "sge-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "SGE executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SGE_OPTIONS",
                    "name": "sge-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum SGE submission interval",
                    "environment": "ROTARI_RUN_SGE_SUBMIT_INTERVAL",
                    "name": "sge-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient SGE submission "
                    "failures",
                    "environment": "ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT",
                    "name": "sge-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this failure kind; may be repeated",
                    "name": "filter-failure-kind",
                    "repeated": True,
                    "value_name": "KIND",
                    "values": [
                        "timeout",
                        "cancelled",
                        "blocked",
                        "oom",
                        "signal",
                        "error",
                    ],
                },
                {
                    "command_line_only": True,
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "run",
            "positional": "[RUN_ID]",
        },
        {
            "description": "run failed and unfinished jobs; with --job-id, run "
            "those jobs; jobs that depend on them execute too",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "project name",
                    "environment": "ROTARI_PROJECT_NAME",
                    "name": "project-name",
                    "short": "p",
                    "value_name": "NAME",
                },
                {
                    "description": "build the retry from this saved run without "
                    "changing the next queue; without it, retry "
                    "copies the latest run only into an empty "
                    "queue and otherwise uses the queued jobs",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "run name label",
                    "environment": "ROTARI_RUN_NAME",
                    "name": "run-name",
                    "value_name": "NAME",
                },
                {
                    "description": "local worker concurrency",
                    "environment": "ROTARI_RUN_LOCAL_CONCURRENCY",
                    "name": "local-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "scheduler job concurrency (Slurm/PBS/LSF/SGE)",
                    "environment": "ROTARI_RUN_BATCH_CONCURRENCY",
                    "name": "batch-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "retry failed jobs up to N times; explicit "
                    "cancellations are not retried",
                    "environment": "ROTARI_RUN_RETRY",
                    "name": "retry",
                    "value_name": "N",
                },
                {
                    "description": "only execute failed jobs, instead of failed "
                    "and unfinished jobs; others carry forward "
                    "their previous result",
                    "environment": "ROTARI_RUN_FAILED",
                    "name": "failed",
                },
                {
                    "description": "only execute unfinished jobs, instead of "
                    "failed and unfinished jobs; others carry "
                    "forward their previous result",
                    "environment": "ROTARI_RUN_UNFINISHED",
                    "name": "unfinished",
                },
                {
                    "description": "only execute successful jobs, instead of "
                    "failed and unfinished jobs; others carry "
                    "forward their previous result",
                    "environment": "ROTARI_RUN_SUCCESS",
                    "name": "success",
                },
                {
                    "description": "only execute this job and the jobs that "
                    "depend on it, through --depends-on or "
                    "--depends-on-finished, instead of failed and "
                    "unfinished jobs; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "only execute this job by name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "only retry jobs in this stage",
                    "environment": "ROTARI_RUN_STAGE",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only retry jobs of this matrix, named by its "
                    "base job name",
                    "environment": "ROTARI_RUN_MATRIX",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "with a result filter, select array jobs per "
                    "task instead of all-or-nothing (default "
                    "true); pass =false to re-execute the whole "
                    "array when any task matches",
                    "environment": "ROTARI_RUN_PARTIAL_ARRAY",
                    "name": "partial-array",
                },
                {
                    "description": "return after starting the run",
                    "environment": "ROTARI_RUN_ASYNC",
                    "name": "async",
                },
                {
                    "description": "when the client input or connection closes "
                    "unexpectedly, detach or cancel (default "
                    "detach)",
                    "environment": "ROTARI_DISCONNECT_ACTION",
                    "name": "disconnect-action",
                    "value_name": "ACTION",
                    "values": ["detach", "cancel"],
                },
                {
                    "description": "suppress progress and completion output",
                    "environment": "ROTARI_RUN_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "execution executor override",
                    "environment": "ROTARI_EXECUTOR",
                    "name": "executor",
                    "short": "e",
                    "value_name": "EXECUTOR",
                    "values": ["local", "lsf", "pbs", "sge", "slurm", "ssh"],
                },
                {
                    "command_line_only": True,
                    "description": "caller environment propagation mode (default ALL)",
                    "name": "env",
                    "value_name": "ALL|NONE",
                    "values": ["ALL", "NONE"],
                },
                {
                    "command_line_only": True,
                    "description": "job identity matching",
                    "name": "match-by",
                    "value_name": "MODE",
                    "values": ["job-id", "fingerprint", "id-and-fingerprint"],
                },
                {
                    "description": "option passed to the selected scheduler "
                    "(sbatch/qsub/...); may be repeated",
                    "environment": "ROTARI_EXECUTOR_OPTIONS",
                    "name": "executor-option",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "SSH executor concurrency",
                    "environment": "ROTARI_RUN_SSH_CONCURRENCY",
                    "name": "ssh-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "SSH executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SSH_OPTIONS",
                    "name": "ssh-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "Slurm executor concurrency",
                    "environment": "ROTARI_RUN_SLURM_CONCURRENCY",
                    "name": "slurm-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "Slurm executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SLURM_OPTIONS",
                    "name": "slurm-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum Slurm submission interval",
                    "environment": "ROTARI_RUN_SLURM_SUBMIT_INTERVAL",
                    "name": "slurm-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient Slurm "
                    "submission failures",
                    "environment": "ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT",
                    "name": "slurm-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "PBS executor concurrency",
                    "environment": "ROTARI_RUN_PBS_CONCURRENCY",
                    "name": "pbs-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "PBS executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_PBS_OPTIONS",
                    "name": "pbs-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum PBS submission interval",
                    "environment": "ROTARI_RUN_PBS_SUBMIT_INTERVAL",
                    "name": "pbs-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient PBS submission "
                    "failures",
                    "environment": "ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT",
                    "name": "pbs-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "LSF executor concurrency",
                    "environment": "ROTARI_RUN_LSF_CONCURRENCY",
                    "name": "lsf-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "LSF executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_LSF_OPTIONS",
                    "name": "lsf-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum LSF submission interval",
                    "environment": "ROTARI_RUN_LSF_SUBMIT_INTERVAL",
                    "name": "lsf-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient LSF submission "
                    "failures",
                    "environment": "ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT",
                    "name": "lsf-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "description": "SGE executor concurrency",
                    "environment": "ROTARI_RUN_SGE_CONCURRENCY",
                    "name": "sge-concurrency",
                    "value_name": "N",
                },
                {
                    "description": "SGE executor dispatch options; may be repeated",
                    "environment": "ROTARI_RUN_SGE_OPTIONS",
                    "name": "sge-options",
                    "repeated": True,
                    "value_name": "OPTION",
                },
                {
                    "description": "minimum SGE submission interval",
                    "environment": "ROTARI_RUN_SGE_SUBMIT_INTERVAL",
                    "name": "sge-submit-interval",
                    "value_name": "DURATION",
                },
                {
                    "description": "maximum retries for transient SGE submission "
                    "failures",
                    "environment": "ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT",
                    "name": "sge-submit-retry-limit",
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "command_line_only": True,
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this failure kind; may be repeated",
                    "name": "filter-failure-kind",
                    "repeated": True,
                    "value_name": "KIND",
                    "values": [
                        "timeout",
                        "cancelled",
                        "blocked",
                        "oom",
                        "signal",
                        "error",
                    ],
                },
                {
                    "command_line_only": True,
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "command_line_only": True,
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "command_line_only": True,
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
                    "command_line_only": True,
                    "description": "apply only if the project is still at this "
                    "revision, as printed by --dry-run",
                    "name": "if-revision",
                    "value_name": "REVISION",
                },
            ],
            "name": "retry",
            "positional": "[RUN_ID]",
        },
        {
            "description": "manage the background server",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "server registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
            ],
            "name": "server",
            "subcommands": [
                {"description": "show server status", "name": "status"},
                {"description": "list servers", "name": "list"},
                {"description": "shut down server", "name": "shutdown"},
            ],
        },
        {
            "description": "serve the web status UI",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "state directory",
                    "environment": "ROTARI_BASEDIR",
                    "name": "basedir",
                    "short": "b",
                    "value_name": "DIR",
                },
                {
                    "description": "HTTP listen host (live server only; cannot "
                    "be combined with --static-dir)",
                    "environment": "ROTARI_WEB_HOST",
                    "name": "host",
                    "value_name": "HOST",
                },
                {
                    "description": "HTTP listen port (live server only; cannot "
                    "be combined with --static-dir)",
                    "environment": "ROTARI_WEB_PORT",
                    "name": "port",
                    "value_name": "PORT",
                },
                {
                    "description": "generate a static web UI",
                    "environment": "ROTARI_WEB_STATIC_DIR",
                    "name": "static-dir",
                    "value_name": "DIR",
                },
                {
                    "description": "enable job control "
                    "(copy/change/remove/cancel/clear) in the "
                    "live server; false makes it read-only; "
                    "incompatible with --static-dir",
                    "environment": "ROTARI_WEB_ALLOW_CONTROL",
                    "name": "allow-control",
                },
                {
                    "description": "require this token in Authorization: Bearer "
                    "or X-Rotari-Token on the live server; cannot "
                    "be combined with --static-dir; prefer "
                    "ROTARI_WEB_AUTH_TOKEN for secrets",
                    "environment": "ROTARI_WEB_AUTH_TOKEN",
                    "name": "auth-token",
                    "value_name": "TOKEN",
                },
                {
                    "description": "default state of the browser "
                    "desktop-notification toggle; pass "
                    "--notifications=false to default it off",
                    "environment": "ROTARI_WEB_NOTIFICATIONS",
                    "name": "notifications",
                },
                {
                    "command_line_only": True,
                    "description": "copy previewable artifact files (up to 10 "
                    "MiB each, 100 MiB in all) into the static "
                    "export so previews work there; requires "
                    "--static-dir",
                    "name": "static-artifact-contents",
                },
                {
                    "command_line_only": True,
                    "description": "also serve recorded artifact files under "
                    "this directory, besides each job's working "
                    "directory (live server only; may be "
                    "repeated; cannot be combined with "
                    "--static-dir)",
                    "name": "artifact-root",
                    "repeated": True,
                    "value_name": "DIR",
                },
            ],
            "name": "web",
        },
        {
            "description": "serve rotari's tools for agents over MCP on stdio",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "config file to use",
                    "name": "config",
                    "value_name": "FILE",
                },
                {
                    "description": "master registry directory whose runs and "
                    "basedirs the tools serve",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
            ],
            "name": "mcp",
        },
        {
            "description": "print shell completion script",
            "name": "completion",
            "subcommands": [
                {"description": "bash completion", "name": "bash"},
                {"description": "zsh completion", "name": "zsh"},
                {"description": "fish completion", "name": "fish"},
                {
                    "description": "install completion for the current shell",
                    "name": "install",
                },
            ],
        },
        {
            "description": "print the CLI schema as JSON",
            "flags": [
                {
                    "command_line_only": True,
                    "description": "print the schema as JSON",
                    "name": "json",
                }
            ],
            "name": "schema",
        },
        {"description": "print a usage guide for coding agents", "name": "guide"},
        {"description": "print version", "name": "version"},
        {"description": "list ROTARI environment variables", "name": "env"},
    ],
    "environments": [
        {
            "array": True,
            "cli_default": True,
            "description": "State directory; --basedir default.",
            "job": True,
            "name": "ROTARI_BASEDIR",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Project name; --project-name default.",
            "job": True,
            "name": "ROTARI_PROJECT_NAME",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Server registry directory; --masterdir default.",
            "job": False,
            "name": "ROTARI_MASTERDIR",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Current run ID; --run-id default.",
            "job": True,
            "name": "ROTARI_RUN_ID",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Current job ID; --job-id default.",
            "job": True,
            "name": "ROTARI_JOB_ID",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Current job attempt ID.",
            "job": True,
            "name": "ROTARI_ATTEMPT_ID",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Current job name; --job-name default.",
            "job": True,
            "name": "ROTARI_JOB_NAME",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Current executor; --executor default.",
            "job": True,
            "name": "ROTARI_EXECUTOR",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Default scheduler executor options.",
            "job": True,
            "name": "ROTARI_EXECUTOR_OPTIONS",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Run name; --run-name default.",
            "job": True,
            "name": "ROTARI_RUN_NAME",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Local worker limit; --local-concurrency default.",
            "job": True,
            "name": "ROTARI_RUN_LOCAL_CONCURRENCY",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Scheduler submission limit; --batch-concurrency default.",
            "job": True,
            "name": "ROTARI_RUN_BATCH_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SSH worker limit; --ssh-concurrency default.",
            "job": False,
            "name": "ROTARI_RUN_SSH_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SSH dispatch options; --ssh-options default.",
            "job": False,
            "name": "ROTARI_RUN_SSH_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Slurm worker limit; --slurm-concurrency default.",
            "job": False,
            "name": "ROTARI_RUN_SLURM_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Slurm dispatch options; --slurm-options default.",
            "job": False,
            "name": "ROTARI_RUN_SLURM_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Slurm submit interval; --slurm-submit-interval default.",
            "job": False,
            "name": "ROTARI_RUN_SLURM_SUBMIT_INTERVAL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Slurm transient submit retry limit; "
            "--slurm-submit-retry-limit default.",
            "job": False,
            "name": "ROTARI_RUN_SLURM_SUBMIT_RETRY_LIMIT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "PBS worker limit; --pbs-concurrency default.",
            "job": False,
            "name": "ROTARI_RUN_PBS_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "PBS dispatch options; --pbs-options default.",
            "job": False,
            "name": "ROTARI_RUN_PBS_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "PBS submit interval; --pbs-submit-interval default.",
            "job": False,
            "name": "ROTARI_RUN_PBS_SUBMIT_INTERVAL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "PBS transient submit retry limit; "
            "--pbs-submit-retry-limit default.",
            "job": False,
            "name": "ROTARI_RUN_PBS_SUBMIT_RETRY_LIMIT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LSF worker limit; --lsf-concurrency default.",
            "job": False,
            "name": "ROTARI_RUN_LSF_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LSF dispatch options; --lsf-options default.",
            "job": False,
            "name": "ROTARI_RUN_LSF_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LSF submit interval; --lsf-submit-interval default.",
            "job": False,
            "name": "ROTARI_RUN_LSF_SUBMIT_INTERVAL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LSF transient submit retry limit; "
            "--lsf-submit-retry-limit default.",
            "job": False,
            "name": "ROTARI_RUN_LSF_SUBMIT_RETRY_LIMIT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SGE worker limit; --sge-concurrency default.",
            "job": False,
            "name": "ROTARI_RUN_SGE_CONCURRENCY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SGE dispatch options; --sge-options default.",
            "job": False,
            "name": "ROTARI_RUN_SGE_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SGE submit interval; --sge-submit-interval default.",
            "job": False,
            "name": "ROTARI_RUN_SGE_SUBMIT_INTERVAL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "SGE transient submit retry limit; "
            "--sge-submit-retry-limit default.",
            "job": False,
            "name": "ROTARI_RUN_SGE_SUBMIT_RETRY_LIMIT",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Retry count; --retry default.",
            "job": True,
            "name": "ROTARI_RUN_RETRY",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Async run mode; --async default.",
            "job": True,
            "name": "ROTARI_RUN_ASYNC",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default action when a synchronous run or wait "
            "client disconnects; detach or cancel.",
            "job": False,
            "name": "ROTARI_DISCONNECT_ACTION",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Global quiet mode; --quiet default.",
            "job": True,
            "name": "ROTARI_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Add command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_ADD_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Copy command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_COPY_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Change command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_CHANGE_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Remove command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_REMOVE_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Reset command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_RESET_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Check command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_CHECK_QUIET",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Run-specific quiet mode; --quiet default.",
            "job": True,
            "name": "ROTARI_RUN_QUIET",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Wait command quiet mode; --quiet default.",
            "job": False,
            "name": "ROTARI_WAIT_QUIET",
        },
        {
            "array": True,
            "cli_default": True,
            "description": "Array range; --array default.",
            "job": True,
            "name": "ROTARI_ARRAY_RANGE",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Absolute path to the rotari binary.",
            "job": True,
            "name": "ROTARI_BIN",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Current run directory.",
            "job": True,
            "name": "ROTARI_RUN_DIR",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Current job directory.",
            "job": True,
            "name": "ROTARI_JOB_DIR",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Working directory from which the run started.",
            "job": True,
            "name": "ROTARI_CWD",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Current array task number.",
            "job": False,
            "name": "ROTARI_ARRAY_TASK_ID",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "First array task number.",
            "job": False,
            "name": "ROTARI_ARRAY_FIRST",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Last array task number.",
            "job": False,
            "name": "ROTARI_ARRAY_LAST",
        },
        {
            "array": True,
            "cli_default": False,
            "description": "Number of tasks in the array.",
            "job": False,
            "name": "ROTARI_ARRAY_SIZE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--timeout default for wait.",
            "job": False,
            "name": "ROTARI_WAIT_TIMEOUT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--host default for web.",
            "job": False,
            "name": "ROTARI_WEB_HOST",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--port default for web.",
            "job": False,
            "name": "ROTARI_WEB_PORT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--static-dir default for web.",
            "job": False,
            "name": "ROTARI_WEB_STATIC_DIR",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--allow-control default for web.",
            "job": False,
            "name": "ROTARI_WEB_ALLOW_CONTROL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--auth-token default for web; never exposed by the Web UI.",
            "job": False,
            "name": "ROTARI_WEB_AUTH_TOKEN",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "--notifications default for web.",
            "job": False,
            "name": "ROTARI_WEB_NOTIFICATIONS",
        },
        {
            "array": False,
            "cli_default": False,
            "description": "Webhook URL; overrides webhook.url in notifications.toml.",
            "job": False,
            "name": "ROTARI_WEBHOOK_URL",
        },
        {
            "array": False,
            "cli_default": False,
            "description": "set to true for 0700/0600 state directory "
            "permissions instead of the default 0755/0644 "
            "(shared state).",
            "job": False,
            "name": "ROTARI_PRIVATE_STATE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari info --json.",
            "job": False,
            "name": "ROTARI_INFO_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari config --list.",
            "job": False,
            "name": "ROTARI_CONFIG_LIST",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari config --format.",
            "job": False,
            "name": "ROTARI_CONFIG_FORMAT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari config --output.",
            "job": False,
            "name": "ROTARI_CONFIG_OUTPUT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari check --json.",
            "job": False,
            "name": "ROTARI_CHECK_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari check --deep.",
            "job": False,
            "name": "ROTARI_CHECK_DEEP",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari cancel --stage.",
            "job": False,
            "name": "ROTARI_CANCEL_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari cancel --matrix.",
            "job": False,
            "name": "ROTARI_CANCEL_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari cancel --wait.",
            "job": False,
            "name": "ROTARI_CANCEL_WAIT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari suspend --stage.",
            "job": False,
            "name": "ROTARI_SUSPEND_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari suspend --matrix.",
            "job": False,
            "name": "ROTARI_SUSPEND_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari resume --stage.",
            "job": False,
            "name": "ROTARI_RESUME_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari resume --matrix.",
            "job": False,
            "name": "ROTARI_RESUME_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --stage.",
            "job": False,
            "name": "ROTARI_CHANGE_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --matrix.",
            "job": False,
            "name": "ROTARI_CHANGE_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-executor-options.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_EXECUTOR_OPTIONS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --working-directory.",
            "job": False,
            "name": "ROTARI_CHANGE_WORKING_DIRECTORY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-working-directory.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_WORKING_DIRECTORY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --env.",
            "job": False,
            "name": "ROTARI_CHANGE_ENV",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-env.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_ENV",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --set-job-name.",
            "job": False,
            "name": "ROTARI_CHANGE_SET_JOB_NAME",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --depends-on.",
            "job": False,
            "name": "ROTARI_CHANGE_DEPENDS_ON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-depends-on.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_DEPENDS_ON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --depends-on-finished.",
            "job": False,
            "name": "ROTARI_CHANGE_DEPENDS_ON_FINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-depends-on-finished.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_DEPENDS_ON_FINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-artifacts.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_ARTIFACTS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-timeout.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_TIMEOUT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-retry.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_RETRY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --retry-delay.",
            "job": False,
            "name": "ROTARI_CHANGE_RETRY_DELAY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --retry-backoff.",
            "job": False,
            "name": "ROTARI_CHANGE_RETRY_BACKOFF",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --retry-max-delay.",
            "job": False,
            "name": "ROTARI_CHANGE_RETRY_MAX_DELAY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari change --clear-status.",
            "job": False,
            "name": "ROTARI_CHANGE_CLEAR_STATUS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari export --format.",
            "job": False,
            "name": "ROTARI_EXPORT_FORMAT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari export --template.",
            "job": False,
            "name": "ROTARI_EXPORT_TEMPLATE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari import --overwrite.",
            "job": False,
            "name": "ROTARI_IMPORT_OVERWRITE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari import --json.",
            "job": False,
            "name": "ROTARI_IMPORT_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari remove --stage.",
            "job": False,
            "name": "ROTARI_REMOVE_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari remove --matrix.",
            "job": False,
            "name": "ROTARI_REMOVE_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari runs --since.",
            "job": False,
            "name": "ROTARI_RUNS_SINCE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari runs --json.",
            "job": False,
            "name": "ROTARI_RUNS_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --queue.",
            "job": False,
            "name": "ROTARI_SHOW_QUEUE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --failed.",
            "job": False,
            "name": "ROTARI_SHOW_FAILED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --unfinished.",
            "job": False,
            "name": "ROTARI_SHOW_UNFINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --success.",
            "job": False,
            "name": "ROTARI_SHOW_SUCCESS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --stage.",
            "job": False,
            "name": "ROTARI_SHOW_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --matrix.",
            "job": False,
            "name": "ROTARI_SHOW_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --logs.",
            "job": False,
            "name": "ROTARI_SHOW_LOGS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --failed-logs.",
            "job": False,
            "name": "ROTARI_SHOW_FAILED_LOGS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --stream.",
            "job": False,
            "name": "ROTARI_SHOW_STREAM",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --follow.",
            "job": False,
            "name": "ROTARI_SHOW_FOLLOW",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --no-pager.",
            "job": False,
            "name": "ROTARI_SHOW_NO_PAGER",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --json.",
            "job": False,
            "name": "ROTARI_SHOW_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --report.",
            "job": False,
            "name": "ROTARI_SHOW_REPORT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari show --artifacts.",
            "job": False,
            "name": "ROTARI_SHOW_ARTIFACTS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari lineage --json.",
            "job": False,
            "name": "ROTARI_LINEAGE_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari jobs --format.",
            "job": False,
            "name": "ROTARI_JOBS_FORMAT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari jobs --since.",
            "job": False,
            "name": "ROTARI_JOBS_SINCE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari jobs --json.",
            "job": False,
            "name": "ROTARI_JOBS_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari wait --until-failure.",
            "job": False,
            "name": "ROTARI_WAIT_UNTIL_FAILURE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari wait --json.",
            "job": False,
            "name": "ROTARI_WAIT_JSON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --working-directory.",
            "job": False,
            "name": "ROTARI_ADD_WORKING_DIRECTORY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --env.",
            "job": False,
            "name": "ROTARI_ADD_ENV",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --stage.",
            "job": False,
            "name": "ROTARI_ADD_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --depends-on.",
            "job": False,
            "name": "ROTARI_ADD_DEPENDS_ON",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --depends-on-finished.",
            "job": False,
            "name": "ROTARI_ADD_DEPENDS_ON_FINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --retry-delay.",
            "job": False,
            "name": "ROTARI_ADD_RETRY_DELAY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --retry-backoff.",
            "job": False,
            "name": "ROTARI_ADD_RETRY_BACKOFF",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --retry-max-delay.",
            "job": False,
            "name": "ROTARI_ADD_RETRY_MAX_DELAY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari add --matrix.",
            "job": False,
            "name": "ROTARI_ADD_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --failed.",
            "job": False,
            "name": "ROTARI_COPY_FAILED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --unfinished.",
            "job": False,
            "name": "ROTARI_COPY_UNFINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --success.",
            "job": False,
            "name": "ROTARI_COPY_SUCCESS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --stage.",
            "job": False,
            "name": "ROTARI_COPY_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --matrix.",
            "job": False,
            "name": "ROTARI_COPY_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --append.",
            "job": False,
            "name": "ROTARI_COPY_APPEND",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari copy --overwrite.",
            "job": False,
            "name": "ROTARI_COPY_OVERWRITE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --failed.",
            "job": False,
            "name": "ROTARI_RUN_FAILED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --unfinished.",
            "job": False,
            "name": "ROTARI_RUN_UNFINISHED",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --success.",
            "job": False,
            "name": "ROTARI_RUN_SUCCESS",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --stage.",
            "job": False,
            "name": "ROTARI_RUN_STAGE",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --matrix.",
            "job": False,
            "name": "ROTARI_RUN_MATRIX",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Default for rotari run --partial-array.",
            "job": False,
            "name": "ROTARI_RUN_PARTIAL_ARRAY",
        },
    ],
    "version": 1,
}
