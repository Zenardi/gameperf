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
	endpoint := fmt.Sprintf("%s/v1/models/%s:generateContent?key=%s",
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

	// Read the full body so we can show it on errors.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gemini read body: %w", err)
	}

	// Surface non-200 errors before attempting JSON decode, because error
	// responses (quota exceeded, payload too large, auth failures, etc.) may
	// contain numeric fields that confuse our response struct.
	if resp.StatusCode != http.StatusOK {
		// Try to extract a human-readable message from the error body.
		var errBody struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(rawBody, &errBody); jsonErr == nil && errBody.Error.Message != "" {
			return "", fmt.Errorf("gemini error (%s): %s", errBody.Error.Status, errBody.Error.Message)
		}
		return "", fmt.Errorf("gemini returned HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var result geminiResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", fmt.Errorf("gemini decode: %w\nraw response: %s", err, string(rawBody))
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini error: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}
