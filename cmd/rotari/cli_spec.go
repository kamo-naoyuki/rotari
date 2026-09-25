package main

import (
	"flag"
	"fmt"
	"os"
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
	"run-name":                 envRunName,
	"provider":                 envLLMProvider,
	"endpoint":                 envLLMEndpoint,
	"model":                    envLLMModel,
	"language":                 envLLMLanguage,
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
	"recover":                  envResetRecover,
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
		{Name: "basedir", Description: "state directory", ValueName: "DIR"},
		{Name: "project-name", Description: "project name", ValueName: "NAME"},
	}
}

var cliCommandSpecs = []cliCommandSpec{
	{
		Name:        "config",
		Description: "generate a config file template",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "list", Description: "list existing config files"},
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
			cliFlagSpec{Name: "recover", Description: "confirm an interrupted run has stopped without prompting"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name: "cancel",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "cancel a running job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "wait", Description: "wait until cancellation is complete"},
		),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "suspend",
		Description: "suspend running jobs",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "suspend a running job; may be repeated", ValueName: "ID"},
		),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "resume",
		Description: "resume suspended jobs",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "job-id", Description: "resume a suspended job; may be repeated", ValueName: "ID"},
		),
		Positional: "[JOB_ID|ATTEMPT_ID|RUN_ID ...]",
	},
	{
		Name:        "delete",
		Description: "delete saved run history",
		Flags:       append(commonCLIFlags(), cliFlagSpec{Name: "run-id", Description: "delete only the specified run", ValueName: "ID"}),
		Positional:  "[RUN_ID]",
	},
	{
		Name:        "gc",
		Description: "find and remove orphan run registry entries",
		Flags: []cliFlagSpec{
			{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			{Name: "apply", Description: "remove the cached orphan entries"},
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
		Description: "change a job in the current or previous batch",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID to use when restoring a batch", ValueName: "ID"},
			cliFlagSpec{Name: "job-id", Description: "target job ID", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "target job name", ValueName: "NAME"},
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
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "<command ...>",
	},
	{
		Name:        "export",
		Description: "export the current queue or saved runs as a workflow manifest",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID to export; may be repeated", ValueName: "ID", Repeated: true},
			cliFlagSpec{Name: "format", Description: "manifest format: yaml, toml, or json", ValueName: "FORMAT", Values: []string{"yaml", "toml", "json"}},
			cliFlagSpec{Name: "template", Description: "print a starter workflow manifest"},
		),
		Positional: "[PROJECT|RUN_ID ...]",
	},
	{
		Name:        "import",
		Description: "validate and replace a queue from a workflow manifest",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue"},
			cliFlagSpec{Name: "dry-run", Description: "validate and print the import plan without writing"},
			cliFlagSpec{Name: "json", Description: "print the import plan as JSON"},
		),
		Positional: "FILE [PROJECT|RUN_ID]",
	},
	{
		Name:        "remove",
		Description: "remove jobs from the current or previous batch",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID to use when restoring a batch", ValueName: "ID"},
			cliFlagSpec{Name: "job-id", Description: "remove a job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "remove a job by name", ValueName: "NAME"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "[JOB_ID ...]",
	},
	{
		Name:        "show",
		Description: "show queue or run status",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "masterdir", Description: "master registry directory", ValueName: "DIR"},
			cliFlagSpec{Name: "run-id", Description: "run ID or latest", ValueName: "ID"},
			cliFlagSpec{Name: "queue", Description: "show the current queue even when a run is selected"},
			cliFlagSpec{Name: "job-id", Description: "job ID", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "job name", ValueName: "NAME"},
			cliFlagSpec{Name: "failed", Description: "show failed jobs only"},
			cliFlagSpec{Name: "stage", Description: "show jobs in this stage only", ValueName: "NAME"},
			cliFlagSpec{Name: "logs", Description: "print output logs for all jobs"},
			cliFlagSpec{Name: "failed-logs", Description: "print output logs for failed jobs"},
			cliFlagSpec{Name: "follow", Description: "follow log output until the run completes"},
			cliFlagSpec{Name: "no-pager", Description: "print logs directly instead of using a pager"},
			cliFlagSpec{Name: "basedirs", Description: "list state directories known to the master registry"},
			cliFlagSpec{Name: "json", Description: "print machine-readable JSON for a run"},
			cliFlagSpec{Name: "report", Description: "print an AI-ready Markdown report"},
		),
		Positional: "[SELECTOR]",
	},
	{
		Name:        "jobs",
		Description: "list running and recently finished jobs across projects",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "masterdir", Description: "master registry directory for --all", ValueName: "DIR"},
			cliFlagSpec{Name: "all", Description: "include all basedirs known to the master registry"},
			cliFlagSpec{Name: "format", Description: "output fields; use %s %b %p %a %n %c %t %f %e (%f is finished time)", ValueName: "FORMAT"},
			cliFlagSpec{Name: "since", Description: "include jobs finished within this duration; use 0 for running jobs only", ValueName: "DURATION"},
		),
		Positional: "[PROJECT]",
	},
	{
		Name:        "diagnose",
		Description: "diagnose one job with an LLM or local error rules",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID", ValueName: "ID"},
			cliFlagSpec{Name: "job-id", Description: "failed job ID", ValueName: "ID"},
			cliFlagSpec{Name: "rules", Description: "use local rule-based diagnosis without calling an LLM"},
			cliFlagSpec{Name: "provider", Description: "LLM provider: openai, openai-chat, anthropic, gemini, or cohere", ValueName: "PROVIDER", Values: []string{"openai", "openai-chat", "anthropic", "gemini", "cohere"}},
			cliFlagSpec{Name: "endpoint", Description: "LLM API endpoint", ValueName: "URL"},
			cliFlagSpec{Name: "model", Description: "LLM model name", ValueName: "MODEL"},
			cliFlagSpec{Name: "language", Description: "response language BCP 47 tag", ValueName: "TAG"},
		),
		Positional: "JOB_ID",
	},
	{
		Name:        "wait",
		Description: "wait for an asynchronous run by project, run name, or run ID",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "run ID; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "timeout", Description: "maximum wait duration", ValueName: "DURATION"},
			cliFlagSpec{Name: "json", Description: "print each completed run as one JSON object"},
		),
		Positional: "[PROJECT_OR_RUN_NAME_OR_RUN_ID ...]",
	},
	{
		Name:        "add",
		Description: "add a command to a queue",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "executor", Description: "job executor", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
			cliFlagSpec{Name: "executor-option", Description: "option passed to the selected scheduler (sbatch/qsub/...); may be repeated", ValueName: "OPTION"},
			cliFlagSpec{Name: "working-directory", Description: "working directory for the job", ValueName: "DIR"},
			cliFlagSpec{Name: "env", Description: "environment variable for the job; may be repeated", ValueName: "KEY=VALUE"},
			cliFlagSpec{Name: "job-name", Description: "job name label", ValueName: "NAME"},
			cliFlagSpec{Name: "stage", Description: "stage that contains the job", ValueName: "NAME"},
			cliFlagSpec{Name: "depends-on", Description: "name of a prerequisite job or stage; may be repeated", ValueName: "NAME"},
			cliFlagSpec{Name: "array", Description: "create an array job range or selected tasks", ValueName: "FIRST-LAST|TASK[,TASK...]"},
			cliFlagSpec{Name: "matrix", Description: "expand a command into jobs from KEY=VALUE[,VALUE...] dimensions; may be repeated", ValueName: "KEY=VALUE[,VALUE...]"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "<command ...>",
	},
	{
		Name:        "copy",
		Description: "copy the latest run's jobs into the queue",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "source run ID; defaults to the latest run", ValueName: "ID"},
			cliFlagSpec{Name: "failed", Description: "include failed jobs; may be combined with result filters"},
			cliFlagSpec{Name: "unfinished", Description: "include unfinished jobs; may be combined with result filters"},
			cliFlagSpec{Name: "success", Description: "include successful jobs; may be combined with result filters"},
			cliFlagSpec{Name: "job-id", Description: "copy a job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "copy a job by name", ValueName: "NAME"},
			cliFlagSpec{Name: "append", Description: "append to a non-empty queue"},
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue"},
			cliFlagSpec{Name: "quiet", Description: "suppress success output"},
		),
		Positional: "[RUN_ID]",
	},
	{
		Name:        "run",
		Description: "execute queued commands, optionally selecting jobs from a run",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "repopulate the queue from this run before executing (copy --run-id + run); defaults to the latest run when a result filter is used", ValueName: "ID"},
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue without prompting; requires --run-id"},
			cliFlagSpec{Name: "run-name", Description: "run name label", ValueName: "NAME"},
			cliFlagSpec{Name: "local-concurrency", Description: "local worker concurrency", ValueName: "N"},
			cliFlagSpec{Name: "batch-concurrency", Description: "scheduler job concurrency (Slurm/PBS/...)", ValueName: "N"},
			cliFlagSpec{Name: "retry", Description: "retry failed jobs up to N times; explicit cancellations are not retried", ValueName: "N"},
			cliFlagSpec{Name: "failed", Description: "only execute failed jobs; others carry forward their previous result"},
			cliFlagSpec{Name: "unfinished", Description: "only execute unfinished jobs; others carry forward their previous result"},
			cliFlagSpec{Name: "success", Description: "only execute successful jobs; others carry forward their previous result"},
			cliFlagSpec{Name: "job-id", Description: "only execute this job; may be repeated; others carry forward their previous result", ValueName: "ID"},
			cliFlagSpec{Name: "job-name", Description: "only execute this job by name", ValueName: "NAME"},
			cliFlagSpec{Name: "partial-array", Description: "with a result filter, select array jobs per task instead of all-or-nothing (default true); pass =false to re-execute the whole array when any task matches"},
			cliFlagSpec{Name: "async", Description: "return after starting the run"},
			cliFlagSpec{Name: "quiet", Description: "suppress progress and completion output"},
			cliFlagSpec{Name: "executor", Description: "execution executor override", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
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
		),
	},
	{
		Name:        "retry",
		Description: "alias for run --failed --unfinished",
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "run-id", Description: "repopulate the queue from this run before executing; defaults to the latest run", ValueName: "ID"},
			cliFlagSpec{Name: "overwrite", Description: "replace a non-empty queue without prompting; requires --run-id"},
			cliFlagSpec{Name: "run-name", Description: "run name label", ValueName: "NAME"},
			cliFlagSpec{Name: "local-concurrency", Description: "local worker concurrency", ValueName: "N"},
			cliFlagSpec{Name: "batch-concurrency", Description: "scheduler job concurrency (Slurm/PBS/...)", ValueName: "N"},
			cliFlagSpec{Name: "retry", Description: "retry failed jobs up to N times; explicit cancellations are not retried", ValueName: "N"},
			cliFlagSpec{Name: "job-id", Description: "also execute this job; may be repeated", ValueName: "ID"},
			cliFlagSpec{Name: "async", Description: "return after starting the run"},
			cliFlagSpec{Name: "quiet", Description: "suppress progress and completion output"},
			cliFlagSpec{Name: "executor", Description: "execution executor override", ValueName: "EXECUTOR", Values: executorRegistry.Names()},
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
		),
	},
	{
		Name:        "server",
		Description: "manage the background server",
		Flags: []cliFlagSpec{
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
		Flags: append(commonCLIFlags(),
			cliFlagSpec{Name: "host", Description: "HTTP listen host", ValueName: "HOST"},
			cliFlagSpec{Name: "port", Description: "HTTP listen port", ValueName: "PORT"},
			cliFlagSpec{Name: "static-dir", Description: "generate a static web UI", ValueName: "DIR"},
			cliFlagSpec{Name: "allow-control", Description: "enable job control (copy/change/remove/cancel/clear); pass --allow-control=false for a read-only UI"},
			cliFlagSpec{Name: "auth-token", Description: "require this token in Authorization: Bearer or X-Rotari-Token; prefer ROTARI_WEB_AUTH_TOKEN for secrets", ValueName: "TOKEN"},
			cliFlagSpec{Name: "notifications", Description: "default state of the browser desktop-notification toggle; pass --notifications=false to default it off"},
		),
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
		Flags:       []cliFlagSpec{{Name: "json", Description: "print the schema as JSON"}},
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

// isHelpArgument reports whether arg requests help, matching the flag package.
func isHelpArgument(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "-help"
}

// printSubcommandHelp writes usage and subcommand descriptions for commands
// that dispatch on a subcommand instead of parsing a FlagSet. Like FlagSet
// help, it writes to stderr and the caller returns exit status 1.
func printSubcommandHelp(name string) {
	var builder strings.Builder
	fmt.Fprintf(&builder, "usage: %s\n", cliUsage(name))
	for _, command := range cliCommandSpecs {
		if command.Name != name || len(command.Subcommands) == 0 {
			continue
		}
		width := 0
		for _, subcommand := range command.Subcommands {
			width = max(width, len(subcommand.Name))
		}
		builder.WriteString("\nSubcommands:\n")
		for _, subcommand := range command.Subcommands {
			fmt.Fprintf(&builder, "  %-*s  %s\n", width, subcommand.Name, subcommand.Description)
		}
	}
	fmt.Fprint(os.Stderr, builder.String())
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
	spec := cliFlag(name)
	defaultValue = configString(name, defaultValue)
	if envName := cliEnvironmentVariable(name); envName != "" {
		if value, ok := os.LookupEnv(envName); ok {
			defaultValue = value
		}
	}
	target := new(string)
	*target = defaultValue
	description := cliFlagDescription(spec)
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

func cliOptionSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(actual *flag.Flag) {
		if actual.Name == name || actual.Name == cliShortFlagNames[name] {
			set = true
		}
	})
	return set
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
	description := spec.Description
	if len(spec.Values) > 0 {
		description += " (choices: " + strings.Join(spec.Values, ", ") + ")"
	}
	if envName := cliEnvironmentVariable(spec.Name); envName != "" {
		description += " (env: " + envName + ")"
		if spec.Name == "quiet" {
			description += " (command env: ROTARI_<COMMAND>_QUIET)"
		}
	}
	return description
}

func cliEnvironmentVariable(name string) string {
	return cliEnvironmentVariables[name]
}

func cliStringVar(fs *flag.FlagSet, target *string, name, defaultValue string) {
	spec := cliFlag(name)
	description := cliFlagDescription(spec)
	fs.StringVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.StringVar(target, short, defaultValue, description+" (shorthand)")
	}
}

