package diagnose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestProviderDiagnosisDecodesProviderResponses(t *testing.T) {
	tests := []struct {
		provider string
		response string
	}{
		{"openai", `{"output":[{"content":[{"type":"output_text","text":"openai"}]}]}`},
		{"openai-chat", `{"choices":[{"message":{"content":"chat"}}]}`},
		{"anthropic", `{"content":[{"type":"text","text":"anthropic"}]}`},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"gemini"}]}}]}`},
		{"cohere", `{"message":{"content":[{"type":"text","text":"cohere"}]}}`},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("request = %s content-type=%q", request.Method, request.Header.Get("Content-Type"))
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()

			answer, err := RequestProviderDiagnosis(context.Background(), test.provider, server.URL, "secret", "model", "prompt")
			if err != nil || answer == "" {
				t.Fatalf("RequestProviderDiagnosis() = %q, %v", answer, err)
			}
		})
	}
}
