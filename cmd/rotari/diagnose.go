package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

type diagnosisJob struct {
	RunID    string
	JobID    string
	Command  []string
	ExitCode *int
	Error    string
	Log      string
}

func cmdDiagnose(args []string) int {
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobID := cliString(fs, "job-id", "")
	provider := cliString(fs, "provider", defaultLLMProvider)
	endpoint := cliString(fs, "endpoint", defaultLLMEndpoint)
	model := cliString(fs, "model", "")
	language := cliString(fs, "language", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 || *jobID == "" || *model == "" {
		printError("usage: " + cliUsage("diagnose") + " (requires --job-id and --model)")
		return 1
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
	job, err := loadDiagnosisJob(paths, runID, *jobID)
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

func loadDiagnosisJob(paths pathSet, runID, jobID string) (diagnosisJob, error) {
	if !isValidPathElement(runID) || !isValidPathElement(jobID) {
		return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	for range 16 {
		runDir, err := validatedRunDir(paths, runID)
		if err != nil {
			return diagnosisJob{}, err
		}
		jobDir, err := validatedJobDir(runDir, jobID)
		if err != nil {
			return diagnosisJob{}, err
		}
		info, err := os.Stat(jobDir)
		if err != nil || !info.IsDir() {
			origin := loadRunOrigin(runDir, jobID)
			if origin == nil {
				return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			runID, jobID = origin.RunID, origin.JobID
			continue
		}
		data, err := os.ReadFile(filepath.Join(jobDir, commandJSONName))
		if err != nil {
			return diagnosisJob{}, fmt.Errorf("read job command: %w", err)
		}
		var spec JobSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return diagnosisJob{}, fmt.Errorf("read job command: %w", err)
		}
		log, err := os.ReadFile(filepath.Join(jobDir, "output"))
		if err != nil && !os.IsNotExist(err) {
			return diagnosisJob{}, fmt.Errorf("read job output: %w", err)
		}
		job := diagnosisJob{RunID: runID, JobID: jobID, Command: spec.Command, Log: tailString(string(log), diagnosisLogLimit)}
		if summary, err := loadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
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
	if len(value) <= limit {
		return value
	}
	return "[earlier log output omitted]\n" + value[len(value)-limit:]
}

func isLanguageTag(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 8 {
			return false
		}
		for _, character := range part {
			if !('a' <= character && character <= 'z' || 'A' <= character && character <= 'Z' || '0' <= character && character <= '9') {
				return false
			}
		}
	}
	return true
}

func diagnosisPrompt(job diagnosisJob, language string) string {
	status := "not recorded"
	if job.ExitCode != nil {
		status = fmt.Sprintf("exit code %d", *job.ExitCode)
	}
	languageInstruction := ""
	if language != "" {
		languageInstruction = fmt.Sprintf(" Respond in the language identified by the BCP 47 tag %q.", language)
	}
	return fmt.Sprintf("Diagnose this failed rotari job. Explain the likely root cause, cite evidence from the log, and give minimal concrete next steps. Do not claim to have executed anything.%s\n\nRun: %s\nJob: %s\nStatus: %s\nScheduler error: %s\nCommand: %s\n\nLog tail:\n%s", languageInstruction, job.RunID, job.JobID, status, job.Error, strings.Join(job.Command, " "), job.Log)
}

func requestDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	return requestProviderDiagnosis(ctx, "openai", endpoint, apiKey, model, prompt)
}

func requestProviderDiagnosis(ctx context.Context, provider, endpoint, apiKey, model, prompt string) (string, error) {
	switch provider {
	case "openai":
		return requestOpenAIDiagnosis(ctx, endpoint, apiKey, model, prompt)
	case "openai-chat":
		return requestChatCompletionsDiagnosis(ctx, endpoint, apiKey, model, prompt)
	case "anthropic":
		return requestAnthropicDiagnosis(ctx, endpoint, apiKey, model, prompt)
	case "gemini":
		return requestGeminiDiagnosis(ctx, endpoint, apiKey, prompt)
	case "cohere":
		return requestCohereDiagnosis(ctx, endpoint, apiKey, model, prompt)
	default:
		return "", fmt.Errorf("unsupported LLM provider %q", provider)
	}
}

func requestOpenAIDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{Model: model, Input: prompt})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	if decoded.OutputText != "" {
		return decoded.OutputText, nil
	}
	for _, output := range decoded.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("API response did not include output text")
}

func requestChatCompletionsDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, choice := range decoded.Choices {
		if choice.Message.Content != "" {
			return choice.Message.Content, nil
		}
	}
	return "", fmt.Errorf("API response did not include message content")
}

func requestAnthropicDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:     model,
		MaxTokens: 4096,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("x-api-key", apiKey)
	request.Header.Set("anthropic-version", "2023-06-01")
	request.Header.Set("Content-Type", "application/json")
	return doAnthropicRequest(request)
}

func doAnthropicRequest(request *http.Request) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, content := range decoded.Content {
		if content.Type == "text" && content.Text != "" {
			return content.Text, nil
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}

func requestGeminiDiagnosis(ctx context.Context, endpoint, apiKey, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{{Parts: []struct {
			Text string `json:"text"`
		}{{Text: prompt}}}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("x-goog-api-key", apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, candidate := range decoded.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				return part.Text, nil
			}
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}

func requestCohereDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Stream   bool   `json:"stream"`
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, content := range decoded.Message.Content {
		if content.Type == "text" && content.Text != "" {
			return content.Text, nil
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}
