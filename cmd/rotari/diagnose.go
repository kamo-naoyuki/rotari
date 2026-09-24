package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	defaultLLMProvider   = "openai"
	defaultLLMEndpoint   = "https://api.openai.com/v1/responses"
	chatLLMEndpoint      = "https://api.openai.com/v1/chat/completions"
	anthropicLLMEndpoint = "https://api.anthropic.com/v1/messages"
	cohereLLMEndpoint    = "https://api.cohere.com/v2/chat"
	geminiLLMEndpoint    = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
	diagnosisLogLimit    = 12000
)

const noRuleDiagnosisName = "No known rule-based diagnosis matched"
const unavailableRuleDiagnosisName = "Rule-based diagnosis unavailable"

type diagnosisJob = diagnose.Job

// cmdDiagnose builds a failure diagnosis prompt and optionally sends it to an
// external provider.
func cmdDiagnose(args []string) int {
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobID := cliString(fs, "job-id", "")
	attemptID := ""
	provider := cliString(fs, "provider", defaultLLMProvider)
	endpoint := cliString(fs, "endpoint", defaultLLMEndpoint)
	model := cliString(fs, "model", "")
	language := cliString(fs, "language", "")
	rules := cliBool(fs, "rules", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && *jobID != "") {
		printError("usage: " + cliUsage("diagnose"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*jobID = fs.Args()[0]
	}
	if *jobID == "" || (!*rules && *model == "") {
		printError("usage: " + cliUsage("diagnose") + " (requires a job ID and --model unless --rules is set)")
		return 1
	}
	if strings.HasPrefix(*jobID, "att_") {
		attemptID = *jobID
		baseDir, resolvedProjectName, resolvedRunID, resolvedJobID, err := resolveAttemptTarget(*jobID, *basedir, *projectName, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		*basedir, *projectName, *runIDOption, *jobID = baseDir, resolvedProjectName, resolvedRunID, resolvedJobID
	}
	if *rules {
		baseDir, queueName, err := resolveExistingRunTarget(*basedir, *projectName, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		paths, err := resolvePaths(baseDir, queueName)
		if err != nil {
			printErrorf("failed to resolve paths: %v", err)
			return 1
		}
		runID, err := selectRunID(paths, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		job, err := loadDiagnosisJob(paths, runID, *jobID, attemptID)
		if err != nil {
			printError(err)
			return 1
		}
		fmt.Print(formatRuleDiagnoses(diagnoseWithRules(job)))
		return 0
	}
	if *provider != "openai" && *provider != "openai-chat" && *provider != "anthropic" && *provider != "gemini" && *provider != "cohere" {
		printErrorf("unsupported LLM provider %q; use openai, openai-chat, anthropic, gemini, or cohere", *provider)
		return 1
	}
	if *provider == "anthropic" && *endpoint == defaultLLMEndpoint {
		*endpoint = anthropicLLMEndpoint
	}
	if *provider == "gemini" && *endpoint == defaultLLMEndpoint {
		*endpoint = fmt.Sprintf(geminiLLMEndpoint, *model)
	}
	if *provider == "openai-chat" && *endpoint == defaultLLMEndpoint {
		*endpoint = chatLLMEndpoint
	}
	if *provider == "cohere" && *endpoint == defaultLLMEndpoint {
		*endpoint = cohereLLMEndpoint
	}
	if *language != "" && !isLanguageTag(*language) {
		printErrorf("invalid language tag %q; use a BCP 47 tag such as ja or en-US", *language)
		return 1
	}
	apiKey := os.Getenv(envLLMAPIKey)
	if apiKey == "" {
		printError("ROTARI_LLM_API_KEY is required; it is not stored in rotari state")
		return 1
	}
	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *projectName, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	runID, err := selectRunID(paths, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	job, err := loadDiagnosisJob(paths, runID, *jobID, attemptID)
	if err != nil {
		printError(err)
		return 1
	}
	answer, err := requestProviderDiagnosis(context.Background(), *provider, *endpoint, apiKey, *model, diagnosisPrompt(job, *language))
	if err != nil {
		printErrorf("LLM diagnosis failed: %v", err)
		return 1
	}
	fmt.Println(answer)
	return 0
}

func loadDiagnosisJob(paths pathSet, runID, jobID string, attemptIDs ...string) (diagnosisJob, error) {
	if !state.IsValidPathElement(runID) || !state.IsValidPathElement(jobID) {
		return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	for range 16 {
		runDir, err := state.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			return diagnosisJob{}, err
		}
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		attemptID := ""
		if len(attemptIDs) > 0 {
			attemptID = attemptIDs[0]
		}
		if attemptID != "" {
			jobDir, err = state.SpecificAttemptJobDir(runDir, jobID, attemptID)
		}
		if err != nil {
			return diagnosisJob{}, err
		}
		info, err := os.Stat(jobDir)
		if err != nil || !info.IsDir() {
			if attemptID != "" {
				return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			origin := loadRunOrigin(runDir, jobID)
			if origin == nil {
				return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			runID, jobID = origin.RunID, origin.JobID
			continue
		}
		var spec JobSpec
		if err := jsonStore().ReadJSON(filepath.Join(jobDir, commandJSONName), &spec); err != nil {
			return diagnosisJob{}, fmt.Errorf("read job command: %w", err)
		}
		log, err := os.ReadFile(filepath.Join(jobDir, "output"))
		if err != nil && !os.IsNotExist(err) {
			return diagnosisJob{}, fmt.Errorf("read job output: %w", err)
		}
		job := diagnosisJob{RunID: runID, JobID: jobID, Command: spec.Command, Log: tailString(string(log), diagnosisLogLimit)}
		if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
			for _, result := range summary.Results {
				if result.ID == jobID {
					exitCode := result.ExitCode
					job.ExitCode, job.Error = &exitCode, result.Error
					break
				}
			}
		}
		return job, nil
	}
	return diagnosisJob{}, fmt.Errorf("job %q has too many carried-forward origins", jobID)
}

func tailString(value string, limit int) string {
	return diagnose.TailLog(value, limit)
}

func diagnoseWithRules(job diagnosisJob) []ruleDiagnosis {
	return diagnose.DiagnoseDefault(job)
}

func diagnoseJobResult(runDir string, result JobResult) JobResult {
	if result.ExitCode == 0 || len(result.Diagnoses) > 0 {
		return result
	}
	if !state.IsValidPathElement(result.ID) {
		return unavailableRuleDiagnosis(result, "The job ID is invalid, so its output could not be inspected.")
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, result.ID)
	if err != nil {
		return unavailableRuleDiagnosis(result, "The job directory could not be resolved: "+err.Error())
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "output"))
	if err != nil && !os.IsNotExist(err) {
		return unavailableRuleDiagnosis(result, "The job output could not be read: "+err.Error())
	}
	result.Diagnoses = diagnoseWithRules(diagnosisJob{
		JobID: result.ID,
		Error: result.Error,
		Log:   tailString(string(data), diagnosisLogLimit),
	})
	if len(result.Diagnoses) == 0 {
		result.Diagnoses = []ruleDiagnosis{{
			Name:       noRuleDiagnosisName,
			Evidence:   "No recognized signature in the recorded scheduler error or log.",
			Suggestion: "Inspect the full job output and scheduler accounting for the failure details.",
		}}
	}
	return result
}

func unavailableRuleDiagnosis(result JobResult, evidence string) JobResult {
	result.Diagnoses = []ruleDiagnosis{{
		Name:       unavailableRuleDiagnosisName,
		Evidence:   evidence,
		Suggestion: "Inspect the job directory and output file permissions, then run rotari diagnose --rules after resolving the read error.",
	}}
	return result
}

func formatRuleDiagnoses(diagnoses []ruleDiagnosis) string {
	if len(diagnoses) == 0 {
		return "No known rule-based diagnosis matched the recorded error or log. Inspect the full job output with rotari show --run-id RUN_ID --job-id JOB_ID.\n"
	}
	var output strings.Builder
	for _, diagnosis := range diagnoses {
		fmt.Fprintf(&output, "%s\nEvidence: %s\nNext: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
	}
	return output.String()
}

func isLanguageTag(value string) bool {
	return diagnose.IsLanguageTag(value)
}

func diagnosisPrompt(job diagnosisJob, language string) string {
	return diagnose.BuildPrompt(job, language)
}

func requestDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	return requestProviderDiagnosis(ctx, "openai", endpoint, apiKey, model, prompt)
}

func requestProviderDiagnosis(ctx context.Context, provider, endpoint, apiKey, model, prompt string) (string, error) {
	return diagnose.RequestProviderDiagnosis(ctx, provider, endpoint, apiKey, model, prompt)
}
