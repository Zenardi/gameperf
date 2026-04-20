package llm

import "context"

// Provider is the interface all LLM backends must satisfy.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Complete sends a prompt and returns the model's text response.
	Complete(ctx context.Context, prompt string) (string, error)
	// Name returns a human-readable identifier such as "ollama/llama3.2".
	Name() string
}
