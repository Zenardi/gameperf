package llm

import "time"

// SetGeminiBackoff sets the initial retry backoff on a GeminiProvider.
// Only for use in tests.
func SetGeminiBackoff(p *GeminiProvider, d time.Duration) {
	p.initialBackoff = d
}
