package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
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

const (
	noMatchDiagnosisNext     = "Inspect the full job output and scheduler accounting for the failure details."
	unavailableDiagnosisNext = "Resolve the read error, then run rotari diagnose --rules."
	outdatedDiagnosisNote    = "Saved with earlier diagnosis rules; rotari diagnose --rules shows the result under the current rules."
)

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
		paths, err := state.ResolveProjectPaths(baseDir, queueName)
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
	if *language != "" && !diagnose.IsLanguageTag(*language) {
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
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
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

func loadDiagnosisJob(paths state.ProjectPaths, runID, jobID string, attemptIDs ...string) (diagnose.Job, error) {
	if !state.IsValidPathElement(runID) || !state.IsValidPathElement(jobID) {
		return diagnose.Job{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	for range 16 {
		runDir, err := state.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			return diagnose.Job{}, err
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
			return diagnose.Job{}, err
		}
		info, err := os.Stat(jobDir)
		if err != nil || !info.IsDir() {
			if attemptID != "" {
				return diagnose.Job{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			origin := loadRunOrigin(runDir, jobID)
			if origin == nil {
				return diagnose.Job{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			runID, jobID = origin.RunID, origin.JobID
			continue
		}
		var spec model.JobSpec
		if err := jsonStore().ReadJSON(filepath.Join(jobDir, commandJSONName), &spec); err != nil {
			return diagnose.Job{}, fmt.Errorf("read job command: %w", err)
		}
		log, err := os.ReadFile(filepath.Join(jobDir, "output"))
		if err != nil && !os.IsNotExist(err) {
			return diagnose.Job{}, fmt.Errorf("read job output: %w", err)
		}
		job := diagnose.Job{RunID: runID, JobID: jobID, Command: spec.Command, Log: diagnose.TailLog(string(log), diagnosisLogLimit)}
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
	return diagnose.Job{}, fmt.Errorf("job %q has too many carried-forward origins", jobID)
}

func diagnoseWithRules(job diagnose.Job) []model.RuleDiagnosis {
	return diagnose.DiagnoseDefault(job)
}

// diagnoseJobResult saves the rule-based analysis of a failed result from its
// latest attempt's output.
func diagnoseJobResult(runDir string, result model.JobResult) model.JobResult {
	if result.ExitCode == 0 || result.DiagnosisStatus != "" {
		return result
	}
	log, err := readDiagnosisLog(runDir, result.ID)
	return diagnose.AnalyzeResult(result, log, err)
}

func readDiagnosisLog(runDir, jobID string) (string, error) {
	if !state.IsValidPathElement(jobID) {
		return "", errors.New("the job ID is invalid, so its output could not be inspected")
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return "", fmt.Errorf("the job directory could not be resolved: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "output"))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("the job output could not be read: %w", err)
	}
	return diagnose.TailLog(string(data), diagnosisLogLimit), nil
}

func formatRuleDiagnoses(diagnoses []model.RuleDiagnosis) string {
	if len(diagnoses) == 0 {
		return "No known rule-based diagnosis matched the recorded error or log. Inspect the full job output with rotari show --run-id RUN_ID --job-id JOB_ID.\n"
	}
	var output strings.Builder
	for _, diagnosis := range diagnoses {
		fmt.Fprintf(&output, "%s\nEvidence: %s\nNext: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
	}
	return output.String()
}

func diagnosisPrompt(job diagnose.Job, language string) string {
	return diagnose.BuildPrompt(job, language)
}

func requestDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	return requestProviderDiagnosis(ctx, "openai", endpoint, apiKey, model, prompt)
}

func requestProviderDiagnosis(ctx context.Context, provider, endpoint, apiKey, model, prompt string) (string, error) {
	return diagnose.RequestProviderDiagnosis(ctx, provider, endpoint, apiKey, model, prompt)
}
