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

// retryableGeminiStatuses are API error statuses worth retrying with backoff.
var retryableGeminiStatuses = map[string]bool{
	"UNAVAILABLE":       true,
	"RESOURCE_EXHAUSTED": true,
}

// GeminiProvider sends requests to the Google Gemini API.
type GeminiProvider struct {
	model          string
	apiKey         string
	url            string
	client         *http.Client
	initialBackoff time.Duration // overridable for testing
}

// NewGeminiProvider creates a GeminiProvider. Pass an empty url to use the
// official Google Generative Language endpoint.
func NewGeminiProvider(model, apiKey, url string) *GeminiProvider {
	if url == "" {
		url = defaultGeminiURL
	}
	return &GeminiProvider{
		model:          model,
		apiKey:         apiKey,
		url:            url,
		client:         &http.Client{Timeout: 60 * time.Second},
		initialBackoff: 5 * time.Second,
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

	const maxAttempts = 3
	backoff := p.initialBackoff
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, apiErr, err := p.doRequest(ctx, endpoint, body)
		if err != nil {
			return "", err
		}
		if apiErr == "" {
			return result, nil
		}
		// Retry transient errors with exponential backoff.
		if attempt < maxAttempts && retryableGeminiStatuses[extractGeminiStatus(apiErr)] {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}
		return "", fmt.Errorf("gemini %s", apiErr)
	}
	return "", fmt.Errorf("gemini: exceeded retry limit")
}

func extractGeminiStatus(errMsg string) string {
	// errMsg format: "error (STATUS): message"
	start := len("error (")
	end := len(errMsg)
	for i := start; i < end; i++ {
		if errMsg[i] == ')' {
			return errMsg[start:i]
		}
	}
	return ""
}

// doRequest performs a single HTTP call and returns (responseText, "error (...): ...", networkError).
func (p *GeminiProvider) doRequest(ctx context.Context, endpoint string, body []byte) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("gemini read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(rawBody, &errBody); jsonErr == nil && errBody.Error.Message != "" {
			return "", fmt.Sprintf("error (%s): %s", errBody.Error.Status, errBody.Error.Message), nil
		}
		return "", fmt.Sprintf("error: HTTP %d: %s", resp.StatusCode, string(rawBody)), nil
	}

	var result geminiResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", "", fmt.Errorf("gemini decode: %w\nraw response: %s", err, string(rawBody))
	}
	if result.Error != nil {
		return "", fmt.Sprintf("error: %s", result.Error.Message), nil
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", "", fmt.Errorf("gemini returned no candidates")
	}
	return result.Candidates[0].Content.Parts[0].Text, "", nil
}
