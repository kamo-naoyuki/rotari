package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	envBaseDir         = "ROTARI_BASEDIR"
	envProjectName     = "ROTARI_PROJECT_NAME"
	envMasterDir       = "ROTARI_MASTERDIR"
	envRunID           = "ROTARI_RUN_ID"
	envJobID           = "ROTARI_JOB_ID"
	envAttemptID       = "ROTARI_ATTEMPT_ID"
	envJobName         = "ROTARI_JOB_NAME"
	envExecutor        = "ROTARI_EXECUTOR"
	envExecutorOpts    = "ROTARI_EXECUTOR_OPTIONS"
	envRunName         = "ROTARI_RUN_NAME"
	envRunLocalConc    = "ROTARI_RUN_LOCAL_CONCURRENCY"
	envRunBatchConc    = "ROTARI_RUN_BATCH_CONCURRENCY"
	envRunSSHConc      = "ROTARI_RUN_SSH_CONCURRENCY"
	envRunSSHOptions   = "ROTARI_RUN_SSH_OPTIONS"
	envRunSlurmConc    = "ROTARI_RUN_SLURM_CONCURRENCY"
	envRunSlurmOptions = "ROTARI_RUN_SLURM_OPTIONS"
	envRunPBSConc      = "ROTARI_RUN_PBS_CONCURRENCY"
	envRunPBSOptions   = "ROTARI_RUN_PBS_OPTIONS"
	envRunLSFConc      = "ROTARI_RUN_LSF_CONCURRENCY"
	envRunLSFOptions   = "ROTARI_RUN_LSF_OPTIONS"
	envRunRetry        = "ROTARI_RUN_RETRY"
	envRunAsync        = "ROTARI_RUN_ASYNC"
	envArrayRange      = "ROTARI_ARRAY_RANGE"
	envResetRecover    = "ROTARI_RESET_RECOVER"
	envWaitTimeout     = "ROTARI_WAIT_TIMEOUT"
	envWebHost         = "ROTARI_WEB_HOST"
	envWebPort         = "ROTARI_WEB_PORT"
	envWebStaticDir    = "ROTARI_WEB_STATIC_DIR"
	envWebAllowControl = "ROTARI_WEB_ALLOW_CONTROL"
	envWebAuthToken    = "ROTARI_WEB_AUTH_TOKEN"
	envLLMAPIKey       = "ROTARI_LLM_API_KEY"
	envLLMProvider     = "ROTARI_LLM_PROVIDER"
	envLLMEndpoint     = "ROTARI_LLM_ENDPOINT"
	envLLMModel        = "ROTARI_LLM_MODEL"
	envLLMLanguage     = "ROTARI_LLM_LANGUAGE"
	envWebhookURL      = "ROTARI_WEBHOOK_URL"
	envWebhookOn       = "ROTARI_WEBHOOK_ON"
	envWebhookFormat   = "ROTARI_WEBHOOK_FORMAT"
	envPrivateState    = "ROTARI_PRIVATE_STATE"
	envBin             = "ROTARI_BIN"
	envRunDir          = "ROTARI_RUN_DIR"
	envJobDir          = "ROTARI_JOB_DIR"
	envCWD             = "ROTARI_CWD"
	envArrayTaskID     = "ROTARI_ARRAY_TASK_ID"
	envArrayFirst      = "ROTARI_ARRAY_FIRST"
	envArrayLast       = "ROTARI_ARRAY_LAST"
	envArraySize       = "ROTARI_ARRAY_SIZE"
)

var propagatedEnvironmentVariables = []string{
	envBaseDir, envProjectName, envMasterDir, envRunID, envJobID, envAttemptID, envJobName,
	envExecutor, envExecutorOpts, envRunName, envRunLocalConc, envRunBatchConc,
	envRunSSHConc, envRunSSHOptions, envRunSlurmConc, envRunSlurmOptions,
	envRunPBSConc, envRunPBSOptions, envRunLSFConc, envRunLSFOptions,
	envRunRetry, envRunAsync, envArrayRange,
}

func validateEnvironment(environment []string) error {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !validEnvironmentName(name) || strings.ContainsRune(value, '\x00') {
			return errors.New("expected KEY=VALUE")
		}
	}
	return nil
}

func validEnvironmentName(name string) bool {
	for index := 0; index < len(name); index++ {
		character := name[index]
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' {
			continue
		}
		if index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return name != ""
}

type environmentDefinition struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Set         bool   `json:"set,omitempty"`
	CLIDefault  bool   `json:"cli_default"`
	Job         bool   `json:"job"`
	Array       bool   `json:"array"`
	Description string `json:"description"`
}

