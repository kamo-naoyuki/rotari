package main

import (
	"context"
	"encoding/json"
	"flag"
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
	got := tailString("0123456789", 4)
	if !strings.HasPrefix(got, "[earlier log output omitted]") || !strings.HasSuffix(got, "6789") {
		t.Fatalf("tailString() = %q", got)
	}
}

func TestDiagnoseWithRulesNormalizesAndMatchesKnownErrors(t *testing.T) {
	tests := []struct {
		name string
		job  diagnosisJob
		want string
	}{
		{name: "CUDA OOM", job: diagnosisJob{Log: "\x1b[31mCUDA Error: Out   Of Memory\x1b[0m"}, want: "CUDA/GPU memory exhausted"},
		{name: "GPU unavailable", job: diagnosisJob{Log: "NVIDIA-SMI has failed because it couldn't communicate"}, want: "CUDA/GPU unavailable"},
		{name: "Slurm memory limit", job: diagnosisJob{Log: "slurmstepd: error: Detected 1 oom-kill event(s) in StepId=1"}, want: "Slurm memory limit exceeded"},
		{name: "Slurm time limit", job: diagnosisJob{Log: "slurmstepd: error: CANCELLED AT 2026-09-21 DUE TO TIME LIMIT"}, want: "Slurm time limit exceeded"},
		{name: "PBS walltime", job: diagnosisJob{Log: "PBS: job exceeded walltime limit"}, want: "PBS resource or walltime limit exceeded"},
		{name: "LSF memory limit", job: diagnosisJob{Log: "Exited with exit code 137. TERM_MEMLIMIT"}, want: "LSF memory limit exceeded"},
		{name: "LSF run limit", job: diagnosisJob{Log: "TERM_RUNLIMIT: job killed after run limit"}, want: "LSF run time limit exceeded"},
		{name: "scheduler submission", job: diagnosisJob{Log: "sbatch: error: Unable to contact slurm controller"}, want: "Scheduler submission failed"},
		{name: "invalid scheduler resource", job: diagnosisJob{Log: "sbatch: error: Invalid qos specification"}, want: "Invalid scheduler resource request"},
		{name: "scheduler cancelled", job: diagnosisJob{Log: "slurmstepd: error: CANCELLED AT 2026-09-21 DUE TO PREEMPTION"}, want: "Scheduler cancelled job"},
		{name: "host OOM", job: diagnosisJob{Log: "Memory cgroup out of memory: Killed process 42"}, want: "Host memory exhausted"},
		{name: "killed", job: diagnosisJob{Log: "Killed"}, want: "Process killed"},
		{name: "kernel fault", job: diagnosisJob{Log: "BUG: unable to handle kernel NULL pointer dereference"}, want: "Kernel panic or kernel fault"},
		{name: "application panic", job: diagnosisJob{Log: "panic: unexpected nil pointer"}, want: "Application panic"},
		{name: "segmentation fault", job: diagnosisJob{Log: "Fatal Python error: Segmentation fault"}, want: "Segmentation fault"},
		{name: "GPU Xid", job: diagnosisJob{Log: "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus."}, want: "NVIDIA GPU driver/device error"},
		{name: "CUDA assertion", job: diagnosisJob{Log: "RuntimeError: CUDA error: device-side assert triggered"}, want: "CUDA device-side assert"},
		{name: "NCCL", job: diagnosisJob{Log: "NCCL WARN unhandled system error"}, want: "NCCL failure"},
		{name: "MPI", job: diagnosisJob{Log: "MPI_ABORT was invoked on rank 0"}, want: "MPI runtime failure"},
		{name: "disk quota", job: diagnosisJob{Log: "write: Disk quota exceeded"}, want: "Disk space or quota exhausted"},
		{name: "storage I/O", job: diagnosisJob{Log: "cp: error writing 'checkpoint': Input/output error"}, want: "Storage device I/O error"},
		{name: "read-only filesystem", job: diagnosisJob{Log: "open output: read-only file system"}, want: "Read-only filesystem"},
		{name: "missing file", job: diagnosisJob{Log: "open input.json: no such file or directory"}, want: "File or directory not found"},
		{name: "permission denied", job: diagnosisJob{Log: "./train: Permission denied"}, want: "Permission denied"},
		{name: "missing command", job: diagnosisJob{Log: "python: command not found"}, want: "Command or executable not found"},
		{name: "shared library ABI", job: diagnosisJob{Log: "ImportError: libstdc++.so.6: version `GLIBCXX_3.4.30' not found"}, want: "Shared library or ABI mismatch"},
		{name: "Python import", job: diagnosisJob{Log: "ModuleNotFoundError: No module named 'torch'"}, want: "Python import or module missing"},
		{name: "Python dependency", job: diagnosisJob{Log: "package-a requires package-b but version 1 is installed"}, want: "Python dependency or version conflict"},
		{name: "Python syntax", job: diagnosisJob{Log: "SyntaxError: invalid syntax"}, want: "Python syntax or indentation error"},
		{name: "Python attribute", job: diagnosisJob{Log: "AttributeError: 'NoneType' object has no attribute 'shape'"}, want: "Python missing name or attribute"},
		{name: "Python type", job: diagnosisJob{Log: "TypeError: expected str, got None"}, want: "Python type or value error"},
		{name: "Python key", job: diagnosisJob{Log: "KeyError: 'learning_rate'"}, want: "Python key or index error"},
		{name: "Python assertion", job: diagnosisJob{Log: "AssertionError: invalid dataset"}, want: "Python assertion failed"},
		{name: "Python memory", job: diagnosisJob{Log: "MemoryError"}, want: "Python memory error"},
		{name: "Python recursion", job: diagnosisJob{Log: "RecursionError: maximum recursion depth exceeded"}, want: "Python recursion limit exceeded"},
		{name: "file descriptors", job: diagnosisJob{Log: "open: Too many open files"}, want: "File descriptor limit exceeded"},
		{name: "process limit", job: diagnosisJob{Log: "fork: retry: Resource temporarily unavailable"}, want: "Process or thread limit exceeded"},
		{name: "DNS", job: diagnosisJob{Log: "curl: (6) Could not resolve host: example.test"}, want: "DNS lookup failed"},
		{name: "connection refused", job: diagnosisJob{Log: "dial tcp: connection refused"}, want: "Network connection refused"},
		{name: "network timeout", job: diagnosisJob{Error: "dial tcp: i/o timeout"}, want: "Network connection timed out"},
		{name: "connection reset", job: diagnosisJob{Log: "read: connection reset by peer"}, want: "Network connection reset or closed"},
		{name: "TLS certificate", job: diagnosisJob{Log: "x509: certificate signed by unknown authority"}, want: "TLS certificate or handshake failure"},
		{name: "HTTP authorization", job: diagnosisJob{Log: "HTTP 403 Forbidden"}, want: "HTTP authentication or authorization failed"},
		{name: "HTTP rate limit", job: diagnosisJob{Log: "request failed: status code 429"}, want: "HTTP rate limited"},
		{name: "HTTP unavailable", job: diagnosisJob{Log: "503 Service Unavailable"}, want: "HTTP service unavailable"},
		{name: "HTTP gateway error", job: diagnosisJob{Log: "HTTP 502 Bad Gateway"}, want: "HTTP server or bad gateway error"},
		{name: "HTTP gateway timeout", job: diagnosisJob{Log: "504 Gateway Timeout"}, want: "HTTP gateway timeout"},
		{name: "SSH key", job: diagnosisJob{Log: "Permission denied (publickey)."}, want: "SSH authentication or host verification failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnoses := diagnoseWithRules(test.job)
			if len(diagnoses) != 1 || diagnoses[0].Name != test.want {
				t.Fatalf("diagnoseWithRules() = %#v, want %q", diagnoses, test.want)
			}
		})
	}
}

