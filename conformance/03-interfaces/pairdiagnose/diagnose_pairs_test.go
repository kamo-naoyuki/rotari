package pairdiagnose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type diagnosisRequest struct {
	Method           string
	Path             string
	Authorization    string
	APIKey           string
	AnthropicVersion string
	GoogleAPIKey     string
	Body             map[string]any
}

func diagnoseServer(t *testing.T) (*httptest.Server, <-chan diagnosisRequest) {
	t.Helper()
	requests := make(chan diagnosisRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		requests <- diagnosisRequest{
			Method: r.Method, Path: r.URL.Path,
			Authorization: r.Header.Get("Authorization"), APIKey: r.Header.Get("x-api-key"),
			AnthropicVersion: r.Header.Get("anthropic-version"), GoogleAPIKey: r.Header.Get("x-goog-api-key"),
			Body: body,
		}
		_, _ = w.Write([]byte(`{"output_text":"mock diagnosis","output":[{"content":[{"type":"output_text","text":"mock diagnosis"}]}],"choices":[{"message":{"content":"mock diagnosis"}}],"content":[{"type":"text","text":"mock diagnosis"}],"candidates":[{"content":{"parts":[{"text":"mock diagnosis"}]}}],"message":{"content":[{"type":"text","text":"mock diagnosis"}]}}`))
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func diagnoseSample(t *testing.T, f pairFixture, flag pairFlag, endpoint string) []string {
	t.Helper()
	values := map[string]string{
		"config": f.Config, "basedir": f.E.Base, "project-name": f.Project,
		"run-id": f.Run, "job-id": f.Bad, "job-name": "bad",
		"provider": "openai-chat", "endpoint": endpoint + "/custom",
		"model": "pair-model", "language": "ja-JP",
	}
	if flag.ValueName == "" {
		return []string{"--" + flag.Name + "=true"}
	}
	value, ok := values[flag.Name]
	if !ok {
		t.Fatalf("no diagnose sample for --%s", flag.Name)
	}
	if len(flag.Values) > 0 {
		valid := false
		for _, allowed := range flag.Values {
			valid = valid || allowed == value
		}
		if !valid {
			t.Fatalf("sample %q not in --%s values %q", value, flag.Name, flag.Values)
		}
	}
	return []string{"--" + flag.Name, value}
}

func diagnoseArgs(t *testing.T, f pairFixture, flags []pairFlag, endpoint string) ([]string, string, string, bool) {
	t.Helper()
	args := []string{"diagnose"}
	args = appendDiagnoseLocationDefaults(args, f, flags)
	for _, flag := range flags {
		args = append(args, diagnoseSample(t, f, flag, endpoint)...)
	}
	if !pairHas(flags, "job-id") && !pairHas(flags, "job-name") {
		args = append(args, "--job-id", f.Bad)
	}
	localRules := pairHas(flags, "rules")
	provider, model := expectedLLMRequest(flags)
	if !localRules {
		args = appendLLMDefaults(args, flags, endpoint, model)
	}
	return args, provider, model, localRules
}

func appendDiagnoseLocationDefaults(args []string, f pairFixture, flags []pairFlag) []string {
	for _, item := range []struct{ name, value string }{
		{"config", f.Config}, {"basedir", f.E.Base}, {"project-name", f.Project}, {"run-id", f.Run},
	} {
		if !pairHas(flags, item.name) {
			args = append(args, "--"+item.name, item.value)
		}
	}
	return args
}

func expectedLLMRequest(flags []pairFlag) (provider, model string) {
	provider, model = "openai", "pair-model"
	if pairHas(flags, "provider") {
		provider = "openai-chat"
	}
	return provider, model
}

func appendLLMDefaults(args []string, flags []pairFlag, endpoint, model string) []string {
	if !pairHas(flags, "model") {
		args = append(args, "--model", model)
	}
	if !pairHas(flags, "endpoint") {
		args = append(args, "--endpoint", endpoint)
	}
	return args
}

func diagnoseCommand(t *testing.T, f pairFixture, flags []pairFlag, endpoint string, requests <-chan diagnosisRequest) (support.Result, diagnosisRequest, bool) {
	t.Helper()
	args, provider, model, localRules := diagnoseArgs(t, f, flags, endpoint)
	e := f.E.WithVar("ROTARI_LLM_API_KEY", "pair-key")
	r := pairInvoke(t, e, args...)
	if localRules || (pairHas(flags, "job-id") && pairHas(flags, "job-name")) {
		return r, diagnosisRequest{}, localRules
	}
	select {
	case request := <-requests:
		assertDiagnosisRequest(t, request, provider, model, flags)
		return r, request, false
	case <-time.After(time.Second):
		t.Fatalf("diagnose did not contact the local fake endpoint: %s", r)
		return r, diagnosisRequest{}, false
	}
}

func pairHas(flags []pairFlag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func hasLLMOption(flags []pairFlag) bool {
	for _, flag := range flags {
		switch flag.Name {
		case "endpoint", "language", "model", "provider":
			return true
		}
	}
	return false
}

func assertDiagnosisRequest(t *testing.T, request diagnosisRequest, provider, model string, flags []pairFlag) {
	t.Helper()
	if request.Method != http.MethodPost {
		t.Fatalf("diagnosis request method = %q", request.Method)
	}
	wantPath := "/"
	if pairHas(flags, "endpoint") {
		wantPath = "/custom"
	}
	if request.Path != wantPath {
		t.Fatalf("diagnosis request path = %q, want %q", request.Path, wantPath)
	}
	assertDiagnosisPayload(t, request, provider, model, flags)
	assertDiagnosisAuthentication(t, request, provider)
}

func assertDiagnosisPayload(t *testing.T, request diagnosisRequest, provider, model string, flags []pairFlag) {
	t.Helper()
	got := requestModelAndPrompt(t, request.Body, provider)
	if got.model != model {
		t.Fatalf("request model = %q, want %q for %s", got.model, model, provider)
	}
	if pairHas(flags, "language") && !strings.Contains(got.prompt, `BCP 47 tag "ja-JP"`) {
		t.Fatalf("requested language missing from diagnosis prompt: %q", got.prompt)
	}
}

func assertDiagnosisAuthentication(t *testing.T, request diagnosisRequest, provider string) {
	t.Helper()
	switch provider {
	case "openai", "openai-chat":
		if request.Authorization != "Bearer pair-key" {
			t.Fatalf("authorization = %q", request.Authorization)
		}
	case "anthropic":
		if request.APIKey != "pair-key" || request.AnthropicVersion != "2023-06-01" {
			t.Fatalf("Anthropic headers = %#v", request)
		}
	case "gemini":
		if request.GoogleAPIKey != "pair-key" {
			t.Fatalf("Gemini API key = %q", request.GoogleAPIKey)
		}
	case "cohere":
		if request.Authorization != "Bearer pair-key" {
			t.Fatalf("Cohere authorization = %q", request.Authorization)
		}
	default:
		t.Fatalf("unexpected provider %q", provider)
	}
}

func requestModelAndPrompt(t *testing.T, body map[string]any, provider string) struct{ model, prompt string } {
	t.Helper()
	var result struct{ model, prompt string }
	if model, ok := body["model"].(string); ok {
		result.model = model
	}
	switch provider {
	case "openai":
		result.prompt, _ = body["input"].(string)
	case "openai-chat", "anthropic", "cohere":
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 {
			message, _ := messages[0].(map[string]any)
			result.prompt, _ = message["content"].(string)
		}
	case "gemini":
		contents, _ := body["contents"].([]any)
		if len(contents) > 0 {
			content, _ := contents[0].(map[string]any)
			parts, _ := content["parts"].([]any)
			if len(parts) > 0 {
				part, _ := parts[0].(map[string]any)
				result.prompt, _ = part["text"].(string)
			}
		}
	}
	if result.prompt == "" {
		t.Fatalf("provider %s request did not contain a prompt: %#v", provider, body)
	}
	return result
}

func TestCLIFlagPairDiagnoseSamples(t *testing.T) {
	f := newPairFixture(t)
	for _, command := range readPairSchema(t, f.E) {
		if command.Name != "diagnose" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(flag.Name, func(t *testing.T) {
				server, requests := diagnoseServer(t)
				r, _, local := diagnoseCommand(t, f, []pairFlag{flag}, server.URL, requests)
				assertDiagnoseOutcome(t, r, local, []pairFlag{flag})
			})
		}
	}
}

func TestCLIFlagPairDiagnose(t *testing.T) {
	f := newPairFixture(t)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.E) {
		if candidate.Name == "diagnose" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("diagnose is missing from schema")
	}
	pairs := commandFlagPairs(command)
	for _, pair := range pairs {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			server, requests := diagnoseServer(t)
			before := savePairTree(t, f.E.Root)
			flagsAB := []pairFlag{pair.A, pair.B}
			ab, _, localAB := diagnoseCommand(t, f, flagsAB, server.URL, requests)
			flagsBA := []pairFlag{pair.B, pair.A}
			ba, _, localBA := diagnoseCommand(t, f, flagsBA, server.URL, requests)
			assertDiagnoseOutcome(t, ab, localAB, flagsAB)
			assertDiagnoseOutcome(t, ba, localBA, flagsBA)
			if ab.Code != ba.Code || ab.Stdout != ba.Stdout || ab.Stderr != ba.Stderr {
				t.Fatalf("order-dependent diagnose pair:\n%s\n%s", ab, ba)
			}
			if after := savePairTree(t, f.E.Root); !reflect.DeepEqual(before, after) {
				t.Fatalf("diagnose changed fixture state:\n%s\n%s", ab, ba)
			}
		})
	}
	t.Logf("diagnose pairs=%d invocations=%d; LLM calls use only a local fake endpoint", len(pairs), 2*len(pairs))
}