func environmentDefinitions() []environmentDefinition {
	return []environmentDefinition{
		{Name: envBaseDir, CLIDefault: true, Job: true, Array: true, Description: "State directory; --basedir default."},
		{Name: envProjectName, CLIDefault: true, Job: true, Array: true, Description: "Project name; --project-name default."},
		{Name: envMasterDir, CLIDefault: true, Description: "Server registry directory; --masterdir default."},
		{Name: envRunID, CLIDefault: true, Job: true, Array: true, Description: "Current run ID; --run-id default."},
		{Name: envJobID, CLIDefault: true, Job: true, Array: true, Description: "Current job ID; --job-id default."},
		{Name: envAttemptID, Job: true, Array: true, Description: "Current job attempt ID."},
		{Name: envJobName, CLIDefault: true, Job: true, Array: true, Description: "Current job name; --job-name default."},
		{Name: envExecutor, CLIDefault: true, Job: true, Array: true, Description: "Current executor; --executor default."},
		{Name: envExecutorOpts, CLIDefault: true, Job: true, Array: true, Description: "Default scheduler executor options."},
		{Name: envRunName, CLIDefault: true, Job: true, Array: true, Description: "Run name; --run-name default."},
		{Name: envRunLocalConc, CLIDefault: true, Job: true, Array: true, Description: "Local worker limit; --local-concurrency default."},
		{Name: envRunBatchConc, CLIDefault: true, Job: true, Array: true, Description: "Scheduler submission limit; --batch-concurrency default."},
		{Name: envRunSSHConc, CLIDefault: true, Description: "SSH worker limit; --ssh-concurrency default."},
		{Name: envRunSSHOptions, CLIDefault: true, Description: "SSH dispatch options; --ssh-options default."},
		{Name: envRunSlurmConc, CLIDefault: true, Description: "Slurm worker limit; --slurm-concurrency default."},
		{Name: envRunSlurmOptions, CLIDefault: true, Description: "Slurm dispatch options; --slurm-options default."},
		{Name: envRunPBSConc, CLIDefault: true, Description: "PBS worker limit; --pbs-concurrency default."},
		{Name: envRunPBSOptions, CLIDefault: true, Description: "PBS dispatch options; --pbs-options default."},
		{Name: envRunLSFConc, CLIDefault: true, Description: "LSF worker limit; --lsf-concurrency default."},
		{Name: envRunLSFOptions, CLIDefault: true, Description: "LSF dispatch options; --lsf-options default."},
		{Name: envRunRetry, CLIDefault: true, Job: true, Array: true, Description: "Retry count; --retry default."},
		{Name: envRunAsync, CLIDefault: true, Job: true, Array: true, Description: "Async run mode; --async default."},
		{Name: envArrayRange, CLIDefault: true, Job: true, Array: true, Description: "Array range; --array default."},
		{Name: envBin, Job: true, Array: true, Description: "Absolute path to the rotari binary."},
		{Name: envRunDir, Job: true, Array: true, Description: "Current run directory."},
		{Name: envJobDir, Job: true, Array: true, Description: "Current job directory."},
		{Name: envCWD, Job: true, Array: true, Description: "Working directory from which the run started."},
		{Name: envArrayTaskID, Array: true, Description: "Current array task number."},
		{Name: envArrayFirst, Array: true, Description: "First array task number."},
		{Name: envArrayLast, Array: true, Description: "Last array task number."},
		{Name: envArraySize, Array: true, Description: "Number of tasks in the array."},
		{Name: envResetRecover, CLIDefault: true, Description: "--recover default for reset."},
		{Name: envWaitTimeout, CLIDefault: true, Description: "--timeout default for wait."},
		{Name: envWebHost, CLIDefault: true, Description: "--host default for web."},
		{Name: envWebPort, CLIDefault: true, Description: "--port default for web."},
		{Name: envWebStaticDir, CLIDefault: true, Description: "--static-dir default for web."},
		{Name: envWebAllowControl, CLIDefault: true, Description: "--allow-control default for web."},
		{Name: envWebAuthToken, CLIDefault: true, Description: "--auth-token default for web; never exposed by the Web UI."},
		{Name: envLLMAPIKey, Description: "API key for the diagnose command; never persisted or passed to jobs."},
		{Name: envLLMProvider, CLIDefault: true, Description: "LLM provider (openai, openai-chat, anthropic, gemini, or cohere); --provider default for diagnose."},
		{Name: envLLMEndpoint, CLIDefault: true, Description: "LLM API endpoint; --endpoint default for diagnose."},
		{Name: envLLMModel, CLIDefault: true, Description: "Model name; --model default for diagnose."},
		{Name: envLLMLanguage, CLIDefault: true, Description: "BCP 47 response language tag; --language default for diagnose."},
		{Name: envWebhookURL, Description: "Run completion webhook URL."},
		{Name: envWebhookOn, Description: "Run completion webhook events: always, success, or failure."},
		{Name: envWebhookFormat, Description: "Run completion webhook format: json, slack, teams, or discord."},
		{Name: envPrivateState, Description: "set to true for 0700/0600 state directory permissions instead of the default 0755/0644 (shared state)."},
	}
}

func cmdEnvironment(args []string) int {
	if len(args) != 0 {
		printError("usage: " + cliUsage("env"))
		return 1
	}
	fmt.Println("VARIABLE\tVALUE\tCLI\tJOB\tARRAY\tDESCRIPTION")
	for _, definition := range environmentDefinitions() {
		value := "-"
		if current, ok := os.LookupEnv(definition.Name); ok {
			value = current
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", definition.Name, value, yesNo(definition.CLIDefault), yesNo(definition.Job), yesNo(definition.Array), definition.Description)
	}
	return 0
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "-"
}
