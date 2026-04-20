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

const (
	defaultAnthropicURL     = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	defaultAnthropicMaxToks = 4096
)

// AnthropicProvider sends requests to the Anthropic Messages API.
type AnthropicProvider struct {
	model  string
	apiKey string
	url    string
	client *http.Client
}

// NewAnthropicProvider creates an AnthropicProvider. Pass an empty url to use
// the official Anthropic endpoint.
func NewAnthropicProvider(model, apiKey, url string) *AnthropicProvider {
	if url == "" {
		url = defaultAnthropicURL
	}
	return &AnthropicProvider{
		model:  model,
		apiKey: apiKey,
		url:    url,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string {
	return fmt.Sprintf("anthropic/%s", p.model)
}

// Anthropic request / response types

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(anthropicRequest{
		Model:     p.model,
		MaxTokens: defaultAnthropicMaxToks,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.url+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		if jsonErr := json.Unmarshal(rawBody, &errBody); jsonErr == nil && errBody.Error.Message != "" {
			return "", fmt.Errorf("anthropic error: %s", errBody.Error.Message)
		}
		return "", fmt.Errorf("anthropic returned HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", fmt.Errorf("anthropic decode: %w\nraw response: %s", err, string(rawBody))
	}

	if result.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic returned no content")
	}

	return result.Content[0].Text, nil
}