func cliBool(fs *flag.FlagSet, name string, defaultValue bool) *bool {
	spec := cliFlag(name)
	defaultValue = configBool(name, defaultValue)
	environmentNames := []string{cliEnvironmentVariable(name)}
	if name == "quiet" {
		environmentNames = []string{envQuiet, commandQuietEnvironmentVariable(fs.Name())}
	}
	for _, environmentName := range environmentNames {
		if value, ok := os.LookupEnv(environmentName); ok {
			if parsed, err := strconv.ParseBool(value); err == nil {
				defaultValue = parsed
			}
		}
	}
	target := new(bool)
	description := cliFlagDescription(spec)
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
	spec := cliFlag(name)
	defaultValue = configInt(name, defaultValue)
	if value, ok := os.LookupEnv(cliEnvironmentVariable(name)); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			defaultValue = parsed
		}
	}
	target := new(int)
	description := cliFlagDescription(spec)
	fs.IntVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.IntVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

func cliDuration(fs *flag.FlagSet, name string, defaultValue time.Duration) *time.Duration {
	spec := cliFlag(name)
	if value, ok := configValue(name); ok {
		if parsed, err := time.ParseDuration(fmt.Sprint(value)); err == nil {
			defaultValue = parsed
		}
	}
	if envName := cliEnvironmentVariable(name); envName != "" {
		if value, ok := os.LookupEnv(envName); ok {
			if parsed, err := time.ParseDuration(value); err == nil {
				defaultValue = parsed
			}
		}
	}
	target := new(time.Duration)
	description := cliFlagDescription(spec)
	fs.DurationVar(target, spec.Name, defaultValue, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.DurationVar(target, short, defaultValue, description+" (shorthand)")
	}
	return target
}

func cliValue(fs *flag.FlagSet, target flag.Value, name string) {
	spec := cliFlag(name)
	for _, value := range configStrings(name) {
		_ = target.Set(value)
	}
	if envName := cliEnvironmentVariable(name); envName != "" {
		value, exists := os.LookupEnv(envName)
		if exists && value != "" {
			if resettable, ok := target.(interface{ Reset() }); ok {
				resettable.Reset()
			}
			_ = target.Set(value)
		}
	}
	description := cliFlagDescription(spec)
	fs.Var(target, spec.Name, description)
	if short := cliShortFlagNames[name]; short != "" {
		fs.Var(target, short, description+" (shorthand)")
	}
}
