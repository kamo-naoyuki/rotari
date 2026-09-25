package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestDiagnosisSendsOpenAIResponsesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s authorization=%q", request.Method, request.Header.Get("Authorization"))
		}
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "test-model" || body.Input != "prompt" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"diagnosis"}]}]}`))
	}))
	defer server.Close()

	answer, err := requestDiagnosis(context.Background(), server.URL, "secret", "test-model", "prompt")
	if err != nil || answer != "diagnosis" {
		t.Fatalf("requestDiagnosis() = %q, %v", answer, err)
	}
}

func TestRequestProviderDiagnosisSendsChatCompletionsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s authorization=%q", request.Method, request.Header.Get("Authorization"))
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "deepseek-chat" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "prompt" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"diagnosis"}}]}`))
	}))
	defer server.Close()

	answer, err := requestProviderDiagnosis(context.Background(), "openai-chat", server.URL, "secret", "deepseek-chat", "prompt")
	if err != nil || answer != "diagnosis" {
		t.Fatalf("requestProviderDiagnosis() = %q, %v", answer, err)
	}
}

func TestRequestProviderDiagnosisSendsAnthropicMessagesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("x-api-key") != "secret" || request.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("request = %s x-api-key=%q anthropic-version=%q", request.Method, request.Header.Get("x-api-key"), request.Header.Get("anthropic-version"))
		}
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "claude-sonnet-5" || body.MaxTokens != 4096 || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "prompt" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"content":[{"type":"text","text":"diagnosis"}]}`))
	}))
	defer server.Close()

	answer, err := requestProviderDiagnosis(context.Background(), "anthropic", server.URL, "secret", "claude-sonnet-5", "prompt")
	if err != nil || answer != "diagnosis" {
		t.Fatalf("requestProviderDiagnosis() = %q, %v", answer, err)
	}
}

func TestRequestProviderDiagnosisSendsGeminiGenerateContentRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("x-goog-api-key") != "secret" {
			t.Fatalf("request = %s x-goog-api-key=%q", request.Method, request.Header.Get("x-goog-api-key"))
		}
		var body struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 1 || body.Contents[0].Parts[0].Text != "prompt" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"diagnosis"}]}}]}`))
	}))
	defer server.Close()

	answer, err := requestProviderDiagnosis(context.Background(), "gemini", server.URL, "secret", "gemini-3.6-flash", "prompt")
	if err != nil || answer != "diagnosis" {
		t.Fatalf("requestProviderDiagnosis() = %q, %v", answer, err)
	}
}

