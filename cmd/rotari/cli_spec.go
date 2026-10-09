package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type cliFlagSpec struct {
	Name        string
	Description string
	ValueName   string
	Values      []string
	Repeated    bool
	// CommandLineOnly keeps config files and environment variables from
	// supplying the flag, for values such as an output path or a per-job
	// setting that shares a name with a run option. cliString and cliInt
	// honor it.
	CommandLineOnly bool
}

type cliSubcommandSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type cliCommandSpec struct {
	Name        string
	Description string
	Flags       []cliFlagSpec
	Subcommands []cliSubcommandSpec
	Positional  string
}

var cliShortFlagNames = map[string]string{
	"basedir":      "b",
	"project-name": "p",
	"run-id":       "r",
	"job-id":       "j",
	"executor":     "e",
	"format":       "o",
}

var cliEnvironmentVariables = map[string]string{
	"basedir":                  envBaseDir,
	"project-name":             envProjectName,
	"masterdir":                envMasterDir,
	"run-id":                   envRunID,
	"job-id":                   envJobID,
	"job-name":                 envJobName,
	"executor":                 envExecutor,
	"executor-option":          envExecutorOpts,
	"disconnect-action":        envDisconnectAction,
	"run-name":                 envRunName,
	"local-concurrency":        envRunLocalConc,
	"batch-concurrency":        envRunBatchConc,
	"ssh-concurrency":          envRunSSHConc,
	"ssh-options":              envRunSSHOptions,
	"slurm-concurrency":        envRunSlurmConc,
	"slurm-options":            envRunSlurmOptions,
	"slurm-submit-interval":    envRunSlurmSubmitInterval,
	"slurm-submit-retry-limit": envRunSlurmSubmitRetryLimit,
	"pbs-concurrency":          envRunPBSConc,
	"pbs-options":              envRunPBSOptions,
	"pbs-submit-interval":      envRunPBSSubmitInterval,
	"pbs-submit-retry-limit":   envRunPBSSubmitRetryLimit,
	"lsf-concurrency":          envRunLSFConc,
	"lsf-options":              envRunLSFOptions,
	"lsf-submit-interval":      envRunLSFSubmitInterval,
	"lsf-submit-retry-limit":   envRunLSFSubmitRetryLimit,
	"sge-concurrency":          envRunSGEConc,
	"sge-options":              envRunSGEOptions,
	"sge-submit-interval":      envRunSGESubmitInterval,
	"sge-submit-retry-limit":   envRunSGESubmitRetryLimit,
	"retry":                    envRunRetry,
	"async":                    envRunAsync,
	"quiet":                    envQuiet,
	"add-quiet":                envAddQuiet,
	"copy-quiet":               envCopyQuiet,
	"change-quiet":             envChangeQuiet,
	"remove-quiet":             envRemoveQuiet,
	"reset-quiet":              envResetQuiet,
	"check-quiet":              envCheckQuiet,
	"run-quiet":                envRunQuiet,
	"array":                    envArrayRange,
	"timeout":                  envWaitTimeout,
	"host":                     envWebHost,
	"port":                     envWebPort,
	"static-dir":               envWebStaticDir,
	"allow-control":            envWebAllowControl,
	"auth-token":               envWebAuthToken,
	"notifications":            envWebNotifications,
}

func commonCLIFlags() []cliFlagSpec {
	return []cliFlagSpec{
		{Name: "config", Description: "config file to use", ValueName: "FILE", CommandLineOnly: true},
		{Name: "basedir", Description: "state directory", ValueName: "DIR"},
		{Name: "project-name", Description: "project name", ValueName: "NAME"},
	}
}

