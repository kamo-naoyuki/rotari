"""Generated from `rotari schema --json`; do not edit manually."""

from __future__ import annotations

from typing import Any

CLI_SCHEMA: dict[str, Any] = {
    "commands": [
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
                {"description": "list existing config files", "name": "list"},
                {
                    "description": "generate notifications.toml instead of "
                    "command defaults",
                    "name": "notifications",
                },
                {
                    "description": "config format: yaml, toml, or json",
                    "name": "format",
                    "short": "o",
                    "value_name": "FORMAT",
                    "values": ["yaml", "toml", "json"],
                },
                {
                    "description": "output config file path",
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
                {"description": "print machine-readable JSON", "name": "json"},
                {
                    "description": "check local executables and working directories",
                    "name": "deep",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
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
                    "description": "confirm an interrupted run has stopped "
                    "without prompting",
                    "environment": "ROTARI_RESET_RECOVER",
                    "name": "recover",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "cancel the unfinished jobs of this matrix",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {"description": "wait until cancellation is complete", "name": "wait"},
                {
                    "description": "cancel the jobs that filters select without asking",
                    "name": "yes",
                },
                {
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running", "pending"],
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "suspend the running jobs of this matrix",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "suspend the jobs that filters select without "
                    "asking",
                    "name": "yes",
                },
                {
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running"],
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "resume the suspended jobs of this matrix",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "resume the jobs that filters select without asking",
                    "name": "yes",
                },
                {
                    "description": "select jobs in this state; may be repeated",
                    "name": "filter-state",
                    "repeated": True,
                    "value_name": "STATE",
                    "values": ["running"],
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
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
                {"description": "delete every run of the project", "name": "all"},
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
            "description": "change jobs in the current or previous batch",
            "flags": [
                {
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
                    "description": "run ID to use when restoring a batch",
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "change every job of a matrix, named by its "
                    "base job name",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {"description": "change every job", "name": "all"},
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
                    "name": "clear-executor-options",
                },
                {
                    "description": "working directory for the job",
                    "name": "working-directory",
                    "value_name": "DIR",
                },
                {
                    "description": "clear the job working directory",
                    "name": "clear-working-directory",
                },
                {
                    "description": "replace job environment variables; may be repeated",
                    "name": "env",
                    "repeated": True,
                    "value_name": "KEY=VALUE",
                },
                {"description": "clear job environment variables", "name": "clear-env"},
                {
                    "description": "replace job name",
                    "name": "set-job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "replace prerequisites; may be repeated",
                    "name": "depends-on",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {"description": "clear prerequisites", "name": "clear-depends-on"},
                {
                    "description": "replace prerequisites that only need to "
                    "finish, whatever their result; may be "
                    "repeated",
                    "name": "depends-on-finished",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "clear prerequisites that only need to finish",
                    "name": "clear-depends-on-finished",
                },
                {
                    "description": "replace the job timeout, such as 90m or 2h",
                    "name": "timeout",
                    "value_name": "DURATION",
                },
                {"description": "remove the job timeout", "name": "clear-timeout"},
                {
                    "description": "replace the job's retry limit; 0 disables retries",
                    "name": "retry",
                    "value_name": "N",
                },
                {
                    "description": "use the run's --retry limit for the job "
                    "again and remove its retry delay settings",
                    "name": "clear-retry",
                },
                {
                    "description": "replace the wait before the job's first "
                    "retry, such as 30s",
                    "name": "retry-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "replace the factor applied to the retry "
                    "delay for each further retry",
                    "name": "retry-backoff",
                    "value_name": "FACTOR",
                },
                {
                    "description": "replace the upper limit of the retry delay",
                    "name": "retry-max-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "mark the job with a status that result "
                    "filters of the next run read in place of its "
                    "recorded result",
                    "name": "status",
                    "value_name": "STATUS",
                    "values": ["success", "failed", "cancelled", "unfinished"],
                },
                {"description": "remove the job's status mark", "name": "clear-status"},
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
                    "name": "format",
                    "short": "o",
                    "value_name": "FORMAT",
                    "values": ["yaml", "toml", "json"],
                },
                {
                    "description": "print a starter workflow manifest",
                    "name": "template",
                },
                {
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
                {"description": "replace a non-empty queue", "name": "overwrite"},
                {"description": "print the import plan as JSON", "name": "json"},
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
            "description": "remove jobs from the current or previous batch",
            "flags": [
                {
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
                    "description": "run ID to use when restoring a batch",
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "remove every job of a matrix, named by its "
                    "base job name",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {"description": "remove every job", "name": "all"},
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
            "description": "show queue or run status",
            "flags": [
                {
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
                    "description": "run ID or latest",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "show the current queue even when a run is selected",
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
                    "name": "failed",
                },
                {
                    "description": "show unfinished jobs; may be combined with "
                    "the other result filters",
                    "name": "unfinished",
                },
                {
                    "description": "show successful jobs; may be combined with "
                    "the other result filters",
                    "name": "success",
                },
                {
                    "description": "show jobs in this stage only",
                    "name": "stage",
                    "value_name": "NAME",
                },
                {
                    "description": "show jobs of this matrix only, named by its "
                    "base job name",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {"description": "print output logs for all jobs", "name": "logs"},
                {
                    "description": "print output logs for failed jobs",
                    "name": "failed-logs",
                },
                {
                    "description": "show both streams or select stdout/stderr",
                    "name": "stream",
                    "value_name": "STREAM",
                },
                {
                    "description": "follow one selected log stream until the run "
                    "completes",
                    "name": "follow",
                },
                {
                    "description": "print logs directly instead of using a pager",
                    "name": "no-pager",
                },
                {
                    "description": "list state directories known to the master "
                    "registry",
                    "name": "basedirs",
                },
                {
                    "description": "print machine-readable JSON for a run",
                    "name": "json",
                },
                {"description": "print an AI-ready Markdown report", "name": "report"},
                {
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "description": "select jobs of this failure kind; may be "
                    "repeated; valid values: timeout, cancelled, "
                    "blocked, oom, signal, error",
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
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
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
                    "name": "json",
                },
            ],
            "name": "lineage",
            "positional": "[RUN_ID ...]",
        },
        {
            "description": "list running and recently finished jobs across projects",
            "flags": [
                {
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
                    "description": "master registry directory for --all",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "description": "include all basedirs known to the master registry",
                    "name": "all-basedirs",
                },
                {
                    "description": "output fields; use %s %b %p %a %n %c %t %f "
                    "%e (%f is finished time)",
                    "name": "format",
                    "short": "o",
                    "value_name": "FORMAT",
                },
                {
                    "description": "include jobs finished within this duration, "
                    "such as 24h or 7d; use 0 for running jobs "
                    "only",
                    "name": "since",
                    "value_name": "DURATION",
                },
            ],
            "name": "jobs",
            "positional": "[PROJECT]",
        },
        {
            "description": "diagnose one job with an LLM or local error rules",
            "flags": [
                {
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
                    "description": "run ID",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "failed job ID",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "failed job name",
                    "environment": "ROTARI_JOB_NAME",
                    "name": "job-name",
                    "value_name": "NAME",
                },
                {
                    "description": "use local rule-based diagnosis without "
                    "calling an LLM",
                    "name": "rules",
                },
                {
                    "description": "LLM provider: openai, openai-chat, "
                    "anthropic, gemini, or cohere",
                    "environment": "ROTARI_LLM_PROVIDER",
                    "name": "provider",
                    "value_name": "PROVIDER",
                    "values": [
                        "openai",
                        "openai-chat",
                        "anthropic",
                        "gemini",
                        "cohere",
                    ],
                },
                {
                    "description": "LLM API endpoint",
                    "environment": "ROTARI_LLM_ENDPOINT",
                    "name": "endpoint",
                    "value_name": "URL",
                },
                {
                    "description": "LLM model name",
                    "environment": "ROTARI_LLM_MODEL",
                    "name": "model",
                    "value_name": "MODEL",
                },
                {
                    "description": "response language BCP 47 tag",
                    "environment": "ROTARI_LLM_LANGUAGE",
                    "name": "language",
                    "value_name": "TAG",
                },
            ],
            "name": "diagnose",
            "positional": "[JOB_ID]",
        },
        {
            "description": "wait for an asynchronous run by project, run name, or "
            "run ID",
            "flags": [
                {
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
                    "name": "until-failure",
                },
                {
                    "description": "print each completed run as one JSON object",
                    "name": "json",
                },
            ],
            "name": "wait",
            "positional": "[PROJECT_OR_RUN_NAME_OR_RUN_ID ...]",
        },
        {
            "description": "add a command to a queue",
            "flags": [
                {
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
                    "description": "stdout destination; stderr also goes here "
                    "unless --error is specified; may be repeated",
                    "name": "output",
                    "repeated": True,
                    "value_name": "FILE",
                },
                {
                    "description": "stderr destination; defaults to --output "
                    "destinations; may be repeated",
                    "name": "error",
                    "repeated": True,
                    "value_name": "FILE",
                },
                {
                    "description": "internal log mode",
                    "name": "log-mode",
                    "value_name": "MODE",
                    "values": ["merge", "separate"],
                },
                {
                    "description": "external output file mode",
                    "name": "open-mode",
                    "value_name": "MODE",
                    "values": ["append", "truncate"],
                },
                {
                    "description": "working directory for the job",
                    "name": "working-directory",
                    "value_name": "DIR",
                },
                {
                    "description": "environment variable for the job; may be repeated",
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
                    "name": "stage",
                    "value_name": "NAME",
                },
                {
                    "description": "name of a prerequisite job or stage; may be "
                    "repeated",
                    "name": "depends-on",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "name of a prerequisite job or stage that "
                    "must finish, whatever its result; may be "
                    "repeated",
                    "name": "depends-on-finished",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "stop the job this long after it starts, such "
                    "as 90m or 2h; it then fails with exit code "
                    "124",
                    "name": "timeout",
                    "value_name": "DURATION",
                },
                {
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
                    "name": "retry-delay",
                    "value_name": "DURATION",
                },
                {
                    "description": "multiply the retry delay by this factor for "
                    "each further retry, such as 2",
                    "name": "retry-backoff",
                    "value_name": "FACTOR",
                },
                {
                    "description": "upper limit of the retry delay, such as 10m",
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
                    "name": "matrix",
                    "repeated": True,
                    "value_name": "KEY=VALUE[,VALUE...]",
                },
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
                    "name": "failed",
                },
                {
                    "description": "include unfinished jobs; may be combined "
                    "with result filters",
                    "name": "unfinished",
                },
                {
                    "description": "include successful jobs; may be combined "
                    "with result filters",
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only copy jobs of this matrix, named by its "
                    "base job name, narrowed by any result filter",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {"description": "append to a non-empty queue", "name": "append"},
                {"description": "replace a non-empty queue", "name": "overwrite"},
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
                {
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "description": "select jobs of this failure kind; may be "
                    "repeated; valid values: timeout, cancelled, "
                    "blocked, oom, signal, error",
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
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
            "a run",
            "flags": [
                {
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
                    "description": "repopulate the queue from this run before "
                    "executing (copy --run-id + run); defaults to "
                    "the latest run when a result filter is used",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "replace a non-empty queue without prompting; "
                    "requires --run-id",
                    "name": "overwrite",
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
                    "name": "failed",
                },
                {
                    "description": "only execute unfinished jobs; others carry "
                    "forward their previous result",
                    "name": "unfinished",
                },
                {
                    "description": "only execute successful jobs; others carry "
                    "forward their previous result",
                    "name": "success",
                },
                {
                    "description": "only execute this job; may be repeated; not "
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
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only execute jobs of this matrix, named by "
                    "its base job name, narrowed by any result "
                    "filter; others carry forward their previous "
                    "result",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "with a result filter, select array jobs per "
                    "task instead of all-or-nothing (default "
                    "true); pass =false to re-execute the whole "
                    "array when any task matches",
                    "name": "partial-array",
                },
                {
                    "description": "return after starting the run",
                    "environment": "ROTARI_RUN_ASYNC",
                    "name": "async",
                },
                {
                    "description": "suppress progress and completion output",
                    "environment": "ROTARI_QUIET",
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
                    "description": "caller environment propagation mode (default ALL)",
                    "name": "env",
                    "value_name": "ALL|NONE",
                    "values": ["ALL", "NONE"],
                },
                {
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
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "description": "select jobs of this failure kind; may be "
                    "repeated; valid values: timeout, cancelled, "
                    "blocked, oom, signal, error",
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
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
            "those jobs",
            "flags": [
                {
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
                    "description": "repopulate the queue from this run before "
                    "executing; defaults to the latest run",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
                {
                    "description": "replace a non-empty queue without prompting; "
                    "requires --run-id",
                    "name": "overwrite",
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
                    "description": "only execute this job instead of failed and "
                    "unfinished jobs; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {
                    "description": "only retry jobs in this stage",
                    "name": "stage",
                    "value_name": "STAGE",
                },
                {
                    "description": "only retry jobs of this matrix, named by its "
                    "base job name",
                    "name": "matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "return after starting the run",
                    "environment": "ROTARI_RUN_ASYNC",
                    "name": "async",
                },
                {
                    "description": "suppress progress and completion output",
                    "environment": "ROTARI_QUIET",
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
                    "description": "caller environment propagation mode (default ALL)",
                    "name": "env",
                    "value_name": "ALL|NONE",
                    "values": ["ALL", "NONE"],
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
                    "description": "select jobs with this result; may be "
                    "repeated; --failed, --unfinished, and "
                    "--success are short forms",
                    "name": "filter-result",
                    "repeated": True,
                    "value_name": "RESULT",
                    "values": ["failed", "unfinished", "success"],
                },
                {
                    "description": "select jobs with this exit code; may be repeated",
                    "name": "filter-exit-code",
                    "repeated": True,
                    "value_name": "N",
                },
                {
                    "description": "select jobs of this failure kind; may be "
                    "repeated; valid values: timeout, cancelled, "
                    "blocked, oom, signal, error",
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
                    "description": "select failed jobs matching a current "
                    "diagnosis rule; may be repeated",
                    "name": "filter-diagnosis",
                    "repeated": True,
                    "value_name": "VALUE",
                },
                {
                    "description": "select queued jobs whose definition changed "
                    "from the reference run",
                    "name": "filter-changed",
                },
                {
                    "description": "select queued jobs with no matching job in "
                    "the reference run",
                    "name": "filter-new",
                },
                {
                    "description": "select jobs run on a matching host; may be "
                    "repeated",
                    "name": "filter-host",
                    "repeated": True,
                    "value_name": "PATTERN",
                },
                {
                    "description": "select jobs started at or after this time",
                    "name": "filter-started-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs started before this time",
                    "name": "filter-started-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished at or after this time",
                    "name": "filter-finished-after",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs finished before this time",
                    "name": "filter-finished-before",
                    "value_name": "TIME",
                },
                {
                    "description": "select jobs running at least this long",
                    "name": "filter-longer-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs running less than this long",
                    "name": "filter-shorter-than",
                    "value_name": "DURATION",
                },
                {
                    "description": "select jobs whose argv matches this Go regexp",
                    "name": "filter-command",
                    "value_name": "RE",
                },
                {
                    "description": "select jobs in this stage; same as --stage",
                    "name": "filter-stage",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs in this stage; may be repeated",
                    "name": "filter-not-stage",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "select jobs of this matrix, named by its "
                    "base job name; same as --matrix",
                    "name": "filter-matrix",
                    "value_name": "NAME",
                },
                {
                    "description": "exclude jobs of this matrix, named by its "
                    "base job name; may be repeated",
                    "name": "filter-not-matrix",
                    "repeated": True,
                    "value_name": "NAME",
                },
                {
                    "description": "print what the command would change, and the "
                    "project revision, without writing",
                    "name": "dry-run",
                },
                {
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
                    "description": "HTTP listen host",
                    "environment": "ROTARI_WEB_HOST",
                    "name": "host",
                    "value_name": "HOST",
                },
                {
                    "description": "HTTP listen port",
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
                    "(copy/change/remove/cancel/clear); pass "
                    "--allow-control=false for a read-only UI",
                    "environment": "ROTARI_WEB_ALLOW_CONTROL",
                    "name": "allow-control",
                },
                {
                    "description": "require this token in Authorization: Bearer "
                    "or X-Rotari-Token; prefer "
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
            ],
            "name": "web",
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
            "flags": [{"description": "print the schema as JSON", "name": "json"}],
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
            "description": "--recover default for reset.",
            "job": False,
            "name": "ROTARI_RESET_RECOVER",
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
            "description": "API key for the diagnose command; never persisted "
            "or passed to jobs.",
            "job": False,
            "name": "ROTARI_LLM_API_KEY",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LLM provider (openai, openai-chat, anthropic, "
            "gemini, or cohere); --provider default for "
            "diagnose.",
            "job": False,
            "name": "ROTARI_LLM_PROVIDER",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "LLM API endpoint; --endpoint default for diagnose.",
            "job": False,
            "name": "ROTARI_LLM_ENDPOINT",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "Model name; --model default for diagnose.",
            "job": False,
            "name": "ROTARI_LLM_MODEL",
        },
        {
            "array": False,
            "cli_default": True,
            "description": "BCP 47 response language tag; --language default "
            "for diagnose.",
            "job": False,
            "name": "ROTARI_LLM_LANGUAGE",
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
    ],
    "version": 1,
}
