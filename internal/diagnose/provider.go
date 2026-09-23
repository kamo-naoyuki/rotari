package diagnose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20

func RequestProviderDiagnosis(ctx context.Context, provider, endpoint, apiKey, model, prompt string) (string, error) {
	var body any
	headers := map[string]string{"Content-Type": "application/json"}
	switch provider {
	case "openai":
		body = struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}{model, prompt}
	case "openai-chat":
		body = chatRequest{Model: model, Messages: []chatMessage{{Role: "user", Content: prompt}}}
		headers["Authorization"] = "Bearer " + apiKey
	case "anthropic":
		body = struct {
			Model     string        `json:"model"`
			MaxTokens int           `json:"max_tokens"`
			Messages  []chatMessage `json:"messages"`
		}{model, 4096, []chatMessage{{Role: "user", Content: prompt}}}
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
	case "gemini":
		body = struct {
			Contents []struct {
				Parts []chatPart `json:"parts"`
			} `json:"contents"`
		}{Contents: []struct {
			Parts []chatPart `json:"parts"`
		}{{Parts: []chatPart{{Text: prompt}}}}}
		headers["x-goog-api-key"] = apiKey
	case "cohere":
		body = struct {
			Stream   bool          `json:"stream"`
			Model    string        `json:"model"`
			Messages []chatMessage `json:"messages"`
		}{false, model, []chatMessage{{Role: "user", Content: prompt}}}
		headers["Authorization"] = "Bearer " + apiKey
	default:
		return "", fmt.Errorf("unsupported LLM provider %q", provider)
	}
	if provider == "openai" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseData, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(responseData)))
	}
	return decodeProviderResponse(provider, responseData)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatPart struct {
	Text string `json:"text"`
}

func decodeProviderResponse(provider string, data []byte) (string, error) {
	var text string
	switch provider {
	case "openai":
		var response struct {
			OutputText string `json:"output_text"`
			Output     []struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", fmt.Errorf("decode API response: %w", err)
		}
		text = response.OutputText
		for _, output := range response.Output {
			for _, content := range output.Content {
				if text == "" && content.Type == "output_text" {
					text = content.Text
				}
			}
		}
	case "openai-chat":
		var response struct {
			Choices []struct {
				Message chatMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", fmt.Errorf("decode API response: %w", err)
		}
		if len(response.Choices) > 0 {
			text = response.Choices[0].Message.Content
		}
	case "anthropic":
		var response struct {
			Content []struct{ Type, Text string } `json:"content"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", fmt.Errorf("decode API response: %w", err)
		}
		for _, content := range response.Content {
			if content.Type == "text" && text == "" {
				text = content.Text
			}
		}
	case "gemini":
		var response struct {
			Candidates []struct {
				Content struct {
					Parts []chatPart `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", fmt.Errorf("decode API response: %w", err)
		}
		for _, candidate := range response.Candidates {
			for _, part := range candidate.Content.Parts {
				if text == "" {
					text = part.Text
				}
			}
		}
	case "cohere":
		var response struct {
			Message struct {
				Content []struct{ Type, Text string } `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return "", fmt.Errorf("decode API response: %w", err)
		}
		for _, content := range response.Message.Content {
			if content.Type == "text" && text == "" {
				text = content.Text
			}
		}
	}
	if text == "" {
		return "", fmt.Errorf("API response did not include text content")
	}
	return text, nil
}