var cliCommandSpecs = []cliCommandSpec{
	{
		Name:        "info",
		Description: "show the current Rotari context and active run state",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			cliFlagSpec{Name: "json", Description: "print machine-readable JSON"},
		),
	},
	{
		Name:        "init",
		Description: "write cwd workspace location defaults without creating state",
		Positional:  "[BASEDIR [PROJECT]]",
	},
	{
		Name:        "config",
		Description: "generate a config file template",
		Flags: append([]cliFlagSpec{{Name: "basedir", Description: "state directory", ValueName: "DIR"}, {Name: "project-name", Description: "project name", ValueName: "NAME"}},
			cliFlagSpec{Name: "list", Description: "list existing config files"},
			cliFlagSpec{Name: "notifications", Description: "generate notifications.toml instead of command defaults", CommandLineOnly: true},
			cliFlagSpec{Name: "format", Description: "config format: yaml, toml, or json", ValueName: "FORMAT", Values: []string{"yaml", "toml", "json"}},
			cliFlagSpec{Name: "output", Description: "output config file path", ValueName: "FILE"},
		),
	},
	{
		Name:        "check",
		Description: "check whether a project is ready to run",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "json", Description: "print machine-readable JSON"},
			cliFlagSpec{Name: "deep", Description: "check local executables and working directories"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name:        "reset",
		Description: "discard the current, not-yet-run queue",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name:        "cancel",
		Description: "cancel the active run or running jobs",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "cancel a running job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "cancel the unfinished jobs with this name; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "cancel the unfinished jobs of this stage", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "cancel the unfinished jobs of this matrix", ValueName: "NAME"},
			cliFlagSpec{Name: "wait", Description: "wait until cancellation is complete"},
			cliFlagSpec{Name: "yes", Description: "cancel the jobs that filters select without asking", CommandLineOnly: true},
		), jobControlFilterSpecs(cancelStates)...),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "suspend",
		Description: "suspend running jobs",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "suspend a running job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "suspend the running jobs with this name; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "suspend the running jobs of this stage", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "suspend the running jobs of this matrix", ValueName: "NAME"},
			cliFlagSpec{Name: "yes", Description: "suspend the jobs that filters select without asking", CommandLineOnly: true},
		), jobControlFilterSpecs(signalStates)...),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "resume",
		Description: "resume suspended jobs",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "resume a suspended job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "resume the suspended jobs with this name; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "resume the suspended jobs of this stage", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "resume the suspended jobs of this matrix", ValueName: "NAME"},
			cliFlagSpec{Name: "yes", Description: "resume the jobs that filters select without asking", CommandLineOnly: true},
		), jobControlFilterSpecs(signalStates)...),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "delete",
		Description: "delete saved run history",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run to delete", ValueName: "ID"},
			cliFlagSpec{Name: "all", Description: "delete every run of the project", CommandLineOnly: true},
		),
		Positional: "[RUN_ID]",
	},
	{
		Name:        "gc",
		Description: "find and remove orphan run registry entries",
		Flags: []cliFlagSpec{
			{Name: "config", Description: "config file to use", ValueName: "FILE", CommandLineOnly: true},
			{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			{Name: "dry-run", Description: "list the orphan entries without removing them", CommandLineOnly: true},
		},
		Positional: "[MASTERDIR]",
	},
	{
		Name:        "unlock",
		Description: "remove a confirmed stale run lock",
		Flags:       append(commonCLIFlags(), cliFlagSpec{Name: "run-id", Description: "verify the run ID recorded in the stale lock", ValueName: "ID"}),
		Positional:  "[PROJECT]",
	},
	{
		Name:        "change",
		Description: "change queued jobs, or with --run-id the jobs of that run, which replace the queue first; a command after the options replaces the jobs' command",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "first replace the queue with this run's jobs, then change them", ValueName: "ID"},
			cliFlagSpec{Name: "job-id", Description: "target job ID", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "target job name", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "change every job in a stage", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "change every job of a matrix, named by its base job name", ValueName: "NAME"},
			cliFlagSpec{Name: "all", Description: "change every job", CommandLineOnly: true},
			cliFlagSpec{Name: "executor", Description: "replace job executor", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
			cliFlagSpec{Name: "executor-option", Description: "replace executor options", ValueName: "OPTION"},
			cliFlagSpec{Name: "clear-executor-options", Description: "clear executor options"},
			cliFlagSpec{Name: "working-directory", Description: "working directory for the job", ValueName: "DIR"},
			cliFlagSpec{Name: "clear-working-directory", Description: "clear the job working directory"},
			cliFlagSpec{Name: "env", Description: "replace job environment variables; may be repeated", ValueName: "KEY=VALUE"},
			cliFlagSpec{Name: "clear-env", Description: "clear job environment variables"},
			cliFlagSpec{Name: "set-job-name", Description: "replace job name", ValueName: "NAME"},
			cliFlagSpec{Name: "depends-on", Description: "replace prerequisites; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "clear-depends-on", Description: "clear prerequisites"},
			cliFlagSpec{Name: "depends-on-finished", Description: "replace prerequisites that only need to finish, whatever their result; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "clear-depends-on-finished", Description: "clear prerequisites that only need to finish"},
			cliFlagSpec{Name: "artifact", Description: "replace the declared artifact paths (see add --artifact); may be repeated", ValueName: "PATH", Repeated: true, CommandLineOnly: true},
			cliFlagSpec{Name: "clear-artifacts", Description: "clear the declared artifact paths"},
			cliFlagSpec{Name: "timeout", Description: "replace the job timeout, such as 90m or 2h", ValueName: "DURATION", CommandLineOnly: true},
			cliFlagSpec{Name: "clear-timeout", Description: "remove the job timeout"},
			cliFlagSpec{Name: "retry", Description: "replace the job's retry limit; 0 disables retries", ValueName: "N", CommandLineOnly: true},
			cliFlagSpec{Name: "clear-retry", Description: "use the run's --retry limit for the job again and remove its retry delay settings"},
			cliFlagSpec{Name: "retry-delay", Description: "replace the wait before the job's first retry, such as 30s", ValueName: "DURATION"},
			cliFlagSpec{Name: "retry-backoff", Description: "replace the factor applied to the retry delay for each further retry", ValueName: "FACTOR"},
			cliFlagSpec{Name: "retry-max-delay", Description: "replace the upper limit of the retry delay", ValueName: "DURATION"},
			cliFlagSpec{Name: "status", Description: "mark the job with a status that result filters of the next run read in place of its recorded result", ValueName: "STATUS", Values: []string{"success", "failed", "cancelled", "unfinished"}, CommandLineOnly: true},
			cliFlagSpec{Name: "clear-status", Description: "remove the job's status mark"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		), jobFilterFlagSpecs(definitionJobFilters)...),
		Positional: "<command ...>",
	},
	{
		Name:        "export",
		Description: "export the current queue or saved runs as a workflow manifest",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID to export; may be repeated", ValueName: "ID", Repeated: true},
			cliFlagSpec{Name: "format", Description: "manifest format: yaml, toml, or json", ValueName: "FORMAT", Values: []string{"yaml", "toml", "json"}},
			cliFlagSpec{Name: "template", Description: "print a starter workflow manifest"},
			cliFlagSpec{Name: "output", Description: "write the workflow manifest to a file", ValueName: "FILE", CommandLineOnly: true},
		),
		Positional: "[TARGET] [FILE]",
	},
	{
		Name:        "import",
		Description: "validate and replace a queue from a workflow manifest",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue"},
			cliFlagSpec{Name: "json", Description: "print the import plan as JSON"},
		),
		Positional: "FILE [PROJECT]",
	},
	{
		Name:        "remove",
		Description: "remove queued jobs, or with --run-id jobs of that run, which replace the queue first",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "first replace the queue with this run's jobs, then remove from them", ValueName: "ID"},
			cliFlagSpec{Name: "job-id", Description: "remove a job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "remove a job by name", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "remove every job in a stage", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "remove every job of a matrix, named by its base job name", ValueName: "NAME"},
			cliFlagSpec{Name: "all", Description: "remove every job", CommandLineOnly: true},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		), jobFilterFlagSpecs(definitionJobFilters)...),
		Positional: "[JOB_ID ...]",
	},
	{
		Name:        "basedirs",
		Description: "list state directories known to the master registry",
		Flags:       []cliFlagSpec{{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"}},
	},
	{
		Name:        "projects",
		Description: "list projects across known state directories",
		Flags:       []cliFlagSpec{{Name: "basedir", Description: "limit the listing to this state directory", ValueName: "DIR"}, {Name: "masterdir", Description: "master registry directory", ValueName: "DIR"}},
	},
	{
		Name:        "runs",
		Description: "list active runs and recently finished runs across known state directories",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			cliFlagSpec{Name: "since", Description: "include finished runs in this time window (default 1d), such as 24h or 7d; active or interrupted runs are always included", ValueName: "DURATION"},
			cliFlagSpec{Name: "json", Description: "print machine-readable JSON"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name:        "show",
		Description: "show details for a project, run, job, or attempt",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID or latest", ValueName: "ID"},
			cliFlagSpec{Name: "queue", Description: "show the current queue even when a run is selected"},
			cliFlagSpec{Name: "job-id", Description: "job ID", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "job name", ValueName: "NAME"},
			cliFlagSpec{Name: "failed", Description: "show failed jobs; may be combined with the other result filters"},
			cliFlagSpec{Name: "unfinished", Description: "show unfinished jobs; may be combined with the other result filters"},
			cliFlagSpec{Name: "success", Description: "show successful jobs; may be combined with the other result filters"},
			cliFlagSpec{Name: "stage", Description: "show jobs in this stage only", ValueName: "NAME"},
			cliFlagSpec{Name: "matrix", Description: "show jobs of this matrix only, named by its base job name", ValueName: "NAME"},
			cliFlagSpec{Name: "logs", Description: "print output logs for all jobs"},
			cliFlagSpec{Name: "failed-logs", Description: "print output logs for failed jobs"},
			cliFlagSpec{Name: "stream", Description: "show both streams or select stdout/stderr", ValueName: "STREAM"},
			cliFlagSpec{Name: "follow", Description: "follow one selected log stream until the run completes"},
			cliFlagSpec{Name: "no-pager", Description: "print logs directly instead of using a pager"},
			cliFlagSpec{Name: "json", Description: "print machine-readable JSON for a run"},
			cliFlagSpec{Name: "report", Description: "print an AI-ready Markdown report"},
			cliFlagSpec{Name: "artifacts", Description: "list every artifact candidate of one job attempt instead of its logs"},
		), jobFilterFlagSpecs(queueRunJobFilters)...),
		Positional: "[SELECTOR]",
	},
	{
		Name:        "lineage",
		Description: "list runs, summarize one run, or compare two runs",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "json", Description: "print the lineage, summary, or comparison as JSON"},
		),
		Positional: "[RUN_ID ...]",
	},
	{
		Name:        "jobs",
		Description: "list running and recently finished jobs across known state directories",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			cliFlagSpec{Name: "format", Description: "output fields; use %s %b %p %a %n %c %t %f %e (%f is finished time)", ValueName: "FORMAT"},
			cliFlagSpec{Name: "since", Description: "include jobs finished within this duration (default 1d), such as 24h or 7d; use 0 for running jobs only", ValueName: "DURATION"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name:        "wait",
		Description: "wait for a run by project, run name, or run ID",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "disconnect-action", Description: "when the wait client's input closes, detach or cancel selected runs (default detach)", ValueName: "ACTION", Values: []string{"detach", "cancel"}},
			cliFlagSpec{Name: "run-id", Description: "run ID; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "timeout", Description: "maximum wait duration", ValueName: "DURATION"},
			cliFlagSpec{Name: "until-failure", Description: "return as soon as a job of the run has failed with no retry left, without waiting for the rest"},
			cliFlagSpec{Name: "json", Description: "print each completed run as one JSON object"},
			cliFlagSpec{Name: "quiet", Description: "suppress normal progress and completion output; keep failure diagnostics and JSON results"},
		),
		Positional: "[PROJECT_OR_RUN_NAME_OR_RUN_ID ...]",
	},
	{
		Name:        "add",
		Description: "add a command to a queue",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "executor", Description: "job executor", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
			cliFlagSpec{Name: "executor-option", Description: "option passed to the selected scheduler (sbatch/qsub/...); may be repeated", ValueName: "OPTION"},
			cliFlagSpec{Name: "output", Description: "stdout destination; stderr also goes here unless --error is specified; may be repeated", ValueName: "FILE", Repeated: true, CommandLineOnly: true},
			cliFlagSpec{Name: "error", Description: "stderr destination; defaults to --output destinations; may be repeated", ValueName: "FILE", Repeated: true, CommandLineOnly: true},
			cliFlagSpec{Name: "artifact", Description: "a file or directory the job writes or reads, recorded as an artifact candidate of each attempt; $ROTARI_ARRAY_TASK_ID, $ROTARI_JOB_DIR, and the job's --env and matrix variables expand; may be repeated", ValueName: "PATH", Repeated: true, CommandLineOnly: true},
			cliFlagSpec{Name: "log-mode", Description: "internal log mode", ValueName: "MODE", Values: []string{"merge", "separate"}, CommandLineOnly: true},
			cliFlagSpec{Name: "open-mode", Description: "external output file mode", ValueName: "MODE", Values: []string{"append", "truncate"}, CommandLineOnly: true},
			cliFlagSpec{Name: "working-directory", Description: "working directory for the job", ValueName: "DIR"},
			cliFlagSpec{Name: "env", Description: "environment variable for the job; may be repeated", ValueName: "KEY=VALUE"},
			cliFlagSpec{Name: "job-name", Description: "job name label", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "stage that contains the job", ValueName: "NAME"},
			cliFlagSpec{Name: "depends-on", Description: "name of a prerequisite job or stage; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "depends-on-finished", Description: "name of a prerequisite job or stage that must finish, whatever its result; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "timeout", Description: "stop the job this long after it starts, such as 90m or 2h; it then fails with exit code 124", ValueName: "DURATION", CommandLineOnly: true},
			cliFlagSpec{Name: "retry", Description: "retry the job up to N times when it fails, instead of the run's --retry; 0 disables retries", ValueName: "N", CommandLineOnly: true},
			cliFlagSpec{Name: "retry-delay", Description: "wait this long before the job's first retry, such as 30s; retries are immediate by default", ValueName: "DURATION"},
			cliFlagSpec{Name: "retry-backoff", Description: "multiply the retry delay by this factor for each further retry, such as 2", ValueName: "FACTOR"},
			cliFlagSpec{Name: "retry-max-delay", Description: "upper limit of the retry delay, such as 10m", ValueName: "DURATION"},
			cliFlagSpec{Name: "array", Description: "create an array job range or selected tasks", ValueName: "FIRST-LAST|TASK[,TASK...]"},
			cliFlagSpec{Name: "matrix", Description: "expand a command into jobs from KEY=VALUE[,VALUE...] dimensions; may be repeated", ValueName: "KEY=VALUE[,VALUE...]"},
			cliFlagSpec{Name: "matrix-exclude", Description: "exclude matrix combinations matching KEY=VALUE[,KEY=VALUE...]; requires --matrix and may be repeated", ValueName: "KEY=VALUE[,KEY=VALUE...]", Repeated: true, CommandLineOnly: true},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "<command ...>",
	},
	{
		Name:        "copy",
		Description: "copy the latest run's jobs into the queue",
		Flags: append(append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "source run ID; defaults to the latest run", ValueName: "ID"},
			cliFlagSpec{Name: "failed", Description: "include failed jobs; may be combined with result filters"},
			cliFlagSpec{Name: "unfinished", Description: "include unfinished jobs; may be combined with result filters"},
			cliFlagSpec{Name: "success", Description: "include successful jobs; may be combined with result filters"},
			cliFlagSpec{Name: "job-id", Description: "copy a job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "copy a job by name", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "only copy jobs in this stage, narrowed by any result filter", ValueName: "STAGE"},
			cliFlagSpec{Name: "matrix", Description: "only copy jobs of this matrix, named by its base job name, narrowed by any result filter", ValueName: "NAME"},
			cliFlagSpec{Name: "append", Description: "append to a non-empty queue"},
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		), jobFilterFlagSpecs(runJobFilters)...),
		Positional: "[RUN_ID]",
	},
	{
		Name:        "run",
		Description: "execute queued commands, optionally selecting jobs from a run; jobs that depend on a selected job execute too",
		Flags:       runCommandFlags(false),
		Positional:  "[RUN_ID]",
	},
	{
		Name:        "retry",
		Description: "run failed and unfinished jobs; with --job-id, run those jobs; jobs that depend on them execute too",
		Flags:       runCommandFlags(true),
		Positional:  "[RUN_ID]",
	},
	{
		Name:        "server",
		Description: "manage the background server",
		Flags: []cliFlagSpec{
			{Name: "config", Description: "config file to use", ValueName: "FILE", CommandLineOnly: true},
			{Name: "basedir", Description: "state directory", ValueName: "DIR"},
			{Name: "masterdir", Description: "server registry directory", ValueName: "DIR"},
		},
		Subcommands: []cliSubcommandSpec{
			{Name: "status", Description: "show server status"},
			{Name: "list", Description: "list servers"},
			{Name: "shutdown", Description: "shut down server"},
		},
	},
	{
		Name:        "web",
		Description: "serve the web status UI",
		Flags: append([]cliFlagSpec{{Name: "config", Description: "config file to use", ValueName: "FILE", CommandLineOnly: true}, {Name: "basedir", Description: "state directory", ValueName: "DIR"}},
			cliFlagSpec{Name: "host", Description: "HTTP listen host (live server only; cannot be combined with --static-dir)", ValueName: "HOST"},
			cliFlagSpec{Name: "port", Description: "HTTP listen port (live server only; cannot be combined with --static-dir)", ValueName: "PORT"},
			cliFlagSpec{Name: "static-dir", Description: "generate a static web UI", ValueName: "DIR"},
			cliFlagSpec{Name: "allow-control", Description: "enable job control (copy/change/remove/cancel/clear) in the live server; false makes it read-only; incompatible with --static-dir"},
			cliFlagSpec{Name: "auth-token", Description: "require this token in Authorization: Bearer or X-Rotari-Token on the live server; cannot be combined with --static-dir; prefer ROTARI_WEB_AUTH_TOKEN for secrets", ValueName: "TOKEN"},
			cliFlagSpec{Name: "notifications", Description: "default state of the browser desktop-notification toggle; pass --notifications=false to default it off"},
			cliFlagSpec{Name: "static-artifact-contents", Description: "copy previewable artifact files (up to 10 MiB each, 100 MiB in all) into the static export so previews work there; requires --static-dir", CommandLineOnly: true},
			cliFlagSpec{Name: "artifact-root", Description: "also serve recorded artifact files under this directory, besides each job's working directory (live server only; may be repeated; cannot be combined with --static-dir)", ValueName: "DIR", Repeated: true, CommandLineOnly: true},
		),
	},
	{
		Name:        "mcp",
		Description: "serve rotari's tools for agents over MCP on stdio",
		Flags: []cliFlagSpec{
			{Name: "config", Description: "config file to use", ValueName: "FILE", CommandLineOnly: true},
			{Name: "masterdir", Description: "master registry directory whose runs and basedirs the tools serve", ValueName: "DIR"},
		},
	},
	{
		Name:        "completion",
		Description: "print shell completion script",
		Subcommands: []cliSubcommandSpec{
			{Name: "bash", Description: "bash completion"},
			{Name: "zsh", Description: "zsh completion"},
			{Name: "fish", Description: "fish completion"},
			{Name: "install", Description: "install completion for the current shell"},
		},
	},
	{
		Name:        "schema",
		Description: "print the CLI schema as JSON",
		Flags:       []cliFlagSpec{{Name: "json", Description: "print the schema as JSON", CommandLineOnly: true}},
	},
	{
		Name:        "guide",
		Description: "print a usage guide for coding agents",
	},
	{
		Name:        "version",
		Description: "print version",
	},
	{
		Name:        "env",
		Description: "list ROTARI environment variables",
	},
}