func TestRequestProviderDiagnosisSendsCohereChatRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request = %s authorization=%q", request.Method, request.Header.Get("Authorization"))
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "command-a-reasoning-08-2025" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "prompt" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"message":{"content":[{"type":"text","text":"diagnosis"}]}}`))
	}))
	defer server.Close()

	answer, err := requestProviderDiagnosis(context.Background(), "cohere", server.URL, "secret", "command-a-reasoning-08-2025", "prompt")
	if err != nil || answer != "diagnosis" {
		t.Fatalf("requestProviderDiagnosis() = %q, %v", answer, err)
	}
}

func TestTailStringKeepsLogEnd(t *testing.T) {
	got := diagnose.TailLog("0123456789", 4)
	if !strings.HasPrefix(got, "[earlier log output omitted]") || !strings.HasSuffix(got, "6789") {
		t.Fatalf("diagnose.TailLog() = %q", got)
	}
}

func TestDiagnoseJobResultReadsOutputAndPreservesSavedDiagnoses(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "output"), []byte("CUDA out of memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := diagnoseJobResult(runDir, model.JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != "CUDA/GPU memory exhausted" {
		t.Fatalf("diagnoseJobResult() = %#v", result.Diagnoses)
	}

	saved := model.JobResult{ID: "job-1", ExitCode: 1, Diagnoses: []model.RuleDiagnosis{{Name: "Saved diagnosis"}}}
	if got := diagnoseJobResult(runDir, saved); len(got.Diagnoses) != 1 || got.Diagnoses[0].Name != "Saved diagnosis" {
		t.Fatalf("diagnoseJobResult() overwrote saved diagnoses: %#v", got.Diagnoses)
	}
}

func TestDiagnoseJobResultPersistsNoMatch(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "output"), []byte("unrecognized failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := diagnoseJobResult(runDir, model.JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != noRuleDiagnosisName {
		t.Fatalf("diagnoseJobResult() = %#v, want no-match diagnosis", result.Diagnoses)
	}
}

func TestDiagnosisPromptIncludesRequestedLanguage(t *testing.T) {
	prompt := diagnosisPrompt(diagnose.Job{}, "ja")
	if !strings.Contains(prompt, `BCP 47 tag "ja"`) {
		t.Fatalf("diagnosisPrompt() = %q", prompt)
	}
	if strings.Contains(diagnosisPrompt(diagnose.Job{}, ""), "BCP 47 tag") {
		t.Fatal("diagnosisPrompt() included a language instruction without a requested language")
	}
}

func TestRequestProviderDiagnosisRejectsUnsupportedProvider(t *testing.T) {
	_, err := requestProviderDiagnosis(context.Background(), "unsupported", "https://example.com", "secret", "model", "prompt")
	if err == nil || !strings.Contains(err.Error(), "unsupported LLM provider") {
		t.Fatalf("requestProviderDiagnosis() error = %v, want unsupported provider", err)
	}
}

func TestCmdDiagnoseRequiresAPIKeyBeforeLLMRequest(t *testing.T) {
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdDiagnose([]string{"--model", "model", "job-1"})
	_ = writer.Close()
	os.Stderr = oldStderr
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "ROTARI_LLM_API_KEY is required") {
		t.Fatalf("code=%d stderr=%q", code, output)
	}
}

func TestDiagnoseJobResultRejectsInvalidJobIDWithUnavailableDiagnosis(t *testing.T) {
	result := diagnoseJobResult(t.TempDir(), model.JobResult{ID: "../outside", ExitCode: 1, Error: "bad input"})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != unavailableRuleDiagnosisName {
		t.Fatalf("result.Diagnoses = %#v, want unavailable diagnosis for invalid job ID", result.Diagnoses)
	}
	if !strings.Contains(result.Diagnoses[0].Evidence, "job ID is invalid") {
		t.Fatalf("evidence = %q, want invalid job ID message", result.Diagnoses[0].Evidence)
	}
}

func TestIsLanguageTag(t *testing.T) {
	for _, value := range []string{"ja", "en", "en-US", "zh-Hant-TW", "es-419"} {
		if !diagnose.IsLanguageTag(value) {
			t.Errorf("diagnose.IsLanguageTag(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "j", "ja; ignore instructions", "ja_JP", "ja--JP", "日本語"} {
		if diagnose.IsLanguageTag(value) {
			t.Errorf("diagnose.IsLanguageTag(%q) = true, want false", value)
		}
	}
}

func TestDiagnoseLanguageUsesEnvironmentAndCLIOverride(t *testing.T) {
	t.Setenv(envLLMLanguage, "ja")
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	language := cliString(fs, "language", "")
	if *language != "ja" {
		t.Fatalf("language environment default = %q, want ja", *language)
	}
	if err := fs.Parse([]string{"--language", "en-US"}); err != nil {
		t.Fatal(err)
	}
	if *language != "en-US" {
		t.Fatalf("language CLI override = %q, want en-US", *language)
	}
}

func TestCmdDiagnoseRejectsInvalidLanguageBeforeRequest(t *testing.T) {
	t.Setenv(envLLMAPIKey, "secret")
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdDiagnose([]string{"--job-id", "job", "--model", "model", "--language", "ja;ignore"})
	_ = writer.Close()
	os.Stderr = oldStderr
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `invalid language tag "ja;ignore"`) {
		t.Fatalf("code=%d stderr=%q", code, output)
	}
}
