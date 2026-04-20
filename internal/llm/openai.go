package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultOpenAIURL = "https://api.openai.com"

// OpenAIProvider sends requests to the OpenAI chat completions API.
// It is also compatible with any OpenAI-compatible endpoint (e.g. local llama.cpp servers).
type OpenAIProvider struct {
	model  string
	apiKey string
	url    string
	client *http.Client
}

// NewOpenAIProvider creates an OpenAIProvider. Pass an empty url to use the
// official OpenAI endpoint.
func NewOpenAIProvider(model, apiKey, url string) *OpenAIProvider {
	if url == "" {
		url = defaultOpenAIURL
	}
	return &OpenAIProvider{
		model:  model,
		apiKey: apiKey,
		url:    url,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string {
	return fmt.Sprintf("openai/%s", p.model)
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(openAIRequest{
		Model:    p.model,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		if jsonErr := json.Unmarshal(rawBody, &errBody); jsonErr == nil && errBody.Error.Message != "" {
			return "", fmt.Errorf("openai error: %s", errBody.Error.Message)
		}
		return "", fmt.Errorf("openai returned HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var result openAIResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", fmt.Errorf("openai decode: %w\nraw response: %s", err, string(rawBody))
	}

	if result.Error != nil {
		return "", fmt.Errorf("openai error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}
