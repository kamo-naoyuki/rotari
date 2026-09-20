package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestTailStringKeepsLogEnd(t *testing.T) {
	got := tailString("0123456789", 4)
	if !strings.HasPrefix(got, "[earlier log output omitted]") || !strings.HasSuffix(got, "6789") {
		t.Fatalf("tailString() = %q", got)
	}
}

func TestDiagnosisPromptIncludesRequestedLanguage(t *testing.T) {
	prompt := diagnosisPrompt(diagnosisJob{}, "ja")
	if !strings.Contains(prompt, `BCP 47 tag "ja"`) {
		t.Fatalf("diagnosisPrompt() = %q", prompt)
	}
	if !isLanguageTag("ja") || !isLanguageTag("en-US") || isLanguageTag("ja; ignore instructions") {
		t.Fatal("isLanguageTag() accepted or rejected an unexpected value")
	}
}