func assertDiagnoseOutcome(t *testing.T, result support.Result, localRules bool, flags []pairFlag) {
	t.Helper()
	if result.Code == 0 {
		if result.Stderr != "" {
			t.Fatalf("diagnose wrote stderr on success: %s", result)
		}
		if localRules {
			if strings.Contains(result.Stdout, "mock diagnosis") {
				t.Fatalf("--rules unexpectedly called the LLM endpoint: %s", result)
			}
		} else if strings.TrimSpace(result.Stdout) != "mock diagnosis" {
			t.Fatalf("LLM diagnosis output = %q", result.Stdout)
		}
		return
	}
	if localRules && hasLLMOption(flags) {
		if result.Code != 1 || !strings.Contains(result.Stderr, "--rules cannot be combined with --") || !strings.Contains(result.Stderr, "only apply to LLM diagnosis") {
			t.Fatalf("LLM options were silently ignored in rules mode: %s", result)
		}
		return
	}
	assertPairOutcome(t, result)
}

func TestCLIFlagPairDiagnoseRulesOptions(t *testing.T) {
	covers(t, "CLI-13")
	f := newPairFixture(t)
	for _, name := range []string{"endpoint", "language", "model", "provider"} {
		t.Run("cli/"+name, func(t *testing.T) {
			args := []string{"diagnose", "--rules", "--run-id", f.Run, "--job-id", f.Bad}
			server, _ := diagnoseServer(t)
			args = append(args, diagnoseSample(t, f, pairFlag{Name: name, ValueName: "VALUE"}, server.URL)...)
			result := pairInvoke(t, f.E, args...)
			assertDiagnoseRulesConflict(t, result, name)
		})
	}
	configPath := filepath.Join(f.E.Root, "diagnose-llm.yaml")
	if err := os.WriteFile(configPath, []byte("diagnose:\n  model: configured-model\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := pairInvoke(t, f.E, "diagnose", "--config", configPath, "--rules", "--run-id", f.Run, "--job-id", f.Bad)
	assertDiagnoseRulesConflict(t, configured, "model")
	env := f.E.WithVar("ROTARI_LLM_MODEL", "environment-model")
	fromEnv := pairInvoke(t, env, "diagnose", "--rules", "--run-id", f.Run, "--job-id", f.Bad)
	assertDiagnoseRulesConflict(t, fromEnv, "model")
}

func assertDiagnoseRulesConflict(t *testing.T, result support.Result, option string) {
	t.Helper()
	if result.Code != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, "--rules cannot be combined with --"+option) || !strings.Contains(result.Stderr, "only apply to LLM diagnosis") {
		t.Fatalf("LLM option was not diagnosed in local rules mode: %s", result)
	}
}

func TestCLIFlagPairDiagnoseRequestWitness(t *testing.T) {
	f := newPairFixture(t)
	server, requests := diagnoseServer(t)
	for _, name := range []string{"provider", "endpoint", "model", "language"} {
		t.Run(name, func(t *testing.T) {
			flag := pairFlag{Name: name, ValueName: "VALUE"}
			result, request, _ := diagnoseCommand(t, f, []pairFlag{flag}, server.URL, requests)
			assertDiagnoseOutcome(t, result, false, []pairFlag{flag})
			if request.Method != http.MethodPost {
				t.Fatalf("--%s did not reach the fake endpoint: %#v", name, request)
			}
		})
	}
}

// All external-provider behavior is exercised using the in-process server.
// The test suite never uses a remote provider or API key.
