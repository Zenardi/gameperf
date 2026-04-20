package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const defaultGeminiURL = "https://generativelanguage.googleapis.com"

// GeminiProvider sends requests to the Google Gemini API.
type GeminiProvider struct {
	model  string
	apiKey string
	url    string
	client *http.Client
}

// NewGeminiProvider creates a GeminiProvider. Pass an empty url to use the
// official Google Generative Language endpoint.
func NewGeminiProvider(model, apiKey, url string) *GeminiProvider {
	if url == "" {
		url = defaultGeminiURL
	}
	return &GeminiProvider{
		model:  model,
		apiKey: apiKey,
		url:    url,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *GeminiProvider) Name() string {
	return fmt.Sprintf("gemini/%s", p.model)
}

// Gemini request / response types

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	})
	if err != nil {
		return "", err
	}

	// Gemini uses the API key as a query parameter, not an Authorization header.
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.url, p.model, p.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("gemini decode: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini error: %s", result.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini returned HTTP %d", resp.StatusCode)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}