func cliCommandNames() []string {
	names := make([]string, 0, len(cliCommandSpecs))
	for _, command := range cliCommandSpecs {
		names = append(names, command.Name)
	}
	return names
}

// cliSimilarCommand returns one unambiguous nearby command name for a typo.
// Commands are short ASCII words, so a small rune-based edit distance is
// sufficient and avoids suggesting an unrelated command.
func cliSimilarCommand(input string) string {
	best, bestDistance := "", 3
	for _, name := range cliCommandNames() {
		distance := cliEditDistance(input, name)
		if distance < bestDistance {
			best, bestDistance = name, distance
		} else if distance == bestDistance {
			best = ""
		}
	}
	return best
}

func cliEditDistance(left, right string) int {
	leftRunes, rightRunes := []rune(left), []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[rightIndex+1] = min(previous[rightIndex+1]+1, current[rightIndex]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	return previous[len(rightRunes)]
}

func cliUsage(name string) string {
	for _, command := range cliCommandSpecs {
		if command.Name == name {
			parts := []string{"rotari", command.Name}
			if len(command.Subcommands) > 0 {
				subcommands := make([]string, 0, len(command.Subcommands))
				for _, subcommand := range command.Subcommands {
					subcommands = append(subcommands, subcommand.Name)
				}
				parts = append(parts, "<"+strings.Join(subcommands, "|")+">")
			}
			for _, flagSpec := range command.Flags {
				longUsage := "--" + flagSpec.Name
				if flagSpec.ValueName != "" {
					longUsage += " " + flagSpec.ValueName
				}
				flagUsage := longUsage
				if short := cliShortFlagNames[flagSpec.Name]; short != "" {
					shortUsage := "-" + short
					if flagSpec.ValueName != "" {
						shortUsage += " " + flagSpec.ValueName
					}
					flagUsage = shortUsage + "|" + longUsage
				}
				parts = append(parts, "["+flagUsage+"]")
			}
			if command.Positional != "" {
				parts = append(parts, command.Positional)
			}
			return strings.Join(parts, " ")
		}
	}
	return "rotari " + name
}

// runCommandFlags returns the options of run, or with retry those of retry,
// which parses run's options and takes every one of them. Options whose
// meaning differs for retry, whose default selection is failed and
// unfinished jobs, are described for it.
func runCommandFlags(retry bool) []cliFlagSpec {
	flags := append(append(commonCLIFlags(),
		cliFlagSpec{Name: "run-id", Description: "build the run from this saved run without changing the next queue; without it, a result filter copies the latest run only into an empty queue and otherwise uses the queued jobs", ValueName: "ID"},
		cliFlagSpec{Name: "run-name", Description: "run name label", ValueName: "NAME"},
		cliFlagSpec{Name: "local-concurrency", Description: "local worker concurrency", ValueName: "N"},
		cliFlagSpec{Name: "batch-concurrency", Description: "scheduler job concurrency (Slurm/PBS/LSF/SGE)", ValueName: "N"},
		cliFlagSpec{Name: "retry", Description: "retry failed jobs up to N times; explicit cancellations are not retried", ValueName: "N"},
		cliFlagSpec{Name: "failed", Description: "only execute failed jobs; others carry forward their previous result"},
		cliFlagSpec{Name: "unfinished", Description: "only execute unfinished jobs; others carry forward their previous result"},
		cliFlagSpec{Name: "success", Description: "only execute successful jobs; others carry forward their previous result"},
		cliFlagSpec{Name: "job-id", Description: "only execute this job and the jobs that depend on it, through --depends-on or --depends-on-finished; may be repeated; not with a result filter; others carry forward their previous result", ValueName: "ID"},
		cliFlagSpec{Name: "job-name", Description: "only execute this job by name", ValueName: "NAME"},
		cliFlagSpec{Name: "stage", Description: "only execute jobs in this stage, narrowed by any result filter; others carry forward their previous result", ValueName: "STAGE"},
		cliFlagSpec{Name: "matrix", Description: "only execute jobs of this matrix, named by its base job name, narrowed by any result filter; others carry forward their previous result", ValueName: "NAME"},
		cliFlagSpec{Name: "partial-array", Description: "with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches"},
		cliFlagSpec{Name: "async", Description: "return after starting the run"},
		cliFlagSpec{Name: "disconnect-action", Description: "when the client input or connection closes unexpectedly, detach or cancel (default detach)", ValueName: "ACTION", Values: []string{"detach", "cancel"}},
		cliFlagSpec{Name: "quiet", Description: "suppress progress and completion output"},
		cliFlagSpec{Name: "executor", Description: "execution executor override", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
		cliFlagSpec{Name: "env", Description: "caller environment propagation mode (default ALL)", ValueName: "ALL|NONE", Values: []string{"ALL", "NONE"}, CommandLineOnly: true},
		cliFlagSpec{Name: "match-by", Description: "job identity matching", ValueName: "MODE", Values: []string{"job-id", "fingerprint", "id-and-fingerprint"}, CommandLineOnly: true},
		cliFlagSpec{Name: "executor-option", Description: "option passed to the selected scheduler (sbatch/qsub/...); may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "ssh-concurrency", Description: "SSH executor concurrency", ValueName: "N"},
		cliFlagSpec{Name: "ssh-options", Description: "SSH executor dispatch options; may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "slurm-concurrency", Description: "Slurm executor concurrency", ValueName: "N"},
		cliFlagSpec{Name: "slurm-options", Description: "Slurm executor dispatch options; may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "slurm-submit-interval", Description: "minimum Slurm submission interval", ValueName: "DURATION"},
		cliFlagSpec{Name: "slurm-submit-retry-limit", Description: "maximum retries for transient Slurm submission failures", ValueName: "N"},
		cliFlagSpec{Name: "pbs-concurrency", Description: "PBS executor concurrency", ValueName: "N"},
		cliFlagSpec{Name: "pbs-options", Description: "PBS executor dispatch options; may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "pbs-submit-interval", Description: "minimum PBS submission interval", ValueName: "DURATION"},
		cliFlagSpec{Name: "pbs-submit-retry-limit", Description: "maximum retries for transient PBS submission failures", ValueName: "N"},
		cliFlagSpec{Name: "lsf-concurrency", Description: "LSF executor concurrency", ValueName: "N"},
		cliFlagSpec{Name: "lsf-options", Description: "LSF executor dispatch options; may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "lsf-submit-interval", Description: "minimum LSF submission interval", ValueName: "DURATION"},
		cliFlagSpec{Name: "lsf-submit-retry-limit", Description: "maximum retries for transient LSF submission failures", ValueName: "N"},
		cliFlagSpec{Name: "sge-concurrency", Description: "SGE executor concurrency", ValueName: "N"},
		cliFlagSpec{Name: "sge-options", Description: "SGE executor dispatch options; may be repeated", ValueName: "OPTION"},
		cliFlagSpec{Name: "sge-submit-interval", Description: "minimum SGE submission interval", ValueName: "DURATION"},
		cliFlagSpec{Name: "sge-submit-retry-limit", Description: "maximum retries for transient SGE submission failures", ValueName: "N"},
	), jobFilterFlagSpecs(queueRunJobFilters)...)
	if !retry {
		return flags
	}
	retryDescriptions := map[string]string{
		"run-id":     "build the retry from this saved run without changing the next queue; without it, retry copies the latest run only into an empty queue and otherwise uses the queued jobs",
		"failed":     "only execute failed jobs, instead of failed and unfinished jobs; others carry forward their previous result",
		"unfinished": "only execute unfinished jobs, instead of failed and unfinished jobs; others carry forward their previous result",
		"success":    "only execute successful jobs, instead of failed and unfinished jobs; others carry forward their previous result",
		"job-id":     "only execute this job and the jobs that depend on it, through --depends-on or --depends-on-finished, instead of failed and unfinished jobs; may be repeated",
		"stage":      "only retry jobs in this stage",
		"matrix":     "only retry jobs of this matrix, named by its base job name",
	}
	for index := range flags {
		if description, ok := retryDescriptions[flags[index].Name]; ok {
			flags[index].Description = description
		}
	}
	return flags
}

// isHelpArgument reports whether arg requests help, matching the flag package.
func isHelpArgument(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "-help"
}

// printSubcommandHelp writes the help of a command that dispatches on a
// subcommand instead of parsing a FlagSet; see writeCommandHelp.
func printSubcommandHelp(name string) {
	writeCommandHelp(os.Stdout, name, nil)
}

// helpShown records that a command printed its help, so run exits 0 although
// the command returns 1 for the flag.ErrHelp its parse reported.
var helpShown bool

// helpCommandName names the command whose help fs belongs to: the command the
// user typed, which differs from fs.Name() when commands share a FlagSet, as
// retry shares run's.
func helpCommandName(fs *flag.FlagSet) string {
	if cliConfigCommand != "" {
		return cliConfigCommand
	}
	return fs.Name()
}

// writeCommandHelp writes command name's help from its cliCommandSpecs entry:
// its description, usage, subcommands, and each option with its
// description and, from fs when given, its effective default, which config
// files and the environment can change. `rotari guide` points here for
// options instead of listing them.
func writeCommandHelp(w io.Writer, name string, fs *flag.FlagSet) {
	helpShown = true
	var command cliCommandSpec
	for _, candidate := range cliCommandSpecs {
		if candidate.Name == name {
			command = candidate
		}
	}
	if command.Name == "" {
		fmt.Fprintf(w, "usage: rotari %s\n", name)
		if fs != nil {
			fs.SetOutput(w)
			fs.PrintDefaults()
		}
		return
	}
	var builder strings.Builder
	if command.Description != "" {
		fmt.Fprintf(&builder, "rotari %s: %s.\n\n", command.Name, upperFirst(command.Description))
	}
	usage := []string{"rotari", command.Name}
	if len(command.Subcommands) > 0 {
		usage = append(usage, "<"+strings.Join(cliSubcommandNames(command.Name), "|")+">")
	}
	if len(command.Flags) > 0 {
		usage = append(usage, "[options]")
	}
	if command.Positional != "" {
		usage = append(usage, command.Positional)
	}
	fmt.Fprintf(&builder, "usage: %s\n", strings.Join(usage, " "))
	if len(command.Subcommands) > 0 {
		width := 0
		for _, subcommand := range command.Subcommands {
			width = max(width, len(subcommand.Name))
		}
		builder.WriteString("\nSubcommands:\n")
		for _, subcommand := range command.Subcommands {
			fmt.Fprintf(&builder, "  %-*s  %s\n", width, subcommand.Name, subcommand.Description)
		}
	}
	// The per-executor and --filter-* options get headings of their own,
	// after the others.
	var general, filters strings.Builder
	executorOptions := map[string][]executorOption{}
	var executorKinds []string
	for _, flagSpec := range command.Flags {
		if kind, executorName := executorOptionKind(flagSpec.Name); kind != "" {
			if executorOptions[kind] == nil {
				executorKinds = append(executorKinds, kind)
			}
			executorOptions[kind] = append(executorOptions[kind], executorOption{spec: flagSpec, executor: executorName, defaultValue: helpDefault(fs, flagSpec.Name)})
			continue
		}
		target := &general
		if strings.HasPrefix(flagSpec.Name, filterFlagPrefix) {
			target = &filters
		}
		option := "--" + flagSpec.Name
		if flagSpec.ValueName != "" {
			option += " " + flagSpec.ValueName
		}
		if short := cliShortFlagNames[flagSpec.Name]; short != "" {
			option += ", -" + short
		}
		description := cliFlagDescriptionFor(name, flagSpec)
		// Some descriptions state their default, which the generated
		// references show; do not repeat it.
		if defaultValue := helpDefault(fs, flagSpec.Name); defaultValue != "" && !strings.Contains(description, "(default ") {
			description += " (default " + defaultValue + ")"
		}
		fmt.Fprintf(target, "  %s\n      %s\n", option, description)
	}
	if general.Len() > 0 {
		builder.WriteString("\nOptions:\n" + general.String())
	}
	if len(executorKinds) > 0 {
		builder.WriteString("\nExecutor options, each for the executor it names:\n")
		for _, kind := range executorKinds {
			writeExecutorOptions(&builder, name, kind, executorOptions[kind])
		}
	}
	if filters.Len() > 0 {
		builder.WriteString("\nFilters:\n" + filters.String())
	}
	fmt.Fprint(w, builder.String())
}

// parseLeadingFlags parses the options before a job command, for add and
// change: unlike cliParse it stops at the first argument that is not an
// option, which begins the job's command. Help is written as cliParse writes
// it.
func parseLeadingFlags(fs *flag.FlagSet, args []string) error {
	fs.Usage = func() {}
	err := parseCLIFlagSet(fs, args, true)
	if errors.Is(err, flag.ErrHelp) {
		writeCommandHelp(os.Stdout, helpCommandName(fs), fs)
	}
	return err
}

// helpDefault returns how help shows the default of the option name, or ""
// when help leaves it out.
func helpDefault(fs *flag.FlagSet, name string) string {
	if fs == nil {
		return ""
	}
	defined := fs.Lookup(name)
	if defined == nil || isZeroDefault(defined.DefValue) {
		return ""
	}
	// Quoted for string flags, as the flag package prints it.
	if kind, _ := flag.UnquoteUsage(defined); kind == "string" {
		return strconv.Quote(defined.DefValue)
	}
	return defined.DefValue
}

// executorOptionDescriptions describes each kind of per-executor option,
// such as --slurm-concurrency, which the scheduler and SSH executors repeat
// under their own names. Help lists each kind once.
var executorOptionDescriptions = map[string]string{
	"concurrency":        "executor concurrency",
	"options":            "executor dispatch options; may be repeated",
	"submit-interval":    "minimum submission interval",
	"submit-retry-limit": "maximum retries for transient submission failures",
}

type executorOption struct {
	spec         cliFlagSpec
	executor     string
	defaultValue string
}

// executorOptionKind returns the kind and executor of a per-executor option,
// such as "concurrency" and "slurm" for slurm-concurrency, or "" for another
// option. --local-concurrency stays among the general options, because the
// local executor is the default.
func executorOptionKind(name string) (kind, executorName string) {
	for _, candidate := range executorRegistry.Names() {
		if candidate == "local" {
			continue
		}
		if rest, ok := strings.CutPrefix(name, candidate+"-"); ok {
			if _, known := executorOptionDescriptions[rest]; known {
				return rest, candidate
			}
		}
	}
	return "", ""
}

// writeExecutorOptions writes one kind of per-executor option: every option
// name on one line, then the shared description. An environment variable or
// default shared by the options is shown once, with <EXECUTOR> standing for
// the executor in the variable's name.
func writeExecutorOptions(builder *strings.Builder, command, kind string, options []executorOption) {
	names := make([]string, 0, len(options))
	var environment, defaults, assignments []string
	for _, option := range options {
		names = append(names, "--"+option.spec.Name)
		if envName := cliCommandEnvironmentVariable(command, option.spec.Name); envName != "" && !option.spec.CommandLineOnly {
			environment = append(environment, strings.Replace(envName, "_"+strings.ToUpper(option.executor)+"_", "_<EXECUTOR>_", 1))
		}
		defaults = append(defaults, option.defaultValue)
		if option.defaultValue != "" {
			assignments = append(assignments, option.spec.Name+"="+option.defaultValue)
		}
	}
	line := "  " + strings.Join(names, ", ")
	if valueName := options[0].spec.ValueName; valueName != "" {
		line += " " + valueName
	}
	description := executorOptionDescriptions[kind]
	if environment = slices.Compact(environment); len(environment) > 0 {
		description += " (env: " + strings.Join(environment, ", ") + ")"
	}
	if len(slices.Compact(defaults)) == 1 && defaults[0] != "" {
		description += " (default " + defaults[0] + ")"
	} else if len(assignments) > 0 {
		description += " (defaults: " + strings.Join(assignments, ", ") + ")"
	}
	fmt.Fprintf(builder, "%s\n      %s\n", line, description)
}

// isZeroDefault reports a default that help leaves out, as the flag package
// does.
func isZeroDefault(value string) bool {
	return value == "" || value == "false" || value == "0" || value == "[]"
}

func cliSubcommandNames(name string) []string {
	for _, command := range cliCommandSpecs {
		if command.Name != name {
			continue
		}
		names := make([]string, 0, len(command.Subcommands))
		for _, subcommand := range command.Subcommands {
			names = append(names, subcommand.Name)
		}
		return names
	}
	return nil
}

// cliCommandFlag returns the metadata of flag name as defined by command, so
// commands that share a flag name keep their own descriptions. Internal
// commands without metadata fall back to cliFlag.
func cliCommandFlag(command, name string) cliFlagSpec {
	commandFound := false
	for _, commandSpec := range cliCommandSpecs {
		if commandSpec.Name != command {
			continue
		}
		commandFound = true
		for _, flagSpec := range commandSpec.Flags {
			if flagSpec.Name == name {
				return flagSpec
			}
		}
	}
	// Tests and internal callers may add a local flag directly to a FlagSet
	// without adding public CLI metadata.
	if !commandFound {
		return cliFlag(name)
	}
	return cliFlagSpec{Name: name}
}

// cliFlag returns the first metadata defined for flag name by any command.
func cliFlag(name string) cliFlagSpec {
	for _, command := range cliCommandSpecs {
		for _, flagSpec := range command.Flags {
			if flagSpec.Name == name {
				return flagSpec
			}
		}
	}
	panic("undefined CLI flag: " + name)
}

func cliString(fs *flag.FlagSet, name, defaultValue string) *string {
	if fs.Name() != "config" && fs.Lookup("config") == nil {
		configSpec := cliCommandFlag(fs.Name(), "config")
		fs.String(configSpec.Name, "", cliFlagDescription(configSpec))
	}
	spec := cliCommandFlag(fs.Name(), name)
	if !spec.CommandLineOnly {
		ignoreImplicitLocation := cliIgnoreImplicitLocationDefaults[name]
		if !ignoreImplicitLocation {
			defaultValue = configString(name, defaultValue)
		}
		if !ignoreImplicitLocation {
			if envName := cliCommandEnvironmentVariable(fs.Name(), name); envName != "" {
				if value, ok := os.LookupEnv(envName); ok {
					defaultValue = value
				}
			}
		}
	}
	target := new(string)
	*target = defaultValue
	description := cliFlagDescriptionFor(fs.Name(), spec)
	if len(spec.Values) > 0 {
		value := &cliChoiceValue{target: target, choices: spec.Values}
		fs.Var(value, spec.Name, description)
		if short := cliShortFlagNames[name]; short != "" {
			fs.Var(value, short, description+" (shorthand)")
		}
		return target
	}
	fs.StringVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.StringVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

// cliParse parses args with options allowed before, between, and after
// positional arguments; "--" ends the options, and every later argument is
// positional. add and change call fs.Parse instead: their positional
// arguments are a job command, whose own options must reach the job.
func cliParse(fs *flag.FlagSet, args []string) error {
	var parseOutput strings.Builder
	originalOutput := fs.Output()
	fs.SetOutput(&parseOutput)
	defer fs.SetOutput(originalOutput)

	var positional []string
	for {
		if err := parseCLIFlagSet(fs, args, false); err != nil {
			output := parseOutput.String()
			if err != flag.ErrHelp {
				if newline := strings.IndexByte(output, '\n'); newline >= 0 {
					// The flag package follows its error with every option's
					// description; point to the command's help instead.
					printError(cliFlagError(output[:newline]))
					if strings.TrimSpace(output[newline+1:]) != "" {
						fmt.Fprintf(originalOutput, "Run 'rotari %s --help' to list its options.\n", helpCommandName(fs))
					}
				} else if output != "" {
					printError(cliFlagError(strings.TrimSuffix(output, "\n")))
				}
			} else {
				writeCommandHelp(os.Stdout, helpCommandName(fs), fs)
			}
			return err
		}
		rest := fs.Args()
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			positional = append(positional, rest...)
			break
		}
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	return fs.Parse(append([]string{"--"}, positional...))
}

// cliFlagErrorPatterns rewrites the flag package's errors to name options
// with the dashes users type: two for long names, one for single letters.
// Each format receives the option name, then the pattern's other groups.
var cliFlagErrorPatterns = []struct {
	pattern *regexp.Regexp
	format  string
}{
	{regexp.MustCompile(`^flag provided but not defined: -(\S+)$`), "unknown option %s"},
	{regexp.MustCompile(`^flag needs an argument: -(\S+)$`), "option %s needs a value"},
	{regexp.MustCompile(`^invalid boolean value (?P<value>".*") for -(\S+): (?P<reason>.*)$`), "invalid boolean value %[2]s for option %[1]s: %[3]s"},
	{regexp.MustCompile(`^invalid value (?P<value>".*") for flag -(\S+): (?P<reason>.*)$`), "invalid value %[2]s for option %[1]s: %[3]s"},
}

func cliFlagError(message string) string {
	for _, rule := range cliFlagErrorPatterns {
		match := rule.pattern.FindStringSubmatch(message)
		if match == nil {
			continue
		}
		args := []any{}
		for index, group := range rule.pattern.SubexpNames()[1:] {
			if group == "" {
				args = append([]any{cliOptionName(match[index+1])}, args...)
			} else {
				args = append(args, match[index+1])
			}
		}
		return fmt.Sprintf(rule.format, args...)
	}
	return message
}

func cliOptionName(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

func parseCLIFlagSet(fs *flag.FlagSet, args []string, stopAtPositional bool) error {
	if err := rejectDuplicateSingleValueFlags(fs, args, stopAtPositional); err != nil {
		fmt.Fprintln(fs.Output(), err)
		return err
	}
	return fs.Parse(args)
}

func cliOptionSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(actual *flag.Flag) {
		if actual.Name == name || actual.Name == cliShortFlagNames[name] {
			set = true
		}
	})
	return set
}

func rejectDuplicateSingleValueFlags(fs *flag.FlagSet, args []string, stopAtPositional bool) error {
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		name, hasValue, isOption, endOptions := cliArgumentOption(args[i])
		if endOptions {
			break
		}
		if !isOption {
			if stopAtPositional {
				break
			}
			continue
		}
		option := fs.Lookup(name)
		if option == nil {
			continue
		}
		canonical := cliCanonicalFlagName(name)
		if err := recordSingleValueOption(fs.Name(), canonical, seen); err != nil {
			return err
		}
		if cliOptionConsumesNextArgument(option, hasValue) {
			i++
		}
	}
	return nil
}

func cliArgumentOption(arg string) (name string, hasValue, isOption, endOptions bool) {
	if arg == "--" {
		return "", false, false, true
	}
	if len(arg) < 2 || arg[0] != '-' {
		return "", false, false, false
	}
	if strings.HasPrefix(arg, "--") {
		arg = strings.TrimPrefix(arg, "--")
	} else {
		arg = strings.TrimPrefix(arg, "-")
	}
	name, _, hasValue = strings.Cut(arg, "=")
	return name, hasValue, true, false
}

func cliCanonicalFlagName(name string) string {
	for long, short := range cliShortFlagNames {
		if short == name {
			return long
		}
	}
	return name
}

func recordSingleValueOption(command, name string, seen map[string]bool) error {
	if cliFlagRepeated(cliCommandFlag(command, name)) {
		return nil
	}
	if seen[name] {
		return fmt.Errorf("flag --%s cannot be specified more than once", name)
	}
	seen[name] = true
	return nil
}

func cliOptionConsumesNextArgument(option *flag.Flag, hasValue bool) bool {
	if hasValue {
		return false
	}
	boolean, ok := option.Value.(interface{ IsBoolFlag() bool })
	return !ok || !boolean.IsBoolFlag()
}

func cliFlagRepeated(spec cliFlagSpec) bool {
	return spec.Repeated || strings.Contains(spec.Description, "may be repeated")
}

type cliChoiceValue struct {
	target  *string
	choices []string
}

func (value *cliChoiceValue) String() string {
	if value == nil || value.target == nil {
		return ""
	}
	return *value.target
}

func (value *cliChoiceValue) Set(candidate string) error {
	for _, choice := range value.choices {
		if candidate == choice {
			*value.target = candidate
			return nil
		}
	}
	return fmt.Errorf("invalid choice %q (choose from %s)", candidate, strings.Join(value.choices, ", "))
}

func cliFlagDescription(spec cliFlagSpec) string {
	return cliFlagDescriptionFor(cliConfigCommand, spec)
}

func cliFlagDescriptionFor(command string, spec cliFlagSpec) string {
	description := spec.Description
	if len(spec.Values) > 0 {
		description += " (choices: " + strings.Join(spec.Values, ", ") + ")"
	}
	if envName := cliCommandEnvironmentVariable(command, spec.Name); envName != "" && !spec.CommandLineOnly {
		description += " (env: " + envName + ")"
		if spec.Name == "quiet" {
			description += " (also ROTARI_QUIET)"
		}
	}
	return description
}

// cliCommandEnvironmentVariable preserves explicit shared/legacy names and
// derives ROTARI_<COMMAND>_<OPTION> for every other CLI-default option.
func cliCommandEnvironmentVariable(command, name string) string {
	if name == "quiet" && command != "" {
		if command == "retry" {
			command = "run" // retry parses the shared run flag set.
		}
		return commandQuietEnvironmentVariable(command)
	}
	if environment, ok := cliEnvironmentVariables[name]; ok {
		return environment
	}
	if command == "" {
		return ""
	}
	if command == "retry" {
		command = "run" // retry parses the shared run flag set.
	}
	return "ROTARI_" + strings.ToUpper(strings.ReplaceAll(command, "-", "_")) + "_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func cliStringVar(fs *flag.FlagSet, target *string, name, defaultValue string) {
	spec := cliCommandFlag(fs.Name(), name)
	description := cliFlagDescriptionFor(fs.Name(), spec)
	fs.StringVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.StringVar(target, short, defaultValue, description+" (shorthand)")
	}
}

func cliBool(fs *flag.FlagSet, name string, defaultValue bool) *bool {
	spec := cliCommandFlag(fs.Name(), name)
	environmentNames := []string{cliCommandEnvironmentVariable(fs.Name(), name)}
	if name == "quiet" {
		environmentNames = []string{envQuiet, cliCommandEnvironmentVariable(fs.Name(), name)}
	}
	if spec.CommandLineOnly {
		environmentNames = nil
	} else {
		defaultValue = configBool(name, defaultValue)
	}
	for _, environmentName := range environmentNames {
		if value, ok := os.LookupEnv(environmentName); ok {
			if parsed, err := strconv.ParseBool(value); err == nil {
				defaultValue = parsed
			}
		}
	}
	target := new(bool)
	description := cliFlagDescriptionFor(fs.Name(), spec)
	fs.BoolVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.BoolVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

func commandQuietEnvironmentVariable(command string) string {
	return "ROTARI_" + strings.ToUpper(strings.ReplaceAll(command, "-", "_")) + "_QUIET"
}

func cliInt(fs *flag.FlagSet, name string, defaultValue int) *int {
	spec := cliCommandFlag(fs.Name(), name)
	if !spec.CommandLineOnly {
		defaultValue = configInt(name, defaultValue)
		if value, ok := os.LookupEnv(cliCommandEnvironmentVariable(fs.Name(), name)); ok {
			if parsed, err := strconv.Atoi(value); err == nil {
				defaultValue = parsed
			}
		}
	}
	target := new(int)
	description := cliFlagDescriptionFor(fs.Name(), spec)
	fs.IntVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.IntVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

func cliDuration(fs *flag.FlagSet, name string, defaultValue time.Duration) *time.Duration {
	spec := cliCommandFlag(fs.Name(), name)
	if spec.CommandLineOnly {
		target := new(time.Duration)
		fs.DurationVar(target, spec.Name, defaultValue, cliFlagDescriptionFor(fs.Name(), spec))
		return target
	}
	if value, ok := configValue(name); ok {
		if parsed, err := time.ParseDuration(fmt.Sprint(value)); err == nil {
			defaultValue = parsed
		}
	}
	if envName := cliCommandEnvironmentVariable(fs.Name(), name); envName != "" {
		if value, ok := os.LookupEnv(envName); ok {
			if parsed, err := time.ParseDuration(value); err == nil {
				defaultValue = parsed
			}
		}
	}
	target := new(time.Duration)
	description := cliFlagDescriptionFor(fs.Name(), spec)
	fs.DurationVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.DurationVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

func cliValue(fs *flag.FlagSet, target flag.Value, name string) {
	spec := cliCommandFlag(fs.Name(), name)
	if !spec.CommandLineOnly {
		for _, value := range configStrings(name) {
			_ = target.Set(value)
		}
	}
	if envName := cliCommandEnvironmentVariable(fs.Name(), name); envName != "" && !spec.CommandLineOnly {
		value, exists := os.LookupEnv(envName)
		if exists && value != "" {
			if resettable, ok := target.(interface{ Reset() }); ok {
				resettable.Reset()
			}
			_ = target.Set(value)
		}
	}
	description := cliFlagDescriptionFor(fs.Name(), spec)
	fs.Var(target, spec.Name, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.Var(target, short, description+" (shorthand)")
	}
}