func TestDiagnoseWithRulesExtractsFinalPythonException(t *testing.T) {
	diagnoses := diagnoseWithRules(diagnosisJob{Log: "Traceback (most recent call last):\n  File \"train.py\", line 10\nValueError: invalid batch size"})
	if len(diagnoses) != 2 || diagnoses[0].Name != "Python type or value error" || diagnoses[1].Name != "Python exception" || diagnoses[1].Evidence != "ValueError: invalid batch size" {
		t.Fatalf("diagnoseWithRules() = %#v, want specific and final Python exception", diagnoses)
	}
}

func TestDiagnoseWithRulesDoesNotGuess(t *testing.T) {
	if diagnoses := diagnoseWithRules(diagnosisJob{Log: "the job failed"}); len(diagnoses) != 0 {
		t.Fatalf("diagnoseWithRules() = %#v, want no diagnosis", diagnoses)
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
	result := diagnoseJobResult(runDir, JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != "CUDA/GPU memory exhausted" {
		t.Fatalf("diagnoseJobResult() = %#v", result.Diagnoses)
	}

	saved := JobResult{ID: "job-1", ExitCode: 1, Diagnoses: []ruleDiagnosis{{Name: "Saved diagnosis"}}}
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
	result := diagnoseJobResult(runDir, JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != noRuleDiagnosisName {
		t.Fatalf("diagnoseJobResult() = %#v, want no-match diagnosis", result.Diagnoses)
	}
}

func TestDiagnosisPromptIncludesRequestedLanguage(t *testing.T) {
	prompt := diagnosisPrompt(diagnosisJob{}, "ja")
	if !strings.Contains(prompt, `BCP 47 tag "ja"`) {
		t.Fatalf("diagnosisPrompt() = %q", prompt)
	}
	if strings.Contains(diagnosisPrompt(diagnosisJob{}, ""), "BCP 47 tag") {
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
	result := diagnoseJobResult(t.TempDir(), JobResult{ID: "../outside", ExitCode: 1, Error: "bad input"})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != unavailableRuleDiagnosisName {
		t.Fatalf("result.Diagnoses = %#v, want unavailable diagnosis for invalid job ID", result.Diagnoses)
	}
	if !strings.Contains(result.Diagnoses[0].Evidence, "job ID is invalid") {
		t.Fatalf("evidence = %q, want invalid job ID message", result.Diagnoses[0].Evidence)
	}
}

func TestIsLanguageTag(t *testing.T) {
	for _, value := range []string{"ja", "en", "en-US", "zh-Hant-TW", "es-419"} {
		if !isLanguageTag(value) {
			t.Errorf("isLanguageTag(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "j", "ja; ignore instructions", "ja_JP", "ja--JP", "日本語"} {
		if isLanguageTag(value) {
			t.Errorf("isLanguageTag(%q) = true, want false", value)
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
