package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaProvider sends requests to a local Ollama server.
type OllamaProvider struct {
	model  string
	url    string
	client *http.Client
}

// NewOllamaProvider creates an OllamaProvider targeting the given base URL.
func NewOllamaProvider(model, url string) *OllamaProvider {
	return &OllamaProvider{
		model:  model,
		url:    url,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama/%s", p.model)
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

func (p *OllamaProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(ollamaRequest{Model: p.model, Prompt: prompt, Stream: false})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode: %w", err)
	}

	// Ollama returns errors both as HTTP 5xx and as a JSON error field.
	if result.Error != "" {
		return "", fmt.Errorf("ollama error: %s", result.Error)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}

	return result.Response, nil
}
