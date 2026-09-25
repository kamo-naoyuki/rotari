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
                    "description": "check local executables and working " "directories",
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
            ],
            "name": "reset",
            "positional": "[PROJECT]",
        },
        {
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
                    "description": "cancel a running job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
                {"description": "wait until cancellation is complete", "name": "wait"},
            ],
            "name": "cancel",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "suspend running jobs",
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
                    "description": "suspend a running job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
            ],
            "name": "suspend",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "resume suspended jobs",
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
                    "description": "resume a suspended job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
                },
            ],
            "name": "resume",
            "positional": "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
        },
        {
            "description": "delete saved run history",
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
                    "description": "delete only the specified run",
                    "environment": "ROTARI_RUN_ID",
                    "name": "run-id",
                    "short": "r",
                    "value_name": "ID",
                },
            ],
            "name": "delete",
            "positional": "[RUN_ID]",
        },
        {
            "description": "find and remove orphan run registry entries",
            "flags": [
                {
                    "description": "master registry directory",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {"description": "remove the cached orphan entries", "name": "apply"},
            ],
            "name": "gc",
            "positional": "[MASTERDIR]",
        },
        {
            "description": "remove a confirmed stale run lock",
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
            "description": "change a job in the current or previous batch",
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
                    "description": "replace job executor",
                    "environment": "ROTARI_EXECUTOR",
                    "name": "executor",
                    "short": "e",
                    "value_name": "EXECUTOR",
                    "values": ["local", "lsf", "pbs", "slurm", "ssh"],
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
                    "description": "replace job environment variables; may be "
                    "repeated",
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
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
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
            ],
            "name": "export",
        },
        {
            "description": "validate and replace a queue from a workflow manifest",
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
                {"description": "replace a non-empty queue", "name": "overwrite"},
                {
                    "description": "validate and print the import plan without "
                    "writing",
                    "name": "dry-run",
                },
                {"description": "print the import plan as JSON", "name": "json"},
            ],
            "name": "import",
            "positional": "FILE",
        },
        {
            "description": "remove jobs from the current or previous batch",
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
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
                },
            ],
            "name": "remove",
            "positional": "[JOB_ID ...]",
        },
        {
            "description": "show queue or run status",
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
                    "description": "show the current queue even when a run is "
                    "selected",
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
                {"description": "show failed jobs only", "name": "failed"},
                {"description": "print output logs for all jobs", "name": "logs"},
                {
                    "description": "print output logs for failed jobs",
                    "name": "failed-logs",
                },
                {
                    "description": "follow log output until the run completes",
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
            ],
            "name": "show",
            "positional": "[SELECTOR]",
        },
        {
            "description": "list running and recently finished jobs across projects",
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
                    "description": "master registry directory for --all",
                    "environment": "ROTARI_MASTERDIR",
                    "name": "masterdir",
                    "value_name": "DIR",
                },
                {
                    "description": "include all basedirs known to the master "
                    "registry",
                    "name": "all",
                },
                {
                    "description": "output fields; use %s %b %p %a %n %c %t %f "
                    "%e (%f is finished time)",
                    "name": "format",
                    "short": "o",
                    "value_name": "FORMAT",
                },
                {
                    "description": "include jobs finished within this duration; "
                    "use 0 for running jobs only",
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
            "positional": "JOB_ID",
        },
        {
            "description": "wait for an asynchronous run by project, run name, or "
            "run ID",
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
                    "values": ["local", "lsf", "pbs", "slurm", "ssh"],
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
                    "description": "working directory for the job",
                    "name": "working-directory",
                    "value_name": "DIR",
                },
                {
                    "description": "environment variable for the job; may be "
                    "repeated",
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
            ],
            "name": "add",
            "positional": "<command ...>",
        },
        {
            "description": "copy the latest run's jobs into the queue",
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
                {"description": "append to a non-empty queue", "name": "append"},
                {"description": "replace a non-empty queue", "name": "overwrite"},
                {
                    "description": "suppress success output",
                    "environment": "ROTARI_QUIET",
                    "name": "quiet",
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
                    "description": "scheduler job concurrency (Slurm/PBS/...)",
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
                    "description": "only execute this job; may be repeated; "
                    "others carry forward their previous result",
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
                    "values": ["local", "lsf", "pbs", "slurm", "ssh"],
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
                    "description": "SSH executor dispatch options; may be " "repeated",
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
                    "description": "Slurm executor dispatch options; may be "
                    "repeated",
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
                    "description": "PBS executor dispatch options; may be " "repeated",
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
                    "description": "LSF executor dispatch options; may be " "repeated",
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
            ],
            "name": "run",
        },
        {
            "description": "alias for run --failed --unfinished",
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
                    "description": "scheduler job concurrency (Slurm/PBS/...)",
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
                    "description": "also execute this job; may be repeated",
                    "environment": "ROTARI_JOB_ID",
                    "name": "job-id",
                    "repeated": True,
                    "short": "j",
                    "value_name": "ID",
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
                    "values": ["local", "lsf", "pbs", "slurm", "ssh"],
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
                    "description": "SSH executor dispatch options; may be " "repeated",
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
                    "description": "Slurm executor dispatch options; may be "
                    "repeated",
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
                    "description": "PBS executor dispatch options; may be " "repeated",
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
                    "description": "LSF executor dispatch options; may be " "repeated",
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
            ],
            "name": "retry",
        },
        {
            "description": "manage the background server",
            "flags": [
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
                    "description": "install completion for the current " "shell",
                    "name": "install",
                },
            ],
        },
        {
            "description": "print the CLI schema as JSON",
            "flags": [{"description": "print the schema as JSON", "name": "json"}],
            "name": "schema",
        },
        {"description": "print version", "name": "version"},
        {"description": "list ROTARI environment variables", "name": "env"},
    ],
    "version": 1,
}
